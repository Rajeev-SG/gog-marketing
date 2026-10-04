package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CommandResult carries provider output only through the package boundary;
// callers never observe raw output.
type CommandResult struct {
	Output []byte
	Stderr []byte
}

// Runner is the seam used to execute provider CLIs.
type Runner interface {
	Run(ctx context.Context, command string, args ...string) (CommandResult, error)
}

// Options contains the preflight dependencies.
type Options struct {
	Runner Runner
}

// Status is a coarse, safe provider state.
type Status string

const (
	// OK means the required provider state was verified.
	OK Status = "ok"
	// Missing means verified provider absence: the checked resource or
	// authentication is demonstrably not present.
	Missing Status = "missing"
	// Unavailable means the provider CLI, access, or output could not be
	// verified. It is the fail-closed default for unclassified failures.
	Unavailable Status = "unavailable"
	// Mismatch means the resource exists but differs from the documented name.
	Mismatch Status = "mismatch"
)

// Check is one safe provider preflight result.
type Check struct {
	Provider string `json:"provider"`
	Resource string `json:"resource"`
	Status   Status `json:"status"`
	Detail   string `json:"detail,omitempty"`
}

// Report is the aggregate provider readiness result.
type Report struct {
	Ready  bool    `json:"ready"`
	Checks []Check `json:"checks"`
}

var (
	errRunnerRequired      = errors.New("provider preflight runner is required")
	errUnexpectedOutput    = errors.New("provider returned unexpected output")
	errCommandNotInstalled = errors.New("provider command is not installed")
)

// Preflight performs read-only checks against the provider CLIs. It returns a
// report containing only provider names, parsed resource names, and safe state
// descriptions. Raw provider output, raw stderr, and secret values are
// intentionally discarded.
func Preflight(ctx context.Context, config Config, options Options) (Report, error) {
	if options.Runner == nil {
		return Report{}, errRunnerRequired
	}

	var report Report

	wranglerAuth := checkAuth(ctx, &report, options.Runner, "wrangler", "authentication", "wrangler", []string{"whoami", "--json"}, verifyWranglerAuth)
	checkNamedJSONArray(ctx, &report, options.Runner, "cloudflare", "d1_database", config.Cloudflare.D1Database, "name", "wrangler", []string{"d1", "list", "--json"}, wranglerAuth == OK)
	worker, workerSecrets := checkWorker(ctx, &report, options.Runner, config, wranglerAuth == OK)
	checkWorkerSecretNames(&report, config, worker, workerSecrets)
	checkWorkerVariables(ctx, &report, options.Runner, config, wranglerAuth == OK, worker)
	checkKVNamespace(ctx, &report, options.Runner, config, wranglerAuth == OK)

	clerkAuth := checkAuth(ctx, &report, options.Runner, "clerk", "authentication", "clerk", []string{"whoami", "--json"}, verifyClerkAuth)
	checkNamedJSONArray(ctx, &report, options.Runner, "clerk", "application", config.Clerk.Application, "name", "clerk", []string{"apps", "list", "--json"}, clerkAuth == OK)

	gcloudAuth := checkAuth(ctx, &report, options.Runner, "gcloud", "authentication", "gcloud", []string{"auth", "list", "--format=json"}, verifyGcloudAuth)
	project := checkProject(ctx, &report, options.Runner, config, gcloudAuth == OK)
	checkArtifact(ctx, &report, options.Runner, config, project == OK)
	checkCloudRunAndIdentity(ctx, &report, options.Runner, config, project == OK)

	report.Ready = allOK(report.Checks)

	return report, nil
}

func addCheck(report *Report, providerName, resource string, status Status, detail string) {
	report.Checks = append(report.Checks, Check{
		Provider: providerName,
		Resource: resource,
		Status:   status,
		Detail:   detail,
	})
}

func allOK(checks []Check) bool {
	for _, check := range checks {
		if check.Status != OK {
			return false
		}
	}

	return len(checks) > 0
}

var (
	unauthenticatedMarkers = []string{"not authenticated", "not logged in", "session expired"}
	unauthorizedMarkers    = []string{"unauthorized", "permission denied", "access denied", "not authorized", "forbidden", "authentication error"}
	notFoundMarkers        = []string{"not found", "no such", "could not find"}
)

