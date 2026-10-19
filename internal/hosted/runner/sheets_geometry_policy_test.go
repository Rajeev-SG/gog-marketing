package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var errSheetsGeometryFixture = errors.New("unexpected Sheets geometry fixture request")

func TestNativeSheetsGeometryWrites(t *testing.T) {
	const metadata = `{"spreadsheetId":"selected-sheet","sheets":[{"properties":{"sheetId":0,"title":"First","index":0,"gridProperties":{"rowCount":10,"columnCount":10}}},{"properties":{"sheetId":7,"title":"Revenue Tab","index":1,"gridProperties":{"rowCount":10,"columnCount":10}}},{"properties":{"sheetId":99,"title":"Untouched","index":2}}]}`
	const fields = "sheets(properties(sheetId,title,index,gridProperties(rowCount,columnCount))),namedRanges(namedRangeId,name,range)"

	for _, scope := range []string{"spreadsheets", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			pixels := map[string]int64{"99/ROWS/0": 77}
			autofit := map[string]bool{}
			merges := map[string]string{"protected": "MERGE_ALL"}
			calls, posts := 0, 0
			var expected sheetsGeometryExpectation
			keyGrid := func(grid *sheets.GridRange) string {
				return fmt.Sprintf("%d/%d/%d/%d/%d", grid.SheetId, grid.StartRowIndex, grid.EndRowIndex, grid.StartColumnIndex, grid.EndColumnIndex)
			}
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Sheets = func(ctx context.Context, account string) (*sheets.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted Sheets geometry identity lost")
					}
					transport := nativeFixtureTransport(func(request *http.Request) (*http.Response, error) {
						calls++

						if request.URL.Hostname() != "sheets.googleapis.com" {
							return nil, errSheetsGeometryFixture
						}
						response := metadata

						switch {
						case request.Method == http.MethodGet && request.URL.Path == "/v4/spreadsheets/selected-sheet":
							if request.URL.Query().Get("fields") != fields {
								t.Errorf("unexpected geometry metadata mask %s", request.URL)
							}
						case request.Method == http.MethodPost && request.URL.Path == "/v4/spreadsheets/selected-sheet:batchUpdate":
							body, err := io.ReadAll(request.Body)
							if err != nil {
								return nil, fmt.Errorf("read geometry body: %w", err)
							}
							var raw map[string]json.RawMessage
							var batch sheets.BatchUpdateSpreadsheetRequest
							var envelope struct {
								Requests []map[string]json.RawMessage `json:"requests"`
							}

							if err := json.Unmarshal(body, &raw); err != nil {
								return nil, fmt.Errorf("decode geometry fields: %w", err)
							}

							if err := json.Unmarshal(body, &batch); err != nil {
								return nil, fmt.Errorf("decode geometry SDK: %w", err)
							}

							if err := json.Unmarshal(body, &envelope); err != nil {
								return nil, fmt.Errorf("decode geometry operations: %w", err)
							}

							if len(raw) != 1 || len(batch.Requests) != 1 || batch.Requests[0] == nil || len(envelope.Requests) != 1 || len(envelope.Requests[0]) != 1 {
								return nil, errSheetsGeometryFixture
							}
							operation := batch.Requests[0]
							checkSheetsGeometrySDKRequest(t, operation, expected)

							var rawOperation map[string]json.RawMessage
							for _, value := range envelope.Requests[0] {
								if err := json.Unmarshal(value, &rawOperation); err != nil {
									return nil, fmt.Errorf("decode geometry range: %w", err)
								}
							}

							rangeKey := "range"
							if expected.auto {
								rangeKey = "dimensions"
							}

							var rawRange map[string]json.RawMessage
							if err := json.Unmarshal(rawOperation[rangeKey], &rawRange); err != nil {
								return nil, fmt.Errorf("decode geometry range fields: %w", err)
							}

							if _, ok := rawRange["sheetId"]; !ok {
								t.Error("geometry omitted sheet id, including zero")
							}

							if expected.grid == nil && expected.start == 0 {
								if value, ok := rawRange["startIndex"]; !ok || string(value) != "0" {
									t.Error("geometry omitted literal zero startIndex")
								}
							}

							switch {
							case operation.MergeCells != nil:
								merges[keyGrid(operation.MergeCells.Range)] = operation.MergeCells.MergeType
							case operation.UnmergeCells != nil:
								delete(merges, keyGrid(operation.UnmergeCells.Range))
							default:
								var dimensions *sheets.DimensionRange
								if operation.AutoResizeDimensions != nil {
									dimensions = operation.AutoResizeDimensions.Dimensions
								} else {
									dimensions = operation.UpdateDimensionProperties.Range
								}

								for index := dimensions.StartIndex; index < dimensions.EndIndex; index++ {
									key := fmt.Sprintf("%d/%s/%d", dimensions.SheetId, dimensions.Dimension, index)
									if expected.auto {
										autofit[key] = true
									} else {
										pixels[key] = operation.UpdateDimensionProperties.Properties.PixelSize
									}
								}
							}

							posts++
							response = `{"spreadsheetId":"selected-sheet","replies":[{}]}`
						default:
							return nil, errSheetsGeometryFixture
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: request}, nil
					})

					return sheets.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			for _, test := range []struct {
				args map[string]any
				want sheetsGeometryExpectation
			}{
				{map[string]any{"spreadsheet_id": "selected-sheet", "columns": "C:A", "width": 120}, sheetsGeometryExpectation{tool: "sheets_resize_columns", axis: "COLUMNS", sheetID: 0, start: 0, end: 3, pixels: 120}},
				{map[string]any{"spreadsheet_id": "selected-sheet", "rows": "'Revenue Tab'!2:1", "height": 24, "auto": false}, sheetsGeometryExpectation{tool: "sheets_resize_rows", axis: "ROWS", sheetID: 7, start: 0, end: 2, pixels: 24}},
				{map[string]any{"spreadsheet_id": "selected-sheet", "columns": "'Revenue Tab'!C:B", "auto": true}, sheetsGeometryExpectation{tool: "sheets_resize_columns", axis: "COLUMNS", sheetID: 7, start: 1, end: 3, auto: true}},
				{map[string]any{"spreadsheet_id": "selected-sheet", "rows": "2:1", "auto": true}, sheetsGeometryExpectation{tool: "sheets_resize_rows", axis: "ROWS", sheetID: 0, start: 0, end: 2, auto: true}},
				{map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!A1:B2"}, sheetsGeometryExpectation{tool: "sheets_merge", mergeType: "MERGE_ALL", grid: &sheets.GridRange{SheetId: 0, EndRowIndex: 2, EndColumnIndex: 2}}},
				{map[string]any{"spreadsheet_id": "selected-sheet", "range": "'Revenue Tab'!A3:C4", "type": "MERGE_ROWS"}, sheetsGeometryExpectation{tool: "sheets_merge", mergeType: "MERGE_ROWS", grid: &sheets.GridRange{SheetId: 7, StartRowIndex: 2, EndRowIndex: 4, EndColumnIndex: 3}}},
				{map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!D1:E3", "type": "MERGE_COLUMNS"}, sheetsGeometryExpectation{tool: "sheets_merge", mergeType: "MERGE_COLUMNS", grid: &sheets.GridRange{SheetId: 0, EndRowIndex: 3, StartColumnIndex: 3, EndColumnIndex: 5}}},
				{map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!A1:B2"}, sheetsGeometryExpectation{tool: "sheets_unmerge", grid: &sheets.GridRange{SheetId: 0, EndRowIndex: 2, EndColumnIndex: 2}}},
			} {
				expected = test.want
				beforeCalls, beforePosts := calls, posts

				result := callSheetsDetail(t, harness, expected.tool, test.args, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = expected.tool == "sheets_merge" })
				if !result.OK || calls != beforeCalls+2 || posts != beforePosts+1 {
					t.Fatalf("%s request count/outcome: %s calls=%d posts=%d", expected.tool, result.Body, calls, posts)
				}
			}

			if len(pixels) != 6 || pixels["0/COLUMNS/0"] != 120 || pixels["0/COLUMNS/2"] != 120 || pixels["7/ROWS/1"] != 24 || pixels["99/ROWS/0"] != 77 || len(autofit) != 4 || !autofit["7/COLUMNS/1"] || !autofit["0/ROWS/0"] || len(merges) != 3 || merges["protected"] != "MERGE_ALL" || merges["0/0/2/0/2"] != "" {
				t.Fatalf("geometry persistence/unrelated state wrong: pixels%v auto%v merges%v", pixels, autofit, merges)
			}

			denials := []func(*ToolAuthorization){func(a *ToolAuthorization) { a.AllowWrite = false }, func(a *ToolAuthorization) {
				a.Scopes = []string{"https://www.googleapis.com/auth/spreadsheets.readonly"}
			}, func(a *ToolAuthorization) { a.Resource = nil }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-sheet" }, func(a *ToolAuthorization) { a.AllowedTools = []string{"sheets_metadata"} }}
			for name, args := range map[string]map[string]any{
				"sheets_merge": {"spreadsheet_id": "selected-sheet", "range": "First!A1:B2"}, "sheets_unmerge": {"spreadsheet_id": "selected-sheet", "range": "First!A1:B2"}, "sheets_resize_columns": {"spreadsheet_id": "selected-sheet", "columns": "A:C", "width": 120}, "sheets_resize_rows": {"spreadsheet_id": "selected-sheet", "rows": "1:2", "auto": true},
			} {
				for _, change := range denials {
					before := calls

					result := callSheetsDetail(t, harness, name, args, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = true; change(a) })
					if result.OK || calls != before {
						t.Fatalf("unauthorized%s reached Google: %s", name, result.Body)
					}
				}
			}
			before := calls

			denied := callSheetsDetail(t, harness, "sheets_merge", map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!A1:B2"}, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = false })
			if denied.OK || calls != before {
				t.Fatal("merge without destructive grant reached Google")
			}
		})
	}
}

func TestSheetsGeometryExactMethodGuard(t *testing.T) {
	for _, name := range []string{"sheets_merge", "sheets_unmerge", "sheets_resize_columns", "sheets_resize_rows"} {
		invocation := &ToolInvocation{Name: name, Arguments: map[string]any{"spreadsheet_id": "selected-sheet"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-sheet"}}}
		guard := nativeRequestGuard(invocation)

		root := "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet"
		for _, test := range []struct {
			method, url string
			allowed     bool
		}{{http.MethodGet, root, true}, {http.MethodPost, root + ":batchUpdate", true}, {http.MethodPost, root, false}, {http.MethodGet, root + ":batchUpdate", false}, {http.MethodDelete, root, false}, {http.MethodGet, root + "/values/A1", false}, {http.MethodPost, "https://sheets.googleapis.com/v4/spreadsheets/foreign-sheet:batchUpdate", false}, {http.MethodPost, "https://foreign.test/v4/spreadsheets/selected-sheet:batchUpdate", false}} {
			request, err := http.NewRequestWithContext(context.Background(), test.method, test.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err := guard(request); (err == nil) != test.allowed {
				t.Errorf("%s %s %s allowed=%t err=%v", name, test.method, test.url, test.allowed, err)
			}
		}
	}
}
