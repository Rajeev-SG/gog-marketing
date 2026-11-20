package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"
	siteverification "google.golang.org/api/siteverification/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeSearchConsoleSitesAddIsAccountLevel(t *testing.T) {
	var requests atomic.Int64

	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.SearchConsole = func(ctx context.Context, _ string) (*searchconsole.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)

				if r.Method != http.MethodPut || r.URL.Path != "/webmasters/v3/sites/sc-domain:example.test" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}

				if r.ContentLength > 0 {
					t.Errorf("sites.add must not send a body")
				}

				return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})

			return searchconsole.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(scope string, allowWrite bool, resource *ResourceGrantInput, site string) capturedResponse {
		name := "searchconsole_sites_add"
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: map[string]any{"site_url": site}, Authorization: ToolAuthorization{
				Service: "searchconsole", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: allowWrite,
				Resource: resource,
			}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	// Sites.Add creates the property, so no pre-existing site grant can scope
	// it: an account-level tool grant carries the exact-argument approval.
	if denied := call("webmasters.readonly", true, nil, "sc-domain:example.test"); denied.OK || requests.Load() != 0 {
		t.Fatal("read scope authorized sites.add")
	}

	if denied := call("webmasters", false, nil, "sc-domain:example.test"); denied.OK || requests.Load() != 0 {
		t.Fatal("missing write grant authorized sites.add")
	}

	if denied := call("webmasters", true, nil, "https://foreign.test/?q=1"); denied.OK || requests.Load() != 0 {
		t.Fatal("invalid site reached Google before admission")
	}

	if denied := call("webmasters", true, &ResourceGrantInput{Service: "searchconsole", ResourceType: "site", ResourceID: "sc-domain:example.test", Enabled: true}, "sc-domain:example.test"); denied.OK || requests.Load() != 0 {
		t.Fatal("account tool accepted a resource grant")
	}

	response := call("webmasters", true, nil, "sc-domain:example.test")
	if !response.OK {
		t.Fatalf("native searchconsole_sites_add failed: %s", response.Body)
	}

	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}

func TestAdmitSiteVerificationRejectsOwnerDelegationAndForeignBody(t *testing.T) {
	invocation := func(name, domain string) *ToolInvocation {
		return &ToolInvocation{
			Name:          name,
			Arguments:     map[string]any{"domain": domain},
			Authorization: ToolAuthorization{Service: "searchconsole", AllowWrite: true},
		}
	}

	request := func(body string) *http.Request {
		r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://www.googleapis.com/siteVerification/v1/webResource?verificationMethod=DNS_TXT", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}

		return r
	}

	verify := invocation("searchconsole_verification_verify", "example.test")
	for _, body := range []string{
		`{"site":{"type":"INET_DOMAIN","identifier":"example.test"},"owners":["attacker@example.test"]}`,
		`{"site":{"type":"INET_DOMAIN","identifier":"other.test"}}`,
		`{"site":{"type":"SITE","identifier":"https://example.test/"}}`,
		`{"site":{"type":"INET_DOMAIN","identifier":"example.test"},"verificationMethod":"DNS_TXT"}`,
	} {
		if admitInvocationSpecificRequest(verify, request(body)) == nil {
			t.Fatalf("verify body was admitted: %s", body)
		}
	}

	token := invocation("searchconsole_verification_token", "example.test")
	for _, body := range []string{
		`{"site":{"type":"INET_DOMAIN","identifier":"example.test"},"verificationMethod":"DNS_CNAME"}`,
		`{"site":{"type":"INET_DOMAIN","identifier":"other.test"},"verificationMethod":"DNS_TXT"}`,
		`{"site":{"type":"INET_DOMAIN","identifier":"example.test"},"verificationMethod":"DNS_TXT","owners":["attacker@example.test"]}`,
	} {
		if admitInvocationSpecificRequest(token, request(body)) == nil {
			t.Fatalf("token body was admitted: %s", body)
		}
	}

	validBody := `{"site":{"type":"INET_DOMAIN","identifier":"example.test"}}`
	for _, query := range []string{"", "verificationMethod=DNS_CNAME", "verificationMethod=DNS_TXT&verificationMethod=DNS_CNAME"} {
		verifyRequest := request(validBody)
		verifyRequest.URL.RawQuery = query

		if admitInvocationSpecificRequest(verify, verifyRequest) == nil {
			t.Fatalf("verify query was admitted: %q", query)
		}
	}

	if err := admitInvocationSpecificRequest(verify, request(validBody)); err != nil {
		t.Fatalf("valid verification body rejected: %v", err)
	}

	if err := admitInvocationSpecificRequest(token, request(`{"site":{"type":"INET_DOMAIN","identifier":"example.test"},"verificationMethod":"DNS_TXT"}`)); err != nil {
		t.Fatalf("valid token body rejected: %v", err)
	}
}

func TestNativeSearchConsoleVerificationUsesVerifyOnlyScope(t *testing.T) {
	var requests atomic.Int64

	verified := false

	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.SiteVerification = func(ctx context.Context, _ string) (*siteverification.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)
				body := `{}`

				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/siteVerification/v1/token":
					var input siteverification.SiteVerificationWebResourceGettokenRequest
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}

					if input.Site == nil || input.Site.Type != "INET_DOMAIN" || input.Site.Identifier != "example.test" || input.VerificationMethod != "DNS_TXT" {
						t.Errorf("unexpected token request: %+v", input)
					}

					body = `{"method":"DNS_TXT","token":"google-site-verification=token"}`
				case r.Method == http.MethodPost && r.URL.Path == "/siteVerification/v1/webResource":
					if r.URL.Query().Get("verificationMethod") != "DNS_TXT" {
						t.Errorf("verificationMethod was lost")
					}

					var input map[string]any
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}

					if _, present := input["owners"]; present {
						t.Errorf("owners must never be delegated: %+v", input)
					}

					site, _ := input["site"].(map[string]any)
					if site["type"] != "INET_DOMAIN" || site["identifier"] != "example.test" {
						t.Errorf("unexpected web resource: %+v", input)
					}

					verified = true
					body = `{"site":{"type":"INET_DOMAIN","identifier":"example.test"},"owners":["owner@example.test"]}`
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return siteverification.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(name, scope string, allowWrite bool, domain string) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: map[string]any{"domain": domain}, Authorization: ToolAuthorization{
				Service: "searchconsole", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: allowWrite,
			}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	// A webmasters-only grant cannot call the Site Verification API.
	if denied := call("searchconsole_verification_token", "webmasters", true, "example.test"); denied.OK || requests.Load() != 0 {
		t.Fatal("webmasters scope authorized Site Verification")
	}

	if denied := call("searchconsole_verification_token", "siteverification.verify_only", true, "https://example.test/"); denied.OK || requests.Load() != 0 {
		t.Fatal("URL-shaped domain reached Site Verification")
	}

	token := call("searchconsole_verification_token", "siteverification.verify_only", true, "example.test")
	if !token.OK || !strings.Contains(string(token.Result), "google-site-verification=token") {
		t.Fatalf("native verification token failed: %s", token.Body)
	}

	if denied := call("searchconsole_verification_verify", "siteverification.verify_only", false, "example.test"); denied.OK || verified {
		t.Fatal("verification bypassed the write grant")
	}

	response := call("searchconsole_verification_verify", "siteverification.verify_only", true, "example.test")
	if !response.OK {
		t.Fatalf("native verification failed: %s", response.Body)
	}

	if !verified {
		t.Fatal("web resource verification did not reach Google")
	}
}
