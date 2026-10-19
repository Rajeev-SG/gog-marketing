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
	"google.golang.org/api/tasks/v1"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeTasksListLifecycleAuthority(t *testing.T) {
	const list = "list-source-1234567890"
	var calls atomic.Int64
	title := "Original"
	deleted := false
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Tasks = func(ctx context.Context, account string) (*tasks.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
				t.Error("Tasks identity lost")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				status := 200

				response := map[string]any{"id": list, "title": title, "etag": "stable-etag"}
				switch r.URL.Path {
				case "/tasks/v1/users/@me/lists/" + list:
					switch r.Method {
					case http.MethodGet:
					case http.MethodPatch:
						var patch map[string]any
						if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
							t.Fatal(err)
						}

						if len(patch) != 1 || patch["title"] != "Renamed" {
							t.Errorf("title patch %+v", patch)
						}
						title = patch["title"].(string)
						response["title"] = title
					case http.MethodDelete:
						deleted = true
						status = 204
					default:
						t.Errorf("unexpectedlistmethod%s", r.Method)
					}
				case "/tasks/v1/lists/" + list + "/tasks/task-1/move":
					if r.Method != http.MethodPost || r.URL.Query().Get("parent") != "parent-1" || r.URL.Query().Get("previous") != "previous-1" || r.URL.Query().Get("destinationTasklist") != "list-dest-1234567890" {
						t.Errorf("wrong move %+v", r.URL)
					}
					response = map[string]any{"id": "new-task-id", "title": "Moved", "parent": "parent-1", "position": "0001"}
				default:
					t.Errorf("foreign/lookup endpoint %s %s", r.Method, r.URL)
				}

				raw, err := json.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}

				if status == 204 {
					raw = nil
				}

				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
			})

			return tasks.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	call := func(name, scope string, args map[string]any, write, destructive bool, mutate func(*ToolAuthorization)) capturedResponse {
		auth := ToolAuthorization{Service: "tasks", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: write, AllowDestructive: destructive}
		if mutate != nil {
			mutate(&auth)
		}
		body := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: auth} })

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	get := map[string]any{"tasklist_id": list}
	for _, scope := range []string{"tasks.readonly", "tasks"} {
		if result := call("tasks_lists_get", scope, get, false, false, nil); !result.OK || !strings.Contains(string(result.Result), "stable-etag") {
			t.Fatalf("listget %s", result.Body)
		}
	}

	mutations := []struct {
		name        string
		args        map[string]any
		destructive bool
	}{
		{"tasks_lists_rename", map[string]any{"tasklist_id": list, "title": "Renamed"}, false},
		{"tasks_move", map[string]any{"tasklist_id": list, "task_id": "task-1", "parent": "parent-1", "previous": "previous-1", "destination_tasklist_id": "list-dest-1234567890"}, false},
		{"tasks_lists_delete", get, true},
	}
	for _, op := range mutations {
		if result := call(op.name, "tasks", op.args, true, op.destructive, nil); !result.OK {
			t.Fatalf("%s %s", op.name, result.Body)
		}
		before := calls.Load()

		for _, mutate := range []func(*ToolAuthorization){
			func(a *ToolAuthorization) { a.AllowWrite = false },
			func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/tasks.readonly"} },
			func(a *ToolAuthorization) { a.AllowedTools = []string{"tasks_lists"} },
			func(a *ToolAuthorization) { a.Service = "docs" },
			func(a *ToolAuthorization) {
				a.Resource = &ResourceGrantInput{Service: "tasks", ResourceType: "file", ResourceID: list, Enabled: true}
			},
		} {
			if result := call(op.name, "tasks", op.args, true, op.destructive, mutate); result.OK {
				t.Fatalf("unauthorized%sadmitted", op.name)
			}
		}

		if op.destructive {
			if result := call(op.name, "tasks", op.args, true, false, nil); result.OK {
				t.Fatal("destructive permission omitted")
			}
		}

		if calls.Load() != before {
			t.Fatal("deniedTasksmutationreachedGoogle")
		}
	}

	if !deleted || title != "Renamed" {
		t.Fatal("list lifecycle did not persist")
	}
	before := calls.Load()

	for _, op := range []struct {
		name string
		args map[string]any
	}{
		{"tasks_lists_delete", map[string]any{"tasklist_id": "@default"}},
		{"tasks_lists_get", map[string]any{"tasklist_id": "a title"}},
		{"tasks_lists_rename", map[string]any{"tasklist_id": list, "title": ""}},
		{"tasks_move", map[string]any{"tasklist_id": list, "task_id": "task-1", "parent": "task-1"}},
		{"tasks_move", map[string]any{"tasklist_id": list, "task_id": "task-1", "destination_tasklist_id": "../foreign"}},
	} {
		if result := call(op.name, "tasks", op.args, true, true, nil); result.OK {
			t.Fatalf("invalidselectorsadmitted%+v", op)
		}
	}

	if calls.Load() != before {
		t.Fatal("invalidTaskselectorreachedGoogle")
	}
}

