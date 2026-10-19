package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeAppScriptDeploymentDeleteAuthority(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AppScript = func(ctx context.Context, _ string) (*script.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodDelete || r.URL.Path != "/v1/projects/project-1/deployments/deployment-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
			})

			return script.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	auth := ToolAuthorization{Service: "appscript", AllowedTools: []string{"appscript_deployment_delete"}, Scopes: []string{"https://www.googleapis.com/auth/script.deployments"}, AllowWrite: true, AllowDestructive: true}
	args := map[string]any{"script_id": "project-1", "deployment_id": "deployment-1"}

	call := func(a ToolAuthorization, input map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: "appscript_deployment_delete", Arguments: input, Authorization: a}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	if result := call(auth, args); !result.OK || calls.Load() != 1 {
		t.Fatalf("delete failed: %s calls=%d", result.Body, calls.Load())
	}

	for _, mutate := range []func(*ToolAuthorization){
		func(a *ToolAuthorization) { a.AllowWrite = false },
		func(a *ToolAuthorization) { a.AllowDestructive = false },
		func(a *ToolAuthorization) { a.AllowedTools = []string{"appscript_deployment_create"} },
		func(a *ToolAuthorization) {
			a.Scopes = []string{"https://www.googleapis.com/auth/script.deployments.readonly"}
		},
	} {
		bad := auth
		mutate(&bad)

		if call(bad, args).OK {
			t.Fatal("unauthorized deletion accepted")
		}
	}

	for _, key := range []string{"script_id", "deployment_id"} {
		bad := map[string]any{"script_id": "project-1", "deployment_id": "deployment-1"}

		bad[key] = "../foreign"
		if call(auth, bad).OK {
			t.Fatal("invalid identifier accepted")
		}
	}

	if calls.Load() != 1 {
		t.Fatalf("denials reached Google: %d", calls.Load())
	}
	invocation := ToolInvocation{Name: "appscript_deployment_delete", Arguments: args, Authorization: auth}
	guard := nativeRequestGuard(&invocation)

	for _, target := range []struct{ method, url string }{
		{http.MethodGet, "https://script.googleapis.com/v1/projects/project-1/deployments/deployment-1"},
		{http.MethodDelete, "https://other.example/v1/projects/project-1/deployments/deployment-1"},
		{http.MethodDelete, "https://script.googleapis.com/v1/projects/foreign/deployments/deployment-1"},
		{http.MethodDelete, "https://script.googleapis.com/v1/projects/project-1/deployments/foreign"},
		{http.MethodDelete, "https://script.googleapis.com/v1/projects/project-1/content"},
	} {
		request, err := http.NewRequestWithContext(t.Context(), target.method, target.url, nil)
		if err != nil {
			t.Fatal(err)
		}

		if err := guard(request); err == nil {
			t.Fatalf("accepted foreign request %s %s", target.method, target.url)
		}
	}
}
