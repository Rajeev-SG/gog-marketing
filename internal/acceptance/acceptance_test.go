package acceptance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	errTestInvalidGrant    = errors.New("invalid_grant")
	errTestConnectionReset = errors.New("connection reset by peer")
)

func TestRetryPolicyOnlyRetriesTransientErrors(t *testing.T) {
	calls := 0
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}

	_, err := policy.Run(context.Background(), "test", func() error {
		calls++
		return errTestInvalidGrant
	})
	if err == nil || calls != 1 {
		t.Fatalf("terminal error retried: calls=%d err=%v", calls, err)
	}
	calls = 0

	_, err = policy.Run(context.Background(), "test", func() error {
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

	info, err := os.Stat(paths.MasterKey)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("master key permissions = %o", info.Mode().Perm())
	}

	if err := ValidateStablePaths(paths); err == nil && strings.Contains(root, os.TempDir()) {
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
