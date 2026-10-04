package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

var (
	ErrCommandNotAllowed = errors.New("provider command is not allowed")
	errCommandNotStarted = errors.New("provider command could not be started")
	errCommandFailed     = errors.New("provider command failed")
)

// allowedCommands is a fixed allowlist of provider CLIs used by the hosted v1
// preflight checks. It is internal to this package and never accepts model- or
// user-provided command names.
var allowedCommands = map[string]bool{
	"clerk":    true,
	"gcloud":   true,
	"wrangler": true,
}

// ExecRunner runs provider CLI commands without returning or logging their raw
// stdout/stderr. Preflight callers receive only structured, safe results.
type ExecRunner struct{}

// Run executes one provider command.
func (r ExecRunner) Run(ctx context.Context, command string, args ...string) (CommandResult, error) {
	if !allowedCommands[command] {
		return CommandResult{}, fmt.Errorf("%s: %w", command, ErrCommandNotAllowed)
	}

	//nolint:gosec // command is checked against the fixed internal provider allowlist above; args come from compile-time preflight checks, never model or user input.
	cmd := exec.CommandContext(ctx, command, args...)
	var output bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return CommandResult{Output: output.Bytes(), Stderr: stderr.Bytes()}, nil
	}

	if errors.Is(err, exec.ErrNotFound) {
		return CommandResult{Stderr: stderr.Bytes()}, fmt.Errorf("%s: %w", command, errCommandNotInstalled)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return CommandResult{Stderr: stderr.Bytes()}, fmt.Errorf("%w: %s exited with status %d", errCommandFailed, command, exitErr.ExitCode())
	}

	return CommandResult{Stderr: stderr.Bytes()}, fmt.Errorf("%s: %w", command, errCommandNotStarted)
}
