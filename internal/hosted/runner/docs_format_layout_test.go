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

func TestNativeDocsFormatLayoutPayload(t *testing.T) {
	const getBody = `{"documentId":"selected-doc","revisionId":"rev-1","body":{"content":[{"startIndex":1,"endIndex":5,"paragraph":{"elements":[{"startIndex":1,"endIndex":5,"textRun":{"content":"abc\n"}}]}}]}}`

	cases := []struct{ name, args string }{
		{"docs_format", `{"bold":false,"space_above":0,"keep_with_next":false}`},
		{"docs_page_layout", `{"margin_left":"0"}`},
		{"docs_section_columns", `{"count":2,"index":1}`},
		{"docs_section_columns", `{"count":2,"at":"abc"}`},
		{"docs_section_columns", `{"count":2}`},
		{"docs_section_columns", `{"count":2,"at_end":true}`},
	}
	for _, tc := range cases {
		for _, scope := range []string{"documents", "drive.file"} {
			t.Run(tc.name+"_"+scope, func(t *testing.T) {
				var posts int
				var factories int
				var batch map[string]any
				harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
					engine.NativeServices.Docs = func(ctx context.Context, _ string) (*docs.Service, error) {
						factories++
						transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
							resp := func(code int, body string) *http.Response {
								return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
							}

							switch {
							case r.Method == http.MethodGet && r.URL.Path == "/v1/documents/selected-doc":
								return resp(http.StatusOK, getBody), nil
							case r.Method == http.MethodPost && r.URL.Path == "/v1/documents/selected-doc:batchUpdate":
								posts++

								raw, err := io.ReadAll(r.Body)
								if err != nil {
									t.Fatalf("read POST body: %v", err)
								}

								if err := json.Unmarshal(raw, &batch); err != nil {
									t.Fatalf("decode batch: %v", err)
								}

								return resp(http.StatusOK, `{"documentId":"selected-doc"}`), nil
							}

							t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)

							return resp(http.StatusInternalServerError, `{}`), nil
						})

						return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
					}
				})

				args := map[string]any{}
				if err := json.Unmarshal([]byte(tc.args), &args); err != nil {
					t.Fatal(err)
				}
				args["document_id"] = "selected-doc"

				res := callDocsNamedEdit(t, harness, tc.name, args, scope, true, false, nil)
				if !res.OK {
					t.Fatalf("%s/%s: unexpected failure: %s", tc.name, scope, res.Body)
				}
				before := factories

				for _, change := range []func(*ToolAuthorization){
					func(a *ToolAuthorization) { a.Resource = nil },
					func(a *ToolAuthorization) { a.Resource.Enabled = false },
					func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-doc" },
					func(a *ToolAuthorization) { a.AllowedTools = []string{"docs_get"} },
					func(a *ToolAuthorization) { a.AllowWrite = false },
					func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/documents.readonly"} },
				} {
					if denied := callDocsNamedEdit(t, harness, tc.name, args, scope, true, false, change); denied.OK {
						t.Fatal("ungranted formatting admitted")
					}
				}

				if factories != before {
					t.Fatal("denied formatting reached Google service")
				}

				if posts != 1 {
					t.Fatalf("%s/%s: POSTs=%d, want 1", tc.name, scope, posts)
				}

				reqs, _ := batch["requests"].([]any)
				if len(reqs) == 0 {
					t.Fatalf("%s/%s: no requests in %v", tc.name, scope, batch)
				}
				byType := map[string]map[string]any{}

				for _, r := range reqs {
					m, ok := r.(map[string]any)
					if !ok {
						t.Fatalf("%s/%s: non-object request %v", tc.name, scope, r)
					}

					for k := range m {
						value, ok := m[k].(map[string]any)
						if !ok {
							t.Fatalf("non-object request body %v", m[k])
						}
						byType[k] = value
					}
				}
				need := func(key string) map[string]any {
					t.Helper()

					m, ok := byType[key]
					if !ok {
						t.Fatalf("%s/%s: missing %s in %v", tc.name, scope, key, batch)
					}

					return m
				}
				sub := func(m map[string]any, key string) map[string]any {
					t.Helper()

					v, ok := m[key].(map[string]any)
					if !ok {
						t.Fatalf("%s/%s: missing %s in %v", tc.name, scope, key, m)
					}

					return v
				}

				switch tc.name {
				case "docs_format":
					ts := sub(need("updateTextStyle"), "textStyle")
					if b, ok := ts["bold"].(bool); !ok || b {
						t.Fatalf("%s/%s: textStyle.bold: %v", tc.name, scope, ts)
					}

					ps := sub(need("updateParagraphStyle"), "paragraphStyle")
					if b, ok := ps["keepWithNext"].(bool); !ok || b {
						t.Fatalf("%s/%s: paragraphStyle.keepWithNext: %v", tc.name, scope, ps)
					}

					if sa := sub(ps, "spaceAbove"); sa["magnitude"] != float64(0) {
						t.Fatalf("%s/%s: spaceAbove: %v", tc.name, scope, sa)
					}
				case "docs_page_layout":
					uds := need("updateDocumentStyle")
					if uds["fields"] != "marginLeft" {
						t.Fatalf("%s/%s: updateDocumentStyle: %v", tc.name, scope, uds)
					}

					if _, ok := uds["documentMode"]; ok {
						t.Fatalf("%s/%s: unexpected documentMode: %v", tc.name, scope, uds)
					}

					ds := sub(uds, "documentStyle")
					if _, ok := ds["documentMode"]; ok {
						t.Fatalf("%s/%s: unexpected documentMode in style: %v", tc.name, scope, ds)
					}

					if ml := sub(ds, "marginLeft"); ml["magnitude"] != float64(0) {
						t.Fatalf("%s/%s: marginLeft: %v", tc.name, scope, ml)
					}
				case "docs_section_columns":
					uss := need("updateSectionStyle")
					secStyle := sub(uss, "sectionStyle")

					cols, _ := secStyle["columnProperties"].([]any)
					if len(cols) != 2 {
						t.Fatalf("%s/%s: columnProperties: %v", tc.name, scope, secStyle)
					}

					wantStart := float64(1)
					if _, indexed := args["index"]; !indexed {
						if _, anchored := args["at"]; !anchored {
							wantStart = 4
						}
					}

					if rng := sub(uss, "range"); rng["startIndex"] != wantStart || rng["endIndex"] != wantStart+1 {
						t.Fatalf("%s/%s: range: %v", tc.name, scope, rng)
					}

					if _, anchored := args["at"]; anchored {
						if wc := sub(batch, "writeControl"); wc["requiredRevisionId"] != "rev-1" {
							t.Fatalf("anchor revision: %v", wc)
						}
					} else if _, present := batch["writeControl"]; present {
						t.Fatalf("unexpected index/end revision: %v", batch)
					}
				}
			})
		}
	}
}
