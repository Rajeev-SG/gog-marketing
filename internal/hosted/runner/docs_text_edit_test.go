package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf16"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeDocsTextEditLifecycle(t *testing.T) {
	for _, scope := range []string{"documents", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			text := "Alpha beta Alpha\n"
			other := "Other Alpha\n"
			header := "Header\n"
			revision := int64(1)
			var calls atomic.Int64
			requireRevision := false
			body := func(value string, start int64) *docs.Body {
				end := start + int64(len(utf16.Encode([]rune(value))))
				return &docs.Body{Content: []*docs.StructuralElement{{StartIndex: start, EndIndex: end, Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{StartIndex: start, EndIndex: end, TextRun: &docs.TextRun{Content: value}}}}}}}
			}
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Docs = func(ctx context.Context, _ string) (*docs.Service, error) {
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)
						var response any

						switch {
						case r.Method == http.MethodGet && r.URL.Path == "/v1/documents/selected-doc":
							response = &docs.Document{DocumentId: "selected-doc", RevisionId: fmt.Sprintf("rev-%d", revision), Body: body(text, 1), Headers: map[string]docs.Header{"header-1": {HeaderId: "header-1", Content: body(header, 0).Content}}, Tabs: []*docs.Tab{{TabProperties: &docs.TabProperties{TabId: "tab-main", Title: "Main"}, DocumentTab: &docs.DocumentTab{Body: body(text, 1), Headers: map[string]docs.Header{"header-1": {HeaderId: "header-1", Content: body(header, 0).Content}}}}, {TabProperties: &docs.TabProperties{TabId: "tab-other", Title: "Other"}, DocumentTab: &docs.DocumentTab{Body: body(other, 1)}}}}
						case r.Method == http.MethodPost && r.URL.Path == "/v1/documents/selected-doc:batchUpdate":
							var batch docs.BatchUpdateDocumentRequest
							if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
								t.Fatal(err)
							}

							hasReplaceAll := len(batch.Requests) == 1 && batch.Requests[0].ReplaceAllText != nil
							if !hasReplaceAll && requireRevision && (batch.WriteControl == nil || batch.WriteControl.RequiredRevisionId != fmt.Sprintf("rev-%d", revision)) {
								t.Errorf("missing revision guard %+v", batch.WriteControl)
							}
							replies := []*docs.Response{}

							for _, request := range batch.Requests {
								switch {
								case request.InsertText != nil:
									value := request.InsertText
									target := &text
									start := int64(1)

									if value.Location.SegmentId == "header-1" {
										target = &header
										start = 0
									}

									if value.Location.TabId == "tab-other" {
										target = &other
									}
									units := utf16.Encode([]rune(*target))
									i := int(value.Location.Index - start)
									insert := utf16.Encode([]rune(value.Text))
									units = append(append(append([]uint16{}, units[:i]...), insert...), units[i:]...)
									*target = string(utf16.Decode(units))

									replies = append(replies, &docs.Response{})
								case request.DeleteContentRange != nil:
									span := request.DeleteContentRange.Range
									target := &text
									start := int64(1)

									if span.SegmentId == "header-1" {
										target = &header
										start = 0
									}

									if span.TabId == "tab-other" {
										target = &other
									}
									units := utf16.Encode([]rune(*target))
									a, b := int(span.StartIndex-start), int(span.EndIndex-start)
									units = append(append([]uint16{}, units[:a]...), units[b:]...)
									*target = string(utf16.Decode(units))

									replies = append(replies, &docs.Response{})
								case request.ReplaceAllText != nil:
									value := request.ReplaceAllText
									if value.ContainsText == nil || (value.ContainsText.Text != "Alpha" && value.ContainsText.Text != "aa") || !value.ContainsText.MatchCase {
										t.Errorf("wrong literal matching %+v", value)
									}
									targets := []*string{&text, &other}

									if value.TabsCriteria != nil {
										if len(value.TabsCriteria.TabIds) != 1 || value.TabsCriteria.TabIds[0] != "tab-main" {
											t.Errorf("wrong tabcriteria %+v", value.TabsCriteria)
										}
										targets = []*string{&text}
									}

									count := int64(0)
									for _, target := range targets {
										count += int64(strings.Count(*target, value.ContainsText.Text))
										*target = strings.ReplaceAll(*target, value.ContainsText.Text, value.ReplaceText)
									}

									replies = append(replies, &docs.Response{ReplaceAllText: &docs.ReplaceAllTextResponse{OccurrencesChanged: count}})
								default:
									t.Errorf("undeclared text edit request %+v", request)
								}
							}

							revision++
							response = &docs.BatchUpdateDocumentResponse{DocumentId: "selected-doc", Replies: replies}
						default:
							t.Errorf("foreign/unexpected request %s %s", r.Method, r.URL)
							response = map[string]any{}
						}

						raw, err := json.Marshal(response)
						if err != nil {
							t.Fatal(err)
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
					})

					return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			call := func(name string, args map[string]any, destructive bool, change func(*ToolAuthorization)) capturedResponse {
				auth := ToolAuthorization{Service: "docs", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive, Resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "selected-doc", Enabled: true}}
				if change != nil {
					change(&auth)
				}
				data := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: auth} })

				return postExecute(t, harness.server, signInvocation(t, data, func(claims map[string]any) { claims["operation"] = OperationToolCall }), data)
			}
			for _, tc := range []struct {
				name                 string
				args                 map[string]any
				destructive          bool
				wantText, wantHeader string
			}{
				{"docs_insert", map[string]any{"document_id": "selected-doc", "text": "😀", "index": 1}, false, "😀Alpha beta Alpha\n", "Header\n"},
				{"docs_delete", map[string]any{"document_id": "selected-doc", "start": 1, "end": 3}, true, "Alpha beta Alpha\n", "Header\n"},
				{"docs_insert", map[string]any{"document_id": "selected-doc", "text": "X", "at": "Alpha", "occurrence": 2, "match_case": true, "tab": "Main"}, false, "Alpha beta XAlpha\n", "Header\n"},
				{"docs_delete", map[string]any{"document_id": "selected-doc", "at": "X", "tab": "Main"}, true, "Alpha beta Alpha\n", "Header\n"},
				{"docs_insert", map[string]any{"document_id": "selected-doc", "text": "X", "index": 0, "tab": "Main", "segment": "header-1"}, false, "Alpha beta Alpha\n", "XHeader\n"},
				{"docs_delete", map[string]any{"document_id": "selected-doc", "start": 0, "end": 1, "tab": "Main", "segment": "header-1"}, true, "Alpha beta Alpha\n", "Header\n"},
				{"docs_find_replace", map[string]any{"document_id": "selected-doc", "find": "Alpha", "replacement": "X", "first": true, "match_case": true, "tab": "Main"}, false, "X beta Alpha\n", "Header\n"},
				{"docs_find_replace", map[string]any{"document_id": "selected-doc", "find": "Alpha", "replacement": "aa", "match_case": true, "tab": "Main"}, false, "X beta aa\n", "Header\n"},
				{"docs_find_replace", map[string]any{"document_id": "selected-doc", "find": "aa", "replacement": "", "match_case": true, "tab": "Main"}, true, "X beta \n", "Header\n"},
				{"docs_clear", map[string]any{"document_id": "selected-doc"}, true, "\n", "Header\n"},
			} {
				_, anchored := tc.args["at"]
				first, _ := tc.args["first"].(bool)
				requireRevision = anchored || first || tc.name == "docs_clear"

				result := call(tc.name, tc.args, tc.destructive, nil)
				if !result.OK || text != tc.wantText || header != tc.wantHeader || other != "Other Alpha\n" {
					t.Fatalf("%s outcome%s text%q header%q other%q", tc.name, result.Body, text, header, other)
				}
			}
			before := calls.Load()

			valid := map[string]map[string]any{"docs_insert": {"document_id": "selected-doc", "text": "X", "index": 1}, "docs_delete": {"document_id": "selected-doc", "start": 1, "end": 2}, "docs_clear": {"document_id": "selected-doc"}, "docs_find_replace": {"document_id": "selected-doc", "find": "Alpha", "replacement": "X"}}
			for name, args := range valid {
				for _, change := range []func(*ToolAuthorization){func(a *ToolAuthorization) { a.AllowWrite = false }, func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/documents.readonly"} }, func(a *ToolAuthorization) { a.Resource = nil }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-doc" }, func(a *ToolAuthorization) { a.AllowedTools = []string{"docs_get"} }} {
					if result := call(name, args, true, change); result.OK {
						t.Fatalf("ungranted%sadmitted", name)
					}
				}
			}

			for name, args := range map[string]map[string]any{"docs_delete": valid["docs_delete"], "docs_clear": valid["docs_clear"], "docs_find_replace": {"document_id": "selected-doc", "find": "Alpha", "replacement": ""}} {
				if result := call(name, args, false, nil); result.OK {
					t.Fatalf("missing destructive%sadmitted", name)
				}
			}

			if calls.Load() != before {
				t.Fatal("denied text edit reached Google")
			}
		})
	}
}

