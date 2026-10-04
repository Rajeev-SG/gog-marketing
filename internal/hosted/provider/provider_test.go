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

const (
	wranglerAuthenticated = `{"loggedIn":true,"authType":"OAuth Token","accounts":[{"id":"account-id","name":"account-name"}]}`
	clerkAuthenticated    = `{"email":"owner@example.com","localSecretKeySource":null,"linked":null}`
	gcloudAuthenticated   = `[{"account":"owner@example.com","status":"ACTIVE"}]`
	gcloudNoActiveAccount = `[{"account":"owner@example.com","status":"INACTIVE"}]`
	clerkKeyless          = `{"email":null,"accountless":{"instanceId":"instance-id"},"keyless":{"instanceId":"instance-id"},"linked":null}`

	d1Present = `[{"name":"gog-marketing"}]`
	kvPresent = `[{"id":"namespace-id","title":"gog-marketing"}]`

	workerSecretNames = `[{"name":"CLERK_SECRET_KEY"},{"name":"GOG_GOOGLE_OAUTH_CLIENT_SECRET"},{"name":"GOG_HOSTED_CREDENTIAL_ENCRYPTION_KEY"},{"name":"GOG_RUNNER_INVOCATION_TOKEN"}]`

	workerActiveDeployment = `{"id":"deployment-1","created_on":"2026-10-04T00:00:00.000Z","versions":[{"version_id":"version-1","percentage":100}]}`

	workerActiveVersionBindings = `{"id":"version-1","resources":{"bindings":[
    {"type":"plain_text","name":"CLERK_PUBLISHABLE_KEY","text":"fake-var-value"},
    {"type":"plain_text","name":"CLERK_ISSUER","text":"fake-var-value"},
    {"type":"plain_text","name":"GOG_GOOGLE_OAUTH_CLIENT_ID","text":"fake-var-value"},
    {"type":"plain_text","name":"GOG_CLOUD_RUN_SERVICE_URL","text":"fake-var-value"}
  ]}}`

	workerActiveVersionMissingGOGCloudRunServiceURL = `{"id":"version-1","resources":{"bindings":[
    {"type":"plain_text","name":"CLERK_PUBLISHABLE_KEY","text":"fake-var-value"},
    {"type":"plain_text","name":"CLERK_ISSUER","text":"fake-var-value"},
    {"type":"plain_text","name":"GOG_GOOGLE_OAUTH_CLIENT_ID","text":"fake-var-value"}
  ]}}`

	clerkApplicationPresent = `[{"name":"gog-marketing"}]`

	gcloudProjectPresent = `{"projectId":"gog-marketing-prod"}`

	artifactRepositoryPresent = `{"name":"projects/gog-marketing-prod/locations/europe-west2/repositories/gog-marketing"}`

	cloudRunServicePresent = `{
  "spec": {
    "template": {
      "spec": {
        "serviceAccountName": "gog-marketing-runner@gog-marketing-prod.iam.gserviceaccount.com",
        "containers": [
          {
            "env": [
              {"name": "GOG_GOOGLE_OAUTH_CLIENT_ID"},
              {"name": "GOG_GOOGLE_OAUTH_CLIENT_SECRET", "valueFrom": {"secretKeyRef": {"name": "gog-google-oauth-client-secret", "key": "latest"}}},
              {"name": "GOG_RUNNER_INVOCATION_TOKEN", "valueFrom": {"secretKeyRef": {"name": "gog-runner-invocation-token", "key": "latest"}}}
            ]
          }
        ]
      }
    }
  }
}`
)

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

