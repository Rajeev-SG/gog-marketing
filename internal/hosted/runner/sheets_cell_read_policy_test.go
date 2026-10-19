package runner

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const (
	cellReadFieldsNotes           = "sheets(properties(title),data(startRow,startColumn,rowData(values(note,formattedValue))))"
	cellReadFieldsLinks           = "sheets(properties(title),data(startRow,startColumn,rowData(values(hyperlink,formattedValue,userEnteredFormat(textFormat(link(uri))),textFormatRuns(format(link(uri)))))))"
	cellReadFieldsValidationGrid  = "sheets(properties(title),data(startRow,startColumn,rowData(values(dataValidation))))"
	cellReadFieldsFormatUser      = "sheets(properties(title),data(startRow,startColumn,rowData(values(userEnteredFormat,formattedValue))))"
	cellReadFieldsFormatEffective = "sheets(properties(title),data(startRow,startColumn,rowData(values(effectiveFormat,formattedValue))))"
	cellReadFieldsCatalog         = "sheets(properties(sheetId,title,index,gridProperties(rowCount,columnCount))),namedRanges(namedRangeId,name,range)"
	cellReadFieldsValidationTable = "sheets(properties(sheetId),tables(tableId,range,rowsProperties(footerColorStyle),columnProperties(columnIndex,columnName,columnType,dataValidationRule(condition(type,values(userEnteredValue))))))"
	cellReadFixtureGrid           = `{"sheets":[{"properties":{"sheetId":2,"title":"Revenue Tab"},"data":[{"startRow":2,"startColumn":1,"rowData":[{"values":[{"formattedValue":"$12.00","note":"Review","hyperlink":"https://foreign.test/direct","userEnteredFormat":{"numberFormat":{"type":"NUMBER","pattern":"0.0"},"textFormat":{"link":{"uri":"https://foreign.test/direct"}}},"effectiveFormat":{"numberFormat":{"type":"CURRENCY","pattern":"$0.00"}},"textFormatRuns":[{"startIndex":0,"format":{"link":{"uri":"https://foreign.test/rich"}}}],"dataValidation":{"condition":{"type":"ONE_OF_LIST","values":[{"userEnteredValue":"Alpha"}]},"strict":true,"showCustomUi":true}}]},{"values":[{}]}]}]}]}`
	cellReadFixtureCatalog        = `{"namedRanges":[{"namedRangeId":"range-1","name":"ReviewRange","range":{"sheetId":2,"startRowIndex":2,"endRowIndex":4,"startColumnIndex":1,"endColumnIndex":2}}],"sheets":[{"properties":{"sheetId":2,"title":"Revenue Tab","index":0,"gridProperties":{"rowCount":10,"columnCount":10}}}]}`
	cellReadFixtureTables         = `{"sheets":[{"properties":{"sheetId":2},"tables":[{"tableId":"table-1","range":{"sheetId":2,"startRowIndex":1,"endRowIndex":4,"startColumnIndex":1,"endColumnIndex":2},"columnProperties":[{"columnIndex":0,"columnName":"Review","columnType":"DROPDOWN","dataValidationRule":{"condition":{"type":"ONE_OF_LIST","values":[{"userEnteredValue":"TableAlpha"}]}}}]}]}]}`
)

