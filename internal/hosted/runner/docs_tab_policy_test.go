package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	docsapi "google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var errDocsTabFixture = errors.New("unexpected Docs tab fixture request")

func TestNativeDocsTabsPersistSelectedHierarchy(t *testing.T) {
	for _, scope := range []string{"documents", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			props := map[string]*docsapi.TabProperties{"t1": {TabId: "t1", Title: "Root"}, "t2": {TabId: "t2", Title: "Other"}}
			order := map[string][]string{"": {"t1", "t2"}}
			calls, posts := 0, 0
			var render func(string) []*docsapi.Tab
			render = func(parent string) []*docsapi.Tab {
				out := make([]*docsapi.Tab, 0, len(order[parent]))
				for i, id := range order[parent] {
					p := *props[id]
					p.Index = int64(i)
					out = append(out, &docsapi.Tab{TabProperties: &p, ChildTabs: render(id)})
				}

				return out
			}
			h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Docs = func(ctx context.Context, account string) (*docsapi.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted tab identity lost")
					}
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls++

						if r.URL.Hostname() != "docs.googleapis.com" {
							return nil, errDocsTabFixture
						}
						var response any

						switch {
						case r.Method == http.MethodGet && r.URL.Path == "/v1/documents/selected-doc":
							if r.URL.Query().Get("includeTabsContent") != "true" {
								t.Error("tab resolution omitted tab content")
							}
							response = &docsapi.Document{DocumentId: "selected-doc", Tabs: render("")}
						case r.Method == http.MethodPost && r.URL.Path == "/v1/documents/selected-doc:batchUpdate":
							body, err := io.ReadAll(r.Body)
							if err != nil {
								return nil, fmt.Errorf("read tab body: %w", err)
							}

							var raw struct {
								Requests []map[string]json.RawMessage `json:"requests"`
							}
							if err := json.Unmarshal(body, &raw); err != nil {
								return nil, fmt.Errorf("decode tab wire: %w", err)
							}

							var batch docsapi.BatchUpdateDocumentRequest
							if err := json.Unmarshal(body, &batch); err != nil {
								return nil, fmt.Errorf("decode tab SDK: %w", err)
							}

							if len(batch.Requests) != 1 || len(raw.Requests) != 1 || len(raw.Requests[0]) != 1 {
								return nil, errDocsTabFixture
							}
							request := batch.Requests[0]
							replies := []any{map[string]any{}}

							switch {
							case request.AddDocumentTab != nil:
								p := request.AddDocumentTab.TabProperties
								if p == nil {
									return nil, errDocsTabFixture
								}

								parent := p.ParentTabId
								if parent != "" && props[parent] == nil {
									return nil, errDocsTabFixture
								}

								index := int(p.Index)
								if index < 0 || index > len(order[parent]) {
									return nil, errDocsTabFixture
								}

								if index == 0 && !strings.Contains(string(raw.Requests[0]["addDocumentTab"]), `"index":0`) {
									t.Error("zero insertion omitted")
									return nil, errDocsTabFixture
								}
								id := fmt.Sprintf("new-%d", posts)
								copyProps := *p
								copyProps.TabId = id
								props[id] = &copyProps

								ids := append([]string{}, order[parent]...)
								ids = append(ids, "")
								copy(ids[index+1:], ids[index:])
								ids[index] = id
								order[parent] = ids
								replies = []any{map[string]any{"addDocumentTab": map[string]any{"tabProperties": &copyProps}}}
							case request.UpdateDocumentTabProperties != nil:
								v := request.UpdateDocumentTabProperties
								if v.Fields != "title" || v.TabProperties == nil || props[v.TabProperties.TabId] == nil {
									return nil, errDocsTabFixture
								}
								props[v.TabProperties.TabId].Title = v.TabProperties.Title
							case request.DeleteTab != nil:
								id := request.DeleteTab.TabId

								p := props[id]
								if p == nil {
									return nil, errDocsTabFixture
								}

								index := slices.Index(order[p.ParentTabId], id)
								if index < 0 {
									return nil, errDocsTabFixture
								}
								order[p.ParentTabId] = append(append([]string{}, order[p.ParentTabId][:index]...), order[p.ParentTabId][index+1:]...)

								delete(props, id)
							default:
								return nil, errDocsTabFixture
							}

							posts++
							response = map[string]any{"documentId": "selected-doc", "replies": replies}
						default:
							return nil, errDocsTabFixture
						}

						body, err := json.Marshal(response)
						if err != nil {
							return nil, fmt.Errorf("marshal tab fixture: %w", err)
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
					})

					return docsapi.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})
			call := func(name string, args map[string]any, destructive bool, change func(*ToolAuthorization)) capturedResponse {
				a := ToolAuthorization{Service: "docs", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive, Resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "selected-doc", Enabled: true}}
				if change != nil {
					change(&a)
				}
				body := nativeToolBody(t, func(r *Request) { r.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: a} })

				return postExecute(t, h.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
			}
			good := func(name string, args map[string]any, destructive bool) {
				t.Helper()

				if r := call(name, args, destructive, nil); !r.OK {
					t.Fatal(r.Body)
				}
			}
			good("docs_add_tab", map[string]any{"document_id": "selected-doc", "title": "Added", "index": 0, "icon_emoji": "🚀"}, false)

			if !slices.Equal(order[""], []string{"new-0", "t1", "t2"}) || props["new-0"].IconEmoji != "🚀" || props["t1"].Title != "Root" {
				t.Fatal("root insertion failed or changed old tabs")
			}

			good("docs_add_tab", map[string]any{"document_id": "selected-doc", "title": "Nested", "index": 0, "parent_tab": "Root", "icon_emoji": "📌"}, false)

			if !slices.Equal(order["t1"], []string{"new-1"}) || props["new-1"].ParentTabId != "t1" {
				t.Fatal("parent resolution failed")
			}

			good("docs_rename_tab", map[string]any{"document_id": "selected-doc", "tab": "Nested", "title": "Child renamed"}, false)

			if props["new-1"].Title != "Child renamed" || props["new-1"].IconEmoji != "📌" || props["t1"].Title != "Root" || props["t2"].Title != "Other" {
				t.Fatal("rename changed other properties/tabs")
			}

			before := calls
			if r := call("docs_delete_tab", map[string]any{"document_id": "selected-doc", "tab": "Child renamed"}, false, nil); r.OK || calls != before {
				t.Fatal("delete without destructive grant reached Google")
			}

			good("docs_delete_tab", map[string]any{"document_id": "selected-doc", "tab": "Child renamed"}, true)

			if len(order["t1"]) != 0 || props["new-1"] != nil || props["t1"] == nil || props["t2"] == nil {
				t.Fatal("child delete changed parent/sibling")
			}

			good("docs_delete_tab", map[string]any{"document_id": "selected-doc", "tab": "Added"}, true)

			if !slices.Equal(order[""], []string{"t1", "t2"}) || props["t2"].Title != "Other" || posts != 5 {
				t.Fatal("delete changed other root tabs")
			}
			before = calls

			denials := []func(*ToolAuthorization){
				func(a *ToolAuthorization) { a.AllowWrite = false },
				func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/documents.readonly"} },
				func(a *ToolAuthorization) { a.Resource = nil },
				func(a *ToolAuthorization) { a.Resource.Enabled = false },
				func(a *ToolAuthorization) { a.AllowedTools = []string{"docs_get"} },
				func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-doc" },
			}

			for name, args := range map[string]map[string]any{
				"docs_add_tab":    {"document_id": "selected-doc", "title": "Denied", "index": 0},
				"docs_rename_tab": {"document_id": "selected-doc", "tab": "Root", "title": "Denied"},
				"docs_delete_tab": {"document_id": "selected-doc", "tab": "Root"},
			} {
				for _, change := range denials {
					if r := call(name, args, true, change); r.OK || calls != before {
						t.Fatalf("unauthorized tab mutation reached Google: %s", r.Body)
					}
				}
			}
		})
	}
}

