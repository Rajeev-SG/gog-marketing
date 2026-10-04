package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/hosted/provider"
)

// overrideRunner returns a fixed failure for exactly one command key and
// delegates every other command to the wrapped runner.
type overrideRunner struct {
	inner provider.Runner
	key   string
	err   error
}

func (r *overrideRunner) Run(ctx context.Context, command string, args ...string) (provider.CommandResult, error) {
	if command+" "+strings.Join(args, " ") == r.key {
		return provider.CommandResult{}, r.err
	}

	result, err := r.inner.Run(ctx, command, args...)
	if err != nil {
		return result, fmt.Errorf("run delegated command: %w", err)
	}

	return result, nil
}

// stderrRunner returns raw stderr with a failure for exactly one command key
// and delegates every other command to the wrapped runner.
type stderrRunner struct {
	inner     provider.Runner
	key       string
	stderr    string
	err       error
	returnStd bool
}

func (r *stderrRunner) Run(ctx context.Context, command string, args ...string) (provider.CommandResult, error) {
	if command+" "+strings.Join(args, " ") == r.key {
		if r.returnStd {
			return provider.CommandResult{Stderr: []byte(r.stderr)}, r.err
		}

		return provider.CommandResult{}, r.err
	}

	result, err := r.inner.Run(ctx, command, args...)
	if err != nil {
		return result, fmt.Errorf("run delegated command: %w", err)
	}

	return result, nil
}

// slowFirstRunner delays the first call to one command key, then delegates to
// the wrapped runner. It is used to prove that per-command deadlines reset.
type slowFirstRunner struct {
	inner  provider.Runner
	key    string
	delay  time.Duration
	slowed bool
}

func (r *slowFirstRunner) Run(ctx context.Context, command string, args ...string) (provider.CommandResult, error) {
	if command+" "+strings.Join(args, " ") == r.key && !r.slowed {
		r.slowed = true
		select {
		case <-time.After(r.delay):
		case <-ctx.Done():
			return provider.CommandResult{}, fmt.Errorf("%s: %w", command, ctx.Err())
		}
	}

	result, err := r.inner.Run(ctx, command, args...)
	if err != nil {
		return result, fmt.Errorf("run delegated command: %w", err)
	}

	return result, nil
}

func summaryFor(t *testing.T, report provider.Report) (map[string]provider.Status, provider.ReportSummary) {
	t.Helper()

	statuses := map[string]provider.Status{}
	for _, check := range report.Checks {
		statuses[check.Provider+"."+check.Resource] = check.Status
	}

	return statuses, report.Summary
}

// presentStateRunnerWithKV returns a fully present provider state whose KV
// namespace list contains a differently titled namespace only.
func presentStateRunnerWithKV(title string) *fakeRunner {
	runner := presentStateRunner()
	runner.responses["wrangler kv namespace list"] = provider.CommandResult{
		Output: []byte(`[{"id":"namespace-id","title":"` + title + `"}]`),
	}

	return runner
}