func containsAnyMarker(text string, markers []string) bool {
	lowered := strings.ToLower(text)
	for _, marker := range markers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}

	return false
}

// surfaceIncludes reports whether a semicolon-separated binding surface string
// contains the exact named surface token. Substring matches never count, so a
// token such as not-cloudflare-worker cannot satisfy the surface check.
func surfaceIncludes(surface, target string) bool {
	for _, token := range strings.Split(surface, ";") {
		if token == target {
			return true
		}
	}

	return false
}

// failure classifies a command failure safely without emitting provider stderr.
// Unauthenticated results are verified absence (missing); access and
// unclassified failures fail closed as unavailable; only explicit not-found
// evidence marks a resource missing.
func failure(err error, stderr []byte) (Status, string) {
	errText := err.Error()
	stderrText := string(stderr)

	switch {
	case containsAnyMarker(errText, unauthenticatedMarkers) || containsAnyMarker(stderrText, unauthenticatedMarkers):
		return Missing, "provider authentication is not present"
	case containsAnyMarker(errText, unauthorizedMarkers) || containsAnyMarker(stderrText, unauthorizedMarkers):
		return Unavailable, "provider access is not authorized"
	case containsAnyMarker(errText, notFoundMarkers) || containsAnyMarker(stderrText, notFoundMarkers):
		return Missing, "verified provider resource is absent"
	case errors.Is(err, errUnexpectedOutput):
		return Unavailable, "provider output could not be interpreted"
	case errors.Is(err, errCommandNotInstalled):
		return Unavailable, "provider command is not installed"
	default:
		return Unavailable, "provider access could not be verified"
	}
}

// checkAuth runs one provider authentication command and applies the
// provider-specific verifier. Verifiers must fail closed: a payload that does
// not positively prove an authenticated session never returns OK.
func checkAuth(ctx context.Context, report *Report, runner Runner, providerName, resource, command string, args []string, verify func(any) (Status, string)) Status {
	result, err := runner.Run(ctx, command, args...)
	if err != nil {
		status, detail := failure(err, result.Stderr)
		addCheck(report, providerName, resource, status, detail)

		return status
	}

	var payload any
	if err := json.Unmarshal(result.Output, &payload); err != nil {
		addCheck(report, providerName, resource, Unavailable, "provider output could not be interpreted")
		return Unavailable
	}

	status, detail := verify(payload)
	addCheck(report, providerName, resource, status, detail)

	return status
}

// verifyWranglerAuth accepts the wrangler whoami --json shape
// {"loggedIn": <bool>, ...}. Any payload that does not positively confirm the
// authenticated session fails closed.
func verifyWranglerAuth(payload any) (Status, string) {
	object, ok := payload.(map[string]any)
	if !ok {
		return Unavailable, "provider output could not be interpreted"
	}

	loggedIn, ok := object["loggedIn"].(bool)
	if !ok || !loggedIn {
		return Unavailable, "provider output does not confirm an authenticated session"
	}

	return OK, "authenticated provider CLI session verified"
}

// verifyClerkAuth accepts the clerk whoami --json shape
// {"email": <string>, ...}. A missing, null, or empty email (for example a
// keyless accountless application) does not prove a logged-in account.
func verifyClerkAuth(payload any) (Status, string) {
	object, ok := payload.(map[string]any)
	if !ok {
		return Unavailable, "provider output could not be interpreted"
	}

	emailValue, present := object["email"]
	if !present {
		return Unavailable, "provider output does not prove an authenticated account"
	}

	email, ok := emailValue.(string)
	if !ok || email == "" {
		return Missing, "Clerk authenticated account not found in whoami output"
	}

	return OK, "authenticated provider CLI session verified"
}

// verifyGcloudAuth accepts the gcloud auth list --format=json shape
// [{"account": <string>, "status": <string>}, ...]. Only an ACTIVE entry
// counts as an authenticated session.
func verifyGcloudAuth(payload any) (Status, string) {
	accounts, ok := payload.([]any)
	if !ok {
		return Unavailable, "provider output could not be interpreted"
	}

	for _, account := range accounts {
		entry, ok := account.(map[string]any)
		if !ok {
			return Unavailable, "provider output could not be interpreted"
		}

		if entry["status"] == "ACTIVE" {
			return OK, "gcloud authenticated session verified"
		}
	}

	return Missing, "gcloud active authenticated session not found"
}

