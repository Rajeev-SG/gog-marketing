package tenants

import (
	"errors"
	"fmt"
	"strings"
)

var (
	errInvalidTenantName = errors.New("invalid tenant name")
	errMissingAccount    = errors.New("missing account")
	// ErrMissingMasterSecret signals that the operator has not provided the
	// hosted master secret needed for per-tenant encrypted token storage.
	ErrMissingMasterSecret = errors.New("tenant master secret")
)

// Tenant is a hosted multi-account connection: one isolated gog home (its own
// config, keyring-backed tokens, and cache), one pinned account, one policy.
type Tenant struct {
	Name       string   `json:"name"`
	Account    string   `json:"account"`
	Client     string   `json:"client,omitempty"`
	ReadOnly   bool     `json:"readonly,omitempty"`
	AllowTools []string `json:"allow_tools,omitempty"`
	Notes      string   `json:"notes,omitempty"`
}

// NormalizeName validates a tenant name. The name becomes a directory path, so
// it must stay a short lowercase slug.
func NormalizeName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return "", fmt.Errorf("%w: empty", errInvalidTenantName)
	}

	if len(name) > 63 {
		return "", fmt.Errorf("%w: longer than 63 characters", errInvalidTenantName)
	}

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}

		return "", fmt.Errorf("%w: %q", errInvalidTenantName, raw)
	}

	return name, nil
}

// NormalizeAccount requires an explicit account identity per tenant.
func NormalizeAccount(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || email == "auto" {
		return "", fmt.Errorf("%w: tenant account must be an explicit email or alias", errMissingAccount)
	}

	return email, nil
}
