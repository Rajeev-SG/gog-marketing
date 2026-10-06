package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/hosted/provider"
)

var (
	errUnexpectedCommand = errors.New("unexpected command")
	errNotAuthenticated  = errors.New("not authenticated")
	errUnauthorized      = errors.New("unauthorized")
	errPermissionDenied  = errors.New("permission denied")
	errNotFound          = errors.New("not found")
	errResourceNotFound  = errors.New("resource not found")
	errNetworkFailure    = errors.New("temporary network failure")
)

// Synthetic fixture identifiers; these are not provider inventory names.
const (
	activeVersionID = "version-1"
	fakeBindingText = "fake-var-value"
)

// Authentication fixtures use synthetic accounts; they carry no inventory
// names and never contain secret values.
const (
	wranglerAuthenticated = `{"loggedIn":true,"authType":"OAuth Token","accounts":[{"id":"account-id","name":"account-name"}]}`
	clerkAuthenticated    = `{"email":"owner@example.com","localSecretKeySource":null,"linked":null}`
	gcloudAuthenticated   = `[{"account":"owner@example.com","status":"ACTIVE"}]`
	gcloudNoActiveAccount = `[{"account":"owner@example.com","status":"INACTIVE"}]`
	clerkKeyless          = `{"email":null,"accountless":{"instanceId":"instance-id"},"keyless":{"instanceId":"instance-id"},"linked":null}`

	workerActiveDeployment = `{"id":"deployment-1","created_on":"2026-10-04T00:00:00.000Z","versions":[{"version_id":"` + activeVersionID + `","percentage":100}]}`
)

// commandKey mirrors the Runner seam's command+args identity so test fixtures
// and failures key on derived command lines instead of duplicated literal
// strings that could drift from the configured inventory.
func commandKey(command string, args ...string) string {
	return command + " " + strings.Join(args, " ")
}

func includesSurface(surface, target string) bool {
	for _, token := range strings.Split(surface, ";") {
		if token == target {
			return true
		}
	}

	return false
}

// surfaceCheckNames derives public preflight check names for inventory
// bindings on the given canonical surface. Deriving expectations from the
// loaded inventory keeps fixture coverage aligned with provider config.
func surfaceCheckNames(config provider.Config, surface string) []string {
	var prefix string

	switch surface {
	case "cloudflare-worker":
		prefix = "cloudflare."
	case "cloud-run-runner":
		prefix = "gcp.cloud_run_"
	default:
		prefix = ""
	}

	names := []string{}

	for _, binding := range config.Environment {
		if includesSurface(binding.Surface, surface) {
			names = append(names, prefix+"env:"+binding.Name)
		}
	}

	for _, binding := range config.Secrets {
		if includesSurface(binding.Surface, surface) {
			names = append(names, prefix+"secret:"+binding.Name)
		}
	}

	return names
}

// dependentDetailMap builds the expected truthful detail for every derived
// dependent check name without repeating inventory literals.
func dependentDetailMap(names []string, detail string) map[string]string {
	details := make(map[string]string, len(names))
	for _, name := range names {
		details[name] = detail
	}

	return details
}

func d1ListKey() string          { return "wrangler d1 list --json" }
func kvNamespaceListKey() string { return "wrangler kv namespace list" }
func clerkAppsListKey() string   { return "clerk apps list --json" }
func gcloudAuthListKey() string  { return "gcloud auth list --format=json" }

func workerSecretListKey(config provider.Config) string {
	return commandKey("wrangler", "secret", "list", "--name", config.Cloudflare.Worker, "--format=json")
}

func workerDeploymentStatusKey(config provider.Config) string {
	return commandKey("wrangler", "deployments", "status", "--name", config.Cloudflare.Worker, "--json")
}

// workerVersionViewKey keys the versions-view fixture on the fixture's active
// version id; production reads the parsed id from the single deployments
// status probe.
func workerVersionViewKey(config provider.Config) string {
	return commandKey("wrangler", "versions", "view", activeVersionID, "--name", config.Cloudflare.Worker, "--json")
}

func projectListKey(config provider.Config) string {
	return commandKey("gcloud", "projects", "list", "--filter=projectId="+config.GCP.Project, "--format=json")
}

func artifactsListKey(config provider.Config) string {
	return commandKey("gcloud", "artifacts", "repositories", "list", "--location="+config.GCP.ArtifactRegistry.Location, "--project="+config.GCP.Project, "--format=json")
}

func runServicesListKey(config provider.Config) string {
	return commandKey("gcloud", "run", "services", "list", "--region="+config.GCP.CloudRun.Region, "--project="+config.GCP.Project, "--format=json")
}

func runServicesDescribeKey(config provider.Config) string {
	return commandKey("gcloud", "run", "services", "describe", config.GCP.CloudRun.Service, "--region="+config.GCP.CloudRun.Region, "--project="+config.GCP.Project, "--format=json")
}

// Fixture builders derive every expected name from the loaded provider
// inventory, so tests cannot duplicate or drift from configured literal
// strings (and redaction corruption cannot silently rewrite expectations).
func d1PresentJSON(config provider.Config) string {
	return fmt.Sprintf(`[{"name":%q}]`, config.Cloudflare.D1Database)
}

func kvPresentJSON(config provider.Config) string {
	return fmt.Sprintf(`[{"id":"namespace-id","title":%q}]`, config.Cloudflare.KVNamespace)
}

func clerkApplicationPresentJSON(config provider.Config) string {
	return fmt.Sprintf(`[{"name":%q}]`, config.Clerk.Application)
}

// workerSecretNamesJSON lists the configured secret names on the Worker
// surface; secret values are never represented.
func workerSecretNamesJSON(config provider.Config) string {
	rows := []string{}

	for _, binding := range config.Secrets {
		if binding.Secret && includesSurface(binding.Surface, "cloudflare-worker") {
			rows = append(rows, fmt.Sprintf(`{"name":%q}`, binding.Name))
		}
	}

	return "[" + strings.Join(rows, ",") + "]"
}