func presentStateRunner() *fakeRunner {
	return &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":                                       {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":                                          {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json":                               {Output: []byte(gcloudAuthenticated)},
			"wrangler d1 list --json":                                      {Output: []byte(d1Present)},
			"wrangler kv namespace list":                                   {Output: []byte(kvPresent)},
			"wrangler secret list --name gog-marketing --format=json":      {Output: []byte(workerSecretNames)},
			"wrangler deployments status --name gog-marketing --json":      {Output: []byte(workerActiveDeployment)},
			"wrangler versions view version-1 --name gog-marketing --json": {Output: []byte(workerActiveVersionBindings)},
			"clerk apps list --json":                                       {Output: []byte(clerkApplicationPresent)},
			"gcloud projects describe gog-marketing-prod --format=json":    {Output: []byte(gcloudProjectPresent)},
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": {Output: []byte(artifactRepositoryPresent)},
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      {Output: []byte(cloudRunServicePresent)},
		},
	}
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
			"wrangler whoami --json":                                    errNotAuthenticated,
			"clerk whoami --json":                                       errNotAuthenticated,
			"gcloud auth list --format=json":                            errNotAuthenticated,
			"wrangler d1 list --json":                                   errUnauthorized,
			"wrangler kv namespace list":                                errUnauthorized,
			"wrangler secret list --name gog-marketing --format=json":   errNotFound,
			"wrangler deployments status --name gog-marketing --json":   errNotFound,
			"clerk apps list --json":                                    errUnauthorized,
			"gcloud projects describe gog-marketing-prod --format=json": errNotFound,
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

	expectedMissing := []string{
		"wrangler.authentication",
		"clerk.authentication",
		"gcloud.authentication",
	}
	for _, name := range expectedMissing {
		if got := statuses[name]; got != provider.Missing {
			t.Errorf("%s status = %q, want %q", name, got, provider.Missing)
		}
	}

	expectedUnchecked := []string{
		"cloudflare.d1_database",
		"cloudflare.kv_namespace",
		"cloudflare.worker",
		"cloudflare.env:CLERK_PUBLISHABLE_KEY",
		"cloudflare.secret:CLERK_SECRET_KEY",
		"clerk.application",
		"gcp.project",
		"gcp.artifact_repository",
		"gcp.cloud_run_service",
		"gcp.cloud_run_identity",
		"gcp.cloud_run_env:GOG_GOOGLE_OAUTH_CLIENT_ID",
		"gcp.cloud_run_secret:GOG_GOOGLE_OAUTH_CLIENT_SECRET",
	}
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
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":                                    {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":                                       {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json":                            {Output: []byte(gcloudAuthenticated)},
			"gcloud projects describe gog-marketing-prod --format=json": {Output: []byte(gcloudProjectPresent)},
		},
		failures: map[string]error{
			"wrangler d1 list --json":                                 errResourceNotFound,
			"wrangler kv namespace list":                              errResourceNotFound,
			"wrangler secret list --name gog-marketing --format=json": errResourceNotFound,
			"wrangler deployments status --name gog-marketing --json": errResourceNotFound,
			"clerk apps list --json":                                  errResourceNotFound,
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": errResourceNotFound,
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      errResourceNotFound,
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

	statuses := map[string]provider.Status{}
	for _, check := range report.Checks {
		statuses[check.Provider+"."+check.Resource] = check.Status
	}
	expectedMissing := map[string]provider.Status{
		"cloudflare.d1_database":  provider.Missing,
		"cloudflare.kv_namespace": provider.Missing,
		"cloudflare.worker":       provider.Missing,
		"clerk.application":       provider.Missing,
		"gcp.artifact_repository": provider.Missing,
		"gcp.cloud_run_service":   provider.Missing,
		"gcp.cloud_run_identity":  provider.Unavailable,
	}
	expectedUnavailable := []string{
		"cloudflare.env:CLERK_PUBLISHABLE_KEY",
		"cloudflare.secret:CLERK_SECRET_KEY",
		"gcp.cloud_run_env:GOG_GOOGLE_OAUTH_CLIENT_ID",
		"gcp.cloud_run_secret:GOG_GOOGLE_OAUTH_CLIENT_SECRET",
	}

	if got := statuses["cloudflare.secret:CLERK_SECRET_KEY"]; got != provider.Unavailable {
		t.Errorf("cloudflare.secret:CLERK_SECRET_KEY status = %q, want %q because the Worker is missing", got, provider.Unavailable)
	}

	for _, name := range expectedUnavailable {
		if got := statuses[name]; got != provider.Unavailable {
			t.Errorf("%s status = %q, want %q", name, got, provider.Unavailable)
		}
	}

	for name, want := range expectedMissing {
		if got := statuses[name]; got != want {
			t.Errorf("%s status = %q, want %q", name, got, want)
		}
	}
}