func TestDocsTextEditExactHTTPGuard(t *testing.T) {
	for _, name := range []string{"docs_insert", "docs_delete", "docs_clear", "docs_find_replace"} {
		guard := nativeRequestGuard(&ToolInvocation{Name: name, Arguments: map[string]any{"document_id": "selected-doc"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-doc"}}})

		base := "https://docs.googleapis.com/v1/documents/selected-doc"
		for _, tc := range []struct {
			method, url string
			allow       bool
		}{
			{http.MethodGet, base, true},
			{http.MethodPost, base + ":batchUpdate", true},
			{http.MethodPost, base, false},
			{http.MethodGet, base + ":batchUpdate", false},
			{http.MethodDelete, base, false},
			{http.MethodPost, "https://docs.googleapis.com/v1/documents/foreign-doc:batchUpdate", false},
			{http.MethodPost, base + ":batchUpdate/extra", false},
			{http.MethodGet, "https://foreign.example/v1/documents/selected-doc", false},
		} {
			request, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err := guard(request); (err == nil) != tc.allow {
				t.Errorf("%s %s %s allowed%t err%v", name, tc.method, tc.url, tc.allow, err)
			}
		}
	}
}

func TestNativeDocsTextEditsSingleAttemptThroughGuard(t *testing.T) {
	for _, name := range []string{"docs_insert", "docs_delete", "docs_clear", "docs_find_replace"} {
		t.Run(name, func(t *testing.T) {
			var effects atomic.Int64
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Docs = func(ctx context.Context, _ string) (*docs.Service, error) {
					base := nativeFixtureTransport(func(request *http.Request) (*http.Response, error) {
						status := 200
						body := `{"documentId":"selected-doc","revisionId":"rev-1","body":{"content":[{"startIndex":1,"endIndex":5,"paragraph":{"elements":[{"textRun":{"content":"abc\n"}}]}}]}}`

						if request.Method == http.MethodPost {
							effects.Add(1)
							status = 503
							body = `{"error":{"code":503,"message":"uncertain"}}`
						}

						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
					})
					retry := googleapi.NewRetryTransport(base)
					retry.MaxRetries5xx = 1

					return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, retry)}))
				}
			})
			args := map[string]any{"document_id": "selected-doc"}

			switch name {
			case "docs_insert":
				args["text"] = "X"
				args["index"] = 1
			case "docs_delete":
				args["start"] = 1
				args["end"] = 2
			case "docs_find_replace":
				args["find"] = "a"
				args["replacement"] = "aa"
			}
			data := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "docs", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/documents"}, AllowWrite: true, AllowDestructive: true, Resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "selected-doc", Enabled: true}}}
			})

			result := postExecute(t, harness.server, signInvocation(t, data, func(claims map[string]any) { claims["operation"] = OperationToolCall }), data)
			if result.OK || effects.Load() != 1 {
				t.Fatalf("%s uncertain mutation repeated effects=%d result=%s", name, effects.Load(), result.Body)
			}
		})
	}
}
