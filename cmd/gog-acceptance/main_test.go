package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateBootstrapInputsAllowsJSONClientID(t *testing.T) {
	clientJSON := filepath.Join(t.TempDir(), "client_secret.json")
	if err := os.WriteFile(clientJSON, []byte(`{"installed":{"client_id":"client-id","client_secret":"client-secret"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateBootstrapInputs("postgres://example", "owner@example.test", "gmail@example.test", "singulyr@example.test", clientJSON, "/opt/homebrew/bin/gog"); err != nil {
		t.Fatalf("valid bootstrap inputs rejected: %v", err)
	}
}

func TestResolveBootstrapClientUsesClientJSON(t *testing.T) {
	raw := []byte(`{"installed":{"client_id":"json-client-id","client_secret":"json-client-secret"}}`)
	credentials, err := resolveBootstrapClient("", raw)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.ClientID != "json-client-id" || credentials.ClientSecret != "json-client-secret" {
		t.Fatalf("resolved credentials = %+v", credentials)
	}
}

func TestResolveBootstrapClientRequiresOneCompleteSource(t *testing.T) {
	if _, err := resolveBootstrapClient("", []byte("raw-secret")); !errors.Is(err, errBootstrapClient) {
		t.Fatalf("missing client ID error = %v", err)
	}
	credentials, err := resolveBootstrapClient("explicit-client-id", []byte("raw-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if credentials.ClientID != "explicit-client-id" || credentials.ClientSecret != "raw-secret" {
		t.Fatalf("explicit credentials = %+v", credentials)
	}
}