func checkNamedJSONArray(ctx context.Context, report *Report, runner Runner, providerName, resource, expectedName, nameField, command string, args []string, enabled bool) {
	if !enabled {
		addCheck(report, providerName, resource, Unavailable, "not checked because provider authentication is missing")
		return
	}

	result, err := runner.Run(ctx, command, args...)
	if err != nil {
		status, detail := failure(err, result.Stderr)
		addCheck(report, providerName, resource, status, detail)

		return
	}

	found, err := containsName(result.Output, nameField, expectedName)
	if err != nil {
		addCheck(report, providerName, resource, Unavailable, "provider output could not be interpreted")
		return
	}

	if found {
		addCheck(report, providerName, resource, OK, "documented resource found")
		return
	}

	addCheck(report, providerName, resource, Missing, fmt.Sprintf("documented resource %q not found", expectedName))
}

func containsName(output []byte, nameField, expectedName string) (bool, error) {
	var payload any
	if err := json.Unmarshal(output, &payload); err != nil {
		return false, errUnexpectedOutput
	}

	var rows []map[string]any

	switch typed := payload.(type) {
	case []any:
		for _, row := range typed {
			object, ok := row.(map[string]any)
			if !ok {
				return false, errUnexpectedOutput
			}

			rows = append(rows, object)
		}
	case map[string]any:
		data, ok := typed["data"].([]any)
		if !ok {
			return false, errUnexpectedOutput
		}

		for _, row := range data {
			object, ok := row.(map[string]any)
			if !ok {
				return false, errUnexpectedOutput
			}

			rows = append(rows, object)
		}
	default:
		return false, errUnexpectedOutput
	}

	for _, row := range rows {
		name, ok := row[nameField].(string)
		if ok && name == expectedName {
			return true, nil
		}
	}

	return false, nil
}

// checkKVNamespace verifies the documented KV namespace by exact structured
// title. Substring, prefix, and suffix matches never count, so names such as
// gog-marketing-test cannot satisfy the check.
func checkKVNamespace(ctx context.Context, report *Report, runner Runner, config Config, enabled bool) {
	resource := "kv_namespace"
	if !enabled {
		addCheck(report, "cloudflare", resource, Unavailable, "not checked because wrangler authentication is missing")
		return
	}

	result, err := runner.Run(ctx, "wrangler", "kv", "namespace", "list")
	if err != nil {
		status, detail := failure(err, result.Stderr)
		addCheck(report, "cloudflare", resource, status, detail)

		return
	}

	found, err := containsExactTitle(result.Output, config.Cloudflare.KVNamespace)
	if err != nil {
		addCheck(report, "cloudflare", resource, Unavailable, "provider output could not be interpreted")
		return
	}

	if !found {
		addCheck(report, "cloudflare", resource, Missing, fmt.Sprintf("documented resource %q not found", config.Cloudflare.KVNamespace))
		return
	}

	addCheck(report, "cloudflare", resource, OK, "documented resource found")
}

func containsExactTitle(output []byte, expectedTitle string) (bool, error) {
	var payload any
	if err := json.Unmarshal(output, &payload); err != nil {
		return false, errUnexpectedOutput
	}

	rows, ok := payload.([]any)
	if !ok {
		return false, errUnexpectedOutput
	}

	for _, row := range rows {
		object, ok := row.(map[string]any)
		if !ok {
			return false, errUnexpectedOutput
		}

		if title, ok := object["title"].(string); ok && title == expectedTitle {
			return true, nil
		}
	}

	return false, nil
}

