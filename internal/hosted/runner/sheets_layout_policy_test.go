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

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var errSheetsLayoutFixture = errors.New("unexpected layout fixture request")

func TestSignedNativeSheetsLayoutPersistsOnlyApprovedFields(t *testing.T) {
	for _, scope := range [][]string{sheetsWriteScope, {"https://www.googleapis.com/auth/drive.file"}} {
		t.Run(scope[0], func(t *testing.T) {
			rows, cols := int64(2), int64(1)
			order := []int64{0, 1, 2}
			googleCalls, writes, moves := 0, 0, 0
			h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Sheets = func(ctx context.Context, account string) (*sheets.Service, error) {
					if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
						t.Error("layout lost trusted execution identity")
					}

					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						googleCalls++
						body := []byte(`{"replies":[]}`)

						if r.URL.Hostname() != nativeSheetsHost {
							t.Error("layout reached a foreign host")
							return nil, errSheetsLayoutFixture
						}

						switch {
						case r.Method == http.MethodGet && r.URL.Path == "/v4/spreadsheets/selected-sheet":
							tabs := make([]map[string]any, 0, len(order))
							for index, id := range order {
								tabs = append(tabs, map[string]any{"properties": map[string]any{
									"sheetId": id, "title": fmt.Sprintf("Sheet%d", id+1), "index": index,
									"gridProperties": map[string]any{"rowCount": 20, "columnCount": 10, "frozenRowCount": rows, "frozenColumnCount": cols},
								}})
							}

							var err error

							body, err = json.Marshal(map[string]any{"spreadsheetId": "selected-sheet", "sheets": tabs})
							if err != nil {
								return nil, fmt.Errorf("layout fixture JSON: %w", err)
							}
						case r.Method == http.MethodPost && r.URL.Path == "/v4/spreadsheets/selected-sheet:batchUpdate":
							var batch struct {
								Requests []map[string]json.RawMessage `json:"requests"`
							}
							if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
								return nil, fmt.Errorf("layout fixture JSON: %w", err)
							}

							if len(batch.Requests) != 1 || len(batch.Requests[0]) != 1 || batch.Requests[0]["updateSheetProperties"] == nil {
								t.Error("layout admitted a request kind beyond one updateSheetProperties")
								return nil, errSheetsLayoutFixture
							}

							var update struct {
								Fields     string `json:"fields"`
								Properties struct {
									ID    *int64 `json:"sheetId"`
									Index *int64 `json:"index"`
									Grid  *struct {
										Rows *int64 `json:"frozenRowCount"`
										Cols *int64 `json:"frozenColumnCount"`
									} `json:"gridProperties"`
								} `json:"properties"`
							}
							if err := json.Unmarshal(batch.Requests[0]["updateSheetProperties"], &update); err != nil {
								return nil, fmt.Errorf("layout fixture JSON: %w", err)
							}

							p := update.Properties
							if p.ID == nil || *p.ID != 0 {
								t.Error("zero selected tab ID was missing or replaced")
								return nil, errSheetsLayoutFixture
							}

							switch update.Fields {
							case "gridProperties.frozenRowCount":
								if p.Grid == nil || p.Grid.Rows == nil || *p.Grid.Rows != 0 || p.Grid.Cols != nil || p.Index != nil {
									t.Error("unfreeze changed unintended fields or omitted explicit zero")
									return nil, errSheetsLayoutFixture
								}
								rows = *p.Grid.Rows
							case "gridProperties.frozenColumnCount":
								if p.Grid == nil || p.Grid.Cols == nil || *p.Grid.Cols != 3 || p.Grid.Rows != nil || p.Index != nil {
									t.Error("column freeze changed unintended fields")
									return nil, errSheetsLayoutFixture
								}
								cols = *p.Grid.Cols
							case "index":
								if p.Index == nil || p.Grid != nil || moves >= 2 {
									return nil, errSheetsLayoutFixture
								}

								expected := []int64{3, 0}[moves]
								if *p.Index != expected {
									t.Errorf("native API move index=%d, want %d", *p.Index, expected)
									return nil, errSheetsLayoutFixture
								}
								old := slices.Index(order, int64(0))

								position := int(*p.Index)
								if position > old {
									position--
								}

								next := make([]int64, 0, len(order))
								for _, id := range order {
									if id != 0 {
										next = append(next, id)
									}
								}

								if old < 0 || position < 0 || position > len(next) {
									return nil, errSheetsLayoutFixture
								}
								next = append(next, 0)
								copy(next[position+1:], next[position:])
								next[position] = 0
								order = next
								moves++
							default:
								t.Errorf("unexpected layout mask %q", update.Fields)
								return nil, errSheetsLayoutFixture
							}

							writes++
						default:
							t.Errorf("foreign layout method/path: %s %s", r.Method, r.URL)
							return nil, errSheetsLayoutFixture
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
					})

					return sheets.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})
			call := func(name string, args map[string]any, auth ToolAuthorization) capturedResponse {
				body := signSheetsInvocation(t, name, args, auth)
				return postExecute(t, h.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			}
			freezeAuth := sheetsLocalAuthorization("sheets_freeze", scope, true, false)

			freeze := map[string]any{"spreadsheet_id": "selected-sheet", "sheet": "Sheet1", "rows": 0}
			if result := call("sheets_freeze", freeze, freezeAuth); !result.OK || rows != 0 || cols != 1 {
				t.Fatalf("unfreeze did not preserve columns: %s", result.Body)
			}

			if result := call("sheets_freeze", map[string]any{"spreadsheet_id": "selected-sheet", "sheet": "Sheet1", "cols": 3}, freezeAuth); !result.OK || rows != 0 || cols != 3 {
				t.Fatalf("column freeze did not preserve rows: %s", result.Body)
			}

			reorderAuth := sheetsLocalAuthorization("sheets_reorder_tab", scope, true, false)
			if result := call("sheets_reorder_tab", map[string]any{"spreadsheet_id": "selected-sheet", "tab": "Sheet1", "to": 2}, reorderAuth); !result.OK || !slices.Equal(order, []int64{1, 2, 0}) || !strings.Contains(string(result.Result), `"index":2`) {
				t.Fatalf("forward move failed: %s order=%v", result.Body, order)
			}

			if result := call("sheets_reorder_tab", map[string]any{"spreadsheet_id": "selected-sheet", "tab": "0", "to": 0}, reorderAuth); !result.OK || !slices.Equal(order, []int64{0, 1, 2}) {
				t.Fatalf("zero-index move failed: %s order=%v", result.Body, order)
			}

			if writes != 4 {
				t.Fatalf("writes=%d, want4", writes)
			}

			if result := call("sheets_reorder_tab", map[string]any{"spreadsheet_id": "selected-sheet", "tab": "Sheet1", "to": 999}, reorderAuth); result.OK || writes != 4 || !slices.Equal(order, []int64{0, 1, 2}) {
				t.Fatal("out-of-range move mutated state")
			}

			if result := call("sheets_freeze", map[string]any{"spreadsheet_id": "selected-sheet", "sheet": "Missing", "rows": 0}, freezeAuth); result.OK || writes != 4 {
				t.Fatal("unknown tab fell back to first tab")
			}

			before := googleCalls

			for name, args := range map[string]map[string]any{
				"sheets_freeze":      freeze,
				"sheets_reorder_tab": {"spreadsheet_id": "selected-sheet", "tab": "Sheet1", "to": 1},
			} {
				for _, change := range []func(*ToolAuthorization){
					func(a *ToolAuthorization) { a.AllowWrite = false },
					func(a *ToolAuthorization) { a.Scopes = sheetsReadScope },
					func(a *ToolAuthorization) { a.Resource = nil },
					func(a *ToolAuthorization) { a.Resource.Enabled = false },
					func(a *ToolAuthorization) { a.AllowedTools = []string{"sheets_metadata"} },
					func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-sheet" },
				} {
					auth := sheetsLocalAuthorization(name, scope, true, false)
					change(&auth)

					if result := call(name, args, auth); result.OK || googleCalls != before {
						t.Fatalf("unauthorized layout reached Google: %s", result.Body)
					}
				}
			}
		})
	}
}
