package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestAccountTasksUseNativeHandlersWithoutSyntheticResources(t *testing.T) {
	title := ""
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Tasks = func(ctx context.Context, _ string) (*tasks.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "tasks.googleapis.com" || r.URL.Path != "/tasks/v1/users/@me/lists" {
					t.Fatalf("unexpected Tasks endpoint: %s", r.URL)
				}

				body := `{"items":[]}`

				if r.Method == http.MethodPost {
					var list tasks.TaskList
					if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
						t.Error(err)
					}
					title = list.Title
					body = `{"id":"created-list","title":"Native task list"}`
				} else if title != "" {
					body = `{"items":[{"id":"created-list","title":"Native task list"}]}`
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return tasks.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	call := func(name string, scope string, write bool) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool.Name = name

			req.Tool.Arguments = map[string]any{}
			if name == "tasks_lists_create" {
				req.Tool.Arguments["title"] = "Native task list"
			}
			req.Tool.Authorization = ToolAuthorization{Service: "tasks", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: write}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	denied := call("tasks_lists_create", "tasks.readonly", true)
	if denied.Status != 400 || title != "" {
		t.Fatal("read scope authorized a task list mutation")
	}

	created := call("tasks_lists_create", "tasks", true)
	if !created.OK || title != "Native task list" {
		t.Fatalf("native create failed: %s", created.Body)
	}

	listed := call("tasks_lists", "tasks.readonly", false)
	if !listed.OK || !strings.Contains(string(listed.Result), "created-list") {
		t.Fatalf("native list did not observe fixture persistence: %s", listed.Body)
	}
}