func checkProject(ctx context.Context, report *Report, runner Runner, config Config, enabled bool) Status {
	if !enabled {
		addCheck(report, "gcp", "project", Unavailable, "not checked because gcloud authentication is missing")
		return Unavailable
	}

	result, err := runner.Run(ctx, "gcloud", "projects", "describe", config.GCP.Project, "--format=json")
	if err != nil {
		status, detail := failure(err, result.Stderr)
		addCheck(report, "gcp", "project", status, detail)

		return status
	}

	var project struct {
		ProjectID string `json:"projectId"` //nolint:tagliatelle // gcloud native wire format uses camelCase ("projectId").
	}
	if err := json.Unmarshal(result.Output, &project); err != nil || project.ProjectID != config.GCP.Project {
		addCheck(report, "gcp", "project", Unavailable, "provider output could not be interpreted")
		return Unavailable
	}

	addCheck(report, "gcp", "project", OK, "documented resource found")

	return OK
}

func checkArtifact(ctx context.Context, report *Report, runner Runner, config Config, enabled bool) {
	resource := "artifact_repository"
	if !enabled {
		addCheck(report, "gcp", resource, Unavailable, "not checked because gcloud project access is missing")
		return
	}

	result, err := runner.Run(
		ctx,
		"gcloud",
		"artifacts",
		"repositories",
		"describe",
		config.GCP.ArtifactRegistry.Repository,
		"--location="+config.GCP.ArtifactRegistry.Location,
		"--project="+config.GCP.Project,
		"--format=json",
	)
	if err != nil {
		status, detail := failure(err, result.Stderr)
		addCheck(report, "gcp", resource, status, detail)

		return
	}

	var repository struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(result.Output, &repository); err != nil || !strings.HasSuffix(repository.Name, "/repositories/"+config.GCP.ArtifactRegistry.Repository) {
		addCheck(report, "gcp", resource, Unavailable, "provider output could not be interpreted")
		return
	}

	addCheck(report, "gcp", resource, OK, "documented resource found")
}

