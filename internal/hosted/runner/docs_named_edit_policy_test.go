package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"

	docs "google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var errDocsNamedEditFixture = errors.New("unexpected Docs named-range fixture request")

func callDocsNamedEdit(t *testing.T, h *testHarness, name string, args map[string]any, scope string, write, destructive bool, change func(*ToolAuthorization)) capturedResponse {
	t.Helper()

	authorization := ToolAuthorization{Service: "docs", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: write, AllowDestructive: destructive, Resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "selected-doc", Enabled: true}}
	if change != nil {
		change(&authorization)
	}
	body := nativeToolBody(t, func(req *Request) {
		req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: authorization}
	})

	return postExecute(t, h.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
}

func TestNativeDocsNamedRangeLifecycle(t *testing.T) {
	for _, scope := range []string{"documents", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			texts := map[string]string{"tab-w": "Hello stable 😀 anchor\n", "tab-r": "😀 anchor plus 😀 anchor\n"}
			ranges := map[string]map[string]*docs.NamedRange{"tab-w": {}, "tab-r": {}}

			ranges["tab-w"]["nr-stable"] = &docs.NamedRange{NamedRangeId: "nr-stable", Name: "stable", Ranges: []*docs.Range{{TabId: "tab-w", StartIndex: 7, EndIndex: 13}}}
			for _, id := range []string{"dup-a", "dup-b"} {
				ranges["tab-w"][id] = &docs.NamedRange{NamedRangeId: id, Name: "Dup", Ranges: []*docs.Range{{TabId: "tab-w", StartIndex: 1, EndIndex: 6}}}
			}

			for _, tab := range []string{"tab-w", "tab-r"} {
				ranges[tab]["shared"] = &docs.NamedRange{NamedRangeId: "shared", Name: "Shared", Ranges: []*docs.Range{{TabId: tab, StartIndex: 1, EndIndex: 3}}}
			}
			revision, created := 1, 0
			var events []string
			var want docsNamedEditExpectation
			conflict := false
			content := func(text string, start int64) *docs.Body {
				end := start + int64(len(utf16.Encode([]rune(text))))
				return &docs.Body{Content: []*docs.StructuralElement{{StartIndex: start, EndIndex: end, Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{StartIndex: start, EndIndex: end, TextRun: &docs.TextRun{Content: text}}}}}}}
			}
			groups := func(tab string) map[string]docs.NamedRanges {
				result := map[string]docs.NamedRanges{}

				ids := make([]string, 0, len(ranges[tab]))
				for id := range ranges[tab] {
					ids = append(ids, id)
				}

				sort.Strings(ids)

				for _, id := range ids {
					value := ranges[tab][id]
					group := result[value.Name]
					group.Name = value.Name
					group.NamedRanges = append(group.NamedRanges, value)
					result[value.Name] = group
				}

				return result
			}
			document := func() *docs.Document {
				work := &docs.DocumentTab{Body: content(texts["tab-w"], 1), NamedRanges: groups("tab-w"), Headers: map[string]docs.Header{"header-1": {Content: content("Header   Alpha\n", 0).Content}}}
				repeat := &docs.DocumentTab{Body: content(texts["tab-r"], 1), NamedRanges: groups("tab-r")}

				return &docs.Document{DocumentId: "selected-doc", RevisionId: fmt.Sprintf("rev-%d", revision), Body: work.Body, NamedRanges: work.NamedRanges, Tabs: []*docs.Tab{{TabProperties: &docs.TabProperties{TabId: "tab-w", Title: "Work 👩‍💻"}, DocumentTab: work}, {TabProperties: &docs.TabProperties{TabId: "tab-r", Title: "Repeat"}, DocumentTab: repeat}}}
			}
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Docs = func(ctx context.Context, account string) (*docs.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted Docs named-range identity lost")
					}
					transport := nativeFixtureTransport(func(request *http.Request) (*http.Response, error) {
						events = append(events, request.Method)
						if request.URL.Hostname() != "docs.googleapis.com" {
							return nil, errDocsNamedEditFixture
						}
						responseCode := 200
						var response any

						switch {
						case request.Method == http.MethodGet && request.URL.Path == "/v1/documents/selected-doc":
							response = document()
						case request.Method == http.MethodPost && request.URL.Path == "/v1/documents/selected-doc:batchUpdate":
							rawBody, err := io.ReadAll(request.Body)
							if err != nil {
								return nil, fmt.Errorf("read Docs named body: %w", err)
							}
							var batch docs.BatchUpdateDocumentRequest
							var envelope struct {
								Requests []map[string]json.RawMessage `json:"requests"`
							}

							if err := json.Unmarshal(rawBody, &batch); err != nil {
								return nil, fmt.Errorf("decode Docs named SDK: %w", err)
							}

							if err := json.Unmarshal(rawBody, &envelope); err != nil {
								return nil, fmt.Errorf("decode Docs named operations: %w", err)
							}

							if len(envelope.Requests) != 1 || len(envelope.Requests[0]) != 1 {
								return nil, errDocsNamedEditFixture
							}

							checkDocsNamedEditSDKRequest(t, &batch, want)

							if conflict {
								revision++
								responseCode = 409
								response = map[string]any{"error": map[string]any{"code": 409, "message": "revision mismatch", "status": "ABORTED"}}

								break
							}
							operation := batch.Requests[0]
							replies := []any{map[string]any{}}

							switch {
							case operation.CreateNamedRange != nil:
								created++
								id := fmt.Sprintf("nr-%d", created)
								value := operation.CreateNamedRange
								span := *value.Range

								tab := span.TabId
								if tab == "" {
									tab = "tab-w"
								}
								span.TabId = tab
								ranges[tab][id] = &docs.NamedRange{NamedRangeId: id, Name: value.Name, Ranges: []*docs.Range{&span}}
								replies = []any{map[string]any{"createNamedRange": map[string]any{"namedRangeId": id}}}
							case operation.DeleteNamedRange != nil:
								tab := "tab-w"
								if value := operation.DeleteNamedRange.TabsCriteria; value != nil && len(value.TabIds) == 1 {
									tab = value.TabIds[0]
								}

								delete(ranges[tab], operation.DeleteNamedRange.NamedRangeId)
							case operation.ReplaceNamedRangeContent != nil:
								value := operation.ReplaceNamedRangeContent

								tab := "tab-w"
								if value.TabsCriteria != nil && len(value.TabsCriteria.TabIds) == 1 {
									tab = value.TabsCriteria.TabIds[0]
								}

								stored := ranges[tab][value.NamedRangeId]
								if stored == nil || len(stored.Ranges) != 1 {
									return nil, errDocsNamedEditFixture
								}
								span := stored.Ranges[0]
								units := utf16.Encode([]rune(texts[tab]))
								start, end := int(span.StartIndex-1), int(span.EndIndex-1)
								replacement := utf16.Encode([]rune(value.Text))
								delta := int64(len(replacement)) - (span.EndIndex - span.StartIndex)
								merged := append(append(append([]uint16{}, units[:start]...), replacement...), units[end:]...)
								texts[tab] = string(utf16.Decode(merged))
								oldEnd := span.EndIndex
								span.EndIndex = span.StartIndex + int64(len(replacement))

								for id, other := range ranges[tab] {
									if id != stored.NamedRangeId {
										for _, otherSpan := range other.Ranges {
											if otherSpan.StartIndex >= oldEnd {
												otherSpan.StartIndex += delta
												otherSpan.EndIndex += delta
											}
										}
									}
								}

								if value.Text == "" {
									var raw struct {
										Text *string `json:"text"`
									}
									if err := json.Unmarshal(envelope.Requests[0]["replaceNamedRangeContent"], &raw); err != nil || raw.Text == nil || *raw.Text != "" {
										t.Error("empty replacement text omitted from wire")
									}
								}
							default:
								return nil, errDocsNamedEditFixture
							}

							revision++
							response = map[string]any{"documentId": "selected-doc", "replies": replies}
						default:
							return nil, errDocsNamedEditFixture
						}

						encoded, err := json.Marshal(response)
						if err != nil {
							return nil, fmt.Errorf("encode Docs named response: %w", err)
						}

						return &http.Response{StatusCode: responseCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded))), Request: request}, nil
					})

					return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			for _, test := range []struct {
				args    map[string]any
				want    docsNamedEditExpectation
				methods []string
			}{
				{map[string]any{"document_id": "selected-doc", "name": "Indexed", "start": 1, "end": 6}, docsNamedEditExpectation{tool: "docs_named_range_create", name: "Indexed", start: 1, end: 6}, []string{"GET", "POST"}},
				{map[string]any{"document_id": "selected-doc", "name": "Anchored", "at": "😀 anchor", "tab": "Work 👩‍💻"}, docsNamedEditExpectation{tool: "docs_named_range_create", name: "Anchored", start: 14, end: 23, tab: "tab-w"}, []string{"GET", "POST"}},
				{map[string]any{"document_id": "selected-doc", "name": "Nth", "at": "anchor", "occurrence": 2, "match_case": true, "tab": "Repeat"}, docsNamedEditExpectation{tool: "docs_named_range_create", name: "Nth", start: 19, end: 25, tab: "tab-r"}, []string{"GET", "POST"}},
				{map[string]any{"document_id": "selected-doc", "name_or_id": "nr-2", "text": "X😀", "tab": "Work 👩‍💻"}, docsNamedEditExpectation{tool: "docs_named_range_replace", id: "nr-2", tab: "tab-w", text: "X😀"}, []string{"GET", "POST", "GET"}},
				{map[string]any{"document_id": "selected-doc", "name_or_id": "Anchored", "text": ""}, docsNamedEditExpectation{tool: "docs_named_range_replace", id: "nr-2", tab: "tab-w", text: ""}, []string{"GET", "GET", "POST", "GET"}},
				{map[string]any{"document_id": "selected-doc", "name_or_id": "Anchored"}, docsNamedEditExpectation{tool: "docs_named_range_delete", id: "nr-2", tab: "tab-w"}, []string{"GET", "GET", "POST"}},
				{map[string]any{"document_id": "selected-doc", "name_or_id": "nr-3", "tab": "Repeat"}, docsNamedEditExpectation{tool: "docs_named_range_delete", id: "nr-3", tab: "tab-r"}, []string{"GET", "POST"}},
			} {
				want = test.want
				want.revision = fmt.Sprintf("rev-%d", revision)
				before := len(events)

				result := callDocsNamedEdit(t, harness, want.tool, test.args, scope, true, want.tool == "docs_named_range_delete" || want.tool == "docs_named_range_replace" && want.text == "", nil)
				if !result.OK || !slices.Equal(events[before:], test.methods) {
					t.Fatalf("%s outcome/sequence %s events%v", want.tool, result.Body, events[before:])
				}

				if want.tool == "docs_named_range_replace" {
					var output struct {
						TextLength int                            `json:"textLength"`
						NamedRange struct{ Ranges []*docs.Range } `json:"namedRange"`
					}
					if err := json.Unmarshal(result.Result, &output); err != nil || output.TextLength != len(utf16.Encode([]rune(want.text))) {
						t.Fatalf("wrong UTF16 replacement readback %s err=%v", result.Result, err)
					}

					if len(output.NamedRange.Ranges) != 1 || output.NamedRange.Ranges[0].StartIndex != 14 || output.NamedRange.Ranges[0].EndIndex != 14+int64(len(utf16.Encode([]rune(want.text)))) || output.NamedRange.Ranges[0].TabId != "tab-w" {
						t.Fatalf("wrong named-range UTF16 span readback %s", result.Result)
					}
				}
			}

			if ranges["tab-w"]["nr-stable"].Ranges[0].StartIndex != 7 || ranges["tab-w"]["nr-stable"].Ranges[0].EndIndex != 13 || ranges["tab-w"]["nr-1"] == nil || ranges["tab-w"]["nr-2"] != nil || ranges["tab-r"]["nr-3"] != nil || texts["tab-w"] != "Hello stable \n" || texts["tab-r"] != "😀 anchor plus 😀 anchor\n" {
				t.Fatal("named-range lifecycle changed unrelated range/tab text")
			}

			for _, test := range []struct {
				name string
				args map[string]any
				gets int
			}{
				{"docs_named_range_create", map[string]any{"document_id": "selected-doc", "name": "Indexed", "start": 1, "end": 2}, 1},
				{"docs_named_range_create", map[string]any{"document_id": "selected-doc", "name": "Ambiguous", "at": "anchor", "tab": "Repeat"}, 1},
				{"docs_named_range_delete", map[string]any{"document_id": "selected-doc", "name_or_id": "missing", "tab": "Work 👩‍💻"}, 1},
				{"docs_named_range_delete", map[string]any{"document_id": "selected-doc", "name_or_id": "Dup", "tab": "Work 👩‍💻"}, 1},
				{"docs_named_range_delete", map[string]any{"document_id": "selected-doc", "name_or_id": "shared"}, 2},
				{"docs_named_range_delete", map[string]any{"document_id": "selected-doc", "name_or_id": "indexed", "tab": "Work 👩‍💻"}, 1},
			} {
				before := len(events)

				result := callDocsNamedEdit(t, harness, test.name, test.args, scope, true, true, nil)
				if result.OK || len(events)-before != test.gets {
					t.Fatalf("ambiguous/unknown target mutated: %s events%v", result.Body, events[before:])
				}

				for _, method := range events[before:] {
					if method != "GET" {
						t.Fatal("rejected target submitted mutation")
					}
				}
			}
			denials := []func(*ToolAuthorization){func(a *ToolAuthorization) { a.AllowWrite = false }, func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/documents.readonly"} }, func(a *ToolAuthorization) { a.Resource = nil }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-doc" }, func(a *ToolAuthorization) { a.AllowedTools = []string{"docs_get"} }}

			valid := map[string]map[string]any{"docs_named_range_create": {"document_id": "selected-doc", "name": "Denied", "start": 1, "end": 3}, "docs_named_range_delete": {"document_id": "selected-doc", "name_or_id": "nr-stable"}, "docs_named_range_replace": {"document_id": "selected-doc", "name_or_id": "nr-stable", "text": "Denied"}}
			for name, args := range valid {
				for _, change := range denials {
					before := len(events)

					result := callDocsNamedEdit(t, harness, name, args, scope, true, true, change)
					if result.OK || len(events) != before {
						t.Fatalf("unauthorized%s reached Google: %s", name, result.Body)
					}
				}
			}

			for name, args := range map[string]map[string]any{"docs_named_range_delete": valid["docs_named_range_delete"], "docs_named_range_replace": {"document_id": "selected-doc", "name_or_id": "nr-stable", "text": ""}} {
				before := len(events)

				result := callDocsNamedEdit(t, harness, name, args, scope, true, false, nil)
				if result.OK || len(events) != before {
					t.Fatalf("destructive%s reached Google: %s", name, result.Body)
				}
			}
			conflict = true
			want = docsNamedEditExpectation{tool: "docs_named_range_create", name: "Concurrent", start: 1, end: 3, revision: fmt.Sprintf("rev-%d", revision)}
			beforeCreated := created

			result := callDocsNamedEdit(t, harness, want.tool, map[string]any{"document_id": "selected-doc", "name": "Concurrent", "start": 1, "end": 3}, scope, true, false, nil)
			if result.OK || created != beforeCreated {
				t.Fatal("stale revision mutation was not rejected")
			}
		})
	}
}