func TestTasksLifecycleStrictGuardAndMoveQuery(t *testing.T) {
	const list = "list-source-1234567890"
	for _, op := range []struct{ name, method, path string }{
		{"tasks_lists_get", http.MethodGet, "/tasks/v1/users/@me/lists/" + list},
		{"tasks_lists_rename", http.MethodPatch, "/tasks/v1/users/@me/lists/" + list},
		{"tasks_lists_delete", http.MethodDelete, "/tasks/v1/users/@me/lists/" + list},
		{"tasks_move", http.MethodPost, "/tasks/v1/lists/" + list + "/tasks/task-1/move"},
	} {
		guard := nativeRequestGuard(&ToolInvocation{Name: op.name, Arguments: map[string]any{"tasklist_id": list, "task_id": "task-1"}})
		for _, tc := range []struct {
			method, url string
			allow       bool
		}{
			{op.method, "https://tasks.googleapis.com" + op.path, true},
			{http.MethodPut, "https://tasks.googleapis.com" + op.path, false},
			{op.method, "https://tasks.googleapis.com" + op.path + "/extra", false},
			{op.method, "https://tasks.googleapis.com/tasks/v1/users/@me/lists", false},
			{op.method, "https://foreign.example" + op.path, false},
		} {
			request, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err := guard(request); (err == nil) != tc.allow {
				t.Errorf("%s %s allowed=%t err=%v", op.name, tc.url, tc.allow, err)
			}
		}
	}
	invocation := &ToolInvocation{Name: "tasks_move", Arguments: map[string]any{"tasklist_id": list, "task_id": "task-1", "parent": "parent-1", "previous": "previous-1", "destination_tasklist_id": "list-dest-1234567890"}}
	guard := nativeRequestGuard(invocation)

	base := "https://tasks.googleapis.com/tasks/v1/lists/" + list + "/tasks/task-1/move"
	for _, tc := range []struct {
		query string
		allow bool
	}{
		{"?parent=parent-1&previous=previous-1&destinationTasklist=list-dest-1234567890&alt=json&prettyPrint=false", true},
		{"?parent=foreign&previous=previous-1&destinationTasklist=list-dest-1234567890", false},
		{"?parent=parent-1&previous=previous-1&destinationTasklist=other-dest-1234567890", false},
		{"?parent=parent-1&parent=foreign&previous=previous-1&destinationTasklist=list-dest-1234567890", false},
		{"?parent=parent-1&previous=previous-1&destinationTasklist=list-dest-1234567890&fields=*", false},
		{"?parent=%ZZ&previous=previous-1&destinationTasklist=list-dest-1234567890", false},
		{"", false},
	} {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+tc.query, nil)
		if err != nil {
			t.Fatal(err)
		}

		if err := guard(request); (err == nil) != tc.allow {
			t.Errorf("query%sallowed=%terr=%v", tc.query, tc.allow, err)
		}
	}
}

func TestNativeTasksLifecycleNeverRetriesThroughGuard(t *testing.T) {
	for _, name := range []string{"tasks_lists_rename", "tasks_lists_delete", "tasks_move"} {
		t.Run(name, func(t *testing.T) {
			var effects atomic.Int64
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Tasks = func(ctx context.Context, _ string) (*tasks.Service, error) {
					base := nativeFixtureTransport(func(request *http.Request) (*http.Response, error) {
						effects.Add(1)
						return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":503,"message":"uncertain"}}`)), Request: request}, nil
					})
					retry := googleapi.NewRetryTransport(base)
					retry.MaxRetries5xx = 1

					return tasks.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, retry)}))
				}
			})

			args := map[string]any{"tasklist_id": "list-source-1234567890"}
			if name == "tasks_lists_rename" {
				args["title"] = "New"
			}

			if name == "tasks_move" {
				args["task_id"] = "task-1"
			}
			body := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "tasks", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/tasks"}, AllowWrite: true, AllowDestructive: true}}
			})

			result := postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			if result.OK || effects.Load() != 1 {
				t.Fatalf("%seffects=%dresponse=%s", name, effects.Load(), result.Body)
			}
		})
	}
}
