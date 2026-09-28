package main

import (
	"errors"
	"fmt"
)

var (
	errAcceptanceUsage       = errors.New("usage: gog-acceptance <doctor|local|live|live-repeat|bootstrap>")
	errAcceptanceRuns        = errors.New("--runs must be at least 1")
	errAcceptanceUnknown     = errors.New("unknown acceptance command")
	errBootstrapArguments    = errors.New("bootstrap arguments are incomplete")
	errBootstrapClient       = errors.New("central OAuth client JSON or explicit client ID and secret is required")
	errBootstrapTokenFile    = errors.New("refresh token file is empty")
	errBootstrapExportBinary = errors.New("bootstrap requires a stable signed gog export binary")
)

func wrapMainError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("gog-acceptance: %w", err)
}
