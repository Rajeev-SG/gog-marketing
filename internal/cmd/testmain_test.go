package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/openclaw/gogcli/internal/tzembed" // Embed IANA timezone database for Windows test support
)

func TestMain(m *testing.M) {
	if os.Getenv("TENANT_FAKE_GOG") == "1" {
		// Test-subprocess mode: act as the tenant-hosted "gog" child. Record
		// selected environment and argv, emit one JSON line, exit 0.
		logPath := os.Getenv("TENANT_TEST_LOG")
		if logPath != "" {
			line := fmt.Sprintf("argv:%s\nGOG_HOME:%s\nGOG_ACCOUNT:%s\nGOG_KEYRING:%s\n",
				strings.Join(os.Args[1:], " "),
				os.Getenv("GOG_HOME"),
				os.Getenv("GOG_ACCOUNT"),
				os.Getenv("GOG_KEYRING_BACKEND"),
			)
			_ = os.WriteFile(logPath, []byte(line), 0o600)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"result": "ok"})
		os.Exit(0)
	}

	contactsSearchWarmupDelay = 0

	root, err := os.MkdirTemp("", "gogcli-tests-*")
	if err != nil {
		panic(err)
	}

	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")

	home := filepath.Join(root, "home")
	xdg := filepath.Join(root, "xdg")
	_ = os.MkdirAll(home, 0o755)
	_ = os.MkdirAll(xdg, 0o755)
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("XDG_CONFIG_HOME", xdg)

	// Ambient GOG_* path overrides and XDG data/state/cache directories escape
	// this sandbox entirely: the layout resolver (internal/config/layout.go)
	// honors them ahead of the HOME- and XDG_CONFIG_HOME-derived defaults set
	// above, pointing tests at shared real directories. Unset rather than
	// redirect: a single shared override directory still cross-contaminates
	// tests. Per-test t.Setenv values are unaffected.
	oldPathEnv := map[string]string{}
	for _, name := range []string{
		"GOG_HOME", "GOG_CONFIG_DIR", "GOG_DATA_DIR", "GOG_STATE_DIR", "GOG_CACHE_DIR",
		"XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME",
	} {
		if value, ok := os.LookupEnv(name); ok {
			oldPathEnv[name] = value
		}
		_ = os.Unsetenv(name)
	}

	code := m.Run()

	if oldHome == "" {
		_ = os.Unsetenv("HOME")
	} else {
		_ = os.Setenv("HOME", oldHome)
	}
	if oldXDG == "" {
		_ = os.Unsetenv("XDG_CONFIG_HOME")
	} else {
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}

	for name, value := range oldPathEnv {
		_ = os.Setenv(name, value)
	}
	_ = os.RemoveAll(root)
	os.Exit(code)
}
