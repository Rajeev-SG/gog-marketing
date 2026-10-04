package main

import (
	"testing"
	"time"
)

// TestPreflightDefaultsAreConservativeForColdEnvironments proves the shipped
// deadline defaults leave room for cold provider CLIs (first credential
// refresh, wrangler first-run work, slow networks): the per-command budget
// must exceed 30s, and the total budget must contain the per-command budget.
func TestPreflightDefaultsAreConservativeForColdEnvironments(t *testing.T) {
	if defaultCommandTimeout <= 30*time.Second {
		t.Fatalf("defaultCommandTimeout = %s, want more than the previous 30s default for cold environments", defaultCommandTimeout)
	}

	if defaultTotalTimeout < defaultCommandTimeout {
		t.Fatalf("defaultTotalTimeout = %s must contain the per-command budget %s", defaultTotalTimeout, defaultCommandTimeout)
	}

	if defaultCommandTimeout < 90*time.Second {
		t.Errorf("defaultCommandTimeout = %s, want at least 90s for cold provider CLIs", defaultCommandTimeout)
	}
}