// workerVersionBindingsJSON builds active-version plain_text binding metadata
// for the configured Worker environment names, optionally omitting names so
// absence scenarios stay inventory-derived.
func workerVersionBindingsJSON(config provider.Config, omitted ...string) string {
	omit := map[string]bool{}
	for _, name := range omitted {
		omit[name] = true
	}

	rows := []string{}

	for _, binding := range config.Environment {
		if includesSurface(binding.Surface, "cloudflare-worker") && !omit[binding.Name] {
			rows = append(rows, fmt.Sprintf(`{"type":"plain_text","name":%q,"text":%q}`, binding.Name, fakeBindingText))
		}
	}

	return `{"id":"` + activeVersionID + `","resources":{"bindings":[` + strings.Join(rows, ",") + `]}}`
}

// cloudRunServiceJSON builds the Cloud Run describe fixture from the
// configured runner identity and cloud-run-runner inventory names. Empty
// runnerIdentity uses the configured identity; omitted names keep absence
// scenarios inventory-derived.
func cloudRunServiceJSON(config provider.Config, runnerIdentity string, omitted ...string) string {
	if runnerIdentity == "" {
		runnerIdentity = config.GCP.CloudRun.RunnerIdentity
	}

	omit := map[string]bool{}
	for _, name := range omitted {
		omit[name] = true
	}

	entries := []string{}

	for _, binding := range config.Environment {
		if includesSurface(binding.Surface, "cloud-run-runner") && !omit[binding.Name] {
			entries = append(entries, fmt.Sprintf(`{"name":%q}`, binding.Name))
		}
	}

	for _, binding := range config.Secrets {
		if includesSurface(binding.Surface, "cloud-run-runner") && !omit[binding.Name] {
			entries = append(entries, fmt.Sprintf(`{"name":%q,"valueFrom":{"secretKeyRef":{"name":%q,"key":"latest"}}}`, binding.Name, binding.Name))
		}
	}

	return fmt.Sprintf(`{"spec":{"template":{"spec":{"serviceAccountName":%q,"containers":[{"env":[%s]}]}}}}`, runnerIdentity, strings.Join(entries, ","))
}

func projectListPresentJSON(config provider.Config) string {
	return fmt.Sprintf(`[{"projectId":%q,"lifecycleState":"ACTIVE"}]`, config.GCP.Project)
}

func artifactsListPresentJSON(config provider.Config) string {
	name := fmt.Sprintf(
		"projects/%s/locations/%s/repositories/%s",
		config.GCP.Project,
		config.GCP.ArtifactRegistry.Location,
		config.GCP.ArtifactRegistry.Repository,
	)

	return fmt.Sprintf(`[{"name":%q,"format":"DOCKER"}]`, name)
}

func runServicesListPresentJSON(config provider.Config) string {
	name := fmt.Sprintf(
		"projects/%s/locations/%s/services/%s",
		config.GCP.Project,
		config.GCP.CloudRun.Region,
		config.GCP.CloudRun.Service,
	)

	return fmt.Sprintf(`[{"metadata":{"name":%q}}]`, name)
}

func presentStateRunner() *fakeRunner {
	config, err := provider.Load()
	if err != nil {
		panic(fmt.Sprintf("presentStateRunner: Load failed: %v", err))
	}

	return &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":          {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":             {Output: []byte(clerkAuthenticated)},
			gcloudAuthListKey():               {Output: []byte(gcloudAuthenticated)},
			d1ListKey():                       {Output: []byte(d1PresentJSON(config))},
			kvNamespaceListKey():              {Output: []byte(kvPresentJSON(config))},
			workerSecretListKey(config):       {Output: []byte(workerSecretNamesJSON(config))},
			workerDeploymentStatusKey(config): {Output: []byte(workerActiveDeployment)},
			workerVersionViewKey(config):      {Output: []byte(workerVersionBindingsJSON(config))},
			clerkAppsListKey():                {Output: []byte(clerkApplicationPresentJSON(config))},
			projectListKey(config):            {Output: []byte(projectListPresentJSON(config))},
			artifactsListKey(config):          {Output: []byte(artifactsListPresentJSON(config))},
			runServicesListKey(config):        {Output: []byte(runServicesListPresentJSON(config))},
			runServicesDescribeKey(config):    {Output: []byte(cloudRunServiceJSON(config, ""))},
		},
	}
}

func TestProviderInventoryCoversRequiredResources(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	required := map[string]string{
		"cloudflare.worker":       config.Cloudflare.Worker,
		"cloudflare.d1_database":  config.Cloudflare.D1Database,
		"cloudflare.kv_namespace": config.Cloudflare.KVNamespace,
		"clerk.application":       config.Clerk.Application,
		"gcp.project":             config.GCP.Project,
		"gcp.artifact_repository": config.GCP.ArtifactRegistry.Repository,
		"gcp.cloud_run_service":   config.GCP.CloudRun.Service,
		"gcp.cloud_run_identity":  config.GCP.CloudRun.RunnerIdentity,
	}

	for name, value := range required {
		if value == "" {
			t.Errorf("required provider inventory value %s is empty", name)
		}
	}
}

type fakeRunner struct {
	responses map[string]provider.CommandResult
	failures  map[string]error
}

func (r *fakeRunner) Run(_ context.Context, command string, args ...string) (provider.CommandResult, error) {
	key := command + " " + strings.Join(args, " ")
	if err, ok := r.failures[key]; ok {
		return provider.CommandResult{}, err
	}

	if result, ok := r.responses[key]; ok {
		return result, nil
	}

	return provider.CommandResult{}, fmt.Errorf("%w: %q", errUnexpectedCommand, key)
}

type countingRunner struct {
	inner *fakeRunner
	calls map[string]int
}

