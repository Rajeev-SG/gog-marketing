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
	"google.golang.org/api/tagmanager/v2"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/hosted/policy"
)

func TestAllNativeTagManagerOperationsUseExactGrantsMethodsAndScopes(t *testing.T) {
	for _, op := range policy.TagManagerOperations() {
		t.Run(op.Name(), func(t *testing.T) {
			args := map[string]any{}
			if op.Selector == "account" {
				args["account_path"] = "accounts/111"
			}

			if op.Selector == "container" {
				args["container_path"] = "accounts/111/containers/222"
			}

			if op.Workspace {
				args["workspace_id"] = "3"
			}

			if op.Item {
				key := "resource_id"
				if op.Family == "versions" {
					key = "version_id"
				}
				args[key] = "4"
			}

			if op.Action == "create" || op.Action == "update" {
				args["body_json"] = `{"name":"Gog fixture","type":"html","usageContext":["web"]}`
			}

			path, err := op.Path(args)
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int64
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.TagManager = func(ctx context.Context, _ string) (*tagmanager.Service, error) {
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)

						if r.URL.Host != "tagmanager.googleapis.com" || r.URL.Path != path || r.Method != op.Method {
							t.Errorf("unexpected native request: %s %s, want %s %s", r.Method, r.URL, op.Method, path)
						}

						if op.Action == "create" || op.Action == "update" {
							var body map[string]any
							if json.NewDecoder(r.Body).Decode(&body) != nil || body["name"] != "Gog fixture" {
								t.Error("inline native mutation body lost")
							}
						}
						item := map[string]any{"name": "Gog fixture", "path": strings.TrimPrefix(path, "/tagmanager/v2/"), "accountId": "111", "containerId": "222", "workspaceId": "3", "tagId": "4", "triggerId": "4", "variableId": "4", "containerVersionId": "4"}
						var body any = item

						if op.Action == "list" {
							key := strings.TrimSuffix(op.Family, "s")
							if op.Family == "versions" {
								key = "containerVersionHeader"
							}
							body = map[string]any{key: []any{item}}
						}

						if op.Action == "delete" || op.Action == "status" || op.Action == "sync" {
							body = map[string]any{}
						}

						if op.Action == "create-version" {
							body = map[string]any{"compilerError": false, "containerVersion": item}
						}
						raw, _ := json.Marshal(body)

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
					})

					return tagmanager.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})
			call := func(scope string, write, destructive bool, a map[string]any) capturedResponse {
				body := nativeToolBody(t, func(req *Request) {
					auth := ToolAuthorization{Service: "tagmanager", AllowedTools: []string{op.Name()}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: write, AllowDestructive: destructive}
					if op.Selector != "" {
						id := "accounts/111"
						if op.Selector == "container" {
							id = "accounts/111/containers/222"
						}
						auth.Resource = &ResourceGrantInput{Service: "tagmanager", ResourceType: op.Selector, ResourceID: id, Enabled: true}
					}
					req.Tool = &ToolInvocation{Name: op.Name(), Arguments: a, Authorization: auth}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			}

			scope := op.Scope
			if op.Method == http.MethodGet {
				scope = "tagmanager.readonly"
			}

			result := call(scope, true, true, args)
			if !result.OK || calls.Load() != 1 {
				t.Fatalf("native operation failed: %s, calls=%d", result.Body, calls.Load())
			}
			before := calls.Load()

			if op.Method != http.MethodGet {
				if call("tagmanager.readonly", true, true, args).OK || call(scope, false, true, args).OK {
					t.Fatal("read/no-write grant authorized mutation")
				}
			}

			if op.Destructive {
				if call(scope, true, false, args).OK {
					t.Fatal("deletion without destructive grant accepted")
				}
			}

			if op.Selector != "" {
				foreign := make(map[string]any)
				for k, v := range args {
					foreign[k] = v
				}

				foreign[op.ResourceArgument()] = "accounts/999/containers/999"
				if call(scope, true, true, foreign).OK {
					t.Fatal("foreign resource accepted")
				}
			}

			if op.Workspace {
				bad := make(map[string]any)
				for k, v := range args {
					bad[k] = v
				}

				bad["workspace_id"] = "../foreign"
				if call(scope, true, true, bad).OK {
					t.Fatal("workspace escape accepted")
				}
			}

			if op.Action == "create" || op.Action == "update" {
				for _, raw := range []string{"@/etc/passwd", `{"path":"accounts/999/containers/999"}`} {
					bad := make(map[string]any)
					for k, v := range args {
						bad[k] = v
					}

					bad["body_json"] = raw
					if call(scope, true, true, bad).OK {
						t.Fatal("file/foreign mutation path accepted")
					}
				}
			}

			if calls.Load() != before {
				t.Fatal("denied request reached Google")
			}
		})
	}
}
