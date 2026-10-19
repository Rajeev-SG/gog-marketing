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

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeAppScriptExecutionAuthority(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AppScript = func(ctx context.Context, account string) (*script.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
				t.Error("trusted execution identity lost")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodPost || r.URL.Path != "/v1/scripts/deployment-1:run" {
					t.Errorf("unexpected execution %s %s", r.Method, r.URL)
				}

				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}

				if payload["function"] != "sum" || payload["devMode"] != nil || len(payload["parameters"].([]any)) != 2 {
					t.Errorf("execution payload %v", payload)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"done":true,"response":{"result":3}}`)), Request: r}, nil
			})

			return script.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	base := ToolAuthorization{Service: "appscript", AllowedTools: []string{"appscript_run"}, Scopes: []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/script.projects"}, AllowWrite: true, AllowDestructive: true}
	call := func(auth ToolAuthorization, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: "appscript_run", Arguments: args, Authorization: auth}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	args := map[string]any{"deployment_id": "deployment-1", "function": "sum", "params_json": "[1,2]"}

	for _, scope := range []string{"userinfo.email", "documents", "spreadsheets"} {
		auth := base

		auth.Scopes = []string{"https://www.googleapis.com/auth/" + scope}
		if result := call(auth, args); !result.OK || !strings.Contains(string(result.Result), `"result":3`) {
			t.Fatalf("scope %s execution failed %s", scope, result.Body)
		}
	}
	before := calls.Load()

	for _, mutate := range []func(*ToolAuthorization){
		func(auth *ToolAuthorization) { auth.AllowWrite = false },
		func(auth *ToolAuthorization) { auth.AllowDestructive = false },
		func(auth *ToolAuthorization) { auth.AllowedTools = []string{"appscript_get"} },
		func(auth *ToolAuthorization) { auth.Service = "docs" },
		func(auth *ToolAuthorization) {
			auth.Scopes = []string{"https://www.googleapis.com/auth/script.projects"}
		},
		func(auth *ToolAuthorization) {
			auth.Scopes = append(append([]string{}, base.Scopes...), "https://www.googleapis.com/auth/bigquery")
		},
		func(auth *ToolAuthorization) {
			auth.Scopes = append(append([]string{}, base.Scopes...), "https://www.googleapis.com/auth/drive")
		},
		func(auth *ToolAuthorization) {
			auth.Scopes = append(append([]string{}, base.Scopes...), "https://www.googleapis.com/auth/script.external_request")
		},
		func(auth *ToolAuthorization) {
			auth.Resource = &ResourceGrantInput{Service: "appscript", ResourceType: "file", ResourceID: "deployment-1", Enabled: true}
		},
	} {
		auth := base
		mutate(&auth)

		if result := call(auth, args); result.OK {
			t.Fatalf("unauthorized execution accepted %+v", auth)
		}
	}

	for _, bad := range []map[string]any{
		{"deployment_id": "../foreign", "function": "sum"},
		{"deployment_id": "deployment-1", "function": "sum", "params_json": "@/tmp/script.json"},
	} {
		if result := call(base, bad); result.OK {
			t.Fatalf("invalid execution accepted %v", bad)
		}
	}

	if calls.Load() != before {
		t.Fatal("denied execution reached Google")
	}
}

func TestAppScriptExecutionRequestGuard(t *testing.T) {
	guard := nativeRequestGuard(&ToolInvocation{Name: "appscript_run", Arguments: map[string]any{"deployment_id": "deployment-1"}})

	for _, tc := range []struct {
		method, url string
		allow       bool
	}{
		{http.MethodPost, "https://script.googleapis.com/v1/scripts/deployment-1:run", true},
		{http.MethodGet, "https://script.googleapis.com/v1/scripts/deployment-1:run", false},
		{http.MethodPost, "https://script.googleapis.com/v1/scripts/foreign:run", false},
		{http.MethodPost, "https://script.googleapis.com/v1/projects/deployment-1", false},
		{http.MethodPost, "https://foreign.example/v1/scripts/deployment-1:run", false},
		{http.MethodPost, "https://script.googleapis.com/v1/scripts/deployment-1:run/extra", false},
	} {
		req, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
		if err != nil {
			t.Fatal(err)
		}

		if err := guard(req); (err == nil) != tc.allow {
			t.Errorf("%s %s allowed=%t err=%v", tc.method, tc.url, tc.allow, err)
		}
	}
}

func TestNativeAppScriptExecutionDoesNotRetryThroughGuard(t *testing.T) {
	var effects atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AppScript = func(ctx context.Context, _ string) (*script.Service, error) {
			base := nativeFixtureTransport(func(req *http.Request) (*http.Response, error) {
				effects.Add(1)
				return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":503,"message":"uncertain after effect"}}`)), Request: req}, nil
			})
			retry := googleapi.NewRetryTransport(base)
			retry.MaxRetries5xx = 1

			return script.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, retry)}))
		}
	})
	body := nativeToolBody(t, func(req *Request) {
		req.Tool = &ToolInvocation{
			Name: "appscript_run", Arguments: map[string]any{"deployment_id": "deployment-1", "function": "mutate"},
			Authorization: ToolAuthorization{Service: "appscript", AllowedTools: []string{"appscript_run"}, Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"}, AllowWrite: true, AllowDestructive: true},
		}
	})

	result := postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	if result.OK || effects.Load() != 1 {
		t.Fatalf("uncertain execution repeated: effects=%d response=%s", effects.Load(), result.Body)
	}
}