func TestDocsTabMethodAndResourceGuard(t *testing.T) {
	for _, name := range []string{"docs_add_tab", "docs_rename_tab", "docs_delete_tab"} {
		t.Run(name, func(t *testing.T) {
			inv := ToolInvocation{Name: name, Arguments: map[string]any{"document_id": "selected-doc"}, Authorization: ToolAuthorization{Service: "docs", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/documents"}, AllowWrite: true, AllowDestructive: true, Resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "selected-doc", Enabled: true}}}
			guard := nativeRequestGuard(&inv)

			root := "https://docs.googleapis.com/v1/documents/selected-doc"
			for _, test := range []struct {
				method, url string
				allow       bool
			}{
				{http.MethodGet, root, true},
				{http.MethodPost, root + ":batchUpdate", true},
				{http.MethodPost, root, false},
				{http.MethodGet, root + ":batchUpdate", false},
				{http.MethodDelete, root, false},
				{http.MethodPut, root + ":batchUpdate", false},
				{http.MethodPatch, root, false},
				{http.MethodPost, "https://docs.googleapis.com/v1/documents/foreign-doc:batchUpdate", false},
				{http.MethodGet, "https://docs.googleapis.com/v1/documents/foreign-doc", false},
				{http.MethodPost, "https://example.test/v1/documents/selected-doc:batchUpdate", false},
				{http.MethodGet, root + "/tabs", false},
			} {
				req, err := http.NewRequestWithContext(context.Background(), test.method, test.url, nil)
				if err != nil {
					t.Fatal(err)
				}

				if err := guard(req); (err == nil) != test.allow {
					t.Errorf("%s %s allow=%t error=%v", test.method, test.url, test.allow, err)
				}
			}
		})
	}
}
