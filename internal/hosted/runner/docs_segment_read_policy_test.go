package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	docs "google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeDocsSegmentAndSuggestionReads(t *testing.T) {
	text := func(content, id string) []*docs.StructuralElement {
		return []*docs.StructuralElement{{StartIndex: 1, EndIndex: 5, Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{StartIndex: 1, EndIndex: 5, TextRun: &docs.TextRun{Content: content, SuggestedInsertionIds: []string{id}}}}}}}
	}
	document := &docs.Document{DocumentId: "selected-doc", Body: &docs.Body{Content: text("base", "s-base")}, Headers: map[string]docs.Header{"h-legacy": {Content: text("head", "s-head")}}, Footers: map[string]docs.Footer{"f-legacy": {Content: text("foot", "s-foot")}}, Tabs: []*docs.Tab{
		{TabProperties: &docs.TabProperties{TabId: "tab-a", Title: "First"}, DocumentTab: &docs.DocumentTab{Headers: map[string]docs.Header{"h-a": {Content: text("aaaa", "s-a")}}, Footers: map[string]docs.Footer{"f-a": {Content: text("aaaa", "s-a")}}}},
		{TabProperties: &docs.TabProperties{TabId: "tab-b", Title: "Tab 👩‍💻"}, DocumentTab: &docs.DocumentTab{Body: &docs.Body{Content: text("body", "s-body-b")}, Headers: map[string]docs.Header{"h-b": {Content: text("bbbb", "s-h-b")}}, Footers: map[string]docs.Footer{"f-b": {Content: text("bbbb", "s-f-b")}}}, ChildTabs: []*docs.Tab{{TabProperties: &docs.TabProperties{TabId: "tab-c", Title: "Child"}, DocumentTab: &docs.DocumentTab{Headers: map[string]docs.Header{"h-c": {Content: text("cccc", "s-c")}}, Footers: map[string]docs.Footer{"f-c": {Content: text("cccc", "s-c")}}}}}},
	}}
	document.Body.Content[0].Paragraph.Elements[0].TextRun.SuggestedDeletionIds = []string{"d-base"}
	document.Tabs[1].DocumentTab.Body.Content[0].Paragraph.Elements[0].TextRun.SuggestedDeletionIds = []string{"d-body-b"}

	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	for _, scope := range []string{"documents.readonly", "documents", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			calls := 0
			expectedTool, expectedTab := "", ""
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Docs = func(ctx context.Context, account string) (*docs.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted Docs identity lost")
					}
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls++

						if r.Method != http.MethodGet || r.URL.Hostname() != "docs.googleapis.com" || r.URL.Path != "/v1/documents/selected-doc" {
							t.Errorf("foreign/mutating request %s %s", r.Method, r.URL)
						}

						q := r.URL.Query()
						if expectedTool == "docs_suggestions_list" {
							if q.Get("suggestionsViewMode") != "SUGGESTIONS_INLINE" || (q.Get("includeTabsContent") == "true") != (expectedTab != "") {
								t.Errorf("wrong suggestions view %s", r.URL)
							}
						} else if q.Get("includeTabsContent") != "true" {
							t.Errorf("segment listing omitted tabs %s", r.URL)
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded))), Request: r}, nil
					})

					return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			call := func(name, tab string, change func(*ToolAuthorization)) capturedResponse {
				expectedTool, expectedTab = name, tab

				authorization := ToolAuthorization{Service: "docs", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, Resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "selected-doc", Enabled: true}}
				if change != nil {
					change(&authorization)
				}

				args := map[string]any{"document_id": "selected-doc"}
				if tab != "" {
					args["tab"] = tab
				}
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: authorization}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			}
			for _, kind := range []string{"header", "footer"} {
				name := "docs_" + kind + "_list"

				prefix := "h-"
				if kind == "footer" {
					prefix = "f-"
				}

				for _, tab := range []string{"", "tab-b", "Tab 👩‍💻"} {
					before := calls

					result := call(name, tab, nil)
					if !result.OK || calls != before+1 {
						t.Fatalf("%s tab %q: %s calls=%d", name, tab, result.Body, calls)
					}

					var decoded map[string]json.RawMessage
					if err := json.Unmarshal(result.Result, &decoded); err != nil {
						t.Fatal(err)
					}

					var items []struct {
						ID     string `json:"id"`
						TabID  string `json:"tabId"`
						Length int64  `json:"length"`
					}
					if err := json.Unmarshal(decoded[kind+"s"], &items); err != nil {
						t.Fatal(err)
					}

					want := 4
					if tab != "" {
						want = 1
					}

					if len(items) != want {
						t.Fatalf("wrong %s subset: %s", kind, result.Result)
					}

					ids := map[string]bool{}
					for _, item := range items {
						ids[item.ID] = true
						if item.Length != 5 {
							t.Fatalf("wrong segment length %s", result.Result)
						}
					}

					if tab != "" && (items[0].ID != prefix+"b" || items[0].TabID != "tab-b") {
						t.Fatalf("wrong selected tab %s", result.Result)
					}

					if tab == "" && (!ids[prefix+"a"] || !ids[prefix+"b"] || !ids[prefix+"c"] || !ids[prefix+"legacy"]) {
						t.Fatalf("missing all-tab/legacy segments %s", result.Result)
					}
				}
			}

			for _, tab := range []string{"", "tab-b", "Tab 👩‍💻"} {
				result := call("docs_suggestions_list", tab, nil)
				if !result.OK {
					t.Fatal(result.Body)
				}

				var decoded struct {
					TabID       string `json:"tabId"`
					Suggestions []struct {
						ID      string `json:"suggestionId"`
						Kind    string `json:"kind"`
						Segment string `json:"segment"`
						Text    string `json:"text"`
					} `json:"suggestions"`
				}
				if err := json.Unmarshal(result.Result, &decoded); err != nil {
					t.Fatal(err)
				}

				ids := map[string]bool{}
				for _, item := range decoded.Suggestions {
					ids[item.ID] = true
					if item.Kind != "insertion" && item.Kind != "deletion" {
						t.Errorf("wrong suggestion kind %s", result.Result)
					}
				}

				if len(ids) != 4 || (tab == "" && (!ids["s-base"] || !ids["d-base"] || !ids["s-head"] || !ids["s-foot"])) || (tab != "" && (decoded.TabID != "tab-b" || !ids["s-body-b"] || !ids["d-body-b"] || !ids["s-h-b"] || !ids["s-f-b"])) {
					t.Fatalf("wrong suggestion projection %s", result.Result)
				}
			}

			denials := []func(*ToolAuthorization){func(a *ToolAuthorization) { a.Resource = nil }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-doc" }, func(a *ToolAuthorization) { a.AllowedTools = []string{"docs_get"} }, func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/drive.readonly"} }}
			for _, name := range []string{"docs_header_list", "docs_footer_list", "docs_suggestions_list"} {
				for _, change := range denials {
					before := calls
					if result := call(name, "", change); result.OK || calls != before {
						t.Fatalf("denied %s reached Google: %s", name, result.Body)
					}
				}
			}
		})
	}
}

func TestDocsSegmentReadExactGuard(t *testing.T) {
	for _, name := range []string{"docs_header_list", "docs_footer_list", "docs_suggestions_list"} {
		invocation := &ToolInvocation{Name: name, Arguments: map[string]any{"document_id": "selected-doc"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-doc"}}}
		guard := nativeRequestGuard(invocation)

		root := "https://docs.googleapis.com/v1/documents/selected-doc"
		for _, test := range []struct {
			method, url string
			allowed     bool
		}{{http.MethodGet, root, true}, {http.MethodHead, root, false}, {http.MethodPost, root, false}, {http.MethodGet, root + ":batchUpdate", false}, {http.MethodPost, root + ":batchUpdate", false}, {http.MethodGet, "https://docs.googleapis.com/v1/documents/foreign-doc", false}, {http.MethodGet, "https://foreign.test/v1/documents/selected-doc", false}} {
			request, err := http.NewRequestWithContext(context.Background(), test.method, test.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err := guard(request); (err == nil) != test.allowed {
				t.Errorf("%s %s allowed=%t err=%v", test.method, test.url, test.allowed, err)
			}
		}
	}
}