// cloudRunEnvEntry is one container env entry from the gcloud Cloud Run
// describe wire shape. valueFrom/secretKeyRef distinguish secret bindings from
// plain variables; only names are ever used.
type cloudRunEnvEntry struct {
	Name      string `json:"name"`
	ValueFrom *struct {
		SecretKeyRef *struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"secretKeyRef"` //nolint:tagliatelle // gcloud native wire format uses camelCase ("secretKeyRef").
	} `json:"valueFrom"` //nolint:tagliatelle // gcloud native wire format uses camelCase ("valueFrom").
}

// cloudRunService mirrors the minimal gcloud run services describe --format=json
// wire shape.
type cloudRunService struct {
	Spec struct {
		Template struct {
			Spec struct {
				ServiceAccountName string              `json:"serviceAccountName"` //nolint:tagliatelle // gcloud native wire format uses camelCase ("serviceAccountName").
				Containers         []cloudRunEnvHolder `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
}

type cloudRunEnvHolder struct {
	Env []cloudRunEnvEntry `json:"env"`
}

// checkCloudRunAndIdentity verifies the documented Cloud Run service and, from
// the same safe describe output, its runner identity and cloud-run-runner
// environment/secret names. One read-only call avoids duplicate describes.
func checkCloudRunAndIdentity(ctx context.Context, report *Report, runner Runner, config Config, enabled bool) {
	serviceResource := "cloud_run_service"
	identityResource := "cloud_run_identity"

	if !enabled {
		addCheck(report, "gcp", serviceResource, Unavailable, "not checked because gcloud project access is missing")
		addCheck(report, "gcp", identityResource, Unavailable, "not checked because gcloud project access is missing")
		addCloudRunRunnerChecks(report, config, Unavailable, nil, nil)

		return
	}

	result, err := runner.Run(
		ctx,
		"gcloud",
		"run",
		"services",
		"describe",
		config.GCP.CloudRun.Service,
		"--region="+config.GCP.CloudRun.Region,
		"--project="+config.GCP.Project,
		"--format=json",
	)
	if err != nil {
		status, detail := failure(err, result.Stderr)
		addCheck(report, "gcp", serviceResource, status, detail)
		addCheck(report, "gcp", identityResource, Unavailable, "not checked because the Cloud Run service is missing")
		addCloudRunRunnerChecks(report, config, Unavailable, nil, nil)

		return
	}

	var service cloudRunService
	if err := json.Unmarshal(result.Output, &service); err != nil {
		addCheck(report, "gcp", serviceResource, Unavailable, "provider output could not be interpreted")
		addCheck(report, "gcp", identityResource, Unavailable, "not checked because the Cloud Run service output could not be interpreted")
		addCloudRunRunnerChecks(report, config, Unavailable, nil, nil)

		return
	}

	addCheck(report, "gcp", serviceResource, OK, "documented resource found")

	variables := map[string]bool{}
	secrets := map[string]bool{}

	for _, container := range service.Spec.Template.Spec.Containers {
		for _, entry := range container.Env {
			if entry.ValueFrom != nil && entry.ValueFrom.SecretKeyRef != nil {
				secrets[entry.Name] = true
				continue
			}
			variables[entry.Name] = true
		}
	}

	if service.Spec.Template.Spec.ServiceAccountName != config.GCP.CloudRun.RunnerIdentity {
		addCheck(report, "gcp", identityResource, Mismatch, fmt.Sprintf("Cloud Run runner identity is not %q", config.GCP.CloudRun.RunnerIdentity))
	} else {
		addCheck(report, "gcp", identityResource, OK, "documented runner identity found")
	}

	addCloudRunRunnerChecks(report, config, OK, variables, secrets)
}

// addCloudRunRunnerChecks verifies, by name only, that the Cloud Run service
// exposes the documented non-secret runner variables and secret references.
func addCloudRunRunnerChecks(report *Report, config Config, serviceStatus Status, variables, secrets map[string]bool) {
	for _, binding := range config.Environment {
		if !surfaceIncludes(binding.Surface, "cloud-run-runner") {
			continue
		}

		if serviceStatus != OK {
			addCheck(report, "gcp", "cloud_run_env:"+binding.Name, Unavailable, "not checked because the Cloud Run service is unavailable")
			continue
		}

		if variables[binding.Name] {
			addCheck(report, "gcp", "cloud_run_env:"+binding.Name, OK, "documented variable name found")
			continue
		}

		addCheck(report, "gcp", "cloud_run_env:"+binding.Name, Missing, fmt.Sprintf("documented variable name %q not found", binding.Name))
	}

	for _, binding := range config.Secrets {
		if !surfaceIncludes(binding.Surface, "cloud-run-runner") {
			continue
		}

		if serviceStatus != OK {
			addCheck(report, "gcp", "cloud_run_secret:"+binding.Name, Unavailable, "not checked because the Cloud Run service is unavailable")
			continue
		}

		if secrets[binding.Name] {
			addCheck(report, "gcp", "cloud_run_secret:"+binding.Name, OK, "documented secret reference name found")
			continue
		}

		addCheck(report, "gcp", "cloud_run_secret:"+binding.Name, Missing, fmt.Sprintf("documented secret reference name %q not found", binding.Name))
	}
}

func checkWorker(ctx context.Context, report *Report, runner Runner, config Config, enabled bool) (Status, []string) {
	if !enabled {
		addCheck(report, "cloudflare", "worker", Unavailable, "not checked because wrangler authentication is missing")
		return Unavailable, nil
	}

	result, err := runner.Run(
		ctx,
		"wrangler",
		"secret",
		"list",
		"--name", config.Cloudflare.Worker,
		"--format=json",
	)
	if err != nil {
		status, detail := failure(err, result.Stderr)
		addCheck(report, "cloudflare", "worker", status, detail)

		return status, nil
	}

	var secrets []map[string]any
	if err := json.Unmarshal(result.Output, &secrets); err != nil {
		addCheck(report, "cloudflare", "worker", Unavailable, "provider output could not be interpreted")
		return Unavailable, nil
	}

	names := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if name, ok := secret["name"].(string); ok {
			names = append(names, name)
		}
	}

	addCheck(report, "cloudflare", "worker", OK, "documented resource found")

	return OK, names
}