func (r *countingRunner) Run(ctx context.Context, command string, args ...string) (provider.CommandResult, error) {
	key := command + " " + strings.Join(args, " ")
	r.calls[key]++

	return r.inner.Run(ctx, command, args...)
}

func TestPreflightReportsMissingProviderStateWithoutSecretValues(t *testing.T) {
	raw := []byte(`{"leaked":"super-secret-value-do-not-log"}`)
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler --version": {Output: raw},
			"clerk --version":    {Output: raw},
			"gcloud --version":   {Output: raw},
		},
		failures: map[string]error{
			"wrangler whoami --json": errNotAuthenticated,
			"clerk whoami --json":    errNotAuthenticated,
			gcloudAuthListKey():      errNotAuthenticated,
		},
	}

	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if report.Ready {
		t.Error("expected report not ready")
	}

	statuses := map[string]provider.Status{}

	for _, check := range report.Checks {
		if check.Status == "" {
			t.Fatalf("check %+v has empty status", check)
		}
		statuses[check.Provider+"."+check.Resource] = check.Status
	}

	// Unstructured whoami failures are not positive proof of verified
	// absence; they must fail closed as unavailable.
	expectedUnavailableOnFailure := []string{
		"wrangler.authentication",
		"clerk.authentication",
		"gcloud.authentication",
	}
	for _, name := range expectedUnavailableOnFailure {
		if got := statuses[name]; got != provider.Unavailable {
			t.Errorf("%s status = %q, want %q", name, got, provider.Unavailable)
		}
	}

	expectedUnchecked := append([]string{
		"cloudflare.d1_database",
		"cloudflare.kv_namespace",
		"cloudflare.worker",
		"clerk.application",
		"gcp.project",
		"gcp.artifact_repository",
		"gcp.cloud_run_service",
		"gcp.cloud_run_identity",
	},
		surfaceCheckNames(config, "cloudflare-worker")...,
	)
	expectedUnchecked = append(expectedUnchecked, surfaceCheckNames(config, "cloud-run-runner")...)

	for _, name := range expectedUnchecked {
		if got := statuses[name]; got != provider.Unavailable {
			t.Errorf("%s status = %q, want %q because authentication is unavailable", name, got, provider.Unavailable)
		}
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if strings.Contains(string(encoded), "super-secret-value-do-not-log") {
		t.Fatalf("report leaked provider output: %s", encoded)
	}
}

func TestPreflightFailsClosedOnUnauthenticatedSuccess(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":         {Output: []byte(`{"loggedIn":false}`)},
			"clerk whoami --json":            {Output: []byte(clerkKeyless)},
			"gcloud auth list --format=json": {Output: []byte(gcloudNoActiveAccount)},
		},
	}

	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if report.Ready {
		t.Fatal("expected report not ready")
	}

	statuses := map[string]provider.Status{}
	for _, check := range report.Checks {
		statuses[check.Provider+"."+check.Resource] = check.Status
	}

	expectedFailClosed := map[string]provider.Status{
		"wrangler.authentication": provider.Unavailable,
		"clerk.authentication":    provider.Missing,
		"gcloud.authentication":   provider.Missing,
	}
	for name, want := range expectedFailClosed {
		if got := statuses[name]; got != want {
			t.Errorf("%s status = %q, want %q", name, got, want)
		}
	}
}

func TestPreflightFailsClosedOnMalformedAuthOutput(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":         {Output: []byte("not json")},
			"clerk whoami --json":            {Output: []byte(`{"linked":{}}`)},
			"gcloud auth list --format=json": {Output: []byte(`{"accounts":[]}`)},
		},
	}

	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if report.Ready {
		t.Fatal("expected report not ready")
	}

	statuses := map[string]provider.Status{}
	for _, check := range report.Checks {
		statuses[check.Provider+"."+check.Resource] = check.Status
	}

	for _, name := range []string{"wrangler.authentication", "clerk.authentication", "gcloud.authentication"} {
		if got := statuses[name]; got != provider.Unavailable {
			t.Errorf("%s status = %q, want %q for malformed provider output", name, got, provider.Unavailable)
		}
	}
}

func TestPreflightReportsMissingResourcesWhenAuthenticated(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json": {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":    {Output: []byte(clerkAuthenticated)},
			gcloudAuthListKey():      {Output: []byte(gcloudAuthenticated)},
		},
		failures: map[string]error{
			d1ListKey():                       errResourceNotFound,
			kvNamespaceListKey():              errResourceNotFound,
			workerSecretListKey(config):       errResourceNotFound,
			workerDeploymentStatusKey(config): errResourceNotFound,
			clerkAppsListKey():                errResourceNotFound,
			artifactsListKey(config):          errResourceNotFound,
			runServicesListKey(config):        errResourceNotFound,
		},
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	statuses := map[string]provider.Status{}
	for _, check := range report.Checks {
		statuses[check.Provider+"."+check.Resource] = check.Status
	}
	// "resource not found" text from the fake command errors is unstructured
	// evidence, so these failures must fail closed as unavailable. Verified
	// absence is covered by positively parsed responses elsewhere.
	expectedUnavailable := append([]string{
		"cloudflare.d1_database",
		"cloudflare.kv_namespace",
		"cloudflare.worker",
		"clerk.application",
		"gcp.artifact_repository",
		"gcp.cloud_run_service",
		"gcp.cloud_run_identity",
	},
		surfaceCheckNames(config, "cloudflare-worker")...,
	)
	expectedUnavailable = append(expectedUnavailable, surfaceCheckNames(config, "cloud-run-runner")...)

	if got := statuses["cloudflare.secret:CLERK_SECRET_KEY"]; got != provider.Unavailable {
		t.Errorf("cloudflare.secret:CLERK_SECRET_KEY status = %q, want %q because the Worker is missing", got, provider.Unavailable)
	}

	for _, name := range expectedUnavailable {
		if got := statuses[name]; got != provider.Unavailable {
			t.Errorf("%s status = %q, want %q", name, got, provider.Unavailable)
		}
	}
}

