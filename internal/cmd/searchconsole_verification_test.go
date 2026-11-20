package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestWrapSiteVerificationError_AccessNotConfiguredUsesAPILibrary(t *testing.T) {
	err := wrapSiteVerificationError(&gapi.Error{
		Code:    http.StatusForbidden,
		Message: "accessNotConfigured: Site Verification API has not been used",
	})
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	got := err.Error()
	if !strings.Contains(got, "https://console.cloud.google.com/apis/library/siteverification.googleapis.com") {
		t.Fatalf("expected Site Verification API Library URL, got %q", got)
	}
}

func TestExecute_SearchConsoleVerificationToken(t *testing.T) {
	var calls int
	svc := newSiteVerificationTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/token") {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Site struct {
				Type       string `json:"type"`
				Identifier string `json:"identifier"`
			} `json:"site"`
			VerificationMethod string `json:"verificationMethod"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Site.Type != "INET_DOMAIN" || body.Site.Identifier != "example.com" {
			t.Fatalf("unexpected site: %#v", body.Site)
		}
		if body.VerificationMethod != "DNS_TXT" {
			t.Fatalf("verificationMethod = %q, want DNS_TXT", body.VerificationMethod)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"method": "DNS_TXT",
			"token":  "google-site-verification=abc123",
		})
	}))
	result := executeWithSiteVerificationTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"searchconsole", "verification", "token", " Example.COM ",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	var parsed struct {
		Domain    string `json:"domain"`
		Method    string `json:"method"`
		Token     string `json:"token"`
		DNSRecord string `json:"dns_record"`
		DNSHost   string `json:"dns_host"`
		DNSValue  string `json:"dns_value"`
		Verified  bool   `json:"verified"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Domain != "example.com" || parsed.Method != "DNS_TXT" ||
		parsed.Token != "google-site-verification=abc123" ||
		parsed.DNSRecord != "TXT" || parsed.DNSHost != "example.com" ||
		parsed.DNSValue != parsed.Token || parsed.Verified {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
	// The verification token is part of the write-only flow: a read-only
	// runtime must not issue it.
	svcReadOnly := newSiteVerificationTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("read-only request reached Google: %s %s", r.Method, r.URL)
	}))
	readOnlyResult := executeWithSiteVerificationTestService(t, []string{
		"--json",
		"--readonly",
		"--account", "a@b.com",
		"searchconsole", "verification", "token", "example.com",
	}, svcReadOnly)
	if !errors.Is(readOnlyResult.err, googleapi.ErrReadOnly) {
		t.Fatalf("readonly token err = %v, want ErrReadOnly", readOnlyResult.err)
	}
}

func TestExecute_SearchConsoleVerificationVerify(t *testing.T) {
	var calls int
	svc := newSiteVerificationTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/webResource") {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("verificationMethod"); got != "DNS_TXT" {
			t.Fatalf("verificationMethod = %q, want DNS_TXT", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, present := body["owners"]; present {
			t.Fatalf("owners must never be delegated: %#v", body)
		}
		site, _ := body["site"].(map[string]any)
		if site["type"] != "INET_DOMAIN" || site["identifier"] != "example.com" {
			t.Fatalf("unexpected site: %#v", site)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"site":   map[string]any{"type": "INET_DOMAIN", "identifier": "example.com"},
			"owners": []string{"a@b.com"},
		})
	}))
	result := executeWithSiteVerificationTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"searchconsole", "verification", "verify", "example.com",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	var parsed struct {
		Domain   string `json:"domain"`
		Method   string `json:"method"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Domain != "example.com" || parsed.Method != "DNS_TXT" || !parsed.Verified {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}

func TestExecute_SearchConsoleVerificationVerify_DryRun(t *testing.T) {
	svc := newSiteVerificationTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry run reached Google: %s %s", r.Method, r.URL)
	}))
	result := executeWithSiteVerificationTestService(t, []string{
		"--json",
		"--dry-run",
		"--account", "a@b.com",
		"searchconsole", "verification", "verify", "example.com",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			Domain   string `json:"domain"`
			SiteType string `json:"site_type"`
			Method   string `json:"verification_method"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !parsed.DryRun || parsed.Op != "searchconsole.verification.verify" ||
		parsed.Request.Domain != "example.com" || parsed.Request.SiteType != "INET_DOMAIN" ||
		parsed.Request.Method != "DNS_TXT" {
		t.Fatalf("unexpected dry-run payload: %#v", parsed)
	}
}

func TestExecute_SearchConsoleVerificationVerify_ReadOnlyBlocked(t *testing.T) {
	svc := newSiteVerificationTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("read-only request reached Google: %s %s", r.Method, r.URL)
	}))
	result := executeWithSiteVerificationTestService(t, []string{
		"--json",
		"--readonly",
		"--account", "a@b.com",
		"searchconsole", "verification", "verify", "example.com",
	}, svc)
	if !errors.Is(result.err, googleapi.ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", result.err)
	}
}

func TestExecute_SearchConsoleVerification_InvalidDomain(t *testing.T) {
	for _, domain := range []string{
		"https://example.com",
		"example.com/path",
		"example.com:443",
		"user@example.com",
		"*.example.com",
		"localhost",
		"1.2.3.4",
		"example.123",
		"-bad.example.com",
		"bad-.example.com",
		"example..com",
		"example.com.",
		"exa mple.com",
	} {
		t.Run(domain, func(t *testing.T) {
			result := executeWithSiteVerificationTestService(t, []string{
				"--json",
				"--account", "a@b.com",
				"searchconsole", "verification", "verify", "--", domain,
			}, nil)
			if result.err == nil || !strings.Contains(result.err.Error(), "invalid domain") {
				t.Fatalf("err = %v, want invalid domain", result.err)
			}
		})
	}
}
