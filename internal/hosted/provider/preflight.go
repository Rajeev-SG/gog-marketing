package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
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
	// CommandTimeout bounds each individual provider CLI invocation. Zero
	// disables the per-command bound; the caller's context still applies.
	CommandTimeout time.Duration
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
	// TimedOut means a context deadline (total or per-command) expired before
	// the provider CLI could complete. It is distinct from generic
	// unavailability so operators can separate latency from access failures.
	TimedOut Status = "timed-out"
)

// Check is one safe provider preflight result.
type Check struct {
	Provider string `json:"provider"`
	Resource string `json:"resource"`
	Status   Status `json:"status"`
	Detail   string `json:"detail,omitempty"`
}

// Action is a safe remediation category derived from a check status. It
// carries no provider identifiers or secret values.
type Action string

const (
	// ActionNone means no action is suggested by this check.
	ActionNone Action = "none"
	// ActionProvisionMissing marks verified-absent resources for the owning
	// provisioning ticket.
	ActionProvisionMissing Action = "provision_missing"
	// ActionInvestigate marks checks that could not be verified (access, CLI,
	// output, or dependency failures).
	ActionInvestigate Action = "investigate_unavailable"
	// ActionFixConfig marks resources that exist with the wrong configuration.
	ActionFixConfig Action = "fix_config_mismatch"
	// ActionRetryTimedOut marks checks whose provider command hit a deadline.
	ActionRetryTimedOut Action = "retry_timed_out"
)

// ReportSummary counts the checks by status and safe action category. It
// contains counts only: no resource names, accounts, or provider output.
type ReportSummary struct {
	Total    int            `json:"total"`
	Statuses map[Status]int `json:"statuses"`
	Actions  map[Action]int `json:"actions"`
}

// Report is the aggregate provider readiness result.
type Report struct {
	Ready   bool          `json:"ready"`
	Summary ReportSummary `json:"summary"`
	Checks  []Check       `json:"checks"`
}

var (
	errRunnerRequired      = errors.New("provider preflight runner is required")
	errUnexpectedOutput    = errors.New("provider returned unexpected output")
	errCommandNotInstalled = errors.New("provider command is not installed")
)

// Preflight performs read-only checks against the provider CLIs. It returns a
// report containing only provider names, parsed resource names, and safe state
// descriptions. Raw provider output, raw stderr, and secret values are
// intentionally discarded. Each command runs under its own CommandTimeout
// deadline in addition to the caller's context.
func Preflight(ctx context.Context, config Config, options Options) (Report, error) {
	if options.Runner == nil {
		return Report{}, errRunnerRequired
	}

	var report Report

	runner := options.Runner
	if options.CommandTimeout > 0 {
		runner = commandTimeoutRunner{inner: runner, timeout: options.CommandTimeout}
	}

	wranglerAuth := checkAuth(ctx, &report, runner, "wrangler", "authentication", "wrangler", []string{"whoami", "--json"}, verifyWranglerAuth)
	checkNamedJSONArray(ctx, &report, runner, "cloudflare", "d1_database", config.Cloudflare.D1Database, "name", "wrangler", []string{"d1", "list", "--json"}, wranglerAuth == OK)
	worker := checkWorker(ctx, &report, runner, config, wranglerAuth == OK)
	checkWorkerSecretNames(&report, config, worker.status, worker.secretStatus, worker.secretNames)
	checkWorkerVariables(ctx, &report, runner, config, worker)
	checkKVNamespace(ctx, &report, runner, config, wranglerAuth == OK)

	clerkAuth := checkAuth(ctx, &report, runner, "clerk", "authentication", "clerk", []string{"whoami", "--json"}, verifyClerkAuth)
	checkNamedJSONArray(ctx, &report, runner, "clerk", "application", config.Clerk.Application, "name", "clerk", []string{"apps", "list", "--json"}, clerkAuth == OK)

	gcloudAuth := checkAuth(ctx, &report, runner, "gcloud", "authentication", "gcloud", []string{"auth", "list", "--format=json"}, verifyGcloudAuth)
	project := checkProject(ctx, &report, runner, config, gcloudAuth == OK)
	checkArtifact(ctx, &report, runner, config, project)
	checkCloudRunAndIdentity(ctx, &report, runner, config, project)

	report.Ready = allOK(report.Checks)
	report.Summary = summarize(report.Checks)

	return report, nil
}

