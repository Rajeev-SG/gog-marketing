//go:build !windows

package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStableProfilePermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	paths := Paths{Root: root, Config: filepath.Join(root, "config.json"), MasterKey: filepath.Join(root, "master.key"), Secrets: filepath.Join(root, "secrets.json"), OutputRoot: filepath.Join(root, "out")}

	if err := EnsureProfile(paths, Profile{Version: 1, DatabaseURL: "postgres://example", OwnerEmail: "owner@example.test", OrganizationSlug: "acceptance"}); err != nil {
		t.Fatal(err)
	}

	assertMode(t, paths.Root, 0o700)
	assertMode(t, paths.OutputRoot, 0o700)
	assertMode(t, paths.Config, 0o600)
	assertMode(t, paths.MasterKey, 0o600)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s permissions = %o, want %o", path, got, want)
	}
}
