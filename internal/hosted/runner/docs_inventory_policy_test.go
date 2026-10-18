package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	docs "google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeDocsInfoAndListTabsSignedExecution(t *testing.T) {
	const fixtureBody = `{"documentId":"doc-123","title":"Selected document","revisionId":"r1","tabs":[{"tabProperties":{"tabId":"tab-1","title":"Overview","index":0}}]}`

	tests := []struct {
		name       string
		tool       string
		args       map[string]any
		checkQuery func(t *testing.T, r *http.Request)
		needles    []string
	}{
		{
			name: "docs_info",
			tool: "docs_info",
			args: map[string]any{"document_id": "doc-123"},
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				if got := r.URL.Query().Get("fields"); got != "documentId,title,revisionId" {
					t.Errorf("native fields: got %q want %q", got, "documentId,title,revisionId")
				}
			},
			needles: []string{`"documentId":"doc-123"`, `"title":"Selected document"`, `"revisionId":"r1"`},
		},
		{
			name: "docs_list_tabs",
			tool: "docs_list_tabs",
			args: map[string]any{"document_id": "doc-123"},
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				switch got := r.URL.Query().Get("includeTabsContent"); got {
				case "true", "True":
				default:
					t.Errorf("native includeTabsContent: got %q want true", got)
				}
			},
			needles: []string{`"id":"tab-1"`, `"title":"Overview"`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64

			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Docs = func(ctx context.Context, account string) (*docs.Service, error) {
					if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted docs execution identity was lost")
					}

					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)

						if r.Method != http.MethodGet || r.URL.Hostname() != "docs.googleapis.com" || r.URL.Path != "/v1/documents/doc-123" {
							t.Errorf("foreign or mutating docs request reached Google: %s %s", r.Method, r.URL)
						}

						test.checkQuery(t, r)

						return &http.Response{
							StatusCode: 200,
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body:       io.NopCloser(strings.NewReader(fixtureBody)),
							Request:    r,
						}, nil
					})

					return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			call := func(tool string, scope string, args map[string]any, resource *ResourceGrantInput, allowedTool string) capturedResponse {
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{
						Name: tool, Arguments: args,
						Authorization: ToolAuthorization{
							Service: "docs", AllowedTools: []string{allowedTool},
							Scopes:   []string{"https://www.googleapis.com/auth/" + scope},
							Resource: resource,
						},
					}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) {
					claims["operation"] = OperationToolCall
				}), body)
			}

			resource := &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "doc-123", Enabled: true}
			scopes := []string{"documents.readonly", "documents", "drive.file"}

			for _, scope := range scopes {
				ok := call(test.tool, scope, test.args, resource, test.tool)
				if !ok.OK {
					t.Fatalf("%s under %s failed: %s", test.tool, scope, ok.Body)
				}

				for _, needle := range test.needles {
					if !strings.Contains(string(ok.Result), needle) {
						t.Fatalf("%s result missing %q: %s", test.tool, needle, ok.Result)
					}
				}
			}

			before := calls.Load()

			for _, denial := range []struct {
				name     string
				args     map[string]any
				resource *ResourceGrantInput
				scope    string
				tool     string
			}{
				{name: "missing resource", args: test.args, resource: nil, scope: "documents.readonly", tool: test.tool},
				{name: "disabled resource", args: test.args, resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "doc-123", Enabled: false}, scope: "documents.readonly", tool: test.tool},
				{name: "foreign document id", args: map[string]any{"document_id": "doc-999"}, resource: resource, scope: "documents.readonly", tool: test.tool},
				{name: "wrong tool grant", args: test.args, resource: resource, scope: "documents.readonly", tool: "docs_get"},
				{name: "wrong scope", args: test.args, resource: resource, scope: "presentations.readonly", tool: test.tool},
				{name: "unsupported arguments", args: map[string]any{"document_id": "doc-123", "unsupported": true}, resource: resource, scope: "documents.readonly", tool: test.tool},
			} {
				t.Run("denied/"+denial.name, func(t *testing.T) {
					denied := call(test.tool, denial.scope, denial.args, denial.resource, denial.tool)
					if denied.OK || calls.Load() != before {
						t.Fatalf("%s reached Google: %s", denial.name, denied.Body)
					}
				})
			}

			foreignTenant := nativeToolBody(t, func(req *Request) {
				req.TenantID = "tenant-b"
				req.Tool = &ToolInvocation{
					Name: test.tool, Arguments: test.args,
					Authorization: ToolAuthorization{
						Service: "docs", AllowedTools: []string{test.tool},
						Scopes:   []string{"https://www.googleapis.com/auth/documents.readonly"},
						Resource: resource,
					},
				}
			})

			foreign := postExecute(t, harness.server, signInvocation(t, foreignTenant, func(claims map[string]any) {
				claims["operation"] = OperationToolCall
			}), foreignTenant)
			if foreign.OK || foreign.Status != http.StatusForbidden || calls.Load() != before {
				t.Fatal("foreign tenant reached docs execution")
			}
		})
	}
}
