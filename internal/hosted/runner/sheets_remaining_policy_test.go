package runner

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const sheetsRemainingFixtureCases = `{
 "sheets_copy":{"title":"Copy"},
 "sheets_banding_clear":{"id":12},
 "sheets_banding_set":{"range":"Revenue Tab!A1:B3","row_properties_json":"{\"firstBandColor\":{\"red\":1}}"},
 "sheets_batch_update":{"data_json":"[{\"range\":\"'Revenue Tab'!A1\",\"values\":[[\"hello\"]]}]"},
 "sheets_chart_create":{"sheet":"Revenue Tab","spec_json":"{\"title\":\"Revenue\",\"basicChart\":{\"chartType\":\"COLUMN\"}}"},
 "sheets_chart_delete":{"chart_id":73},
 "sheets_chart_update":{"chart_id":73,"spec_json":"{\"title\":\"Updated\"}"},
 "sheets_conditional_format_add":{"range":"Revenue Tab!A1:B3","type":"blank","format_json":"{\"textFormat\":{\"bold\":true}}"},
 "sheets_conditional_format_clear":{"sheet":"Revenue Tab","index":0},
 "sheets_copy_paste":{"source":"Revenue Tab!A1:B2","dest":"Revenue Tab!C1:D2","type":"VALUES"},
 "sheets_delete_dimension":{"target":"Revenue Tab!2:2","dimension":"ROWS"},
 "sheets_duplicate_tab":{"source_tab":"Revenue Tab","new_name":"Copy"},
 "sheets_filter_set":{"range":"Revenue Tab!A1:B3"},
 "sheets_find_replace":{"sheet":"Revenue Tab","find":"old","replace":"new"},
 "sheets_format":{"range":"Revenue Tab!A1:B3","format_json":"{\"textFormat\":{\"bold\":false}}"},
 "sheets_insert":{"sheet":"Revenue Tab","dimension":"rows","start":2},
 "sheets_links_set":{"cell":"Revenue Tab!A1","url":"https://example.test/","text":"Example"},
 "sheets_raw":{},
 "sheets_table_create":{"range":"Revenue Tab!A1:B3","name":"Created","columns_json":"[{\"columnName\":\"A\",\"columnType\":\"TEXT\"},{\"columnName\":\"B\",\"columnType\":\"TEXT\"}]"},
 "sheets_table_append":{"table_id":"table-1","values_json":"[[\"appended\",42]]"},
 "sheets_table_clear":{"table_id":"table-1"},
 "sheets_table_delete":{"table_id":"table-1"},
 "sheets_validation_clear":{"range":"Revenue Tab!D1:D3"},
 "sheets_validation_set":{"range":"Revenue Tab!D1:D3","type":"BOOLEAN"}
}`

