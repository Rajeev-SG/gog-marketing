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

func TestNativeAppScriptManagementPermissions(t *testing.T) {
	var calls atomic.Int64

	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AppScript = func(ctx context.Context, _ string) (*script.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				response := `{"scriptId":"project-1"}`

				switch r.URL.Path {
				case "/v1/projects/project-1/content":
					if r.Method != http.MethodPut || len(body["files"].([]any)) != 2 {
						t.Errorf("bad push request %s %+v", r.Method, body)
					}
				case "/v1/projects/project-1/versions":
					if r.Method != http.MethodPost || body["description"] != "release" {
						t.Errorf("bad version request %s %+v", r.Method, body)
					}
					response = `{"versionNumber":7}`
				case "/v1/projects/project-1/deployments":
					if r.Method != http.MethodPost || body["versionNumber"] != float64(7) {
						t.Errorf("bad deployment request %s %+v", r.Method, body)
					}
					response = `{"deploymentId":"deployment-1"}`
				case "/v1/projects":
					if r.Method != http.MethodPost || body["parentId"] != "parent-1" {
						t.Errorf("bad bound-create request %s %+v", r.Method, body)
					}
				default:
					t.Errorf("unexpected management endpoint %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
			})

			return script.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	for _, tc := range []struct {
		name, scope        string
		args               map[string]any
		destructive, bound bool
	}{
		{"appscript_push", "script.projects", map[string]any{"script_id": "project-1", "files_json": `[{"name":"appsscript","type":"JSON","source":"{}"},{"name":"Code","type":"SERVER_JS","source":"function hello(){return 1;}"}]`}, true, false},
		{"appscript_version_create", "script.projects", map[string]any{"script_id": "project-1", "description": "release"}, false, false},
		{"appscript_deployment_create", "script.deployments", map[string]any{"script_id": "project-1", "version_number": 7}, true, false},
		{"appscript_bound_create", "script.projects", map[string]any{"title": "Bound", "parent_id": "parent-1"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := ToolAuthorization{Service: "appscript", AllowedTools: []string{tc.name}, Scopes: []string{"https://www.googleapis.com/auth/" + tc.scope}, AllowWrite: true, AllowDestructive: tc.destructive}
			if tc.bound {
				auth.Resource = &ResourceGrantInput{Service: "appscript", ResourceType: "file", ResourceID: "parent-1", Enabled: true}
			}
			call := func(auth ToolAuthorization) capturedResponse {
				body := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: tc.name, Arguments: tc.args, Authorization: auth} })
				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			}
			before := calls.Load()

			result := call(auth)
			if !result.OK || calls.Load() != before+1 {
				t.Fatalf("management failed %s", result.Body)
			}
			before = calls.Load()
			bad := auth

			bad.AllowWrite = false
			if call(bad).OK {
				t.Fatal("missing write permission admitted")
			}
			bad = auth

			bad.AllowedTools = []string{"appscript_get"}
			if call(bad).OK {
				t.Fatal("wrong exact tool admitted")
			}
			bad = auth

			bad.Scopes = []string{"https://www.googleapis.com/auth/script.projects.readonly"}
			if call(bad).OK {
				t.Fatal("read-only scope admitted mutation")
			}

			if tc.destructive {
				bad = auth

				bad.AllowDestructive = false
				if call(bad).OK {
					t.Fatal("missing destructive permission admitted")
				}
			}

			if tc.bound {
				bad = auth

				bad.Resource = nil
				if call(bad).OK {
					t.Fatal("missing parent grant admitted")
				}
				bad = auth
				other := *auth.Resource
				other.ResourceID = "foreign-parent"

				bad.Resource = &other
				if call(bad).OK {
					t.Fatal("foreign parent admitted")
				}
			}

			if calls.Load() != before {
				t.Fatal("denied invocation reached Google")
			}
		})
	}
}

func TestAppScriptManagementExactMethodGuard(t *testing.T) {
	for _, tc := range []struct{ name, method, path string }{
		{"appscript_push", http.MethodPut, "/v1/projects/project-1/content"},
		{"appscript_version_create", http.MethodPost, "/v1/projects/project-1/versions"},
		{"appscript_deployment_create", http.MethodPost, "/v1/projects/project-1/deployments"},
		{"appscript_bound_create", http.MethodPost, "/v1/projects"},
	} {
		guard := nativeRequestGuard(&ToolInvocation{Name: tc.name, Arguments: map[string]any{"script_id": "project-1", "parent_id": "parent-1"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "parent-1"}}})
		for _, req := range []struct {
			method, url string
			allow       bool
		}{
			{tc.method, "https://script.googleapis.com" + tc.path, true},
			{http.MethodDelete, "https://script.googleapis.com" + tc.path, false},
			{http.MethodGet, "https://script.googleapis.com" + tc.path, false},
			{tc.method, "https://script.googleapis.com/v1/projects/foreign/content", false},
			{tc.method, "https://script.googleapis.com" + tc.path + "/extra", false},
			{http.MethodPost, "https://script.googleapis.com/v1/scripts/project-1:run", false},
			{tc.method, "https://foreign.example" + tc.path, false},
		} {
			r, err := http.NewRequestWithContext(context.Background(), req.method, req.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err := guard(r); (err == nil) != req.allow {
				t.Errorf("%s %s %s allowed=%t err=%v", tc.name, req.method, req.url, req.allow, err)
			}
		}
	}
}