func TestPreflightVerifiesConfiguredProviderResources(t *testing.T) {
	runner := presentStateRunner()

	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if !report.Ready {
		t.Fatalf("expected ready report, got %#v", report)
	}

	for _, check := range report.Checks {
		if check.Status != provider.OK {
			t.Errorf("%s.%s status = %q, want %q", check.Provider, check.Resource, check.Status, provider.OK)
		}
	}
}

func TestPreflightReportsAccessFailuresAsUnavailable(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":          {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":             {Output: []byte(clerkAuthenticated)},
			gcloudAuthListKey():               {Output: []byte(gcloudAuthenticated)},
			workerSecretListKey(config):       {Output: []byte(workerSecretNamesJSON(config))},
			workerDeploymentStatusKey(config): {Output: []byte(workerActiveDeployment)},
			workerVersionViewKey(config):      {Output: []byte(workerVersionBindingsJSON(config))},
			projectListKey(config):            {Output: []byte(projectListPresentJSON(config))},
			runServicesListKey(config):        {Output: []byte(runServicesListPresentJSON(config))},
			runServicesDescribeKey(config):    {Output: []byte(cloudRunServiceJSON(config, ""))},
		},
		failures: map[string]error{
			d1ListKey():                errUnauthorized,
			kvNamespaceListKey():       errPermissionDenied,
			clerkAppsListKey():         errUnauthorized,
			artifactsListKey(config):   errResourceNotFound,
			runServicesListKey(config): errNetworkFailure,
		},
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if report.Ready {
		t.Fatal("expected report not ready")
	}

	statuses := map[string]provider.Status{}
	for _, check := range report.Checks {
		statuses[check.Provider+"."+check.Resource] = check.Status
	}

	expectedUnavailable := []string{
		"cloudflare.d1_database",
		"cloudflare.kv_namespace",
		"clerk.application",
		"gcp.cloud_run_service",
	}
	for _, name := range expectedUnavailable {
		if got := statuses[name]; got != provider.Unavailable {
			t.Errorf("%s status = %q, want %q for an access or unclassified failure", name, got, provider.Unavailable)
		}
	}

	if got := statuses["gcp.artifact_repository"]; got != provider.Unavailable {
		t.Errorf("gcp.artifact_repository status = %q, want %q: unstructured not-found text must fail closed", got, provider.Unavailable)
	}
}

// kvTestResponses builds the KV exact-title test state with a controlled KV
// namespace list response and inventory-derived responses everywhere else.
func kvTestResponses(config provider.Config, kvResponse string) map[string]provider.CommandResult {
	return map[string]provider.CommandResult{
		"wrangler whoami --json":          {Output: []byte(wranglerAuthenticated)},
		"clerk whoami --json":             {Output: []byte(clerkAuthenticated)},
		gcloudAuthListKey():               {Output: []byte(gcloudAuthenticated)},
		d1ListKey():                       {Output: []byte(d1PresentJSON(config))},
		kvNamespaceListKey():              {Output: []byte(kvResponse)},
		workerSecretListKey(config):       {Output: []byte(workerSecretNamesJSON(config))},
		workerDeploymentStatusKey(config): {Output: []byte(workerActiveDeployment)},
		workerVersionViewKey(config):      {Output: []byte(workerVersionBindingsJSON(config))},
		clerkAppsListKey():                {Output: []byte(clerkApplicationPresentJSON(config))},
		projectListKey(config):            {Output: []byte(projectListPresentJSON(config))},
		artifactsListKey(config):          {Output: []byte(artifactsListPresentJSON(config))},
		runServicesListKey(config):        {Output: []byte(runServicesListPresentJSON(config))},
		runServicesDescribeKey(config):    {Output: []byte(cloudRunServiceJSON(config, ""))},
	}
}

func TestPreflightKVMatchesExactTitleOnly(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	runner := &fakeRunner{responses: kvTestResponses(config, `[{"id":"a","title":"gog-marketing-test"},{"id":"b","title":"test-gog-marketing"},{"id":"c","title":"gog-marketingx"},{"id":"d","title":"xgog-marketing"},{"id":"e","title":"gog-marketing-prod"}]`)}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	var kvStatus provider.Status

	for _, check := range report.Checks {
		if check.Provider == "cloudflare" && check.Resource == "kv_namespace" {
			kvStatus = check.Status
		}
	}

	if kvStatus != provider.Missing {
		t.Fatalf("kv_namespace status = %q, want %q because only suffix/prefix/substring titles exist", kvStatus, provider.Missing)
	}

	malformed := &fakeRunner{responses: kvTestResponses(config, `{"data":[]}`)}

	report, err = provider.Preflight(context.Background(), config, provider.Options{Runner: malformed})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	for _, check := range report.Checks {
		if check.Provider == "cloudflare" && check.Resource == "kv_namespace" {
			if check.Status != provider.Unavailable {
				t.Errorf("kv_namespace status = %q, want %q for malformed output", check.Status, provider.Unavailable)
			}

			return
		}
	}

	t.Fatal("expected a kv_namespace check")
}