// commandTimeoutRunner applies a fresh deadline to every provider command so
// one slow CLI cannot consume the budget of the next commands.
type commandTimeoutRunner struct {
	inner   Runner
	timeout time.Duration
}

func (r commandTimeoutRunner) Run(ctx context.Context, command string, args ...string) (CommandResult, error) {
	commandCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	result, err := r.inner.Run(commandCtx, command, args...)
	if err != nil {
		return result, fmt.Errorf("run provider command: %w", err)
	}

	return result, nil
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

// statusAction maps a check status to its safe remediation category.
func statusAction(status Status) Action {
	switch status {
	case OK:
		return ActionNone
	case Missing:
		return ActionProvisionMissing
	case Mismatch:
		return ActionFixConfig
	case TimedOut:
		return ActionRetryTimedOut
	default:
		return ActionInvestigate
	}
}

// summarize builds the identifier-free report summary.
func summarize(checks []Check) ReportSummary {
	summary := ReportSummary{
		Total:    len(checks),
		Statuses: make(map[Status]int, 5),
		Actions:  make(map[Action]int, 5),
	}

	for _, check := range checks {
		summary.Statuses[check.Status]++
		summary.Actions[statusAction(check.Status)]++
	}

	return summary
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

// failure classifies a command failure safely without emitting provider
// stderr. Context deadline failures get the distinct timed-out status; every
// other failure — including unstructured "not found" text in stderr or error
// strings — fails closed as unavailable. Verified absence (missing) is
// reserved for positively parsed provider responses such as an empty or
// name-absent structured list, or a parsed unauthenticated whoami payload.
func failure(err error) (Status, string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return TimedOut, "provider command timed out before completion; retry with a larger --command-timeout (cold environments such as first credential refresh may need several minutes)"
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
		status, detail := failure(err)
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
		status, detail := failure(err)
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
		status, detail := failure(err)
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

// listStringField reports whether a parsed JSON list contains an entry whose
// string field exactly equals the expected value. It distinguishes positively
// parsed absence from an uninterpretable payload: an empty list, or a list
// whose every entry carries the field without a match, proves verified
// absence. Any entry that is not a plain object or that lacks the field is an
// anomaly and must fail closed as unavailable instead of guessing absence.
func listStringField(output []byte, field, expected string) (bool, error) {
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

		value, ok := object[field].(string)
		if !ok {
			return false, errUnexpectedOutput
		}

		if value == expected {
			return true, nil
		}
	}

	return false, nil
}

// serviceListContains reports whether a parsed Cloud Run service list names
// the documented service. Cloud Run names are fully qualified resource paths
// such as projects/P/locations/R/services/S; exact equality or the exact
// "/services/<name>" suffix counts, and any entry without a metadata.name
// string fails closed as an anomaly instead of guessing absence.
func serviceListContains(output []byte, service string) (bool, error) {
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

		metadata, ok := object["metadata"].(map[string]any)
		if !ok {
			return false, errUnexpectedOutput
		}

		name, ok := metadata["name"].(string)
		if !ok {
			return false, errUnexpectedOutput
		}

		if name == service || strings.HasSuffix(name, "/services/"+service) {
			return true, nil
		}
	}

	return false, nil
}

// parentDependentDetail explains why a dependent check was not performed
// using the parent check's actual classification. It must claim the parent is
// missing only when the parent was genuinely verified-absent; timed-out and
// unverified parents must produce their own truthful reasons.
func parentDependentDetail(parentStatus Status, parentName string) string {
	switch parentStatus {
	case Missing:
		return fmt.Sprintf("not checked because the %s is missing", parentName)
	case TimedOut:
		return fmt.Sprintf("not checked because the %s command timed out", parentName)
	default:
		return fmt.Sprintf("not checked because the %s could not be verified", parentName)
	}
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

	// Existence is verified from a read-only list-style scoped query so
	// absence is positively parsed: an empty or name-absent list proves the
	// project is absent, while any command or parse failure fails closed.
	result, err := runner.Run(ctx, "gcloud", "projects", "list", "--filter=projectId="+config.GCP.Project, "--format=json")
	if err != nil {
		status, detail := failure(err)
		addCheck(report, "gcp", "project", status, detail)

		return status
	}

	found, err := listStringField(result.Output, "projectId", config.GCP.Project)
	if err != nil {
		addCheck(report, "gcp", "project", Unavailable, "provider output could not be interpreted")
		return Unavailable
	}

	if !found {
		addCheck(report, "gcp", "project", Missing, fmt.Sprintf("documented resource %q not found", config.GCP.Project))
		return Missing
	}

	addCheck(report, "gcp", "project", OK, "documented resource found")

	return OK
}

func checkArtifact(ctx context.Context, report *Report, runner Runner, config Config, projectStatus Status) {
	resource := "artifact_repository"

	enabled := projectStatus == OK
	if !enabled {
		addCheck(report, "gcp", resource, Unavailable, parentDependentDetail(projectStatus, "GCP project"))
		return
	}

	// Existence is verified from a read-only list-style scoped query with an
	// exact fully qualified resource-name match, so absence is positively
	// parsed instead of inferred from command failure text.
	result, err := runner.Run(
		ctx,
		"gcloud",
		"artifacts",
		"repositories",
		"list",
		"--location="+config.GCP.ArtifactRegistry.Location,
		"--project="+config.GCP.Project,
		"--format=json",
	)
	if err != nil {
		status, detail := failure(err)
		addCheck(report, "gcp", resource, status, detail)

		return
	}

	expectedName := fmt.Sprintf(
		"projects/%s/locations/%s/repositories/%s",
		config.GCP.Project,
		config.GCP.ArtifactRegistry.Location,
		config.GCP.ArtifactRegistry.Repository,
	)

	found, err := listStringField(result.Output, "name", expectedName)
	if err != nil {
		addCheck(report, "gcp", resource, Unavailable, "provider output could not be interpreted")
		return
	}

	if !found {
		addCheck(report, "gcp", resource, Missing, fmt.Sprintf("documented resource %q not found", config.GCP.ArtifactRegistry.Repository))
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
func checkCloudRunAndIdentity(ctx context.Context, report *Report, runner Runner, config Config, projectStatus Status) {
	serviceResource := "cloud_run_service"
	identityResource := "cloud_run_identity"
	serviceName := "Cloud Run service"

	if projectStatus != OK {
		projectDetail := parentDependentDetail(projectStatus, "GCP project")
		addCheck(report, "gcp", serviceResource, Unavailable, projectDetail)
		addCheck(report, "gcp", identityResource, Unavailable, projectDetail)
		addCloudRunRunnerChecks(report, config, Unavailable, projectDetail, nil, nil)

		return
	}

	// Existence is verified first from a read-only list-style scoped query so
	// absence is positively parsed from the structured list instead of being
	// inferred from describe command failure text.
	result, err := runner.Run(
		ctx,
		"gcloud",
		"run",
		"services",
		"list",
		"--region="+config.GCP.CloudRun.Region,
		"--project="+config.GCP.Project,
		"--format=json",
	)
	if err != nil {
		status, detail := failure(err)
		addCheck(report, "gcp", serviceResource, status, detail)
		addCheck(report, "gcp", identityResource, Unavailable, parentDependentDetail(status, serviceName))
		addCloudRunRunnerChecks(report, config, Unavailable, parentDependentDetail(status, serviceName), nil, nil)

		return
	}

	found, err := serviceListContains(result.Output, config.GCP.CloudRun.Service)
	if err != nil {
		addCheck(report, "gcp", serviceResource, Unavailable, "provider output could not be interpreted")
		addCheck(report, "gcp", identityResource, Unavailable, "not checked because the Cloud Run service output could not be interpreted")
		addCloudRunRunnerChecks(report, config, Unavailable, "not checked because the Cloud Run service output could not be interpreted", nil, nil)

		return
	}

	if !found {
		addCheck(report, "gcp", serviceResource, Missing, fmt.Sprintf("documented resource %q not found", config.GCP.CloudRun.Service))
		addCheck(report, "gcp", identityResource, Unavailable, parentDependentDetail(Missing, serviceName))
		addCloudRunRunnerChecks(report, config, Unavailable, parentDependentDetail(Missing, serviceName), nil, nil)

		return
	}

	addCheck(report, "gcp", serviceResource, OK, "documented resource found")

	result, err = runner.Run(
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
		// The service existence is already positively verified from the list
		// query; a describe failure only prevents reading the spec-dependent
		// identity and environment names. The detail reflects the actual
		// limitation instead of claiming the service is missing.
		addCheck(report, "gcp", identityResource, Unavailable, "not checked because the Cloud Run service spec could not be verified")
		addCloudRunRunnerChecks(report, config, Unavailable, "not checked because the Cloud Run service spec could not be verified", nil, nil)

		return
	}

	var service cloudRunService
	if err := json.Unmarshal(result.Output, &service); err != nil {
		addCheck(report, "gcp", serviceResource, Unavailable, "provider output could not be interpreted")
		addCheck(report, "gcp", identityResource, Unavailable, "not checked because the Cloud Run service output could not be interpreted")
		addCloudRunRunnerChecks(report, config, Unavailable, "not checked because the Cloud Run service output could not be interpreted", nil, nil)

		return
	}

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

	addCloudRunRunnerChecks(report, config, OK, "", variables, secrets)
}

// addCloudRunRunnerChecks verifies, by name only, that the Cloud Run service
// exposes the documented non-secret runner variables and secret references.
func addCloudRunRunnerChecks(report *Report, config Config, serviceStatus Status, serviceDetail string, variables, secrets map[string]bool) {
	for _, binding := range config.Environment {
		if !surfaceIncludes(binding.Surface, "cloud-run-runner") {
			continue
		}

		if serviceStatus != OK {
			addCheck(report, "gcp", "cloud_run_env:"+binding.Name, Unavailable, serviceDetail)
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
			addCheck(report, "gcp", "cloud_run_secret:"+binding.Name, Unavailable, serviceDetail)
			continue
		}

		if secrets[binding.Name] {
			addCheck(report, "gcp", "cloud_run_secret:"+binding.Name, OK, "documented secret reference name found")
			continue
		}

		addCheck(report, "gcp", "cloud_run_secret:"+binding.Name, Missing, fmt.Sprintf("documented secret reference name %q not found", binding.Name))
	}
}

// workerState carries the parsed Worker probe results through the dependent
// checks: existence status, secret-name status, parsed secret names, and the
// active published version ID when exactly one version holds 100% of traffic.
type workerState struct {
	status        Status
	secretStatus  Status
	secretNames   []string
	activeVersion string
}

// checkWorker verifies Worker existence from a positively parsed read-only
// deployments status probe, then reads the documented secret names from
// wrangler secret list. The deployments status probe is run exactly once; its
// response is parsed once and reused for both existence and active-version
// selection.
func checkWorker(ctx context.Context, report *Report, runner Runner, config Config, enabled bool) workerState {
	if !enabled {
		addCheck(report, "cloudflare", "worker", Unavailable, "not checked because wrangler authentication is missing")
		return workerState{status: Unavailable, secretStatus: Unavailable}
	}

	// Worker absence is verified only from the documented structured
	// Cloudflare error code 10007 with the exact sentence "This Worker does
	// not exist on your account", emitted by the read-only deployments status
	// probe (verified against wrangler 4.147.0). A raw generic "not found"
	// marker — for example the `Worker "name" not found.` text emitted by
	// wrangler secret list — can never prove verified absence, so the secret
	// list is never used as the existence probe.
	result, err := runner.Run(
		ctx,
		"wrangler",
		"deployments",
		"status",
		"--name", config.Cloudflare.Worker,
		"--json",
	)
	if err != nil {
		if wranglerWorkerMissing(result.Stderr) {
			addCheck(report, "cloudflare", "worker", Missing, fmt.Sprintf("documented resource %q not found (Cloudflare error code 10007)", config.Cloudflare.Worker))

			return workerState{status: Missing, secretStatus: Missing}
		}

		status, detail := failure(err)
		addCheck(report, "cloudflare", "worker", status, detail)

		return workerState{status: status, secretStatus: status}
	}

	// Positively parse the probe response: a non-empty deployment id proves
	// the Worker exists. Exactly one version at 100% traffic selects the
	// active published version for the dependent variable checks; any other
	// shape leaves the variable checks fail-closed without re-fetching.
	var deployment struct {
		ID       string `json:"id"`
		Versions []struct {
			VersionID  string  `json:"version_id"`
			Percentage float64 `json:"percentage"`
		} `json:"versions"`
	}
	if unmarshalErr := json.Unmarshal(result.Output, &deployment); unmarshalErr != nil || deployment.ID == "" {
		addCheck(report, "cloudflare", "worker", Unavailable, "provider output could not be interpreted")

		return workerState{status: Unavailable, secretStatus: Unavailable}
	}

	addCheck(report, "cloudflare", "worker", OK, "documented resource found")

	state := workerState{status: OK, secretStatus: OK}
	if len(deployment.Versions) == 1 && deployment.Versions[0].VersionID != "" && deployment.Versions[0].Percentage == 100 {
		state.activeVersion = deployment.Versions[0].VersionID
	}

	secretResult, err := runner.Run(
		ctx,
		"wrangler",
		"secret",
		"list",
		"--name", config.Cloudflare.Worker,
		"--format=json",
	)
	if err != nil {
		status, _ := failure(err)
		state.secretStatus = status

		return state
	}

	var secrets []map[string]any
	if err := json.Unmarshal(secretResult.Output, &secrets); err != nil {
		state.secretStatus = Unavailable

		return state
	}

	names := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if name, ok := secret["name"].(string); ok {
			names = append(names, name)
		}
	}

	state.secretNames = names

	return state
}

// wranglerWorkerMissing reports whether a failed wrangler command carries the
// tightly documented structured absence form: Cloudflare API error code 10007
// together with the exact sentence "This Worker does not exist on your
// account". Raw generic "not found" markers never match this classifier.
func wranglerWorkerMissing(stderr []byte) bool {
	return strings.Contains(string(stderr), "This Worker does not exist on your account") &&
		strings.Contains(string(stderr), "[code: 10007]")
}

func checkWorkerSecretNames(report *Report, config Config, workerStatus, secretStatus Status, actualNames []string) {
	actual := make(map[string]bool, len(actualNames))
	for _, name := range actualNames {
		actual[name] = true
	}

	uncheckedDetail := parentDependentDetail(workerStatus, "Worker")
	if workerStatus == OK && secretStatus != OK {
		uncheckedDetail = "not checked because Worker secret names could not be verified"
	}

	for _, binding := range config.Secrets {
		if !binding.Secret || !surfaceIncludes(binding.Surface, "cloudflare-worker") {
			continue
		}

		if workerStatus != OK {
			addCheck(report, "cloudflare", "secret:"+binding.Name, workerStatus, uncheckedDetail)
			continue
		}

		if secretStatus != OK {
			addCheck(report, "cloudflare", "secret:"+binding.Name, secretStatus, uncheckedDetail)
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
func checkWorkerVariables(ctx context.Context, report *Report, runner Runner, config Config, worker workerState) {
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
		if worker.status != OK {
			addCheck(report, "cloudflare", "env:"+binding.Name, worker.status, parentDependentDetail(worker.status, "Worker"))
		}
	}

	if worker.status != OK {
		return
	}

	// The active version was already parsed from the single deployments
	// status probe; a traffic-split or unverifiable deployment keeps these
	// checks fail-closed without any additional probe.
	if worker.activeVersion == "" {
		addUnavailable()
		return
	}

	names, err := activeWorkerPlainBindingNames(ctx, runner, config.Cloudflare.Worker, worker.activeVersion)
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

// activeWorkerPlainBindingNames reads the published (active) Worker version
// metadata and returns its plain-text binding names. The active version was
// already selected by the single deployments status probe (exactly one version
// at 100% traffic); wrangler versions list is intentionally not used because
// it returns deployable versions, including uploads that were never published,
// so its latest entry does not prove live deployed readiness.
func activeWorkerPlainBindingNames(ctx context.Context, runner Runner, worker, versionID string) ([]string, error) {
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
