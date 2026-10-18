package runner

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	drive "google.golang.org/api/drive/v3"
	formsapi "google.golang.org/api/forms/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const (
	nativeDriveCommentsListFixture = `{"comments":[{"id":"comment-1","content":"Selected comment"}],"nextPageToken":"next-comments"}`
	nativeDriveCommentsPageFixture = `{"comments":[{"id":"comment-2","content":"Second"}]}`
	nativeDriveCommentsGetFixture  = `{"id":"comment-1","content":"Selected comment"}`
	nativeFormsWatchesFixture      = `{"watches":[{"id":"watch-1","eventType":"RESPONSES"}]}`
)

type commentsWatchNativeCase struct {
	name, service, tool, wrongTool, checkHost, checkPath string
	resourceArg, foreignID, resourceID                   string
	args                                                 map[string]any
	scopes                                               []string
	needles                                              []string
	paginated                                            bool
	fixtures                                             func(t *testing.T, r *http.Request) string
}

func TestNativeDriveCommentsAndFormsWatchesSignedExecution(t *testing.T) {
	tests := []commentsWatchNativeCase{
		{
			name:        "drive_comments_list",
			service:     "drive",
			tool:        "drive_comments_list",
			args:        map[string]any{"file_id": "file-123", "max": 2},
			resourceArg: "file_id",
			foreignID:   "file-999",
			resourceID:  "file-123",
			scopes:      []string{"drive.file"},
			wrongTool:   "drive_get",
			checkHost:   "www.googleapis.com",
			checkPath:   "/drive/v3/files/file-123/comments",
			paginated:   true,
			fixtures: func(t *testing.T, r *http.Request) string {
				t.Helper()

				if gotPageSize := r.URL.Query().Get("pageSize"); gotPageSize != "2" {
					t.Errorf("native pageSize: got %q want 2", gotPageSize)
				}

				switch got := r.URL.Query().Get("pageToken"); got {
				case "":
					return nativeDriveCommentsListFixture
				case "next-comments":
					return nativeDriveCommentsPageFixture
				default:
					t.Errorf("native pageToken: got %q", got)
					return nativeDriveCommentsPageFixture
				}
			},
		},
		{
			name:        "drive_comments_get",
			service:     "drive",
			tool:        "drive_comments_get",
			args:        map[string]any{"file_id": "file-123", "comment_id": "comment-1"},
			resourceArg: "file_id",
			foreignID:   "file-999",
			resourceID:  "file-123",
			scopes:      []string{"drive.file"},
			wrongTool:   "drive_get",
			checkHost:   "www.googleapis.com",
			checkPath:   "/drive/v3/files/file-123/comments/comment-1",
			needles:     []string{`"id":"comment-1"`, `"content":"Selected comment"`},
			fixtures: func(t *testing.T, _ *http.Request) string {
				t.Helper()
				return nativeDriveCommentsGetFixture
			},
		},
		{
			name:        "forms_watch_list",
			service:     "forms",
			tool:        "forms_watch_list",
			args:        map[string]any{"form_id": "form-123"},
			resourceArg: "form_id",
			foreignID:   "form-999",
			resourceID:  "form-123",
			scopes:      []string{"drive.file", "forms.body.readonly", "forms.body", "forms.responses.readonly"},
			wrongTool:   "forms_get",
			checkHost:   "forms.googleapis.com",
			checkPath:   "/v1/forms/form-123/watches",
			needles:     []string{`"id":"watch-1"`, `"eventType":"RESPONSES"`},
			fixtures: func(t *testing.T, _ *http.Request) string {
				t.Helper()
				return nativeFormsWatchesFixture
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64

			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
					calls.Add(1)

					if r.Method != http.MethodGet || r.URL.Hostname() != test.checkHost || r.URL.Path != test.checkPath {
						t.Errorf("foreign or mutating native request reached Google: %s %s", r.Method, r.URL)
					}
					body := test.fixtures(t, r)

					return &http.Response{
						StatusCode: 200,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(body)),
						Request:    r,
					}, nil
				})

				if test.service == "drive" {
					engine.NativeServices.Drive = func(ctx context.Context, account string) (*drive.Service, error) {
						if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
							t.Error("trusted drive execution identity was lost")
						}

						return drive.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
					}

					return
				}
				engine.NativeServices.Forms = func(ctx context.Context, account string) (*formsapi.Service, error) {
					if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
						t.Error("trusted forms execution identity was lost")
					}

					return formsapi.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			call := func(tool string, scope string, args map[string]any, resource *ResourceGrantInput, allowedTool string) capturedResponse {
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{
						Name: tool, Arguments: args,
						Authorization: ToolAuthorization{
							Service: test.service, AllowedTools: []string{allowedTool},
							Scopes:   []string{"https://www.googleapis.com/auth/" + scope},
							Resource: resource,
						},
					}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) {
					claims["operation"] = OperationToolCall
				}), body)
			}

			resource := &ResourceGrantInput{Service: test.service, ResourceType: "file", ResourceID: test.resourceID, Enabled: true}

			for _, scope := range test.scopes {
				if test.paginated {
					first := call(test.tool, scope, test.args, resource, test.tool)
					if !first.OK {
						t.Fatalf("%s first page under %s failed: %s", test.tool, scope, first.Body)
					}

					var firstPage struct {
						Comments      []struct{ ID, Content string } `json:"comments"`
						NextPageToken string                         `json:"nextPageToken"`
					}
					if err := json.Unmarshal(first.Result, &firstPage); err != nil {
						t.Fatalf("parse first page: %v", err)
					}

					if len(firstPage.Comments) != 1 || firstPage.Comments[0].ID != "comment-1" || firstPage.Comments[0].Content != "Selected comment" {
						t.Fatalf("first page comments wrong: %s", first.Result)
					}

					if firstPage.NextPageToken != "next-comments" {
						t.Fatalf("first page cursor: got %q want next-comments", firstPage.NextPageToken)
					}
					cursorArgs := maps.Clone(test.args)
					cursorArgs["page"] = firstPage.NextPageToken

					second := call(test.tool, scope, cursorArgs, resource, test.tool)
					if !second.OK {
						t.Fatalf("%s cursor page under %s failed: %s", test.tool, scope, second.Body)
					}

					var secondPage struct {
						Comments      []struct{ ID, Content string } `json:"comments"`
						NextPageToken string                         `json:"nextPageToken"`
					}
					if err := json.Unmarshal(second.Result, &secondPage); err != nil {
						t.Fatalf("parse cursor page: %v", err)
					}

					if len(secondPage.Comments) != 1 || secondPage.Comments[0].ID != "comment-2" || secondPage.Comments[0].Content != "Second" {
						t.Fatalf("cursor page comments wrong: %s", second.Result)
					}

					if secondPage.NextPageToken != "" {
						t.Fatalf("terminal cursor not empty: %s", second.Result)
					}

					continue
				}

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
				{name: "missing resource", args: test.args, resource: nil, scope: test.scopes[0], tool: test.tool},
				{name: "disabled resource", args: test.args, resource: &ResourceGrantInput{Service: test.service, ResourceType: "file", ResourceID: test.resourceID, Enabled: false}, scope: test.scopes[0], tool: test.tool},
				{name: "mismatched resource id", args: func() map[string]any {
					m := maps.Clone(test.args)
					m[test.resourceArg] = test.foreignID

					return m
				}(), resource: resource, scope: test.scopes[0], tool: test.tool},
				{name: "wrong tool grant", args: test.args, resource: resource, scope: test.scopes[0], tool: test.wrongTool},
				{name: "wrong scope", args: test.args, resource: resource, scope: "documents.readonly", tool: test.tool},
				{name: "unsupported arguments", args: func() map[string]any {
					m := maps.Clone(test.args)
					m["all"] = true

					return m
				}(), resource: resource, scope: test.scopes[0], tool: test.tool},
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
						Service: test.service, AllowedTools: []string{test.tool},
						Scopes:   []string{"https://www.googleapis.com/auth/" + test.scopes[0]},
						Resource: resource,
					},
				}
			})

			foreign := postExecute(t, harness.server, signInvocation(t, foreignTenant, func(claims map[string]any) {
				claims["operation"] = OperationToolCall
			}), foreignTenant)
			if foreign.OK || foreign.Status != http.StatusForbidden || calls.Load() != before {
				t.Fatal("foreign tenant reached native execution")
			}
		})
	}
}
