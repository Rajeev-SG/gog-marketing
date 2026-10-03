package main

import (
	"os"
	"strings"
	"testing"
)

func TestReadmeDocumentsSafeAgentContract(t *testing.T) {
	raw, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(raw)

	for _, required := range []string{
		"gog auth credentials set",
		"gog auth add",
		"--readonly --no-input --json",
		"make acceptance-local",
		"make acceptance-doctor",
		"make acceptance-live-repeat N=3",
		"Keep `client_secret_*.json`, refresh tokens, and keyring passwords out of",
	} {
		if !strings.Contains(readme, required) {
			t.Fatalf("README is missing %q", required)
		}
	}

	if strings.Contains(readme, "auth add") && strings.Contains(readme, "--login") {
		t.Fatal("README documents the obsolete auth add --login flag")
	}
}