func checkWorkerSecretNames(report *Report, config Config, workerStatus Status, actualNames []string) {
	actual := make(map[string]bool, len(actualNames))
	for _, name := range actualNames {
		actual[name] = true
	}

	for _, binding := range config.Secrets {
		if !binding.Secret || !surfaceIncludes(binding.Surface, "cloudflare-worker") {
			continue
		}

		if workerStatus != OK {
			addCheck(report, "cloudflare", "secret:"+binding.Name, Unavailable, "not checked because the Worker is missing")
			continue
		}

		if actual[binding.Name] {
			addCheck(report, "cloudflare", "secret:"+binding.Name, OK, "documented secret name found")
			continue
		}

		addCheck(report, "cloudflare", "secret:"+binding.Name, Missing, fmt.Sprintf("documented secret name %q not found", binding.Name))
	}
}

// checkWorkerVariables verifies the documented non-secret Worker variable
// names from the published (active) Worker deployment metadata. Worker
// metadata is fetched once per preflight and names are derived once; secret
// values are never requested or emitted.
func checkWorkerVariables(ctx context.Context, report *Report, runner Runner, config Config, enabled bool, workerStatus Status) {
	var bindings []Binding

	for _, binding := range config.Environment {
		if surfaceIncludes(binding.Surface, "cloudflare-worker") {
			bindings = append(bindings, binding)
		}
	}

	if len(bindings) == 0 {
		return
	}

	addUnavailable := func() {
		for _, binding := range bindings {
			addCheck(report, "cloudflare", "env:"+binding.Name, Unavailable, "Worker variable names could not be read from the active Worker deployment metadata")
		}
	}

	for _, binding := range bindings {
		switch {
		case !enabled:
			addCheck(report, "cloudflare", "env:"+binding.Name, Unavailable, "not checked because wrangler authentication is missing")
		case workerStatus != OK:
			addCheck(report, "cloudflare", "env:"+binding.Name, Unavailable, "not checked because the Worker is missing")
		}
	}

	if !enabled || workerStatus != OK {
		return
	}

	names, err := activeWorkerPlainBindingNames(ctx, runner, config.Cloudflare.Worker)
	if err != nil {
		addUnavailable()
		return
	}

	actual := make(map[string]bool, len(names))
	for _, name := range names {
		actual[name] = true
	}

	for _, binding := range bindings {
		if actual[binding.Name] {
			addCheck(report, "cloudflare", "env:"+binding.Name, OK, "documented variable name found")
			continue
		}

		addCheck(report, "cloudflare", "env:"+binding.Name, Missing, fmt.Sprintf("documented variable name %q not found", binding.Name))
	}
}

// activeWorkerPlainBindingNames reads the published (active) Worker
// deployment's version metadata once and returns its plain-text binding names.
// wrangler versions list is intentionally not used: it returns deployable
// versions, including uploads that were never published, so its latest entry
// does not prove live deployed readiness. The active deployment must expose
// exactly one version at 100% traffic; anything else fails closed.
func activeWorkerPlainBindingNames(ctx context.Context, runner Runner, worker string) ([]string, error) {
	deploymentResult, err := runner.Run(ctx, "wrangler", "deployments", "status", "--name", worker, "--json")
	if err != nil {
		return nil, fmt.Errorf("run wrangler deployments status: %w", err)
	}

	var deployment struct {
		Versions []struct {
			VersionID  string  `json:"version_id"`
			Percentage float64 `json:"percentage"`
		} `json:"versions"`
	}
	var parseErr error

	if parseErr = json.Unmarshal(deploymentResult.Output, &deployment); parseErr != nil ||
		len(deployment.Versions) != 1 ||
		deployment.Versions[0].VersionID == "" ||
		deployment.Versions[0].Percentage != 100 {
		return nil, errUnexpectedOutput
	}

	versionID := deployment.Versions[0].VersionID

	versionResult, err := runner.Run(ctx, "wrangler", "versions", "view", versionID, "--name", worker, "--json")
	if err != nil {
		return nil, fmt.Errorf("run wrangler versions view: %w", err)
	}

	var version struct {
		Resources struct {
			Bindings []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"bindings"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(versionResult.Output, &version); err != nil {
		return nil, errUnexpectedOutput
	}

	names := make([]string, 0, len(version.Resources.Bindings))
	for _, binding := range version.Resources.Bindings {
		if binding.Type == "plain_text" && binding.Name != "" {
			names = append(names, binding.Name)
		}
	}

	return names, nil
}