// TestPreflightDowngradesUnstructuredNotFoundEvidence verifies that raw
// stderr/error text is never treated as verified resource absence. Marker
// collisions and unrelated credential or network failures must fail closed as
// unavailable; only positively parsed provider responses may report missing.
func TestPreflightDowngradesUnstructuredNotFoundEvidence(t *testing.T) {
	authRunner := &fakeRunner{
		responses: map[string]provider.CommandResult{
			"wrangler whoami --json":         {Output: []byte(wranglerAuthenticated)},
			"clerk whoami --json":            {Output: []byte(clerkAuthenticated)},
			"gcloud auth list --format=json": {Output: []byte(gcloudAuthenticated)},
		},
	}

	scenarios := []struct {
		name      string
		err       error
		stderr    string
		returnStd bool
		key       string
		resource  string
	}{
		{
			name:      "unrelated not-found phrase in stderr",
			err:       errNetworkFailure,
			stderr:    "filter gog-marketing not found in output",
			returnStd: true,
			key:       "wrangler d1 list --json",
			resource:  "cloudflare.d1_database",
		},
		{
			name:     "unrelated not-found text in wrapped error",
			err:      errNotFound,
			key:      "wrangler kv namespace list",
			resource: "cloudflare.kv_namespace",
		},
		{
			name:      "credential error mentioning not found",
			err:       errNetworkFailure,
			stderr:    "could not find credentials for the requested account",
			returnStd: true,
			key:       "clerk apps list --json",
			resource:  "clerk.application",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			runner := &stderrRunner{inner: authRunner, key: scenario.key, stderr: scenario.stderr, err: scenario.err, returnStd: scenario.returnStd}

			config, err := provider.Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}

			report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
			if err != nil {
				t.Fatalf("Preflight failed: %v", err)
			}

			_, _ = summaryFor(t, report)
			var found bool

			for _, check := range report.Checks {
				if check.Provider+"."+check.Resource != scenario.resource {
					continue
				}

				found = true

				if check.Status != provider.Unavailable {
					t.Fatalf("%s status = %q, want %q: unstructured stderr/error text must not assert verified absence", scenario.resource, check.Status, provider.Unavailable)
				}

				if check.Detail == "verified provider resource is absent" {
					t.Fatalf("%s detail = %q, must not claim verified absence", scenario.resource, check.Detail)
				}
			}

			if !found {
				t.Fatalf("expected a %s check", scenario.resource)
			}
		})
	}
}

// TestPreflightReportsTimedOutStatus verifies that context deadline failures
// get a distinct timed-out status instead of a generic unavailable status.
func TestPreflightReportsTimedOutStatus(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	deadlineError := fmt.Errorf("gcloud: %w", context.DeadlineExceeded)
	runner := &overrideRunner{
		inner: presentStateRunner(),
		key:   projectListKey(config),
		err:   deadlineError,
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	statuses, summary := summaryFor(t, report)

	if got := statuses["gcp.project"]; got != provider.TimedOut {
		t.Fatalf("gcp.project status = %q, want %q", got, provider.TimedOut)
	}

	var projectDetail string

	for _, check := range report.Checks {
		if check.Provider == "gcp" && check.Resource == "project" {
			projectDetail = check.Detail
		}
	}

	if !strings.Contains(projectDetail, "--command-timeout") {
		t.Errorf("gcp.project detail = %q, want retry advice mentioning a larger --command-timeout", projectDetail)
	}

	if strings.Contains(strings.ToLower(projectDetail), "missing") {
		t.Errorf("gcp.project detail = %q, must not claim verified absence for a timeout", projectDetail)
	}

	if got := statuses["gcp.artifact_repository"]; got != provider.Unavailable {
		t.Errorf("gcp.artifact_repository status = %q, want %q because the timed-out project check is fail-closed", got, provider.Unavailable)
	}

	if report.Ready {
		t.Error("expected report not ready after a provider command timed out")
	}

	if summary.Statuses[provider.TimedOut] != 1 {
		t.Errorf("summary timed-out count = %d, want 1", summary.Statuses[provider.TimedOut])
	}
}

// TestPreflightPerCommandDeadlineResetsForEachCommand proves the per-command
// timeout bounds each CLI invocation separately: one slow command must not
// consume the deadline budget of the following commands.
func TestPreflightPerCommandDeadlineResetsForEachCommand(t *testing.T) {
	runner := &slowFirstRunner{
		inner: presentStateRunner(),
		key:   "wrangler whoami --json",
		delay: 100 * time.Millisecond,
	}

	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner, CommandTimeout: 250 * time.Millisecond})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	statuses, _ := summaryFor(t, report)

	if got := statuses["wrangler.authentication"]; got != provider.OK {
		t.Errorf("wrangler.authentication status = %q, want %q after a slow-but-within-limit command", got, provider.OK)
	}

	if got := statuses["cloudflare.d1_database"]; got != provider.OK {
		t.Errorf("cloudflare.d1_database status = %q, want %q: the next command must get its own deadline", got, provider.OK)
	}
}

