package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestDocsStructuralExactHTTPGuard(t *testing.T) {
	for _, binding := range nativeDocsStructuralBindings() {
		guard := nativeRequestGuard(&ToolInvocation{Name: binding.Name, Arguments: map[string]any{"document_id": "selected-doc"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-doc"}}})

		for _, tc := range []struct {
			method, url string
			allow       bool
		}{
			{"GET", "https://docs.googleapis.com/v1/documents/selected-doc", true},
			{"POST", "https://docs.googleapis.com/v1/documents/selected-doc:batchUpdate", true},
			{"POST", "https://docs.googleapis.com/v1/documents/selected-doc", false},
			{"GET", "https://docs.googleapis.com/v1/documents/selected-doc:batchUpdate", false},
			{"DELETE", "https://docs.googleapis.com/v1/documents/selected-doc", false},
			{"POST", "https://docs.googleapis.com/v1/documents/other-doc:batchUpdate", false},
			{"POST", "https://foreign.example/v1/documents/selected-doc:batchUpdate", false},
		} {
			request, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err = guard(request); (err == nil) != tc.allow {
				t.Errorf("%s %s %s allowed %v err %v", binding.Name, tc.method, tc.url, tc.allow, err)
			}
		}
	}
}

func TestNativeDocsStructuralExecution(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       map[string]any
		requestKey string
		writes     int
	}{
		{"docs_table_row_insert", map[string]any{}, "insertTableRow", 1},
		{"docs_table_row_delete", map[string]any{"row": 1}, "deleteTableRow", 1},
		{"docs_table_column_insert", map[string]any{}, "insertTableColumn", 1},
		{"docs_table_column_delete", map[string]any{"col": 1}, "deleteTableColumn", 1},
		{"docs_table_merge", map[string]any{"range": "1,1:1,2"}, "mergeTableCells", 1},
		{"docs_table_unmerge", map[string]any{"cell": "1,1"}, "unmergeTableCells", 1},
		{"docs_table_row_pin_header", map[string]any{"rows": 0}, "pinTableHeaderRows", 1},
		{"docs_insert_footnote", map[string]any{"text": "footnote", "index": 1}, "createFootnote", 2},
		{"docs_insert_horizontal_rule", map[string]any{"index": 1}, "insertText", 1},
		{"docs_insert_page_break", map[string]any{"index": 1}, "insertPageBreak", 1},
		{"docs_insert_section_break", map[string]any{"index": 1}, "insertSectionBreak", 1},
		{"docs_insert_table", map[string]any{"index": 1, "rows": 2, "cols": 2}, "insertTable", 1},
		{"docs_insert_person", map[string]any{"index": 1, "email": "user@example.test"}, "insertPerson", 1},
		{"docs_insert_date_chip", map[string]any{"index": 1, "date": "2026-10-20"}, "insertDate", 1},
		{"docs_header_create", map[string]any{"text": "header"}, "createHeader", 2},
		{"docs_footer_create", map[string]any{"text": "footer"}, "createFooter", 2},
		{"docs_header_delete", map[string]any{"segment_id": "header-1"}, "deleteHeader", 1},
		{"docs_footer_delete", map[string]any{"segment_id": "footer-1"}, "deleteFooter", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			failWrite := false
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Docs = func(ctx context.Context, _ string) (*docs.Service, error) {
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						body := `{"documentId":"selected-doc","revisionId":"rev-1","body":{"content":[{"startIndex":1,"endIndex":5,"paragraph":{"elements":[{"startIndex":1,"endIndex":5,"textRun":{"content":"abc\n"}}]}}]},"headers":{"header-1":{"headerId":"header-1"}},"footers":{"footer-1":{"footerId":"footer-1"}}}`
						if strings.HasPrefix(tc.name, "docs_table_") {
							body = `{"documentId":"selected-doc","revisionId":"rev-1","body":{"content":[{"startIndex":1,"endIndex":30,"table":{"rows":2,"columns":2,"tableRows":[{"tableCells":[{"startIndex":3,"tableCellStyle":{"rowSpan":1,"columnSpan":1},"content":[{"startIndex":4,"endIndex":6,"paragraph":{"elements":[{"textRun":{"content":"A\n"}}]}}]},{"startIndex":7,"tableCellStyle":{"rowSpan":1,"columnSpan":1},"content":[{"startIndex":8,"endIndex":10,"paragraph":{"elements":[{"textRun":{"content":"B\n"}}]}}]}]},{"tableCells":[{"startIndex":11,"tableCellStyle":{"rowSpan":1,"columnSpan":1},"content":[{"startIndex":12,"endIndex":14,"paragraph":{"elements":[{"textRun":{"content":"C\n"}}]}}]},{"startIndex":15,"tableCellStyle":{"rowSpan":1,"columnSpan":1},"content":[{"startIndex":16,"endIndex":18,"paragraph":{"elements":[{"textRun":{"content":"D\n"}}]}}]}]}]}}]}}`
						}

						if tc.name == "docs_insert_table" && writes > 0 {
							body = `{"documentId":"selected-doc","body":{"content":[{"startIndex":2,"endIndex":20,"table":{"rows":2,"columns":2,"tableRows":[{"tableCells":[{},{}]},{"tableCells":[{},{}]}]}}]}}`
						}

						if r.Method == http.MethodPost {
							writes++

							var batch map[string]any
							if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
								t.Fatal(err)
							}

							requests, _ := batch["requests"].([]any)
							if len(requests) == 0 {
								t.Fatal("missing requests")
							}

							if writes == 1 {
								first := requests[0].(map[string]any)
								if _, ok := first[tc.requestKey]; !ok {
									t.Errorf("expected %s in %v", tc.requestKey, first)
								}
							}
							body = `{"documentId":"selected-doc","replies":[{"createFootnote":{"footnoteId":"footnote-1"},"createHeader":{"headerId":"header-new"},"createFooter":{"footerId":"footer-new"}}]}`
						}

						status := 200
						if failWrite && r.Method == http.MethodPost {
							status = 503
							body = `{"error":{"code":503,"message":"uncertain write"}}`
						}

						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})
					retry := googleapi.NewRetryTransport(transport)
					retry.MaxRetries5xx = 1

					return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, retry)}))
				}
			})
			tc.args["document_id"] = "selected-doc"
			call := func(change func(*ToolAuthorization)) capturedResponse {
				auth := ToolAuthorization{Service: "docs", AllowedTools: []string{tc.name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, AllowWrite: true, AllowDestructive: true, Resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "selected-doc", Enabled: true}}
				if change != nil {
					change(&auth)
				}
				data := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: tc.name, Arguments: tc.args, Authorization: auth} })

				return postExecute(t, harness.server, signInvocation(t, data, func(claims map[string]any) { claims["operation"] = OperationToolCall }), data)
			}

			result := call(nil)
			if !result.OK || writes != tc.writes {
				t.Fatalf("result %s writes %d want %d", result.Body, writes, tc.writes)
			}

			for _, change := range []func(*ToolAuthorization){func(a *ToolAuthorization) { a.AllowWrite = false }, func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-doc" }, func(a *ToolAuthorization) { a.AllowedTools = []string{"docs_get"} }, func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/documents.readonly"} }} {
				if deniedResult := call(change); deniedResult.OK {
					t.Fatal("unauthorized operation admitted")
				}
			}

			if strings.HasSuffix(tc.name, "_delete") {
				if deniedResult := call(func(a *ToolAuthorization) { a.AllowDestructive = false }); deniedResult.OK {
					t.Fatal("destructive permission bypassed")
				}
			}

			if writes != tc.writes {
				t.Fatal("denied invocation reached Google")
			}
			failWrite = true

			result = call(nil)
			if result.OK || writes != tc.writes+1 {
				t.Fatalf("uncertain write replayed or accepted: writes %d result %s", writes, result.Body)
			}
		})
	}
}
