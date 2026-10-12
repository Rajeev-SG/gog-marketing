package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeSitesMetadataRequiresSelectedFileAndDriveFileScope(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.SitesDrive = func(ctx context.Context, _ string) (*drive.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodGet || r.URL.Host != "www.googleapis.com" || r.URL.Path != "/drive/v3/files/selected-site" {
					t.Errorf("foreign site request: %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"selected-site","name":"Beta site","mimeType":"application/vnd.google-apps.site","webViewLink":"https://sites.google.com/d/selected-site/edit"}`)), Request: r}, nil
			})

			return drive.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(name, id, scope string, enabled bool) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: map[string]any{"site_id": id}, Authorization: ToolAuthorization{Service: "sites", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, Resource: &ResourceGrantInput{Service: "sites", ResourceType: "file", ResourceID: "selected-site", Enabled: enabled}}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, name := range []string{"sites_get", "sites_url"} {
		before := calls.Load()
		for _, denied := range []capturedResponse{call(name, "foreign-site", "drive.file", true), call(name, "selected-site", "drive.file", false), call(name, "selected-site", "documents.readonly", true)} {
			if denied.OK || calls.Load() != before {
				t.Fatal("site resource/scope denial reached Google")
			}
		}

		result := call(name, "selected-site", "drive.file", true)
		if !result.OK || !strings.Contains(string(result.Result), "selected-site") {
			t.Fatalf("native selected-site %s failed: %s", name, result.Body)
		}
	}
}