// TestPreflightPerCommandDeadlineReportsTimeout proves the configured
// per-command deadline is enforced through the preflight seam.
func TestPreflightPerCommandDeadlineReportsTimeout(t *testing.T) {
	runner := &slowFirstRunner{
		inner: presentStateRunner(),
		key:   "wrangler whoami --json",
		delay: 5 * time.Second,
	}

	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner, CommandTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	statuses, summary := summaryFor(t, report)

	if got := statuses["wrangler.authentication"]; got != provider.TimedOut {
		t.Errorf("wrangler.authentication status = %q, want %q for an exceeded per-command deadline", got, provider.TimedOut)
	}

	if report.Ready {
		t.Error("expected report not ready after a per-command timeout")
	}

	if summary.Statuses[provider.TimedOut] != 1 {
		t.Errorf("summary timed-out count = %d, want 1", summary.Statuses[provider.TimedOut])
	}
}

// TestReportSummaryGroupsStatusesAndActionCategories verifies the mixed-status
// summary: counts by status plus safe action categories, with no identifiers.
func TestReportSummaryGroupsStatusesAndActionCategories(t *testing.T) {
	config, err := provider.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	runner := &overrideRunner{
		inner: &stderrRunner{
			inner: presentStateRunnerWithKV("other-namespace"),
			key:   clerkAppsListKey(),
			err:   errUnauthorized,
		},
		key: projectListKey(config),
		err: fmt.Errorf("gcloud: %w", context.DeadlineExceeded),
	}

	report, err := provider.Preflight(context.Background(), config, provider.Options{Runner: runner})
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if report.Ready {
		t.Fatal("expected report not ready for a mixed-status run")
	}

	summary := report.Summary

	if summary.Total != len(report.Checks) {
		t.Fatalf("summary total = %d, want %d", summary.Total, len(report.Checks))
	}

	if summary.Statuses[provider.Missing] != 1 {
		t.Errorf("summary missing count = %d, want 1", summary.Statuses[provider.Missing])
	}

	if summary.Statuses[provider.TimedOut] != 1 {
		t.Errorf("summary timed-out count = %d, want 1", summary.Statuses[provider.TimedOut])
	}

	if summary.Statuses[provider.Unavailable] != 7 {
		t.Errorf("summary unavailable count = %d, want 7", summary.Statuses[provider.Unavailable])
	}

	if summary.Statuses[provider.Mismatch] != 0 {
		t.Errorf("summary mismatch count = %d, want 0", summary.Statuses[provider.Mismatch])
	}

	expectedActions := map[provider.Action]int{
		provider.ActionProvisionMissing: 1,
		provider.ActionRetryTimedOut:    1,
		provider.ActionInvestigate:      7,
		provider.ActionFixConfig:        0,
	}
	for action, want := range expectedActions {
		if got := summary.Actions[action]; got != want {
			t.Errorf("summary action %q count = %d, want %d", action, got, want)
		}
	}

	encoded, err := json.Marshal(report.Summary)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if strings.Contains(string(encoded), "owner@example.com") || strings.Contains(string(encoded), "gog-marketing") {
		t.Fatalf("summary must not contain account or resource identifiers: %s", encoded)
	}
}

// TestExecRunnerHonorsContextDeadline proves the real executor maps a context
// deadline to a deadline error: this is what drives the timed-out status.
func TestExecRunnerHonorsContextDeadline(t *testing.T) {
	// Re-execute the test binary as the wrangler stub instead of a POSIX shell
	// script: Windows PATH lookup requires an executable image (for example
	// wrangler.exe), so the shell fixture never started there and the real
	// subprocess kill/deadline path was never exercised.
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary: %v", err)
	}

	binary, err := os.ReadFile(self)
	if err != nil {
		t.Fatalf("read test binary: %v", err)
	}

	dir := t.TempDir()

	stub := "wrangler"
	if runtime.GOOS == "windows" {
		stub += ".exe"
	}

	if err := os.WriteFile(filepath.Join(dir, stub), binary, 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	t.Setenv("GOG_PROVIDER_TEST_SLEEP_CHILD", "1")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, runErr := (provider.ExecRunner{}).Run(ctx, "wrangler", "whoami")
	if !errors.Is(runErr, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want a context.DeadlineExceeded error", runErr)
	}
}
