package provider_test

import (
	"os"
	"testing"
	"time"
)

// GOG_PROVIDER_TEST_SLEEP_CHILD flips a re-executed copy of this test binary
// into slow provider-CLI mode so tests can exercise real subprocess behavior
// hermetically on every platform including Windows (same pattern as the cmd
// package's TENANT_FAKE_GOG child mode).
func TestMain(m *testing.M) {
	if os.Getenv("GOG_PROVIDER_TEST_SLEEP_CHILD") == "1" {
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}

	os.Exit(m.Run())
}
