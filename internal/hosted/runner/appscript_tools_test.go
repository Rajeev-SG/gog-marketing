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

func TestNativeAppsScriptProjectOperationsNeverExecuteScript(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AppScript = func(ctx context.Context, _ string) (*script.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				body := `{"scriptId":"project-1","title":"Beta project"}`

				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/projects":
				case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/project-1":
				case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/project-1/content":
					body = `{"scriptId":"project-1","files":[{"name":"Code","type":"SERVER_JS","source":"function beta() { return 1; }"}]}`
				default:
					t.Errorf("undeclared/script execution endpoint reached: %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return script.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	call := func(name, scope string, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "appscript", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	denied := call("appscript_create", "script.projects.readonly", map[string]any{"title": "Beta project"})
	if denied.OK || calls.Load() != 0 {
		t.Fatal("read project scope authorized creation")
	}

	for _, name := range []string{"appscript_create", "appscript_get", "appscript_content"} {
		args := map[string]any{"script_id": "project-1"}
		if name == "appscript_create" {
			args = map[string]any{"title": "Beta project"}
		}

		result := call(name, "script.projects", args)
		if !result.OK || !strings.Contains(string(result.Result), "project-1") {
			t.Fatalf("native %s failed: %s", name, result.Body)
		}
	}
	before := calls.Load()

	malformed := call("appscript_get", "script.projects.readonly", map[string]any{"script_id": "../foreign"})
	if malformed.OK || calls.Load() != before {
		t.Fatal("invalid project selector reached Google")
	}

	execution := call("appscript_run", "script.projects", map[string]any{"script_id": "project-1"})
	if execution.OK || calls.Load() != before {
		t.Fatal("management-only permission admitted execution")
	}
}
