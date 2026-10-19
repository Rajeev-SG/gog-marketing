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
	"google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeAppScriptDeploymentUpdateAuthority(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AppScript = func(ctx context.Context, _ string) (*script.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if (r.Method != http.MethodGet && r.Method != http.MethodPut) || r.URL.Path != "/v1/projects/project-1/deployments/deployment-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}

				if r.Method == http.MethodPut {
					var body struct {
						Config struct {
							Version     int64  `json:"versionNumber"`
							Description string `json:"description"`
							Manifest    string `json:"manifestFileName"`
						} `json:"deploymentConfig"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("bad PUT body: %v", err)
					}

					if body.Config.Version != 2 || body.Config.Description != "keep" || body.Config.Manifest != "appsscript" {
						t.Errorf("omitted configuration was not preserved: %+v", body)
					}
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"deploymentConfig":{"versionNumber":1,"description":"keep","manifestFileName":"appsscript"}}`)), Request: r}, nil
			})

			return script.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	auth := ToolAuthorization{Service: "appscript", AllowedTools: []string{"appscript_deployment_update"}, Scopes: []string{"https://www.googleapis.com/auth/script.deployments"}, AllowWrite: true, AllowDestructive: true}
	args := map[string]any{"script_id": "project-1", "deployment_id": "deployment-1", "version_number": 2}

	call := func(a ToolAuthorization, input map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: "appscript_deployment_update", Arguments: input, Authorization: a}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	if result := call(auth, args); !result.OK || calls.Load() != 2 {
		t.Fatalf("update failed: %s calls=%d", result.Body, calls.Load())
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
			t.Fatal("unauthorized update accepted")
		}
	}

	for _, key := range []string{"script_id", "deployment_id"} {
		bad := map[string]any{"script_id": "project-1", "deployment_id": "deployment-1", "version_number": 2}

		bad[key] = "../foreign"
		if call(auth, bad).OK {
			t.Fatal("invalid identifier accepted")
		}
	}

	if calls.Load() != 2 {
		t.Fatalf("denials reached Google: %d", calls.Load())
	}
	invocation := ToolInvocation{Name: "appscript_deployment_update", Arguments: args, Authorization: auth}
	guard := nativeRequestGuard(&invocation)

	for _, target := range []struct{ method, url string }{
		{http.MethodDelete, "https://script.googleapis.com/v1/projects/project-1/deployments/deployment-1"},
		{http.MethodPut, "https://other.example/v1/projects/project-1/deployments/deployment-1"},
		{http.MethodPut, "https://script.googleapis.com/v1/projects/foreign/deployments/deployment-1"},
		{http.MethodPut, "https://script.googleapis.com/v1/projects/project-1/deployments/foreign"},
		{http.MethodPut, "https://script.googleapis.com/v1/projects/project-1/content"},
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
