package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	docs "google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeDocsInventorySignedExecution(t *testing.T) {
	const fixtureBody = `{
	  "documentId":"doc-123",
	  "title":"Selected document",
	  "revisionId":"r1",
	  "tabs":[{"tabProperties":{"tabId":"tab-1","title":"Overview","index":0}}],
	  "body":{"content":[
	    {"startIndex":0,"endIndex":1,"sectionBreak":{}},
	    {"startIndex":1,"endIndex":12,"paragraph":{
	      "paragraphStyle":{"namedStyleType":"HEADING_1","headingId":"h.section"},
	      "elements":[{"startIndex":1,"endIndex":11,"textRun":{"content":"Section text"}}]
	    }},
	    {"startIndex":12,"endIndex":20,"paragraph":{
	      "paragraphStyle":{"namedStyleType":"NORMAL_TEXT"},
	      "elements":[
	        {"startIndex":12,"endIndex":17,"textRun":{"content":"Body "}},
	        {"startIndex":17,"endIndex":19,"inlineObjectElement":{"inlineObjectId":"img-1"}}
	      ]
	    }},
	    {"startIndex":20,"endIndex":60,"table":{"rows":1,"columns":1,"tableRows":[{"tableCells":[{"content":[{"startIndex":21,"endIndex":26,"paragraph":{"paragraphStyle":{"namedStyleType":"NORMAL_TEXT"},"elements":[{"startIndex":21,"endIndex":26,"textRun":{"content":"Cell"}}]}}]}]}]}}
	  ]},
	  "inlineObjects":{"img-1":{"inlineObjectProperties":{"embeddedObject":{
	    "title":"Selected image",
	    "imageProperties":{"contentUri":"https://example.test/image.png"},
	    "size":{"width":{"magnitude":12,"unit":"PT"},"height":{"magnitude":8,"unit":"PT"}}
	  }}}},
	  "namedRanges":{"Intro":{"name":"Intro","namedRanges":[{"name":"Intro","namedRangeId":"range-1","ranges":[{"startIndex":1,"endIndex":12}]}]}}
	}`

	var decoded docs.Document
	if err := json.Unmarshal([]byte(fixtureBody), &decoded); err != nil {
		t.Fatalf("invalid native Docs fixture: %v", err)
	}

	tests := []struct {
		name       string
		tool       string
		checkQuery func(t *testing.T, r *http.Request)
		needles    []string
	}{
		{
			name: "docs_info",
			tool: "docs_info",
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				if got := r.URL.Query().Get("fields"); got != "documentId,title,revisionId" {
					t.Errorf("native url: %s fields: got %q want %q", r.URL, got, "documentId,title,revisionId")
				}
			},
			needles: []string{`"documentId":"doc-123"`, `"title":"Selected document"`, `"revisionId":"r1"`},
		},
		{
			name: "docs_list_tabs",
			tool: "docs_list_tabs",
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				switch got := r.URL.Query().Get("includeTabsContent"); got {
				case "true", "True":
				default:
					t.Errorf("native includeTabsContent: got %q want true", got)
				}
			},
			needles: []string{`"tabs":[{`, `"id":"tab-1"`, `"title":"Overview"`},
		},
		{
			name: "docs_structure",
			tool: "docs_structure",
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				q := r.URL.Query()
				if len(q) != 2 || q.Get("alt") != "json" || q.Get("prettyPrint") != "false" {
					t.Errorf("unexpected default native query: %s", r.URL.RawQuery)
				}
			},
			needles: []string{`"num":1`, `"text":"Section text"`, `"elemType":"table"`},
		},
		{
			name: "docs_headings_list",
			tool: "docs_headings_list",
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				q := r.URL.Query()
				if len(q) != 2 || q.Get("alt") != "json" || q.Get("prettyPrint") != "false" {
					t.Errorf("unexpected default native query: %s", r.URL.RawQuery)
				}
			},
			needles: []string{`"headingId":"h.section"`, `"style":"HEADING_1"`},
		},
		{
			name: "docs_paragraphs_list",
			tool: "docs_paragraphs_list",
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				q := r.URL.Query()
				if len(q) != 2 || q.Get("alt") != "json" || q.Get("prettyPrint") != "false" {
					t.Errorf("unexpected default native query: %s", r.URL.RawQuery)
				}
			},
			needles: []string{`"text":"Body "`, `"text":"Cell"`},
		},
		{
			name: "docs_tables_list",
			tool: "docs_tables_list",
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				q := r.URL.Query()
				if len(q) != 2 || q.Get("alt") != "json" || q.Get("prettyPrint") != "false" {
					t.Errorf("unexpected default native query: %s", r.URL.RawQuery)
				}
			},
			needles: []string{`"rows":1`, `"columns":1`, `"header":["Cell"]`},
		},
		{
			name: "docs_images_list",
			tool: "docs_images_list",
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				q := r.URL.Query()
				if len(q) != 2 || q.Get("alt") != "json" || q.Get("prettyPrint") != "false" {
					t.Errorf("unexpected default native query: %s", r.URL.RawQuery)
				}
			},
			needles: []string{`"objectId":"img-1"`, `"alt":"Selected image"`},
		},
		{
			name: "docs_named_range_list",
			tool: "docs_named_range_list",
			checkQuery: func(t *testing.T, r *http.Request) {
				t.Helper()

				q := r.URL.Query()
				if len(q) != 2 || q.Get("alt") != "json" || q.Get("prettyPrint") != "false" {
					t.Errorf("unexpected default native query: %s", r.URL.RawQuery)
				}
			},
			needles: []string{`"namedRanges":[{`, `"name":"Intro"`, `"namedRangeId":"range-1"`},
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
				ok := call(test.tool, scope, map[string]any{"document_id": "doc-123"}, resource, test.tool)
				if !ok.OK {
					t.Fatalf("%s under %s failed: %s", test.tool, scope, ok.Body)
				}
				if scope == "documents.readonly" {
					t.Logf("%s result: %s", test.tool, ok.Result)
				}

				for _, needle := range test.needles {
					if !strings.Contains(string(ok.Result), needle) {
						t.Fatalf("%s result missing %q: %s", test.tool, needle, ok.Result)
					}
				}

				var structured map[string]any
				if err := json.Unmarshal(ok.Result, &structured); err != nil {
					t.Fatalf("%s result was not structured: %v", test.tool, err)
				}
				if len(structured) == 0 {
					t.Fatalf("%s structured output was empty", test.tool)
				}
			}

			before := calls.Load()
			args := map[string]any{"document_id": "doc-123"}

			for _, denial := range []struct {
				name     string
				args     map[string]any
				resource *ResourceGrantInput
				scope    string
				tool     string
			}{
				{name: "missing resource", args: args, resource: nil, scope: "documents.readonly", tool: test.tool},
				{name: "disabled resource", args: args, resource: &ResourceGrantInput{Service: "docs", ResourceType: "file", ResourceID: "doc-123", Enabled: false}, scope: "documents.readonly", tool: test.tool},
				{name: "foreign document id", args: map[string]any{"document_id": "doc-999"}, resource: resource, scope: "documents.readonly", tool: test.tool},
				{name: "wrong tool grant", args: args, resource: resource, scope: "documents.readonly", tool: "docs_get"},
				{name: "wrong scope", args: args, resource: resource, scope: "presentations.readonly", tool: test.tool},
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
					Name: test.tool, Arguments: map[string]any{"document_id": "doc-123"},
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