func TestNativeSheetsRemainingSignedFixtures(t *testing.T) {
	var cases map[string]map[string]any
	if err := json.Unmarshal([]byte(sheetsRemainingFixtureCases), &cases); err != nil {
		t.Fatal(err)
	}

	for _, scope := range []string{"spreadsheets", "drive.file"} {
		for _, binding := range nativeSheetsRemainingBindings() {
			if binding.Name == "sheets_copy" && scope != "drive.file" {
				continue
			}

			t.Run(scope+"/"+binding.Name, func(t *testing.T) {
				calls, posts := 0, 0
				failPosts := false
				var operation string
				h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
					client := func(ctx context.Context, account string) *http.Client {
						if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
							t.Error("trusted identity missing")
						}
						transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
							calls++

							if failPosts && r.Method == http.MethodPost {
								posts++
								return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":503,"message":"uncertain write"}}`)), Request: r}, nil
							}
							body := strings.Replace(sheetsDetailFixture, `"columnProperties":[{"columnIndex":0,"columnName":"Revenue","columnType":"TEXT"}]`, `"columnProperties":[{"columnIndex":0,"columnName":"Revenue","columnType":"TEXT"},{"columnIndex":1,"columnName":"Count","columnType":"TEXT"}]`, 1)

							if r.Method == http.MethodPost {
								posts++

								switch {
								case strings.HasSuffix(r.URL.Path, "/copy"):
									body = `{"id":"created","name":"Copy"}`
								case strings.HasSuffix(r.URL.Path, "/values:batchUpdate"):
									operation = "values"

									var payload sheets.BatchUpdateValuesRequest
									if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.Data) != 1 || payload.Data[0].Values[0][0] != "hello" {
										t.Fatalf("bad values body %+v %v", payload, err)
									}
									body = `{"totalUpdatedCells":1}`
								case strings.Contains(r.URL.Path, "/values/"):
									action := ":append"
									expectedRange := "'Revenue Tab'!A1:B3"
									operation = "append"

									if binding.Name == "sheets_table_clear" {
										action = ":clear"
										expectedRange = "'Revenue Tab'!A2:B3"
										operation = "clear"
									}

									if r.URL.Path != "/v4/spreadsheets/selected-sheet/values/"+expectedRange+action {
										t.Fatalf("derived range path %s", r.URL.Path)
									}

									if action == ":append" {
										var values sheets.ValueRange
										if err := json.NewDecoder(r.Body).Decode(&values); err != nil || len(values.Values) != 1 || len(values.Values[0]) != 2 || values.Values[0][0] != "appended" {
											t.Fatalf("append body %+v %v", values, err)
										}

										if r.URL.Query().Get("valueInputOption") != "RAW" || r.URL.Query().Get("insertDataOption") != "INSERT_ROWS" {
											t.Fatalf("unsafe append options %s", r.URL)
										}
									}
									body = `{"updates":{"updatedCells":2,"updatedRange":"'Revenue Tab'!A4:B4"},"clearedRange":"'Revenue Tab'!A2:B3"}`
								default:
									var payload map[string]json.RawMessage
									if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
										t.Fatal(err)
									}

									var requests []map[string]json.RawMessage
									if err := json.Unmarshal(payload["requests"], &requests); err != nil {
										t.Fatal(err)
									}

									if len(payload) != 1 || len(requests) == 0 {
										t.Fatalf("bad batch %v", payload)
									}

									for _, req := range requests {
										if len(req) != 1 {
											t.Fatalf("generic mixed request %v", req)
										}

										for key := range req {
											if operation == "" {
												operation = key
											}
										}
									}
									body = `{"replies":[{"duplicateSheet":{"properties":{"sheetId":8,"title":"Copy","index":1}}}]}`
								}
							} else if r.URL.Hostname() == "www.googleapis.com" {
								body = `{"id":"selected-sheet","mimeType":"application/vnd.google-apps.spreadsheet"}`
							}
							// Include deterministic rule/banding metadata for targeted deletions.
							body = strings.Replace(body, `"charts":[`, `"conditionalFormats":[{"ranges":[{"sheetId":2}]}],"bandedRanges":[{"bandedRangeId":12}],"charts":[`, 1)

							return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
						})

						return &http.Client{Transport: googleapi.GuardedTransport(ctx, googleapi.NewRetryTransport(transport))}
					}
					engine.NativeServices.Sheets = func(ctx context.Context, account string) (*sheets.Service, error) {
						c := client(ctx, account)

						return sheets.NewService(ctx, option.WithHTTPClient(c))
					}
					engine.NativeServices.SheetsHTTP = func(ctx context.Context, account string) (*http.Client, error) { return client(ctx, account), nil }
					engine.NativeServices.Drive = func(ctx context.Context, account string) (*drive.Service, error) {
						c := client(ctx, account)

						return drive.NewService(ctx, option.WithHTTPClient(c))
					}
				})
				args := cases[binding.Name]
				args["spreadsheet_id"] = "selected-sheet"

				result := callSheetsDetail(t, h, binding.Name, args, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = true })
				if !result.OK || calls == 0 || (binding.Write && posts != 1) {
					t.Fatalf("result=%s calls=%d posts=%d", result.Body, calls, posts)
				}

				expected := map[string]string{"sheets_table_append": "append", "sheets_table_clear": "clear", "sheets_banding_clear": "deleteBanding", "sheets_banding_set": "addBanding", "sheets_batch_update": "values", "sheets_chart_create": "addChart", "sheets_chart_delete": "deleteEmbeddedObject", "sheets_chart_update": "updateChartSpec", "sheets_conditional_format_add": "addConditionalFormatRule", "sheets_conditional_format_clear": "deleteConditionalFormatRule", "sheets_copy_paste": "copyPaste", "sheets_delete_dimension": "deleteDimension", "sheets_duplicate_tab": "duplicateSheet", "sheets_filter_set": "setBasicFilter", "sheets_find_replace": "findReplace", "sheets_format": "repeatCell", "sheets_insert": "insertDimension", "sheets_links_set": "updateCells", "sheets_table_create": "addTable", "sheets_table_delete": "deleteTable", "sheets_validation_clear": "setDataValidation", "sheets_validation_set": "setDataValidation"}
				if want := expected[binding.Name]; want != "" && operation != want {
					t.Fatalf("native request=%s want=%s", operation, want)
				}

				unsafe := map[string]map[string]any{
					"sheets_batch_update":           {"data_json": `[{"range":"Sheet1!A1","values":[["\u003d\u0049MPORTRANGE(\"foreign\",\"A1\")"]]}]`},
					"sheets_find_replace":           {"replace": " =importrange (\"foreign\",\"A1\")"},
					"sheets_conditional_format_add": {"type": "custom-formula", "expr": "=IMPORTRANGE(\"foreign\",\"A1\")"},
					"sheets_validation_set":         {"type": "CUSTOM_FORMULA", "value": "=IMPORTRANGE(\"foreign\",\"A1\")"},
					"sheets_table_create":           {"columns_json": `[{"columnName":"x","columnType":"DROPDOWN","dataValidationRule":{"condition":{"type":"CUSTOM_FORMULA","values":[{"userEnteredValue":"\u003dIMPO\u0052TRANGE(\"foreign\",\"A1\")"}]}}}]`},
				}
				if changes, ok := unsafe[binding.Name]; ok {
					bad := maps.Clone(args)
					for k, v := range changes {
						bad[k] = v
					}
					before := calls

					denied := callSheetsDetail(t, h, binding.Name, bad, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = true })
					if denied.OK || calls != before {
						t.Fatalf("cross-file formula reached transport: %s", denied.Body)
					}
				}

				if binding.Write {
					before := posts
					failPosts = true
					failed := callSheetsDetail(t, h, binding.Name, args, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = true })
					failPosts = false

					if failed.OK || posts != before+1 {
						t.Fatalf("uncertain write replayed: posts=%d before=%d result=%s", posts, before, failed.Body)
					}
				}

				denials := []func(*ToolAuthorization){func(a *ToolAuthorization) { a.Resource = nil }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign" }, func(a *ToolAuthorization) { a.AllowedTools = []string{"sheets_metadata"} }, func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/drive.readonly"} }}
				if binding.Write {
					denials = append(denials, func(a *ToolAuthorization) { a.AllowWrite = false }, func(a *ToolAuthorization) {
						a.Scopes = []string{"https://www.googleapis.com/auth/spreadsheets.readonly"}
					})
				}

				if binding.destructive {
					denials = append(denials, func(a *ToolAuthorization) { a.AllowDestructive = false })
				}

				for i, deny := range denials {
					before := calls

					r := callSheetsDetail(t, h, binding.Name, args, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = true; deny(a) })
					if r.OK || calls != before {
						t.Fatalf("denial %d reached transport: %s", i, r.Body)
					}
				}
			})
		}
	}
}

