package config

import (
	"errors"
	"fmt"
	"strings"
)

var (
	errInvalidConnectionName = errors.New("invalid connection name")
	errMissingConnection     = errors.New("missing connection")
)

// Connection is a named binding that makes the Google identity and execution
// surfaces explicit for multi-account work: the account (email or alias), the
// OAuth client that holds its credentials, the services it is scoped for, and
// the quota/billing project for Google APIs that bill or execute against a
// project (BigQuery execution, quota project for API usage).
type Connection struct {
	Name            string   `json:"name"`
	Account         string   `json:"account"`
	Client          string   `json:"client,omitempty"`
	Services        []string `json:"services,omitempty"`
	QuotaProject    string   `json:"quota_project,omitempty"`
	BigQueryProject string   `json:"bigquery_project,omitempty"`
	Description     string   `json:"description,omitempty"`
}

// NormalizeConnectionName validates a connection name. Connection names follow
// the same character rules as OAuth client names and must not collide with the
// reserved "auto" account selector.
func NormalizeConnectionName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return "", fmt.Errorf("%w: empty", errInvalidConnectionName)
	}

	if name == "auto" {
		return "", fmt.Errorf("%w: %q is reserved", errInvalidConnectionName, raw)
	}

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}

		return "", fmt.Errorf("%w: %q", errInvalidConnectionName, raw)
	}

	return name, nil
}

// NormalizeConnectionAccount lowercases and trims an account email/alias.
func NormalizeConnectionAccount(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || email == "auto" {
		return "", fmt.Errorf("%w: account must be an explicit email or alias", errMissingEmail)
	}

	return email, nil
}
