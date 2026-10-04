package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/openclaw/gogcli/internal/hosted/provider"
)

// TestExecRunnerRejectsNonAllowlistedCommands verifies the fixed internal
// provider allowlist: no model- or user-supplied command name can be executed.
func TestExecRunnerRejectsNonAllowlistedCommands(t *testing.T) {
	if _, err := (provider.ExecRunner{}).Run(context.Background(), "echo", "hello"); !errors.Is(err, provider.ErrCommandNotAllowed) {
		t.Fatalf("Run(echo) error = %v, want a command-not-allowed error", err)
	}

	if _, err := (provider.ExecRunner{}).Run(context.Background(), "sh", "-c", "echo hello"); !errors.Is(err, provider.ErrCommandNotAllowed) {
		t.Fatalf("Run(sh) error = %v, want a command-not-allowed error", err)
	}
}