func TestNativeSheetsRemainingHTTPGuards(t *testing.T) {
	for _, binding := range nativeSheetsRemainingBindings() {
		t.Run(binding.Name, func(t *testing.T) {
			invocation := &ToolInvocation{Name: binding.Name, Arguments: map[string]any{"spreadsheet_id": "selected-sheet"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-sheet"}}}
			guard := nativeRequestGuard(invocation)
			root := "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet"

			action := root + ":batchUpdate"
			if binding.Name == "sheets_table_append" {
				action = root + "/values/'Revenue Tab'!A1:B3:append"
			}

			if binding.Name == "sheets_table_clear" {
				action = root + "/values/'Revenue Tab'!A2:B3:clear"
			}

			if binding.Name == "sheets_copy" {
				root = "https://www.googleapis.com/drive/v3/files/selected-sheet"
				action = root + "/copy"
			}

			if binding.Name == "sheets_batch_update" {
				action = root + "/values:batchUpdate"
			}

			tests := []struct {
				method, url string
				allow       bool
			}{{"GET", root, binding.Name != "sheets_batch_update"}, {"POST", action, binding.Write}, {"POST", root, false}, {"GET", action, false}, {"DELETE", root, false}, {"GET", root + "/values/A1", false}, {"POST", strings.Replace(action, "selected-sheet", "foreign", 1), false}, {"POST", strings.Replace(action, "googleapis.com", "evil.test", 1), false}}
			for _, test := range tests {
				r, e := http.NewRequestWithContext(context.Background(), test.method, test.url, nil)
				if e != nil {
					t.Fatal(e)
				}

				if e = guard(r); (e == nil) != test.allow {
					t.Errorf("%s %s allow=%t err=%v", test.method, test.url, test.allow, e)
				}
			}
		})
	}
}

func TestNativeSheetsRemainingMethodPolicy(t *testing.T) {
	for _, binding := range nativeSheetsRemainingBindings() {
		root := "/v4/spreadsheets/selected-sheet"

		action := root + ":batchUpdate"
		if binding.Name == "sheets_table_append" {
			action = root + "/values/'Revenue Tab'!A1:B3:append"
		}

		if binding.Name == "sheets_table_clear" {
			action = root + "/values/'Revenue Tab'!A2:B3:clear"
		}

		if binding.Name == "sheets_copy" {
			root = "/drive/v3/files/selected-sheet"
			action = root + "/copy"
		}

		if binding.Name == "sheets_batch_update" {
			action = root + "/values:batchUpdate"
		}

		for _, test := range []struct {
			method, path string
			allow        bool
		}{
			{"GET", root, binding.Name != "sheets_batch_update"},
			{"POST", root, false},
			{"POST", action, binding.Write},
			{"GET", action, binding.Name == "sheets_raw"},
			{"DELETE", root, false},
		} {
			if err := admitSheetsRemainingRequestMethod(binding.Name, test.path, test.method); (err == nil) != test.allow {
				t.Errorf("%s %s %s allow=%t err=%v", binding.Name, test.method, test.path, test.allow, err)
			}
		}
	}
}

func TestNativeSheetsRemainingDynamicPath(t *testing.T) {
	for _, name := range []string{"sheets_table_append", "sheets_table_clear"} {
		suffix := ":append"
		if name == "sheets_table_clear" {
			suffix = ":clear"
		}
		invocation := &ToolInvocation{Name: name, Arguments: map[string]any{"spreadsheet_id": "selected-sheet"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-sheet"}}}

		root := "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet"
		for _, test := range []struct {
			method, url string
			allowed     bool
		}{
			{"GET", root, true},
			{"POST", root, false},
			{"POST", root + "/values/'Revenue Tab'!A2:B3" + suffix, true},
			{"POST", root + "/values/A2:B3" + suffix, false},
			{"GET", root + "/values/Sheet1!A2:B3" + suffix, false},
			{"POST", root + "/values/Sheet1!A2:B3:batchUpdate", false},
			{"POST", root + "/values/Sheet1!A2:B3:clear:append", false},
			{"POST", root + "/values/Sheet1!A:B" + suffix, false},
			{"POST", root + "/values/Sheet1!A0:B3" + suffix, false},
			{"POST", root + "/values/../foreign" + suffix, false},
			{"POST", root + "/values/Sheet1!A2:B3%2Fother" + suffix, false},
			{"POST", strings.Replace(root, "selected-sheet", "foreign", 1) + "/values/Sheet1!A2:B3" + suffix, false},
			{"POST", strings.Replace(root, "googleapis.com", "evil.test", 1) + "/values/Sheet1!A2:B3" + suffix, false},
			{"POST", root + ":batchUpdate", false},
		} {
			r, err := http.NewRequestWithContext(context.Background(), test.method, test.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			handled, allowed := admitSheetsRemainingDynamicPath(invocation, r)
			if !handled || allowed != test.allowed {
				t.Errorf("%s %s %s handled=%t allowed=%t", name, test.method, test.url, handled, allowed)
			}
		}
		invocation.Authorization.Resource.ResourceID = "foreign"

		r, _ := http.NewRequestWithContext(context.Background(), "GET", root, nil)
		if _, allowed := admitSheetsRemainingDynamicPath(invocation, r); allowed {
			t.Error("foreign grant admitted")
		}
	}

	if handled, _ := admitSheetsRemainingDynamicPath(&ToolInvocation{Name: "sheets_metadata"}, nil); handled {
		t.Error("unrelated tool handled")
	}
}

func TestNativeSheetsValidationPreservesLiteralComma(t *testing.T) {
	const formula = "=AND(A1>0,A1<10)"
	writes := 0
	h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Sheets = func(ctx context.Context, _ string) (*sheets.Service, error) {
			base := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				body := sheetsDetailFixture

				if r.Method == http.MethodPost {
					writes++

					var batch sheets.BatchUpdateSpreadsheetRequest
					if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
						t.Fatal(err)
					}

					if len(batch.Requests) != 1 || batch.Requests[0].SetDataValidation == nil {
						t.Fatalf("wrong validation request %+v", batch)
					}

					values := batch.Requests[0].SetDataValidation.Rule.Condition.Values
					if len(values) != 1 || values[0].UserEnteredValue != formula {
						t.Fatalf("validation formula split or corrupted: %+v", values)
					}
					body = `{"spreadsheetId":"selected-sheet","replies":[{}]}`
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return sheets.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, base)}))
		}
	})

	result := callSheetsDetail(t, h, "sheets_validation_set", map[string]any{"spreadsheet_id": "selected-sheet", "range": "Revenue Tab!D1:D3", "type": "CUSTOM_FORMULA", "value": formula}, "drive.file", func(a *ToolAuthorization) { a.AllowWrite = true })
	if !result.OK || writes != 1 {
		t.Fatalf("literal formula failed: writes=%d %s", writes, result.Body)
	}
}

func TestNativeSheetsRemainingRejectsPlusPrefixedImports(t *testing.T) {
	calls := 0

	h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Sheets = func(context.Context, string) (*sheets.Service, error) { calls++; return nil, errSlidesRemainingFixture }
	})
	for _, data := range []string{`[{"range":"Revenue Tab!A1","values":[["+IMPORTRANGE(\"foreign\",\"A1\")"]]}]`, `[{"range":"Revenue Tab!A1","values":[["\u002bIMPO\u0052TRANGE(\"foreign\",\"A1\")"]]}]`} {
		result := callSheetsDetail(t, h, "sheets_batch_update", map[string]any{"spreadsheet_id": "selected-sheet", "data_json": data}, "drive.file", func(a *ToolAuthorization) { a.AllowWrite = true })
		if result.OK || calls != 0 {
			t.Fatalf("plus-prefixed foreign formula admitted: calls=%d %s", calls, result.Body)
		}
	}
}