func TestPreflightReportsMissingBoundNames(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cloudRunOnlyClientSecret := cloudRunServiceJSON(
		config,
		"",
		"GOG_GOOGLE_OAUTH_CLIENT_ID",
		"GOG_RUNNER_INVOCATION_TOKEN",
	)
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":          {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":             {Output: []byte(clerkAuthenticated)},
			gcloudAuthListKey():               {Output: []byte(gcloudAuthenticated)},
			d1ListKey():                       {Output: []byte(d1PresentJSON(config))},
			kvNamespaceListKey():              {Output: []byte(kvPresentJSON(config))},
			workerSecretListKey(config):       {Output: []byte(workerSecretNamesJSON(config))},
			workerDeploymentStatusKey(config): {Output: []byte(workerActiveDeployment)},
			workerVersionViewKey(config):      {Output: []byte(workerVersionBindingsJSON(config, "GOG_CLOUD_RUN_SERVICE_URL"))},
			clerkAppsListKey():                {Output: []byte(clerkApplicationPresentJSON(config))},
			projectListKey(config):            {Output: []byte(projectListPresentJSON(config))},
			artifactsListKey(config):          {Output: []byte(artifactsListPresentJSON(config))},
			runServicesListKey(config):        {Output: []byte(runServicesListPresentJSON(config))},
			runServicesDescribeKey(config):    {Output: []byte(cloudRunOnlyClientSecret)},
		},
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if report.Ready {
		t.Fatal("expected report not ready when a documented bound name is missing")
	}

	statuses := map[string]provider.Status{}
	for _, check := range report.Checks {
		statuses[check.Provider+"."+check.Resource] = check.Status
	}

	expectedMissing := map[string]provider.Status{
		"cloudflare.env:GOG_CLOUD_RUN_SERVICE_URL":         provider.Missing,
		"gcp.cloud_run_env:GOG_GOOGLE_OAUTH_CLIENT_ID":     provider.Missing,
		"gcp.cloud_run_secret:GOG_RUNNER_INVOCATION_TOKEN": provider.Missing,
	}
	for name, want := range expectedMissing {
		if got := statuses[name]; got != want {
			t.Errorf("%s status = %q, want %q", name, got, want)
		}
	}
}

func TestPreflightReportsMismatchedRunnerIdentity(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	wrongIdentity := cloudRunServiceJSON(config, "wrong-identity@gog-marketing-prod.iam.gserviceaccount.com")
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":          {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":             {Output: []byte(clerkAuthenticated)},
			gcloudAuthListKey():               {Output: []byte(gcloudAuthenticated)},
			d1ListKey():                       {Output: []byte(d1PresentJSON(config))},
			kvNamespaceListKey():              {Output: []byte(kvPresentJSON(config))},
			workerSecretListKey(config):       {Output: []byte(workerSecretNamesJSON(config))},
			workerDeploymentStatusKey(config): {Output: []byte(workerActiveDeployment)},
			workerVersionViewKey(config):      {Output: []byte(workerVersionBindingsJSON(config))},
			clerkAppsListKey():                {Output: []byte(clerkApplicationPresentJSON(config))},
			projectListKey(config):            {Output: []byte(projectListPresentJSON(config))},
			artifactsListKey(config):          {Output: []byte(artifactsListPresentJSON(config))},
			runServicesListKey(config):        {Output: []byte(runServicesListPresentJSON(config))},
			runServicesDescribeKey(config):    {Output: []byte(wrongIdentity)},
		},
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if report.Ready {
		t.Error("expected report not ready")
	}

	var found bool
	var d1OK bool

	for _, check := range report.Checks {
		if check.Provider == "gcp" && check.Resource == "cloud_run_identity" {
			found = true

			if check.Status != provider.Mismatch {
				t.Errorf("cloud_run_identity status = %q, want %q", check.Status, provider.Mismatch)
			}
		}

		if check.Provider == "cloudflare" && check.Resource == "d1_database" && check.Status == provider.OK {
			d1OK = true
		}
	}

	if !found {
		t.Fatal("expected a cloud_run_identity check")
	}

	if !d1OK {
		t.Error("unrelated resource checks were affected by runner identity mismatch")
	}
}

func TestPreflightSurfaceMatchesExactTokensOnly(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	for i, binding := range config.Environment {
		if binding.Name == "CLERK_PUBLISHABLE_KEY" {
			config.Environment[i].Surface = "not-cloudflare-worker"
		}
	}

	for i, binding := range config.Secrets {
		if binding.Name == "GOG_RUNNER_INVOCATION_TOKEN" {
			config.Secrets[i].Surface = "cloud-run-runner;not-cloudflare-worker"
		}
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: presentStateRunner()})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	statuses := map[string]provider.Status{}
	for _, check := range report.Checks {
		statuses[check.Provider+"."+check.Resource] = check.Status
	}

	if got, present := statuses["cloudflare.env:CLERK_PUBLISHABLE_KEY"]; present {
		t.Errorf("surface not-cloudflare-worker must not match; got %s check with status %q", "cloudflare.env:CLERK_PUBLISHABLE_KEY", got)
	}

	if got, present := statuses["gcp.cloud_run_secret:GOG_GOOGLE_OAUTH_CLIENT_SECRET"]; present {
		t.Errorf("surface not-cloud-run-runner must not match; got %s check with status %q", "gcp.cloud_run_secret:GOG_GOOGLE_OAUTH_CLIENT_SECRET", got)
	}

	runnerTokenName := "gcp.cloud_run_secret:GOG_RUNNER_INVOCATION_TOKEN"
	if got := statuses[runnerTokenName]; got != provider.OK {
		t.Errorf("%s status = %q, want %q; exact runner token membership should still match", runnerTokenName, got, provider.OK)
	}

	workerTokenName := "cloudflare.secret:GOG_RUNNER_INVOCATION_TOKEN"
	if got, present := statuses[workerTokenName]; present {
		t.Errorf("surface not-cloudflare-worker must not create %s; got status %q", workerTokenName, got)
	}
}

