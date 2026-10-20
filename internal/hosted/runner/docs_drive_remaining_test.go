package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

const remainingDocFixture = `{"documentId":"selected-file","revisionId":"rev-1","body":{"content":[{"startIndex":1,"endIndex":5,"paragraph":{"elements":[{"startIndex":1,"endIndex":5,"textRun":{"content":"abc\n"}}]}},{"startIndex":6,"endIndex":20,"table":{"rows":1,"columns":1,"tableRows":[{"tableCells":[{"startIndex":8,"tableCellStyle":{"rowSpan":1,"columnSpan":1},"content":[{"startIndex":9,"endIndex":13,"paragraph":{"elements":[{"startIndex":9,"endIndex":13,"textRun":{"content":"abc\n"}}]}}]}]}]}}]},"unknownZero":0,"unknownFalse":false}`

func TestNativeDocsDriveRemaining(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       map[string]any
		requestKey string
		write      bool
	}{
		{"docs_cell_style", map[string]any{"row": 1, "col": 1, "background_color": "#112233"}, "updateTableCellStyle", true},
		{"docs_cell_update", map[string]any{"row": 1, "col": 1, "content": "NEW"}, "deleteContentRange", true},
		{"docs_table_column_width", map[string]any{"col": 1, "width": 30}, "updateTableColumnProperties", true},
		{"docs_table_row_style", map[string]any{"min_height": "12pt", "prevent_overflow": false}, "updateTableRowStyle", true},
		{"docs_update", map[string]any{"text": "NEW", "at": "abc", "occurrence": 1}, "deleteContentRange", true},
		{"docs_sed", map[string]any{"expressions": []any{"s/abc/NEW/g"}}, "replaceAllText", true},
		{"docs_sed", map[string]any{"expressions": []any{"s/a,b/NEW/g"}}, "replaceAllText", true},
		{"docs_copy", map[string]any{"title": "Copied"}, "", true},
		{"docs_raw", map[string]any{}, "", false},
		{"drive_raw", map[string]any{}, "", false},
		{"drive_url", map[string]any{}, "", false},
		{"drive_mkdir", map[string]any{"name": "Folder"}, "", true},
		{"drive_share", map[string]any{"to": "anyone", "role": "writer"}, "", true},
		{"drive_unshare", map[string]any{"permission_id": "perm-public"}, "", true},
		{"drive_bulk_remove_public", map[string]any{}, "", true},
		{"drive_bulk_update_role", map[string]any{"from": "reader", "to": "commenter", "type": "user"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes, calls := 0, 0
			failWrite := false
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				transport := func(ctx context.Context) http.RoundTripper {
					base := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls++

						body := `{"id":"selected-file","name":"Selected","mimeType":"application/vnd.google-apps.document","webViewLink":"https://never-follow.example/file","resourceKey":"secret-key","exportLinks":{"text/plain":"secret-export"},"properties":{"secret":"private"}}`
						if strings.HasPrefix(r.URL.Path, "/v1/documents/") {
							body = remainingDocFixture
						}

						if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/permissions") {
							body = `{"permissions":[{"id":"perm-public","type":"anyone","role":"reader"},{"id":"perm-user","type":"user","role":"reader","emailAddress":"user@example.test"},{"id":"perm-inherited","type":"anyone","role":"reader","permissionDetails":[{"inherited":true}]}]}`
						}
						status := 200

						if r.Method != http.MethodGet {
							writes++

							if r.Method != http.MethodDelete {
								var payload map[string]any
								if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
									t.Fatal(err)
								}

								switch {
								case tc.requestKey != "":
									requests, _ := payload["requests"].([]any)
									if len(requests) == 0 {
										t.Fatal("no typed requests")
									}

									if _, ok := requests[0].(map[string]any)[tc.requestKey]; !ok {
										t.Fatalf("expected %s, got %v", tc.requestKey, requests)
									}

									control, _ := payload["writeControl"].(map[string]any)
									if tc.name != "docs_sed" && control["requiredRevisionId"] != "rev-1" {
										t.Fatalf("lost native revision guard %v", payload)
									}
								case tc.name == "docs_copy":
									if payload["name"] != "Copied" {
										t.Fatalf("bad copy %v", payload)
									}
								case tc.name == "drive_mkdir":
									parents, _ := payload["parents"].([]any)
									if len(parents) != 1 || parents[0] != "selected-file" || payload["mimeType"] != "application/vnd.google-apps.folder" {
										t.Fatalf("foreign parent or kind %v", payload)
									}
								case tc.name == "drive_share":
									if payload["type"] != "anyone" || payload["role"] != "writer" {
										t.Fatalf("bad share %v", payload)
									}
								case tc.name == "drive_bulk_update_role":
									if payload["role"] != "commenter" || !strings.HasSuffix(r.URL.Path, "/perm-user") {
										t.Fatalf("wrong permission mutation %s %v", r.URL.Path, payload)
									}
								}
							}

							if strings.Contains(r.URL.Path, "perm-inherited") {
								t.Fatal("inherited permission mutated")
							}
							body = `{"documentId":"selected-file","id":"created","name":"Created","replies":[{}]}`

							if failWrite {
								status = 503
								body = `{"error":{"code":503,"message":"uncertain"}}`
							}
						}

						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})
					retry := googleapi.NewRetryTransport(base)
					retry.MaxRetries5xx = 1

					return googleapi.GuardedTransport(ctx, retry)
				}
				engine.NativeServices.Docs = func(ctx context.Context, _ string) (*docs.Service, error) {
					return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: transport(ctx)}))
				}
				engine.NativeServices.Drive = func(ctx context.Context, _ string) (*drive.Service, error) {
					return drive.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: transport(ctx)}))
				}
				engine.NativeServices.DocsHTTP = func(ctx context.Context, _ string) (*http.Client, error) {
					return &http.Client{Transport: transport(ctx)}, nil
				}
				engine.NativeServices.DriveHTTP = func(ctx context.Context, _ string) (*http.Client, error) {
					return &http.Client{Transport: transport(ctx)}, nil
				}
			})

			binding, ok := nativeBinding(tc.name)
			if !ok {
				t.Fatal("missing binding")
			}
			tc.args[binding.ResourceArgument] = "selected-file"
			call := func(change func(*ToolAuthorization)) capturedResponse {
				auth := ToolAuthorization{Service: binding.Service, AllowedTools: []string{tc.name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, AllowWrite: true, AllowDestructive: true, Resource: &ResourceGrantInput{Service: binding.Service, ResourceType: "file", ResourceID: "selected-file", Enabled: true}}
				if change != nil {
					change(&auth)
				}
				data := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: tc.name, Arguments: tc.args, Authorization: auth} })

				return postExecute(t, harness.server, signInvocation(t, data, func(claims map[string]any) { claims["operation"] = OperationToolCall }), data)
			}

			result := call(nil)
			if !result.OK || calls == 0 || tc.write && writes != 1 {
				t.Fatalf("%s writes %d calls %d", result.Body, writes, calls)
			}

			if tc.name == "drive_raw" && (strings.Contains(string(result.Result), "secret-key") || strings.Contains(string(result.Result), "secret-export") || strings.Contains(string(result.Result), "private")) {
				t.Fatal("native raw redaction lost")
			}

			if tc.name == "docs_raw" && (!strings.Contains(string(result.Result), `"unknownZero":0`) || !strings.Contains(string(result.Result), `"unknownFalse":false`)) {
				t.Fatalf("raw values lost %s", result.Result)
			}
			before := calls

			changes := []func(*ToolAuthorization){func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-file" }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.AllowedTools = []string{"other_tool"} }, func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/tasks"} }}
			if tc.write {
				changes = append(changes, func(a *ToolAuthorization) { a.AllowWrite = false })
			}

			if binding.destructive {
				changes = append(changes, func(a *ToolAuthorization) { a.AllowDestructive = false })
			}

			for _, change := range changes {
				if denied := call(change); denied.OK {
					t.Fatal("unauthorized call admitted")
				}
			}

			if calls != before {
				t.Fatal("denied call reached Google")
			}

			if tc.write {
				beforeWrites := writes
				failWrite = true

				if failed := call(nil); failed.OK || writes != beforeWrites+1 {
					t.Fatalf("uncertain write replayed: %d -> %d %s", beforeWrites, writes, failed.Body)
				}
			}
		})
	}
}

