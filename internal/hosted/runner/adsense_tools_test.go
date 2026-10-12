package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/adsense/v2"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeAdSenseReadsRequireReadScopeAndCanonicalAccounts(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AdSense = func(ctx context.Context, _ string) (*adsense.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				body := `{"name":"accounts/pub-1234567890123456","displayName":"Beta publisher","state":"READY"}`

				if r.Method != http.MethodGet || r.URL.Host != "adsense.googleapis.com" {
					t.Errorf("unexpected AdSense request: %s %s", r.Method, r.URL)
				}

				switch r.URL.Path {
				case "/v2/accounts":
					body = `{"accounts":[{"name":"accounts/pub-1234567890123456","displayName":"Beta publisher"}],"nextPageToken":"next"}`
				case "/v2/accounts/pub-1234567890123456":
				default:
					t.Errorf("foreign publisher path: %s", r.URL.Path)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return adsense.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	call := func(name, scope string, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "adsense", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	denied := call("adsense_accounts_list", "analytics.readonly", map[string]any{})
	if denied.OK || calls.Load() != 0 {
		t.Fatal("unrelated scope admitted AdSense")
	}

	for _, name := range []string{"adsense_accounts_list", "adsense_accounts_get"} {
		args := map[string]any{}
		if name == "adsense_accounts_get" {
			args["account_name"] = "accounts/pub-1234567890123456"
		}

		result := call(name, "adsense.readonly", args)
		if !result.OK || !strings.Contains(string(result.Result), "Beta publisher") {
			t.Fatalf("native %s failed: %s", name, result.Body)
		}
	}
	before := calls.Load()

	invalid := call("adsense_accounts_get", "adsense.readonly", map[string]any{"account_name": "accounts/../foreign"})
	if invalid.OK || calls.Load() != before {
		t.Fatal("invalid publisher selector reached Google")
	}
}
