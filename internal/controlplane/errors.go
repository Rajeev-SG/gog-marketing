package controlplane

import (
	"errors"
	"fmt"
)

var (
	ErrOIDCNonceMismatch       = errors.New("OIDC nonce mismatch")
	ErrGoogleIdentityHTTP      = errors.New("google identity HTTP error")
	ErrGoogleIdentitySubject   = errors.New("google identity is missing subject")
	ErrGoogleRevokeHTTP        = errors.New("google token revoke HTTP error")
	ErrSecretManagerProject    = errors.New("secret manager project is required")
	ErrSecretMasterKey         = errors.New("secret master key must be at least 16 bytes")
	ErrSecretCiphertext        = errors.New("invalid secret ciphertext")
	ErrDiscovererNotConfigured = errors.New("resource discoverer is not configured")
	ErrSessionKey              = errors.New("session key must be at least 16 bytes")
	ErrWebDependencies         = errors.New("web handler dependencies are required")
	ErrMissingScopes           = errors.New("missing required scopes")
	ErrGoogleAdsUnavailable    = errors.New("google ads API unavailable")
)

func wrapControlPlaneError(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("control plane: %w", err)
}
