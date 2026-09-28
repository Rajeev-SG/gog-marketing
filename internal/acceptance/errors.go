package acceptance

import (
	"errors"
	"fmt"
)

var (
	ErrUsage                 = errors.New("usage error")
	ErrInvalidProfile        = errors.New("invalid acceptance profile")
	ErrTemporaryProfile      = errors.New("acceptance profile uses an ephemeral path")
	ErrMissingOAuthClient    = errors.New("OAuth client unavailable")
	ErrMissingEnabledGrant   = errors.New("no enabled resource grant")
	ErrMissingFakeAuthURL    = errors.New("missing fake authorization URL")
	ErrMissingResourceGrant  = errors.New("resource grant missing")
	ErrExpectedTwoReads      = errors.New("expected two fake reads")
	ErrRestartPersistence    = errors.New("connections did not survive restart")
	ErrBigQueryGrant         = errors.New("BigQuery dataset grant is missing project:id")
	ErrUnsupportedReader     = errors.New("live reader does not support service")
	ErrBootstrapExportBinary = errors.New("bootstrap requires a stable signed gog export binary")
	ErrEmptyRefreshTokenFile = errors.New("refresh token file is empty")
	ErrBootstrapClient       = errors.New("central OAuth client configuration is required")
	ErrBootstrapTokens       = errors.New("bootstrap refresh-token exports are required")
	ErrNeedsReconnect        = errors.New("needs_reconnect")
)

const bootstrapAction = "run make acceptance-bootstrap"

func wrapAcceptanceError(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("acceptance: %w", err)
}
