package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativePeopleSelfProfileScopesAndNoLocalFallback(t *testing.T) {
	for _, scope := range []string{"profile", "https://www.googleapis.com/auth/userinfo.profile"} {
		t.Run(scope, func(t *testing.T) {
			var calls atomic.Int64
			var unavailable atomic.Bool
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.PeopleContacts = func(ctx context.Context, _ string) (*people.Service, error) {
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)

						if r.Method != http.MethodGet || r.URL.Host != "people.googleapis.com" || r.URL.Path != "/v1/people/me" {
							t.Errorf("unselected person request: %s", r.URL)
						}
						status := 200
						body := `{"resourceName":"people/me","names":[{"displayName":"Beta owner"}]}`

						if unavailable.Load() {
							status = 403
							body = `{"error":{"code":403,"errors":[{"reason":"accessNotConfigured"}],"message":"People API disabled"}}`
						}

						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})

					return people.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})
			body := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{Name: "people_me", Arguments: map[string]any{}, Authorization: ToolAuthorization{Service: "people", AllowedTools: []string{"people_me"}, Scopes: []string{scope}}}
			})
			invoke := func() capturedResponse {
				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			}

			profile := invoke()
			if !profile.OK || !strings.Contains(string(profile.Result), "Beta owner") {
				t.Fatalf("native self-profile failed: %s", profile.Body)
			}

			unavailable.Store(true)

			failed := invoke()
			if failed.OK || failed.Status != 502 || calls.Load() != 2 {
				t.Fatalf("API unavailable didn't fail without fallback: %s", failed.Body)
			}
		})
	}
}