func TestPreflightWorkerVariablesUseActiveDeploymentOnly(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	scenarios := []struct {
		name       string
		deployment string
		version    string
		want       provider.Status
	}{
		{
			name:       "unpublished upload is ignored and active deployment is checked",
			deployment: workerActiveDeployment,
			version:    workerVersionBindingsJSON(config, "GOG_CLOUD_RUN_SERVICE_URL"),
			want:       provider.Missing,
		},
		{
			name:       "traffic-split deployment fails closed",
			deployment: `{"id":"deployment-2","versions":[{"version_id":"` + activeVersionID + `","percentage":50},{"version_id":"version-2","percentage":50}]}`,
			version:    workerVersionBindingsJSON(config),
			want:       provider.Unavailable,
		},
		{
			name:       "malformed deployment output fails closed",
			deployment: `{"id":"deployment-3"}`,
			version:    workerVersionBindingsJSON(config),
			want:       provider.Unavailable,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			runner := &countingRunner{
				inner: &fakeRunner{
					responses: map[string]provider.CommandResult{
						"wrangler whoami --json":          {Output: []byte(wranglerAuthenticated)},
						"clerk whoami --json":             {Output: []byte(clerkAuthenticated)},
						gcloudAuthListKey():               {Output: []byte(gcloudAuthenticated)},
						d1ListKey():                       {Output: []byte(d1PresentJSON(config))},
						kvNamespaceListKey():              {Output: []byte(kvPresentJSON(config))},
						workerSecretListKey(config):       {Output: []byte(workerSecretNamesJSON(config))},
						workerDeploymentStatusKey(config): {Output: []byte(scenario.deployment)},
						workerVersionViewKey(config):      {Output: []byte(scenario.version)},
						clerkAppsListKey():                {Output: []byte(clerkApplicationPresentJSON(config))},
						projectListKey(config):            {Output: []byte(projectListPresentJSON(config))},
						artifactsListKey(config):          {Output: []byte(artifactsListPresentJSON(config))},
						runServicesListKey(config):        {Output: []byte(runServicesListPresentJSON(config))},
						runServicesDescribeKey(config):    {Output: []byte(cloudRunServiceJSON(config, ""))},
					},
				},
				calls: map[string]int{},
			}

			report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
			if err != nil {
				t.Fatalf("Preflight failed: %v", err)
			}

			statuses := map[string]provider.Status{}
			for _, check := range report.Checks {
				statuses[check.Provider+"."+check.Resource] = check.Status
			}

			if got := statuses["cloudflare.env:GOG_CLOUD_RUN_SERVICE_URL"]; got != scenario.want {
				t.Errorf("cloudflare.env:GOG_CLOUD_RUN_SERVICE_URL status = %q, want %q", got, scenario.want)
			}

			if got := runner.calls[commandKey("wrangler", "versions", "list", "--name", config.Cloudflare.Worker, "--json")]; got != 0 {
				t.Errorf("preflight called deployable versions list %d times; the unpublished latest version must not satisfy readiness", got)
			}
		})
	}
}

func TestPreflightFetchesWorkerVersionMetadataOncePerPreflight(t *testing.T) {
	runner := &countingRunner{
		inner: presentStateRunner(),
		calls: map[string]int{},
	}

	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if !report.Ready {
		t.Fatalf("expected ready report, got %#v", report)
	}

	for key, want := range map[string]int{
		workerDeploymentStatusKey(config): 1,
		workerVersionViewKey(config):      1,
		workerSecretListKey(config):       1,
	} {
		if got := runner.calls[key]; got != want {
			t.Errorf("%q was run %d times, want %d per preflight", key, got, want)
		}
	}
}

func TestPreflightChecksRunnerIdentityWhenOnlyArtifactRegistryIsMissing(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":          {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":             {Output: []byte(clerkAuthenticated)},
			gcloudAuthListKey():               {Output: []byte(gcloudAuthenticated)},
			d1ListKey():                       {Output: []byte(d1PresentJSON(config))},
			kvNamespaceListKey():              {Output: []byte(kvPresentJSON(config))},
			workerSecretListKey(config):       {Output: []byte(workerSecretNamesJSON(config))},
			workerDeploymentStatusKey(config): {Output: []byte(workerActiveDeployment)},
			workerVersionViewKey(config):      {Output: []byte(workerVersionBindingsJSON(config))},
			clerkAppsListKey():                {Output: []byte(clerkApplicationPresentJSON(config))},
			projectListKey(config):            {Output: []byte(projectListPresentJSON(config))},
			runServicesListKey(config):        {Output: []byte(runServicesListPresentJSON(config))},
			runServicesDescribeKey(config):    {Output: []byte(cloudRunServiceJSON(config, ""))},
		},
		failures: map[string]error{
			artifactsListKey(config): errResourceNotFound,
		},
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	var found bool

	for _, check := range report.Checks {
		if check.Provider == "gcp" && check.Resource == "cloud_run_identity" {
			found = true

			if check.Status != provider.OK {
				t.Errorf("cloud_run_identity status = %q, want %q", check.Status, provider.OK)
			}
		}
	}

	if !found {
		t.Fatal("expected a cloud_run_identity check")
	}
}

func TestProviderInventoryNamesEnvironmentAndSecrets(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(config.Environment) == 0 {
		t.Fatal("expected non-secret environment names in provider inventory")
	}

	if len(config.Secrets) == 0 {
		t.Fatal("expected secret names in provider inventory")
	}

	for _, item := range append(append([]provider.Binding{}, config.Environment...), config.Secrets...) {
		if item.Name == "" || item.Surface == "" {
			t.Fatalf("environment/secret inventory entry %+v lacks name or surface", item)
		}
	}

	for _, item := range config.Environment {
		if item.Secret {
			t.Errorf("environment entry %s must not be marked secret", item.Name)
		}
	}

	for _, item := range config.Secrets {
		if !item.Secret {
			t.Errorf("secret entry %s must be marked secret", item.Name)
		}
	}
}

func TestProviderInventoryDoesNotContainSecretValues(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "provider", "config.json"))
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}

	valueLike := regexp.MustCompile(`(?i)(secret|token|password|api[_-]?key)[[:space:]]*[:=][[:space:]]*[A-Za-z0-9_+/=-]{20,}`)
	if match := valueLike.Find(raw); match != nil {
		t.Fatalf("inventory contains a value-like secret assignment: %s", match)
	}
}