func TestNativeSheetsCellMetadataReads(t *testing.T) {
	const a1 = "'Revenue Tab'!B3:B4"

	for _, scope := range []string{"spreadsheets.readonly", "spreadsheets", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			var observed []string
			expectedRange := a1
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Sheets = func(ctx context.Context, account string) (*sheets.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted Sheets cell identity lost")
					}
					transport := nativeFixtureTransport(func(request *http.Request) (*http.Response, error) {
						if request.Method != http.MethodGet || request.URL.Hostname() != "sheets.googleapis.com" || request.URL.Path != "/v4/spreadsheets/selected-sheet" {
							t.Errorf("foreign/mutating request %s %s", request.Method, request.URL)
						}
						query := request.URL.Query()
						fields := query.Get("fields")
						observed = append(observed, fields)
						var fixture string

						switch fields {
						case cellReadFieldsCatalog, cellReadFieldsValidationTable:
							if query.Has("ranges") || query.Has("includeGridData") {
								t.Errorf("metadata fetch exposed range/grid options %s", request.URL)
							}

							fixture = cellReadFixtureCatalog
							if fields == cellReadFieldsValidationTable {
								fixture = cellReadFixtureTables
							}
						case cellReadFieldsNotes, cellReadFieldsLinks, cellReadFieldsFormatUser, cellReadFieldsFormatEffective, cellReadFieldsValidationGrid:
							if !slices.Equal(query["ranges"], []string{expectedRange}) || query.Get("includeGridData") != "true" {
								t.Errorf("wrong selected range/view %s", request.URL)
							}
							fixture = cellReadFixtureGrid
						default:
							t.Errorf("unexpected metadata mask %q", fields)
							fixture = `{}`
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(fixture)), Request: request}, nil
					})

					return sheets.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			for _, test := range []struct {
				name, kind string
				args       map[string]any
				fields     []string
			}{
				{"sheets_notes", "notes", map[string]any{"spreadsheet_id": "selected-sheet", "range": a1}, []string{cellReadFieldsNotes}},
				{"sheets_links_get", "links", map[string]any{"spreadsheet_id": "selected-sheet", "range": a1}, []string{cellReadFieldsLinks}},
				{"sheets_read_format", "format-user", map[string]any{"spreadsheet_id": "selected-sheet", "range": a1, "effective": false}, []string{cellReadFieldsFormatUser}},
				{"sheets_read_format", "format-effective", map[string]any{"spreadsheet_id": "selected-sheet", "range": a1, "effective": true}, []string{cellReadFieldsFormatEffective}},
				{"sheets_validation_get", "validation", map[string]any{"spreadsheet_id": "selected-sheet", "range": a1}, []string{cellReadFieldsCatalog, cellReadFieldsValidationGrid, cellReadFieldsValidationTable}},
				{"sheets_validation_get", "validation", map[string]any{"spreadsheet_id": "selected-sheet", "range": "ReviewRange"}, []string{cellReadFieldsCatalog, cellReadFieldsValidationGrid, cellReadFieldsValidationTable}},
				{"sheets_validation_get", "validation", map[string]any{"spreadsheet_id": "selected-sheet", "range": "reviewrange"}, []string{cellReadFieldsCatalog, cellReadFieldsValidationGrid, cellReadFieldsValidationTable}},
				{"sheets_validation_get", "validation", map[string]any{"spreadsheet_id": "selected-sheet", "range": "range-1"}, []string{cellReadFieldsCatalog, cellReadFieldsValidationGrid, cellReadFieldsValidationTable}},
			} {
				expectedRange = a1
				if test.args["range"] != a1 {
					expectedRange = "ReviewRange"
				}

				before := len(observed)

				result := callSheetsDetail(t, harness, test.name, test.args, scope, nil)
				if !result.OK {
					t.Fatalf("%s %s: %s", test.name, test.kind, result.Body)
				}

				if !slices.Equal(observed[before:], test.fields) {
					t.Fatalf("%s fields/call count: got%q want%q", test.name, observed[before:], test.fields)
				}

				checkSheetsCellMetadataResult(t, test.kind, result.Result)
			}

			denials := []func(*ToolAuthorization){
				func(a *ToolAuthorization) { a.Resource = nil }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-sheet" }, func(a *ToolAuthorization) { a.AllowedTools = []string{"sheets_get"} }, func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/drive.readonly"} },
			}
			for _, name := range []string{"sheets_notes", "sheets_links_get", "sheets_read_format", "sheets_validation_get"} {
				for _, change := range denials {
					before := len(observed)

					result := callSheetsDetail(t, harness, name, map[string]any{"spreadsheet_id": "selected-sheet", "range": a1}, scope, change)
					if result.OK || len(observed) != before {
						t.Fatalf("denied%s reached Google: %s", name, result.Body)
					}
				}
			}
		})
	}
}

func TestSheetsCellMetadataExactGuard(t *testing.T) {
	for _, name := range []string{"sheets_notes", "sheets_links_get", "sheets_read_format", "sheets_validation_get"} {
		invocation := &ToolInvocation{Name: name, Arguments: map[string]any{"spreadsheet_id": "selected-sheet"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-sheet"}}}
		guard := nativeRequestGuard(invocation)

		root := "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet"
		for _, test := range []struct {
			method, url string
			allowed     bool
		}{{http.MethodGet, root, true}, {http.MethodHead, root, false}, {http.MethodPost, root, false}, {http.MethodPost, root + ":batchUpdate", false}, {http.MethodGet, root + "/values/A1", false}, {http.MethodGet, "https://sheets.googleapis.com/v4/spreadsheets/foreign-sheet", false}, {http.MethodGet, "https://foreign.test/v4/spreadsheets/selected-sheet", false}} {
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