func TestDocsDriveRemainingHTTPGuard(t *testing.T) {
	for _, binding := range nativeDocsDriveRemainingBindings() {
		args := map[string]any{binding.ResourceArgument: "selected-file", "permission_id": "perm-public"}
		invocation := &ToolInvocation{Name: binding.Name, Arguments: args, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-file"}}}
		guard := nativeRequestGuard(invocation)

		paths, err := binding.argumentPaths(args)
		if err != nil {
			t.Fatal(err)
		}

		for _, path := range paths {
			for _, method := range []string{"GET", "POST", "PATCH", "DELETE", "PUT"} {
				req, e := http.NewRequestWithContext(context.Background(), method, "https://"+binding.hosts[0]+path, nil)
				if e != nil {
					t.Fatal(e)
				}
				var expected bool

				switch binding.Name {
				case "docs_copy":
					expected = (method == "GET" && path == "/drive/v3/files/selected-file") || (method == "POST" && path == "/drive/v3/files/selected-file/copy")
				case "drive_mkdir":
					expected = method == "POST" && path == "/drive/v3/files"
				case "drive_share":
					expected = (method == "GET" && path == "/drive/v3/files/selected-file") || (method == "POST" && path == "/drive/v3/files/selected-file/permissions")
				case "drive_unshare":
					expected = method == "DELETE" && path == "/drive/v3/files/selected-file/permissions/perm-public"
				case "drive_bulk_remove_public", "drive_bulk_update_role":
					expected = method == "GET" && (path == "/drive/v3/files/selected-file" || path == "/drive/v3/files/selected-file/permissions")
				case "drive_raw", "drive_url":
					expected = method == "GET" && path == "/drive/v3/files/selected-file"
				case "docs_raw":
					expected = method == "GET" && path == "/v1/documents/selected-file"
				default:
					expected = (method == "GET" && path == "/v1/documents/selected-file") || (method == "POST" && path == "/v1/documents/selected-file:batchUpdate")
				}

				if e = guard(req); (e == nil) != expected {
					t.Errorf("%s %s %s %v", binding.Name, method, path, e)
				}

				req.URL.Path = strings.ReplaceAll(path, "selected-file", "foreign-file")
				if strings.Contains(path, "selected-file") {
					if e = guard(req); e == nil {
						t.Errorf("foreign path admitted %s", binding.Name)
					}
				}
			}
		}

		if strings.HasPrefix(binding.Name, "drive_bulk_") {
			method := "DELETE"
			if binding.Name == "drive_bulk_update_role" {
				method = "PATCH"
			}

			req, _ := http.NewRequestWithContext(context.Background(), method, "https://www.googleapis.com/drive/v3/files/selected-file/permissions/perm-public", nil)
			if err = guard(req); err != nil {
				t.Errorf("selected member denied %v", err)
			}

			req.URL.Path += "/extra"
			if err = guard(req); err == nil {
				t.Fatal("permission traversal admitted")
			}
		}

		for _, query := range []string{"alt=media", "alt=json&alt=media", "access_token=forged"} {
			req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://"+binding.hosts[0]+paths[0]+"?"+query, nil)
			if err = guard(req); err == nil {
				t.Errorf("%s unsafe query admitted %s", binding.Name, query)
			}
		}
	}
}

func TestNativeSedMultiImageDoesNotReplayUncertainWrite(t *testing.T) {
	posts := 0
	h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Docs = func(ctx context.Context, _ string) (*docs.Service, error) {
			base := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				status, body := 200, remainingDocFixture

				if r.Method == http.MethodPost {
					posts++
					status = 503
					body = `{"error":{"code":503,"message":"uncertain"}}`
				}

				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, googleapi.NewRetryTransport(base))}))
		}
	})
	args := map[string]any{"document_id": "selected-file", "expressions": []any{"s|abc|![alt](https://example.test/image.png)|g", "s|xyz|![other](https://example.test/other.png)|g"}}
	auth := ToolAuthorization{Service: "docs", AllowedTools: []string{"docs_sed"}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, AllowWrite: true, AllowDestructive: true, Resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "selected-file", Enabled: true}}
	body := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: "docs_sed", Arguments: args, Authorization: auth} })

	result := postExecute(t, h.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	if result.OK || posts != 1 {
		t.Fatalf("internal image retry replayed: posts=%d %s", posts, result.Body)
	}
}

