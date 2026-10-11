package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
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

func TestAllHostedTaskItemCommandsReuseNativeEngine(t *testing.T) {
	var requests atomic.Int64
	var inserts atomic.Int64
	var failSecondInsert atomic.Bool
	task := tasks.Task{Id: "task-1", Title: "original", Status: "needsAction"}
	transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		root := "/tasks/v1/lists/@default/tasks"
		status := 200
		var body any = &task

		switch {
		case r.Method == http.MethodGet && r.URL.Path == root:
			body = map[string]any{"items": []*tasks.Task{&task}}
		case r.Method == http.MethodGet && r.URL.Path == root+"/task-1":
		case r.Method == http.MethodPost && r.URL.Path == root:
			if failSecondInsert.Load() && inserts.Add(1) == 2 {
				return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"synthetic provider outage"}}`)), Request: r}, nil
			}

			if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
				t.Error(err)
			}
			task.Id = "task-1"
		case r.Method == http.MethodPatch && r.URL.Path == root+"/task-1":
			var patch map[string]any
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Error(err)
			}

			if value, ok := patch["title"].(string); ok {
				task.Title = value
			}

			if value, ok := patch["status"].(string); ok {
				task.Status = value
			}

			if value, ok := patch["notes"].(string); ok {
				task.Notes = value
			}
		case r.Method == http.MethodDelete && r.URL.Path == root+"/task-1":
			status = 204
		case r.Method == http.MethodPost && r.URL.Path == "/tasks/v1/lists/@default/clear":
			status = 204
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			status = 500
		}

		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal fixture: %w", err)
		}

		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
	})
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		client := func(ctx context.Context) *http.Client {
			return &http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}
		}
		engine.NativeServices.TasksHTTP = func(ctx context.Context, _ string) (*http.Client, error) { return client(ctx), nil }
		engine.NativeServices.Tasks = func(ctx context.Context, _ string) (*tasks.Service, error) {
			return tasks.NewService(ctx, option.WithHTTPClient(client(ctx)))
		}
	})

	call := func(name string, args map[string]any, scope string, destructive bool) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool.Name = name
			req.Tool.Arguments = args
			req.Tool.Authorization = ToolAuthorization{Service: "tasks", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, test := range []struct {
		name     string
		args     map[string]any
		expected string
	}{
		{"tasks_add", map[string]any{"tasklist_id": "@default", "title": "created", "notes": "notes"}, "created"},
		{"tasks_list", map[string]any{"tasklist_id": "@default", "show_completed": false}, "created"},
		{"tasks_get", map[string]any{"tasklist_id": "@default", "task_id": "task-1"}, "created"},
		{"tasks_raw", map[string]any{"tasklist_id": "@default", "task_id": "task-1"}, "created"},
		{"tasks_update", map[string]any{"tasklist_id": "@default", "task_id": "task-1", "title": "updated", "notes": "updated notes"}, "updated"},
		{"tasks_done", map[string]any{"tasklist_id": "@default", "task_id": "task-1"}, "completed"},
		{"tasks_undo", map[string]any{"tasklist_id": "@default", "task_id": "task-1"}, "needsAction"},
		{"tasks_delete", map[string]any{"tasklist_id": "@default", "task_id": "task-1"}, "deleted"},
		{"tasks_clear", map[string]any{"tasklist_id": "@default"}, "cleared"},
	} {
		t.Run(test.name, func(t *testing.T) {
			binding, _ := nativeBinding(test.name)
			if binding.Write {
				before := requests.Load()

				denied := call(test.name, test.args, "tasks.readonly", true)
				if denied.Status != 400 || requests.Load() != before {
					t.Fatalf("read-only scope admitted %s", test.name)
				}
			}

			if binding.destructive {
				before := requests.Load()

				denied := call(test.name, test.args, "tasks", false)
				if denied.Status != 400 || requests.Load() != before {
					t.Fatalf("destructive denial admitted %s", test.name)
				}
			}

			response := call(test.name, test.args, "tasks", true)
			if !response.OK || !strings.Contains(string(response.Result), test.expected) {
				t.Fatalf("native %s failed: %s", test.name, response.Body)
			}
		})
	}

	if task.Title != "updated" || task.Notes != "updated notes" || task.Status != "needsAction" {
		t.Fatalf("updates not persisted in fixture: %+v", task)
	}
	before := requests.Load()

	denied := call("tasks_get", map[string]any{"tasklist_id": "../../foreign", "task_id": "task-1"}, "tasks", false)
	if denied.Status != 400 || requests.Load() != before {
		t.Fatal("invalid selector reached Google")
	}

	for _, args := range []map[string]any{
		{"tasklist_id": "@default", "task_id": "task-1", "notes": ""},
		{"tasklist_id": "@default", "task_id": "task-1", "title": ""},
	} {
		response := call("tasks_update", args, "tasks", true)
		if response.OK || requests.Load() != before {
			t.Fatal("unsupported empty native patch was reported as successful")
		}
	}

	for _, args := range []map[string]any{
		{"tasklist_id": "@default", "title": "Repeated", "repeat": "daily", "due": "2026-10-12", "repeat_until": "9999-12-31"},
		{"tasklist_id": "@default", "title": "Repeated", "repeat": "daily", "due": "2026-10-12", "repeat_count": 1001},
	} {
		response := call("tasks_add", args, "tasks", false)
		if response.OK || requests.Load() != before {
			t.Fatal("unbounded repeated task creation reached Google")
		}
	}

	repeatedArgs := map[string]any{"tasklist_id": "@default", "title": "Repeated", "repeat": "daily", "due": "2026-10-12", "repeat_count": 2}

	repeated := call("tasks_add", repeatedArgs, "tasks", false)
	if !repeated.OK || requests.Load() != before+2 || !strings.Contains(string(repeated.Result), `"count":2`) {
		t.Fatalf("native repeated creation failed: %s", repeated.Body)
	}

	failSecondInsert.Store(true)

	partial := call("tasks_add", repeatedArgs, "tasks", false)
	if partial.OK || partial.Status != 502 || !strings.HasSuffix(task.Title, "(#1/2)") {
		t.Fatalf("partial native write failure was hidden: %s", partial.Body)
	}

	readBack := call("tasks_get", map[string]any{"tasklist_id": "@default", "task_id": "task-1"}, "tasks.readonly", false)
	if !readBack.OK || !strings.Contains(string(readBack.Result), "(#1/2)") {
		t.Fatalf("partial successful fixture write did not persist: %s", readBack.Body)
	}
}

func TestTaskItemRequestGuardRejectsAlternateMethodsAndResources(t *testing.T) {
	for _, name := range []string{"tasks_list", "tasks_get", "tasks_raw", "tasks_add", "tasks_update", "tasks_done", "tasks_undo", "tasks_delete", "tasks_clear"} {
		t.Run(name, func(t *testing.T) {
			binding, _ := nativeBinding(name)
			invocation := &ToolInvocation{Name: name, Arguments: map[string]any{"tasklist_id": "@default", "task_id": "task-1"}}

			paths, err := binding.argumentPaths(invocation.Arguments)
			if err != nil {
				t.Fatal(err)
			}
			guard := nativeRequestGuard(invocation)

			for _, request := range []struct{ method, path string }{
				{http.MethodPut, paths[0]},
				{binding.methods[0], "/tasks/v1/lists/foreign-long-list-id/tasks/task-1"},
			} {
				req, err := http.NewRequestWithContext(t.Context(), request.method, "https://tasks.googleapis.com"+request.path, nil)
				if err != nil {
					t.Fatal(err)
				}

				if guard(req) == nil {
					t.Fatalf("alternate request admitted: %s %s", request.method, request.path)
				}
			}
		})
	}
}
