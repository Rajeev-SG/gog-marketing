package acceptance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/controlplane"
)

var (
	errTestInvalidGrant    = errors.New("invalid_grant")
	errTestConnectionReset = errors.New("connection reset by peer")
)

func TestRetryPolicyOnlyRetriesTransientErrors(t *testing.T) {
	calls := 0
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}

	_, err := policy.Run(context.Background(), "test", func(context.Context) error {
		calls++
		return errTestInvalidGrant
	})
	if err == nil || calls != 1 {
		t.Fatalf("terminal error retried: calls=%d err=%v", calls, err)
	}
	calls = 0

	_, err = policy.Run(context.Background(), "test", func(context.Context) error {
		calls++
		if calls < 2 {
			return errTestConnectionReset
		}

		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("transient error did not retry: calls=%d err=%v", calls, err)
	}
}

func TestStableProfileRoundTrip(t *testing.T) {
	root := t.TempDir()
	paths := Paths{Root: root, Config: filepath.Join(root, "config.json"), MasterKey: filepath.Join(root, "master.key"), Secrets: filepath.Join(root, "secrets.json"), OutputRoot: filepath.Join(root, "out")}

	profile := Profile{Version: 1, DatabaseURL: "postgres://example", OwnerEmail: "owner@example.test", OrganizationSlug: "acceptance"}
	if err := EnsureProfile(paths, profile); err != nil {
		t.Fatal(err)
	}

	loaded, key, err := LoadProfile(paths)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.DatabaseURL != profile.DatabaseURL || len(key) < 16 {
		t.Fatalf("profile round trip failed: %+v", loaded)
	}

	if err := ValidateStablePaths(paths); err == nil {
		t.Fatal("temporary profile path was accepted")
	}
}

func TestLocalAcceptanceIsHermetic(t *testing.T) {
	root := t.TempDir()
	paths := Paths{Root: root, Config: filepath.Join(root, "config.json"), MasterKey: filepath.Join(root, "master.key"), Secrets: filepath.Join(root, "secrets.json"), OutputRoot: filepath.Join(root, "out")}

	manifest, err := RunLocal(context.Background(), paths, "")
	if err != nil {
		t.Fatal(err)
	}

	if manifest.Status != "PASS" {
		t.Fatalf("local acceptance failed: %+v", manifest)
	}

	if manifest.Interaction["browser"] != 0 || manifest.Interaction["keychain_dialog"] != 0 || manifest.Interaction["oauth"] != 0 {
		t.Fatalf("local acceptance interacted: %+v", manifest.Interaction)
	}
}

func TestLiveFailsFastWithoutOAuthClient(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	root, err := os.MkdirTemp(configDir, "gog-acceptance-test-")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(root) }()

	paths := Paths{Root: root, Config: filepath.Join(root, "config.json"), MasterKey: filepath.Join(root, "master.key"), Secrets: filepath.Join(root, "secrets.json"), OutputRoot: filepath.Join(root, "out")}
	if profileErr := EnsureProfile(paths, Profile{Version: 1, DatabaseURL: "postgres://unused", OwnerEmail: "owner@example.test"}); profileErr != nil {
		t.Fatal(err)
	}

	_, err = OpenRuntime(context.Background(), paths, true)
	if err == nil || !strings.Contains(err.Error(), "make acceptance-bootstrap") {
		t.Fatalf("missing client did not fail fast: %v", err)
	}
}

func TestManifestRedactsAndHashesResources(t *testing.T) {
	manifest := NewManifest("test", "binary", "commit")
	manifest.Add("check", "PASS", "", "refresh_token=s3cr3t", 1)

	if strings.Contains(manifest.Checks[0].Detail, "s3cr3t") {
		t.Fatal("manifest retained token marker")
	}

	if HashResourceID("properties/123") == "properties/123" {
		t.Fatal("resource ID was not hashed")
	}

	if manifest.Interaction["go_run"] != 0 {
		t.Fatal("manifest allows go_run")
	}
}

func TestProfilePersistsControlPlaneIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	paths := Paths{Root: root, Config: filepath.Join(root, "config.json"), MasterKey: filepath.Join(root, "master.key"), Secrets: filepath.Join(root, "secrets.json"), OutputRoot: filepath.Join(root, "out")}

	if err := EnsureProfile(paths, Profile{Version: 1, DatabaseURL: "postgres://example", OwnerEmail: "owner@example.test", OrganizationSlug: "acceptance"}); err != nil {
		t.Fatal(err)
	}

	if err := SetControlPlaneIdentity(paths, "user-1", "org-1"); err != nil {
		t.Fatal(err)
	}

	loaded, _, err := LoadProfile(paths)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.UserID != "user-1" || loaded.OrganizationID != "org-1" {
		t.Fatalf("control-plane identity round trip failed: %+v", loaded)
	}
}

func TestEnsureProfileRejectsMissingOwner(t *testing.T) {
	if err := EnsureProfile(Paths{}, Profile{DatabaseURL: "postgres://example"}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("missing owner error = %v", err)
	}
}

func TestDoctorOAuthClientCheckMatchesBootstrapProfile(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	root, err := os.MkdirTemp(configDir, "gog-acceptance-doctor-test-")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(root) }()

	paths := Paths{Root: root, Config: filepath.Join(root, "config.json"), MasterKey: filepath.Join(root, "master.key"), Secrets: filepath.Join(root, "secrets.json"), OutputRoot: filepath.Join(root, "out")}
	if profileErr := EnsureProfile(paths, Profile{Version: 1, DatabaseURL: "postgres://example", OwnerEmail: "owner@example.test", OrganizationSlug: "acceptance", GoogleClientID: "client-id"}); profileErr != nil {
		t.Fatal(profileErr)
	}

	_, key, err := LoadProfile(paths)
	if err != nil {
		t.Fatal(err)
	}

	secrets, err := controlplane.NewFileSecretStore(paths.Secrets, key)
	if err != nil {
		t.Fatal(err)
	}

	reference, err := secrets.Put(context.Background(), "acceptance-org", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	if refErr := SetGoogleClientSecretRef(paths, reference); refErr != nil {
		t.Fatal(refErr)
	}

	profile, _, err := LoadProfile(paths)
	if err != nil {
		t.Fatal(err)
	}

	check := doctorOAuthClientCheck(context.Background(), profile, secrets)
	if check.Result != "PASS" {
		t.Fatalf("doctor OAuth check = %+v", check)
	}
}

func TestRetryAttemptTimeoutDoesNotConsumeBackoffBudget(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, AttemptTimeout: 10 * time.Millisecond}
	calls := 0

	attempts, err := policy.Run(context.Background(), "test", func(ctx context.Context) error {
		calls++
		if calls == 1 {
			return &googleapi.Error{Code: 429, Message: "rate limited"}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
			return nil
		}
	})
	if err != nil || attempts != 2 {
		t.Fatalf("retry outcome: attempts=%d err=%v", attempts, err)
	}
}

func TestMigrationCheckUsesRegistryAndFailsMissing(t *testing.T) {
	required := controlplane.RequiredMigrationVersions()
	if len(required) < 2 {
		t.Fatalf("migration registry too small: %v", required)
	}

	if missing := missingVersions(required[:len(required)-1], required...); len(missing) != 1 || missing[0] != required[len(required)-1] {
		t.Fatalf("missing migration detection = %v", missing)
	}

	report := DoctorReport{Status: "FAIL"}
	if err := finalizeDoctorReport(report); err == nil {
		t.Fatal("FAIL doctor report returned nil error")
	}
}

func TestRetryAttemptTimeoutRemainsRetryable(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 1, AttemptTimeout: time.Millisecond}

	_, err := policy.Run(context.Background(), "test", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err == nil || !Retryable(err) {
		t.Fatalf("deadline outcome = %v", err)
	}
}