// TestProviderInventorySecretSurfaceBoundary pins the canonical secret
// placement: Go gets ephemeral access tokens, so refresh/client secret and
// root encryption material remain Worker-only. The runner's invocation
// signing token is intentionally shared, while Worker workload-identity
// signing material stays Worker-only.
func TestProviderInventorySecretSurfaceBoundary(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	surfaces := map[string]string{}
	for _, binding := range config.Secrets {
		surfaces[binding.Name] = binding.Surface
	}

	workerOnlySecrets := []string{
		"GOG_GOOGLE_OAUTH_CLIENT_SECRET",
		"GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY",
		"GOG_RUNNER_WIF_SIGNING_KEY",
	}
	for _, name := range workerOnlySecrets {
		surface, found := surfaces[name]
		if !found {
			t.Fatalf("required secret %s is absent from inventory", name)
		}

		if !includesSurface(surface, "cloudflare-worker") || includesSurface(surface, "cloud-run-runner") {
			t.Errorf("%s surface = %q, want cloudflare-worker only", name, surface)
		}
	}

	runnerTokenSurface, found := surfaces["GOG_RUNNER_INVOCATION_TOKEN"]
	if !found {
		t.Fatal("canonical runner secret GOG_RUNNER_INVOCATION_TOKEN is absent from inventory")
	}

	if !includesSurface(runnerTokenSurface, "cloudflare-worker") || !includesSurface(runnerTokenSurface, "cloud-run-runner") {
		t.Errorf("GOG_RUNNER_INVOCATION_TOKEN surface = %q, want both cloudflare-worker and cloud-run-runner", runnerTokenSurface)
	}
}

// TestPreflightVerifiesListStyleAbsencePositively proves each GCP list-style
// scoped query proves verified absence independently: with all upstream
// prerequisites positively present, an empty structured list reports the
// target resource missing with truthful detail text. A verified-absent
// project correctly gates its dependents as unavailable, and that gate is
// asserted explicitly rather than conflated with resource absence.
func TestPreflightVerifiesListStyleAbsencePositively(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	runnerSurfaceNames := surfaceCheckNames(config, "cloud-run-runner")

	scenarios := []struct {
		name            string
		emptyListKey    string
		wantMissing     string
		gatedNames      []string
		gatedDetail     string
		dependentDetail map[string]string
	}{
		{
			name:         "absent GCP project gates its dependents unavailable",
			emptyListKey: projectListKey(config),
			wantMissing:  "gcp.project",
			gatedNames: append([]string{
				"gcp.artifact_repository",
				"gcp.cloud_run_service",
				"gcp.cloud_run_identity",
			}, runnerSurfaceNames...),
			gatedDetail: "not checked because the GCP project is missing",
		},
		{
			name:         "absent artifact repository is positively parsed",
			emptyListKey: artifactsListKey(config),
			wantMissing:  "gcp.artifact_repository",
		},
		{
			name:            "absent Cloud Run service is positively parsed",
			emptyListKey:    runServicesListKey(config),
			wantMissing:     "gcp.cloud_run_service",
			dependentDetail: dependentDetailMap(runnerSurfaceNames, "not checked because the Cloud Run service is missing"),
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			base := presentStateRunner()

			responses := map[string]provider.CommandResult{}
			for key, value := range base.responses {
				responses[key] = value
			}
			responses[scenario.emptyListKey] = provider.CommandResult{Output: []byte("[]")}

			runner := &fakeRunner{responses: responses}

			report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
			if err != nil {
				t.Fatalf("Preflight failed: %v", err)
			}

			if report.Ready {
				t.Fatal("expected report not ready when a documented resource is verified absent")
			}

			statuses := map[string]provider.Status{}
			details := map[string]string{}

			for _, check := range report.Checks {
				statuses[check.Provider+"."+check.Resource] = check.Status
				details[check.Provider+"."+check.Resource] = check.Detail
			}

			if got := statuses[scenario.wantMissing]; got != provider.Missing {
				t.Errorf("%s status = %q, want %q from an empty positively parsed list", scenario.wantMissing, got, provider.Missing)
			}

			for name, wantDetail := range scenario.dependentDetail {
				if got := details[name]; got != wantDetail {
					t.Errorf("%s detail = %q, want %q", name, got, wantDetail)
				}
			}

			for _, name := range scenario.gatedNames {
				if got := statuses[name]; got != provider.Unavailable {
					t.Errorf("%s status = %q, want %q while the verified-absent project gates the check", name, got, provider.Unavailable)
				}

				if got := details[name]; got != scenario.gatedDetail {
					t.Errorf("%s detail = %q, want %q", name, got, scenario.gatedDetail)
				}
			}
		})
	}
}

// TestPreflightListShapeAnomalyFailsClosed proves that list entries without
// the exact expected name field are interpreted, never guessed: an anomaly
// fails closed as unavailable instead of asserting false absence.
func TestPreflightListShapeAnomalyFailsClosed(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":          {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":             {Output: []byte(clerkAuthenticated)},
			gcloudAuthListKey():               {Output: []byte(gcloudAuthenticated)},
			d1ListKey():                       {Output: []byte(d1PresentJSON(config))},
			kvNamespaceListKey():              {Output: []byte(kvPresentJSON(config))},
			workerSecretListKey(config):       {Output: []byte(workerSecretNamesJSON(config))},
			workerDeploymentStatusKey(config): {Output: []byte(workerActiveDeployment)},
			workerVersionViewKey(config):      {Output: []byte(workerVersionBindingsJSON(config))},
			clerkAppsListKey():                {Output: []byte(clerkApplicationPresentJSON(config))},
			projectListKey(config):            {Output: []byte(`[{"name":"not-a-project-id-field"}]`)},
			artifactsListKey(config):          {Output: []byte(`[{"repository":"gog-marketing"}]`)},
			runServicesListKey(config):        {Output: []byte(`[{"metadata":{"uid":"service-uid"}}]`)},
		},
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	statuses := map[string]provider.Status{}

	for _, check := range report.Checks {
		statuses[check.Provider+"."+check.Resource] = check.Status
	}

	for _, name := range []string{"gcp.project", "gcp.artifact_repository", "gcp.cloud_run_service"} {
		if got := statuses[name]; got != provider.Unavailable {
			t.Errorf("%s status = %q, want %q for a list entry without the expected field shape", name, got, provider.Unavailable)
		}
	}
}

