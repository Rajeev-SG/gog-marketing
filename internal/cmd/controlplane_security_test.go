package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/openclaw/gogcli/internal/config"
)

func TestControlPlaneSecurityValidation(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name       string
		listen     string
		backend    string
		project    string
		adminToken string
		wantError  bool
	}{
		{name: "loopback file", listen: "127.0.0.1:8080", backend: "file", adminToken: token},
		{name: "remote file rejected", listen: "0.0.0.0:8080", backend: "file", adminToken: token, wantError: true},
		{name: "remote secret manager", listen: "0.0.0.0:8080", backend: "secret-manager", project: "prod", adminToken: token},
		{name: "secret manager project required", listen: "0.0.0.0:8080", backend: "secret-manager", adminToken: token, wantError: true},
		{name: "backend required", listen: "127.0.0.1:8080", adminToken: token, wantError: true},
		{name: "admin token required", listen: "127.0.0.1:8080", backend: "file", adminToken: "short", wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateControlPlaneSecurity(tc.listen, tc.backend, tc.project, tc.adminToken)
			if tc.wantError && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("validation failed: %v", err)
			}
		})
	}
}

func TestResolveControlPlaneGoogleOAuthClient(t *testing.T) {
	readerCalls := 0
	reader := func(_ context.Context, client string) (config.ClientCredentials, error) {
		readerCalls++
		if client != "personal-owned" {
			t.Fatalf("client = %q", client)
		}
		return config.ClientCredentials{ClientID: "stored-id", ClientSecret: "stored-secret"}, nil
	}

	stored, err := resolveControlPlaneGoogleOAuthClient(context.Background(), "personal-owned", "", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ClientID != "stored-id" || stored.ClientSecret != "stored-secret" || readerCalls != 1 {
		t.Fatalf("stored credentials = %+v, calls=%d", stored, readerCalls)
	}

	explicit, err := resolveControlPlaneGoogleOAuthClient(context.Background(), "", "explicit-id", "explicit-secret", reader)
	if err != nil {
		t.Fatal(err)
	}
	if explicit.ClientID != "explicit-id" || explicit.ClientSecret != "explicit-secret" || readerCalls != 1 {
		t.Fatalf("explicit credentials = %+v, calls=%d", explicit, readerCalls)
	}

	if _, err := resolveControlPlaneGoogleOAuthClient(context.Background(), "personal-owned", "explicit-id", "", reader); err == nil {
		t.Fatal("ambiguous client configuration succeeded")
	}
	if _, err := resolveControlPlaneGoogleOAuthClient(context.Background(), "", "explicit-id", "", reader); err == nil {
		t.Fatal("incomplete explicit client configuration succeeded")
	}

	failing := func(context.Context, string) (config.ClientCredentials, error) {
		return config.ClientCredentials{}, errors.New("keychain unavailable")
	}
	if _, err := resolveControlPlaneGoogleOAuthClient(context.Background(), "personal-owned", "", "", failing); err == nil {
		t.Fatal("stored client read failure was ignored")
	}
}

func TestControlPlaneDiscovererWiresGoogleAdsConfig(t *testing.T) {
	command := &ControlPlaneCmd{
		GoogleAdsDeveloperToken: "developer-token",
		GoogleAdsLoginCustomer:  "123-456-7890",
		BigQueryProjects:        "project-a,project-b",
	}

	discoverer := controlPlaneDiscoverer(command)
	if discoverer.GoogleAdsDeveloperToken != command.GoogleAdsDeveloperToken {
		t.Fatal("Google Ads developer token was not wired")
	}
	if discoverer.GoogleAdsLoginCustomer != command.GoogleAdsLoginCustomer {
		t.Fatal("Google Ads login customer was not wired")
	}
	if len(discoverer.BigQueryProjects) != 2 || discoverer.BigQueryProjects[0] != "project-a" || discoverer.BigQueryProjects[1] != "project-b" {
		t.Fatalf("BigQuery projects were not wired: %+v", discoverer.BigQueryProjects)
	}
}
