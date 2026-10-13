package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/drivelabels/v2"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeDriveLabelSchemaReadsRequireExactScopeAndPaths(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.DriveLabels = func(ctx context.Context, _ string) (*drivelabels.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodGet || r.URL.Host != "drivelabels.googleapis.com" {
					t.Errorf("unexpected schema request: %s %s", r.Method, r.URL)
				}

				body := `{"name":"labels/label-1@published","id":"label-1","properties":{"title":"Beta label"}}`
				if r.URL.Path == "/v2/labels" {
					body = `{"labels":[{"name":"labels/label-1@published","id":"label-1","properties":{"title":"Beta label"}}],"nextPageToken":"next"}`
				} else if r.URL.Path != "/v2/labels/label-1@published" {
					t.Errorf("foreign label path: %s", r.URL.Path)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return drivelabels.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(name, scope string, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "drivelabels", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, name := range []string{"drivelabels_list", "drivelabels_get"} {
		args := map[string]any{}
		if name == "drivelabels_get" {
			args["label_name"] = "labels/label-1@published"
		}

		denied := call(name, "drive.file", args)
		if denied.OK || calls.Load() != 0 {
			t.Fatal("selected-file scope incorrectly authorized global schema reads")
		}

		read := call(name, "drive.labels.readonly", args)
		if !read.OK || !strings.Contains(string(read.Result), "Beta label") {
			t.Fatalf("native %s failed: %s", name, read.Body)
		}
		// Reset only the fixture request counter between independent scope checks.
		calls.Store(0)
	}

	denied := call("drivelabels_get", "drive.labels.readonly", map[string]any{"label_name": "labels/../foreign"})
	if denied.OK || calls.Load() != 0 {
		t.Fatal("invalid schema selector reached Google")
	}
}

func TestNativeDriveLabelsWorkspaceErrorIsSanitized(t *testing.T) {
	const secret = "SECRET_TOKEN"
	const email = "user@example.com"
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.DriveLabels = func(ctx context.Context, _ string) (*drivelabels.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`{"error":{"code":400,"message":"Drive Labels API requires a Google Workspace customer; consumer Google accounts without a valid customer: %s %s"}}`, secret, email)
				return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return drivelabels.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(name string, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "drivelabels", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/drive.labels.readonly"}}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	response := call("drivelabels_list", map[string]any{})
	if response.OK || response.Status != http.StatusUnprocessableEntity || response.Error.Code != "workspace_required" {
		t.Fatalf("expected sanitized workspace_required 422, got %d %s", response.Status, response.Body)
	}

	payload := response.Body
	if strings.Contains(payload, secret) || strings.Contains(payload, email) {
		t.Fatalf("response leaked provider content: %s", payload)
	}
}