// TestPreflightWorkerMissingVerifiedOnlyByStructuredCode proves Worker absence
// is classified only from the documented Cloudflare error code 10007 with the
// exact sentence emitted by wrangler deployments status; the raw generic
// "Worker ... not found." text emitted by wrangler secret list never proves
// absence, and dependent details always reflect the actual parent status.
func TestPreflightWorkerMissingVerifiedOnlyByStructuredCode(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	scenarios := []struct {
		name          string
		stderr        string
		wantWorker    provider.Status
		wantSecretDep provider.Status
		wantEnvDetail string
	}{
		{
			name:          "structured Cloudflare error code 10007 proves absence",
			stderr:        "A request to the Cloudflare API failed. This Worker does not exist on your account. [code: 10007]",
			wantWorker:    provider.Missing,
			wantSecretDep: provider.Missing,
			wantEnvDetail: "not checked because the Worker is missing",
		},
		{
			name:          "raw generic not-found marker fails closed",
			stderr:        "Worker \"" + config.Cloudflare.Worker + "\" not found.",
			wantWorker:    provider.Unavailable,
			wantSecretDep: provider.Unavailable,
			wantEnvDetail: "not checked because the Worker could not be verified",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			runner := &stderrRunner{
				inner:     presentStateRunner(),
				key:       workerDeploymentStatusKey(config),
				stderr:    scenario.stderr,
				err:       errNetworkFailure,
				returnStd: true,
			}

			report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
			if err != nil {
				t.Fatalf("Preflight failed: %v", err)
			}

			statuses := map[string]provider.Status{}
			details := map[string]string{}

			for _, check := range report.Checks {
				statuses[check.Provider+"."+check.Resource] = check.Status
				details[check.Provider+"."+check.Resource] = check.Detail
			}

			if got := statuses["cloudflare.worker"]; got != scenario.wantWorker {
				t.Fatalf("cloudflare.worker status = %q, want %q", got, scenario.wantWorker)
			}

			if got := statuses["cloudflare.secret:"+config.Secrets[0].Name]; got != scenario.wantSecretDep {
				t.Errorf("secret dependent status = %q, want %q", got, scenario.wantSecretDep)
			}

			envDetail := details["cloudflare.env:CLERK_PUBLISHABLE_KEY"]
			if envDetail != scenario.wantEnvDetail {
				t.Errorf("env dependent detail = %q, want %q", envDetail, scenario.wantEnvDetail)
			}

			if scenario.wantWorker != provider.Missing && strings.Contains(envDetail, "missing") {
				t.Errorf("env dependent detail = %q, must not claim the Worker is missing", envDetail)
			}
		})
	}
}

// TestPreflightDependentDetailsReflectParentStatus proves no dependent check
// ever claims its parent is missing when the parent actually timed out or was
// unverified, and that timed-out details carry the larger --command-timeout
// retry advice.
func TestPreflightDependentDetailsReflectParentStatus(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	deadlineError := fmt.Errorf("gcloud: %w", context.DeadlineExceeded)

	scenarios := []struct {
		name              string
		key               string
		wantServiceStatus provider.Status
		wantDetailPart    string
		forbiddenPart     string
	}{
		{
			name:              "timed-out Cloud Run service list keeps dependents truthful",
			key:               runServicesListKey(config),
			wantServiceStatus: provider.TimedOut,
			wantDetailPart:    "timed out",
			forbiddenPart:     "missing",
		},
		{
			name:              "unverified Cloud Run service list keeps dependents truthful",
			key:               runServicesListKey(config),
			wantServiceStatus: provider.Unavailable,
			wantDetailPart:    "could not be verified",
			forbiddenPart:     "missing",
		},
		{
			name:              "timed-out Worker probe keeps dependents truthful",
			key:               workerDeploymentStatusKey(config),
			wantServiceStatus: provider.TimedOut,
			wantDetailPart:    "timed out",
			forbiddenPart:     "missing",
		},
	}

	for i, scenario := range scenarios {
		errValue := deadlineError
		if i == 1 {
			errValue = errNetworkFailure
		}

		t.Run(scenario.name, func(t *testing.T) {
			runner := &overrideRunner{
				inner: presentStateRunner(),
				key:   scenario.key,
				err:   errValue,
			}

			report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
			if err != nil {
				t.Fatalf("Preflight failed: %v", err)
			}

			checks := map[string]provider.Check{}
			for _, check := range report.Checks {
				checks[check.Provider+"."+check.Resource] = check
			}

			var dependentNames []string
			var parentName string

			switch scenario.key {
			case runServicesListKey(config):
				parentName = "gcp.cloud_run_service"

				dependentNames = append(
					[]string{"gcp.cloud_run_identity"},
					surfaceCheckNames(config, "cloud-run-runner")...,
				)
			default:
				parentName = "cloudflare.worker"
				dependentNames = []string{"cloudflare.secret:CLERK_SECRET_KEY", "cloudflare.env:CLERK_PUBLISHABLE_KEY"}
			}

			if got := checks[parentName].Status; got != scenario.wantServiceStatus {
				t.Fatalf("%s status = %q, want %q", parentName, got, scenario.wantServiceStatus)
			}

			for _, name := range dependentNames {
				detail := checks[name].Detail
				if !strings.Contains(detail, scenario.wantDetailPart) {
					t.Errorf("%s detail = %q, want it to reflect the parent's %q state", name, detail, scenario.wantDetailPart)
				}

				if strings.Contains(strings.ToLower(detail), scenario.forbiddenPart) {
					t.Errorf("%s detail = %q, must not claim the parent is %s", name, detail, scenario.forbiddenPart)
				}
			}
		})
	}
}