func TestNativeSedStructuralExpansionAdmission(t *testing.T) {
	for _, test := range []struct {
		expression string
		allowed    bool
	}{
		{"s/abc/abc{cols=2}/", true},
		{"s/abc/abc{cols=1000000000}/", false},
		{"s/abc/prefix{cols=1000000000 t=styled}/", false},
	} {
		t.Run(test.expression, func(t *testing.T) {
			calls, columns := 0, 0
			h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Docs = func(ctx context.Context, _ string) (*docs.Service, error) {
					base := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls++
						body := `{"documentId":"selected-file","revisionId":"rev-1","body":{"content":[{"startIndex":1,"endIndex":5,"paragraph":{"elements":[{"startIndex":1,"endIndex":5,"textRun":{"content":"abc\n"}}]}}]}}`

						if r.Method == http.MethodPost {
							var batch docs.BatchUpdateDocumentRequest
							if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
								t.Fatal(err)
							}

							for _, request := range batch.Requests {
								if request.UpdateSectionStyle != nil {
									columns = len(request.UpdateSectionStyle.SectionStyle.ColumnProperties)
								}
							}
							body = `{"documentId":"selected-file","replies":[{}]}`
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})

					return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, base)}))
				}
			})
			auth := ToolAuthorization{Service: "docs", AllowedTools: []string{"docs_sed"}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, AllowWrite: true, AllowDestructive: true, Resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "selected-file", Enabled: true}}
			body := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{Name: "docs_sed", Arguments: map[string]any{"document_id": "selected-file", "expressions": []any{test.expression}}, Authorization: auth}
			})

			result := postExecute(t, h.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			if test.allowed {
				if !result.OK || columns != 2 {
					t.Fatalf("valid structural edit failed: columns=%d %s", columns, result.Body)
				}
			} else if result.OK || calls != 0 {
				t.Fatalf("oversized structural expansion reached native execution: calls=%d %s", calls, result.Body)
			}
		})
	}
}
