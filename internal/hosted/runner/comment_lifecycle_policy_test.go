package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeCommentLifecycleExecution(t *testing.T) {
	for _, binding := range nativeCommentLifecycleBindings() {
		t.Run(binding.Name, func(t *testing.T) {
			calls, writes := 0, 0
			failWrite := false
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				transport := func(ctx context.Context) http.RoundTripper {
					base := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls++
						body := `{"id":"comment-1","content":"abc","quotedFileContent":{"value":"abc"},"action":"resolve"}`
						status := 200

						if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/comments") {
							body = `{"comments":[{"id":"comment-1","content":"abc"}],"nextPageToken":"next"}`
						}

						if strings.HasPrefix(r.URL.Path, "/v1/documents/") {
							body = `{"documentId":"selected-file","body":{"content":[{"startIndex":1,"endIndex":5,"paragraph":{"elements":[{"startIndex":1,"endIndex":5,"textRun":{"content":"abc\n"}}]}}]}}`
						}

						if r.Method != http.MethodGet {
							writes++

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
				engine.NativeServices.Drive = func(ctx context.Context, _ string) (*drive.Service, error) {
					return drive.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: transport(ctx)}))
				}
				engine.NativeServices.Docs = func(ctx context.Context, _ string) (*docs.Service, error) {
					return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: transport(ctx)}))
				}
			})
			args := map[string]any{binding.ResourceArgument: "selected-file"}

			verb := strings.TrimPrefix(binding.Name, binding.Service+"_comments_")
			if verb != "add" && verb != "create" && verb != "list" {
				args["comment_id"] = "comment-1"
			}

			if verb == "add" || verb == "create" || verb == "update" || verb == "reply" {
				args["content"] = "literal content"
			}
			call := func(change func(*ToolAuthorization)) capturedResponse {
				auth := ToolAuthorization{Service: binding.Service, AllowedTools: []string{binding.Name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, AllowWrite: true, AllowDestructive: true, Resource: &ResourceGrantInput{Service: binding.Service, ResourceType: "file", ResourceID: "selected-file", Enabled: true}}
				if change != nil {
					change(&auth)
				}
				data := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{Name: binding.Name, Arguments: args, Authorization: auth}
				})

				return postExecute(t, harness.server, signInvocation(t, data, func(claims map[string]any) { claims["operation"] = OperationToolCall }), data)
			}

			result := call(nil)
			if !result.OK || calls == 0 {
				t.Fatalf("%s calls %d", result.Body, calls)
			}
			before := calls

			changes := []func(*ToolAuthorization){func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-file" }, func(a *ToolAuthorization) { a.AllowedTools = []string{"drive_comments_get"} }, func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/documents"} }}
			if binding.Write {
				changes = append(changes, func(a *ToolAuthorization) { a.AllowWrite = false })
			}

			if binding.destructive {
				changes = append(changes, func(a *ToolAuthorization) { a.AllowDestructive = false })
			}

			for _, change := range changes {
				if denied := call(change); denied.OK {
					t.Fatal("unauthorized invocation admitted")
				}
			}

			if calls != before {
				t.Fatal("denied call reached Google")
			}

			if binding.Write {
				beforeWrites := writes
				failWrite = true

				if failed := call(nil); failed.OK || writes != beforeWrites+1 {
					t.Fatalf("uncertain mutation replayed: %d -> %d", beforeWrites, writes)
				}
			}
		})
	}
}

func TestCommentLifecycleHTTPGuard(t *testing.T) {
	for _, binding := range nativeCommentLifecycleBindings() {
		args := map[string]any{binding.ResourceArgument: "selected-file", "comment_id": "comment-1"}
		guard := nativeRequestGuard(&ToolInvocation{Name: binding.Name, Arguments: args, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-file"}}})

		paths, err := binding.argumentPaths(args)
		if err != nil {
			t.Fatal(err)
		}

		for _, path := range paths {
			host := nativeDocsSlidesDriveHost
			if strings.HasPrefix(path, "/v1/documents/") {
				host = "docs.googleapis.com"
			}

			good := "https://" + host + path

			if binding.Name == "docs_comments_locate" {
				wrongHost := "docs.googleapis.com"
				if host == wrongHost {
					wrongHost = nativeDocsSlidesDriveHost
				}

				crossed, crossErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+wrongHost+path, nil)
				if crossErr != nil {
					t.Fatal(crossErr)
				}

				if crossErr = guard(crossed); crossErr == nil {
					t.Fatal("locate admitted host/path cross product")
				}
			}

			for _, tc := range []struct {
				method, url string
				allow       bool
			}{{binding.methods[0], good, true}, {binding.methods[0], strings.Replace(good, "selected-file", "foreign-file", 1), false}, {"PUT", good, false}, {binding.methods[0], "https://foreign.example" + path, false}} {
				request, e := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
				if e != nil {
					t.Fatal(e)
				}

				if e = guard(request); (e == nil) != tc.allow {
					t.Errorf("%s %s %s: %v", binding.Name, tc.method, tc.url, e)
				}
			}
		}
	}
}