func TestPreflightVerifiesConfiguredProviderResources(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":                                       {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":                                          {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json":                               {Output: []byte(gcloudAuthenticated)},
			"wrangler d1 list --json":                                      {Output: []byte(d1Present)},
			"wrangler kv namespace list":                                   {Output: []byte(kvPresent)},
			"wrangler secret list --name gog-marketing --format=json":      {Output: []byte(workerSecretNames)},
			"wrangler deployments status --name gog-marketing --json":      {Output: []byte(workerActiveDeployment)},
			"wrangler versions view version-1 --name gog-marketing --json": {Output: []byte(workerActiveVersionBindings)},
			"clerk apps list --json":                                       {Output: []byte(clerkApplicationPresent)},
			"gcloud projects describe gog-marketing-prod --format=json":    {Output: []byte(gcloudProjectPresent)},
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": {Output: []byte(artifactRepositoryPresent)},
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      {Output: []byte(cloudRunServicePresent)},
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
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":                                       {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":                                          {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json":                               {Output: []byte(gcloudAuthenticated)},
			"wrangler secret list --name gog-marketing --format=json":      {Output: []byte(workerSecretNames)},
			"wrangler deployments status --name gog-marketing --json":      {Output: []byte(workerActiveDeployment)},
			"wrangler versions view version-1 --name gog-marketing --json": {Output: []byte(workerActiveVersionBindings)},
			"clerk apps list --json":                                       {Output: []byte(clerkApplicationPresent)},
			"gcloud projects describe gog-marketing-prod --format=json":    {Output: []byte(gcloudProjectPresent)},
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": {Output: []byte(artifactRepositoryPresent)},
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      {Output: []byte(cloudRunServicePresent)},
		},
		failures: map[string]error{
			"wrangler d1 list --json":    errUnauthorized,
			"wrangler kv namespace list": errPermissionDenied,
			"clerk apps list --json":     errUnauthorized,
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": errResourceNotFound,
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      errNetworkFailure,
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

	if got := statuses["gcp.artifact_repository"]; got != provider.Missing {
		t.Errorf("gcp.artifact_repository status = %q, want %q for verified absence", got, provider.Missing)
	}
}

func TestPreflightKVMatchesExactTitleOnly(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":                                       {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":                                          {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json":                               {Output: []byte(gcloudAuthenticated)},
			"wrangler d1 list --json":                                      {Output: []byte(d1Present)},
			"wrangler kv namespace list":                                   {Output: []byte(`[{"id":"a","title":"gog-marketing-test"},{"id":"b","title":"test-gog-marketing"},{"id":"c","title":"gog-marketingx"},{"id":"d","title":"xgog-marketing"},{"id":"e","title":"gog-marketing-prod"}]`)},
			"wrangler secret list --name gog-marketing --format=json":      {Output: []byte(workerSecretNames)},
			"wrangler deployments status --name gog-marketing --json":      {Output: []byte(workerActiveDeployment)},
			"wrangler versions view version-1 --name gog-marketing --json": {Output: []byte(workerActiveVersionBindings)},
			"clerk apps list --json":                                       {Output: []byte(clerkApplicationPresent)},
			"gcloud projects describe gog-marketing-prod --format=json":    {Output: []byte(gcloudProjectPresent)},
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": {Output: []byte(artifactRepositoryPresent)},
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      {Output: []byte(cloudRunServicePresent)},
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

	var kvStatus provider.Status

	for _, check := range report.Checks {
		if check.Provider == "cloudflare" && check.Resource == "kv_namespace" {
			kvStatus = check.Status
		}
	}

	if kvStatus != provider.Missing {
		t.Fatalf("kv_namespace status = %q, want %q because only suffix/prefix/substring titles exist", kvStatus, provider.Missing)
	}

	malformed := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":                                       {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":                                          {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json":                               {Output: []byte(gcloudAuthenticated)},
			"wrangler d1 list --json":                                      {Output: []byte(d1Present)},
			"wrangler kv namespace list":                                   {Output: []byte(`{"data":[]}`)},
			"wrangler secret list --name gog-marketing --format=json":      {Output: []byte(workerSecretNames)},
			"wrangler deployments status --name gog-marketing --json":      {Output: []byte(workerActiveDeployment)},
			"wrangler versions view version-1 --name gog-marketing --json": {Output: []byte(workerActiveVersionBindings)},
			"clerk apps list --json":                                       {Output: []byte(clerkApplicationPresent)},
			"gcloud projects describe gog-marketing-prod --format=json":    {Output: []byte(gcloudProjectPresent)},
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": {Output: []byte(artifactRepositoryPresent)},
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      {Output: []byte(cloudRunServicePresent)},
		},
	}

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
	cloudRunOnlyClientSecret := `{"spec":{"template":{"spec":{"serviceAccountName":"gog-marketing-runner@gog-marketing-prod.iam.gserviceaccount.com","containers":[{"env":[{"name":"GOG_GOOGLE_OAUTH_CLIENT_SECRET","valueFrom":{"secretKeyRef":{"name":"gog-google-oauth-client-secret","key":"latest"}}}]}]}}}}`
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":                                       {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":                                          {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json":                               {Output: []byte(gcloudAuthenticated)},
			"wrangler d1 list --json":                                      {Output: []byte(d1Present)},
			"wrangler kv namespace list":                                   {Output: []byte(kvPresent)},
			"wrangler secret list --name gog-marketing --format=json":      {Output: []byte(workerSecretNames)},
			"wrangler deployments status --name gog-marketing --json":      {Output: []byte(workerActiveDeployment)},
			"wrangler versions view version-1 --name gog-marketing --json": {Output: []byte(workerActiveVersionMissingGOGCloudRunServiceURL)},
			"clerk apps list --json":                                       {Output: []byte(clerkApplicationPresent)},
			"gcloud projects describe gog-marketing-prod --format=json":    {Output: []byte(gcloudProjectPresent)},
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": {Output: []byte(artifactRepositoryPresent)},
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      {Output: []byte(cloudRunOnlyClientSecret)},
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
	wrongIdentity := strings.Replace(cloudRunServicePresent, "gog-marketing-runner@gog-marketing-prod.iam.gserviceaccount.com", "wrong-identity@gog-marketing-prod.iam.gserviceaccount.com", 1)
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":                                       {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":                                          {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json":                               {Output: []byte(gcloudAuthenticated)},
			"wrangler d1 list --json":                                      {Output: []byte(d1Present)},
			"wrangler kv namespace list":                                   {Output: []byte(kvPresent)},
			"wrangler secret list --name gog-marketing --format=json":      {Output: []byte(workerSecretNames)},
			"wrangler deployments status --name gog-marketing --json":      {Output: []byte(workerActiveDeployment)},
			"wrangler versions view version-1 --name gog-marketing --json": {Output: []byte(workerActiveVersionBindings)},
			"clerk apps list --json":                                       {Output: []byte(clerkApplicationPresent)},
			"gcloud projects describe gog-marketing-prod --format=json":    {Output: []byte(gcloudProjectPresent)},
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": {Output: []byte(artifactRepositoryPresent)},
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      {Output: []byte(wrongIdentity)},
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
		if binding.Name == "GOG_GOOGLE_OAUTH_CLIENT_SECRET" {
			config.Secrets[i].Surface = "cloudflare-worker;not-cloud-run-runner"
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

	if got := statuses["cloudflare.secret:GOG_GOOGLE_OAUTH_CLIENT_SECRET"]; got != provider.OK {
		t.Errorf("surface token membership should keep the Worker surface; got status %q", got)
	}
}

func TestPreflightWorkerVariablesUseActiveDeploymentOnly(t *testing.T) {
	scenarios := []struct {
		name       string
		deployment any
		version    string
		want       provider.Status
	}{
		{
			name:       "unpublished upload is ignored and active deployment is checked",
			deployment: workerActiveDeployment,
			version:    workerActiveVersionMissingGOGCloudRunServiceURL,
			want:       provider.Missing,
		},
		{
			name:       "traffic-split deployment fails closed",
			deployment: `{"id":"deployment-2","versions":[{"version_id":"version-1","percentage":50},{"version_id":"version-2","percentage":50}]}`,
			version:    workerActiveVersionBindings,
			want:       provider.Unavailable,
		},
		{
			name:       "malformed deployment output fails closed",
			deployment: `{"id":"deployment-3"}`,
			version:    workerActiveVersionBindings,
			want:       provider.Unavailable,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			runner := &countingRunner{
				inner: &fakeRunner{
					responses: map[string]provider.CommandResult{
						"wrangler whoami --json":                                       {Output: []byte(wranglerAuthenticated)},
						"clerk whoami --json":                                          {Output: []byte(clerkAuthenticated)},
						"gcloud auth list --format=json":                               {Output: []byte(gcloudAuthenticated)},
						"wrangler d1 list --json":                                      {Output: []byte(d1Present)},
						"wrangler kv namespace list":                                   {Output: []byte(kvPresent)},
						"wrangler secret list --name gog-marketing --format=json":      {Output: []byte(workerSecretNames)},
						"wrangler deployments status --name gog-marketing --json":      {Output: []byte(scenario.deployment.(string))},
						"wrangler versions view version-1 --name gog-marketing --json": {Output: []byte(scenario.version)},
						"clerk apps list --json":                                       {Output: []byte(clerkApplicationPresent)},
						"gcloud projects describe gog-marketing-prod --format=json":    {Output: []byte(gcloudProjectPresent)},
						"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": {Output: []byte(artifactRepositoryPresent)},
						"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json":      {Output: []byte(cloudRunServicePresent)},
					},
				},
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

			statuses := map[string]provider.Status{}
			for _, check := range report.Checks {
				statuses[check.Provider+"."+check.Resource] = check.Status
			}

			if got := statuses["cloudflare.env:GOG_CLOUD_RUN_SERVICE_URL"]; got != scenario.want {
				t.Errorf("cloudflare.env:GOG_CLOUD_RUN_SERVICE_URL status = %q, want %q", got, scenario.want)
			}

			if got := runner.calls["wrangler versions list --name gog-marketing --json"]; got != 0 {
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
		"wrangler deployments status --name gog-marketing --json":      1,
		"wrangler versions view version-1 --name gog-marketing --json": 1,
	} {
		if got := runner.calls[key]; got != want {
			t.Errorf("%q was run %d times, want %d per preflight", key, got, want)
		}
	}
}

func TestPreflightChecksRunnerIdentityWhenOnlyArtifactRegistryIsMissing(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":                                       {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":                                          {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json":                               {Output: []byte(gcloudAuthenticated)},
			"wrangler d1 list --json":                                      {Output: []byte(d1Present)},
			"wrangler kv namespace list":                                   {Output: []byte(kvPresent)},
			"wrangler secret list --name gog-marketing --format=json":      {Output: []byte(workerSecretNames)},
			"wrangler deployments status --name gog-marketing --json":      {Output: []byte(workerActiveDeployment)},
			"wrangler versions view version-1 --name gog-marketing --json": {Output: []byte(workerActiveVersionBindings)},
			"clerk apps list --json":                                       {Output: []byte(clerkApplicationPresent)},
			"gcloud projects describe gog-marketing-prod --format=json":    {Output: []byte(gcloudProjectPresent)},
			"gcloud run services describe gog-marketing-runner --region=europe-west2 --project=gog-marketing-prod --format=json": {Output: []byte(cloudRunServicePresent)},
		},
		failures: map[string]error{
			"gcloud artifacts repositories describe gog-marketing --location=europe-west2 --project=gog-marketing-prod --format=json": errResourceNotFound,
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
