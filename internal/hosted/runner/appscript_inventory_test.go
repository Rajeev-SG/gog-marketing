package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeAppsScriptInventoryListsExactPagedEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		firstScope string
		nextScope  string
		firstID    string
		secondID   string
		firstToken string
		nextToken  string
		firstBody  string
		nextBody   string
	}{
		{
			name: "deployments", tool: "appscript_deployments",
			firstScope: "script.deployments.readonly", nextScope: "script.deployments",
			firstID: "deploy-1", secondID: "deploy-2",
			firstToken: "deployments-page-2", nextToken: "deployments-page-3",
			firstBody: `{"deployments":[{"deploymentId":"deploy-1","deploymentConfig":{"versionNumber":1,"description":"First"}}],"nextPageToken":"deployments-page-2"}`,
			nextBody:  `{"deployments":[{"deploymentId":"deploy-2","deploymentConfig":{"versionNumber":2}}]}`,
		},
		{
			name: "versions", tool: "appscript_versions",
			firstScope: "script.projects.readonly", nextScope: "script.projects",
			firstID: "1", secondID: "2",
			firstToken: "versions-page-2", nextToken: "versions-page-3",
			firstBody: `{"versions":[{"versionNumber":1,"description":"First"}],"nextPageToken":"versions-page-2"}`,
			nextBody:  `{"versions":[{"versionNumber":2}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.AppScript = func(ctx context.Context, account string) (*scriptapi.Service, error) {
					if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted inventory execution identity was lost")
					}

					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						number := calls.Add(1)

						path := "/v1/projects/project-1/" + test.name
						if r.Method != http.MethodGet || r.URL.Path != path || r.URL.Hostname() != "script.googleapis.com" {
							t.Errorf("foreign or mutating inventory request reached Google: %s %s", r.Method, r.URL)
						}

						if r.URL.Query().Get("pageSize") != "2" {
							t.Errorf("native page size was lost: %q", r.URL.Query().Get("pageSize"))
						}

						token := r.URL.Query().Get("pageToken")
						var body string

						switch number {
						case 1, 3:
							if token != "" {
								t.Errorf("first page carried %q", token)
							}
							body = test.firstBody
						case 2, 4:
							if token != test.firstToken {
								t.Errorf("second page token: got %q want %q", token, test.firstToken)
							}
							body = test.nextBody
						default:
							t.Errorf("unexpected inventory request: %s %s", r.Method, r.URL)
						}

						return &http.Response{
							StatusCode: 200,
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body:       io.NopCloser(strings.NewReader(body)),
							Request:    r,
						}, nil
					})

					return scriptapi.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			call := func(tool, scope string, args map[string]any) capturedResponse {
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{
						Name: tool, Arguments: args,
						Authorization: ToolAuthorization{
							Service: "appscript", AllowedTools: []string{tool},
							Scopes: []string{"https://www.googleapis.com/auth/" + scope},
						},
					}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) {
					claims["operation"] = OperationToolCall
				}), body)
			}

			first := call(test.tool, test.firstScope, map[string]any{"script_id": "project-1", "max": 2})
			if !first.OK || !strings.Contains(string(first.Result), test.firstID) {
				t.Fatalf("first %s page failed: %s", test.name, first.Body)
			}

			second := call(test.tool, test.nextScope, map[string]any{
				"script_id": "project-1", "max": 2, "page": test.firstToken, "fail_empty": true,
			})
			if !second.OK || !strings.Contains(string(second.Result), test.secondID) {
				t.Fatalf("second %s page failed: %s", test.name, second.Body)
			}

			before := calls.Load()

			for _, args := range []map[string]any{
				{"script_id": "../foreign", "max": 2},
				{"script_id": "project/../foreign", "max": 2},
				{"script_id": "--account=foreign@example.com", "max": 2},
			} {
				denied := call(test.tool, test.firstScope, args)
				if denied.OK || calls.Load() != before {
					t.Fatalf("malformed selector or foreign account flag reached Google: %s", denied.Body)
				}
			}

			wrongScope := "script.projects"
			if test.tool == "appscript_versions" {
				wrongScope = "script.deployments"
			}

			if wrongScope == test.firstScope || wrongScope == test.nextScope {
				wrongScope = "script.projects"
			}

			denied := call(test.tool, wrongScope, map[string]any{"script_id": "project-1", "max": 2})
			if denied.OK || calls.Load() != before {
				t.Fatal("unauthorized inventory scope reached Google")
			}

			otherTool := "appscript_versions"
			if test.tool == otherTool {
				otherTool = "appscript_deployments"
			}
			deniedBody := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{
					Name: test.tool, Arguments: map[string]any{"script_id": "project-1", "max": 2},
					Authorization: ToolAuthorization{
						Service: "appscript", AllowedTools: []string{otherTool},
						Scopes: []string{"https://www.googleapis.com/auth/" + test.firstScope},
					},
				}
			})

			denied = postExecute(t, harness.server, signInvocation(t, deniedBody, func(claims map[string]any) {
				claims["operation"] = OperationToolCall
			}), deniedBody)
			if denied.OK || calls.Load() != before {
				t.Fatal("disabled tool grant reached Google")
			}

			foreignTenant := nativeToolBody(t, func(req *Request) {
				req.TenantID = "tenant-b"
				req.Tool = &ToolInvocation{
					Name: test.tool, Arguments: map[string]any{"script_id": "project-1", "max": 2},
					Authorization: ToolAuthorization{
						Service: "appscript", AllowedTools: []string{test.tool},
						Scopes: []string{"https://www.googleapis.com/auth/" + test.firstScope},
					},
				}
			})

			foreign := postExecute(t, harness.server, signInvocation(t, foreignTenant, func(claims map[string]any) {
				claims["operation"] = OperationToolCall
			}), foreignTenant)
			if foreign.OK || foreign.Status != http.StatusForbidden || calls.Load() != before {
				t.Fatal("foreign tenant reached inventory execution")
			}
		})
	}
}

func TestNativeAppsScriptInventoryBindingsStayReadOnly(t *testing.T) {
	for _, name := range []string{"appscript_deployments", "appscript_versions"} {
		binding, ok := nativeBinding(name)
		if !ok || binding.Write || binding.ResourceType != "" || len(binding.methods) != 1 || binding.methods[0] != http.MethodGet {
			t.Fatalf("%s has an unsafe binding: %+v", name, binding)
		}

		expected := []string{"script.deployments.readonly", "script.deployments"}
		if name == "appscript_versions" {
			expected = []string{"script.projects.readonly", "script.projects"}
		}

		if fmt.Sprint(binding.scopes) != fmt.Sprint(expected) {
			t.Fatalf("%s has unexpected scopes: %v", name, binding.scopes)
		}
	}
}
