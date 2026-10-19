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

	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var errSheetsRangeNoteFixture = errors.New("unexpected Sheets range/note fixture request")

func TestNativeSheetsRangeNoteWrites(t *testing.T) {
	const fields = "sheets(properties(sheetId,title,index,gridProperties(rowCount,columnCount))),namedRanges(namedRangeId,name,range)"
	const noteFields = "sheets(properties(title),data(startRow,startColumn,rowData(values(note,formattedValue))))"
	first := &sheets.GridRange{SheetId: 0, EndRowIndex: 2, EndColumnIndex: 2}
	second := &sheets.GridRange{SheetId: 7, StartRowIndex: 1, EndRowIndex: 3, StartColumnIndex: 1, EndColumnIndex: 3}
	third := &sheets.GridRange{SheetId: 0, StartRowIndex: 3, EndRowIndex: 5, StartColumnIndex: 3, EndColumnIndex: 5}

	for _, scope := range []string{"spreadsheets", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			ranges := map[string]*sheets.NamedRange{"range-1": {NamedRangeId: "range-1", Name: "Existing", Range: first}, "keep-id": {NamedRangeId: "keep-id", Name: "Keep", Range: second}, "dup-a": {NamedRangeId: "dup-a", Name: "Duplicate", Range: first}, "dup-b": {NamedRangeId: "dup-b", Name: "DUPLICATE", Range: first}}
			notes := map[string]string{"99/0/0": "Protected", "0/4/4": "OtherCell"}
			var events []string
			var expected sheetsRangeNoteExpectation
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Sheets = func(ctx context.Context, account string) (*sheets.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted Sheets range/note identity lost")
					}
					transport := nativeFixtureTransport(func(request *http.Request) (*http.Response, error) {
						events = append(events, request.Method)
						if request.URL.Hostname() != "sheets.googleapis.com" {
							return nil, errSheetsRangeNoteFixture
						}
						var response any

						switch {
						case request.Method == http.MethodGet && request.URL.Path == "/v4/spreadsheets/selected-sheet":
							if request.URL.Query().Get("fields") == noteFields {
								if !slices.Equal(request.URL.Query()["ranges"], []string{"First!A1:B2"}) {
									t.Errorf("wrong note readback range %s", request.URL)
								}
								data := &sheets.GridData{RowData: []*sheets.RowData{}}

								for row := 0; row < 2; row++ {
									values := []*sheets.CellData{}
									for col := 0; col < 2; col++ {
										values = append(values, &sheets.CellData{Note: notes[fmt.Sprintf("0/%d/%d", row, col)]})
									}

									data.RowData = append(data.RowData, &sheets.RowData{Values: values})
								}
								response = &sheets.Spreadsheet{Sheets: []*sheets.Sheet{{Properties: &sheets.SheetProperties{Title: "First"}, Data: []*sheets.GridData{data}}}}
							} else {
								if request.URL.Query().Get("fields") != fields {
									t.Errorf("unexpected range/note metadata mask %s", request.URL)
								}

								ids := make([]string, 0, len(ranges))
								for id := range ranges {
									ids = append(ids, id)
								}

								sort.Strings(ids)

								catalog := &sheets.Spreadsheet{SpreadsheetId: "selected-sheet", Sheets: []*sheets.Sheet{{Properties: &sheets.SheetProperties{SheetId: 0, Title: "First", GridProperties: &sheets.GridProperties{RowCount: 10, ColumnCount: 10}}}, {Properties: &sheets.SheetProperties{SheetId: 7, Title: "Revenue Tab", GridProperties: &sheets.GridProperties{RowCount: 10, ColumnCount: 10}}}}}
								for _, id := range ids {
									catalog.NamedRanges = append(catalog.NamedRanges, ranges[id])
								}
								response = catalog
							}
						case request.Method == http.MethodPost && request.URL.Path == "/v4/spreadsheets/selected-sheet:batchUpdate":
							body, err := io.ReadAll(request.Body)
							if err != nil {
								return nil, fmt.Errorf("read range/note body: %w", err)
							}
							var batch sheets.BatchUpdateSpreadsheetRequest
							var envelope struct {
								Requests []map[string]json.RawMessage `json:"requests"`
							}

							var raw map[string]json.RawMessage
							if err := json.Unmarshal(body, &raw); err != nil {
								return nil, fmt.Errorf("decode range/note envelope: %w", err)
							}

							if err := json.Unmarshal(body, &batch); err != nil {
								return nil, fmt.Errorf("decode range/note SDK: %w", err)
							}

							if err := json.Unmarshal(body, &envelope); err != nil {
								return nil, fmt.Errorf("decode range/note operation: %w", err)
							}

							if len(raw) != 1 || len(batch.Requests) != 1 || len(envelope.Requests) != 1 || len(envelope.Requests[0]) != 1 {
								return nil, errSheetsRangeNoteFixture
							}
							operation := batch.Requests[0]
							checkSheetsRangeNoteSDKRequest(t, operation, expected)
							replies := []any{map[string]any{}}

							switch {
							case operation.AddNamedRange != nil:
								created := *operation.AddNamedRange.NamedRange
								created.NamedRangeId = "created-1"
								ranges[created.NamedRangeId] = &created
								replies = []any{map[string]any{"addNamedRange": map[string]any{"namedRange": &created}}}
							case operation.UpdateNamedRange != nil:
								update := operation.UpdateNamedRange.NamedRange

								stored := ranges[update.NamedRangeId]
								if stored == nil {
									return nil, errSheetsRangeNoteFixture
								}

								for _, field := range strings.Split(operation.UpdateNamedRange.Fields, ",") {
									switch field {
									case "name":
										stored.Name = update.Name
									case "range":
										stored.Range = update.Range
									default:
										return nil, errSheetsRangeNoteFixture
									}
								}
							case operation.DeleteNamedRange != nil:
								delete(ranges, operation.DeleteNamedRange.NamedRangeId)
							case operation.RepeatCell != nil:
								var rawOperation struct {
									Cell map[string]json.RawMessage `json:"cell"`
								}
								if err := json.Unmarshal(envelope.Requests[0]["repeatCell"], &rawOperation); err != nil {
									return nil, fmt.Errorf("decode note cell: %w", err)
								}

								if _, ok := rawOperation.Cell["note"]; !ok || len(rawOperation.Cell) != 1 {
									t.Error("note field missing or unrelated cell values exposed")
								}

								span := operation.RepeatCell.Range
								for row := span.StartRowIndex; row < span.EndRowIndex; row++ {
									for col := span.StartColumnIndex; col < span.EndColumnIndex; col++ {
										key := fmt.Sprintf("%d/%d/%d", span.SheetId, row, col)
										if operation.RepeatCell.Cell.Note == "" {
											delete(notes, key)
										} else {
											notes[key] = operation.RepeatCell.Cell.Note
										}
									}
								}
							default:
								return nil, errSheetsRangeNoteFixture
							}
							response = map[string]any{"spreadsheetId": "selected-sheet", "replies": replies}
						default:
							return nil, errSheetsRangeNoteFixture
						}

						encoded, err := json.Marshal(response)
						if err != nil {
							return nil, fmt.Errorf("encode range/note response: %w", err)
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded))), Request: request}, nil
					})

					return sheets.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			for _, test := range []struct {
				args       map[string]any
				want       sheetsRangeNoteExpectation
				methods    []string
				resultName string
			}{
				{map[string]any{"spreadsheet_id": "selected-sheet", "name": "NewRange", "range": "First!A1:B2"}, sheetsRangeNoteExpectation{tool: "sheets_named_ranges_add", name: "NewRange", grid: first}, []string{"GET", "POST"}, "NewRange"},
				{map[string]any{"spreadsheet_id": "selected-sheet", "name_or_id": "created-1", "name": "Renamed"}, sheetsRangeNoteExpectation{tool: "sheets_named_ranges_update", id: "created-1", name: "Renamed", fields: "name"}, []string{"GET", "POST", "GET"}, "Renamed"},
				{map[string]any{"spreadsheet_id": "selected-sheet", "name_or_id": "renamed", "range": "'Revenue Tab'!B2:C3"}, sheetsRangeNoteExpectation{tool: "sheets_named_ranges_update", id: "created-1", fields: "range", grid: second}, []string{"GET", "POST", "GET"}, "Renamed"},
				{map[string]any{"spreadsheet_id": "selected-sheet", "name_or_id": "created-1", "name": "FinalRange", "range": "First!D4:E5"}, sheetsRangeNoteExpectation{tool: "sheets_named_ranges_update", id: "created-1", name: "FinalRange", fields: "name,range", grid: third}, []string{"GET", "POST", "GET"}, "FinalRange"},
				{map[string]any{"spreadsheet_id": "selected-sheet", "name_or_id": "created-1", "name": "", "range": "'Revenue Tab'!B2:C3"}, sheetsRangeNoteExpectation{tool: "sheets_named_ranges_update", id: "created-1", fields: "range", grid: second}, []string{"GET", "POST", "GET"}, "FinalRange"},
				{map[string]any{"spreadsheet_id": "selected-sheet", "name_or_id": "finalrange"}, sheetsRangeNoteExpectation{tool: "sheets_named_ranges_delete", id: "created-1"}, []string{"GET", "POST"}, ""},
				{map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!A1:B2", "note": "Line 1\n你好 🌍"}, sheetsRangeNoteExpectation{tool: "sheets_update_note", note: "Line 1\n你好 🌍", grid: first}, []string{"GET", "POST"}, ""},
				{map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!A1:B2", "note": " "}, sheetsRangeNoteExpectation{tool: "sheets_update_note", note: " ", grid: first}, []string{"GET", "POST"}, ""},
				{map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!A1:B2", "note": "=IMPORTRANGE(\"outside\", \"A1\")"}, sheetsRangeNoteExpectation{tool: "sheets_update_note", note: "=IMPORTRANGE(\"outside\", \"A1\")", grid: first}, []string{"GET", "POST"}, ""},
				{map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!A1:B2", "note": ""}, sheetsRangeNoteExpectation{tool: "sheets_update_note", note: "", grid: first}, []string{"GET", "POST"}, ""},
				{map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!A1:B2", "note": "@/tmp/not-a-file"}, sheetsRangeNoteExpectation{tool: "sheets_update_note", note: "@/tmp/not-a-file", grid: first}, []string{"GET", "POST"}, ""},
			} {
				expected = test.want
				before := len(events)

				result := callSheetsDetail(t, harness, expected.tool, test.args, scope, func(a *ToolAuthorization) {
					a.AllowWrite = true
					a.AllowDestructive = expected.tool == "sheets_named_ranges_delete" || expected.tool == "sheets_update_note" && expected.note == ""
				})
				if !result.OK || !slices.Equal(events[before:], test.methods) {
					t.Fatalf("%s outcome/sequence: %s events%v", expected.tool, result.Body, events[before:])
				}

				if test.resultName != "" {
					var output struct {
						NamedRange sheets.NamedRange `json:"namedRange"`
					}
					if err := json.Unmarshal(result.Result, &output); err != nil || output.NamedRange.NamedRangeId != "created-1" || output.NamedRange.Name != test.resultName {
						t.Fatalf("wrong named-range readback %s err=%v", result.Result, err)
					}
				}

				if expected.tool == "sheets_update_note" {
					read := callSheetsDetail(t, harness, "sheets_notes", map[string]any{"spreadsheet_id": "selected-sheet", "range": "First!A1:B2"}, scope, nil)
					var output struct {
						Notes []struct {
							Note string `json:"note"`
						} `json:"notes"`
					}

					if !read.OK {
						t.Fatal(read.Body)
					}

					if err := json.Unmarshal(read.Result, &output); err != nil {
						t.Fatal(err)
					}

					count := 4
					if expected.note == "" {
						count = 0
					}

					if len(output.Notes) != count {
						t.Fatalf("wrong note readback %s", read.Result)
					}

					for _, cell := range output.Notes {
						if cell.Note != expected.note {
							t.Fatalf("note changed in SDK readback %s", read.Result)
						}
					}
				}
			}

			if len(ranges) != 4 || ranges["range-1"].Name != "Existing" || ranges["keep-id"].Name != "Keep" || notes["99/0/0"] != "Protected" || notes["0/4/4"] != "OtherCell" || len(notes) != 6 {
				t.Fatalf("unrelated range/note state changed: ranges%v notes%v", ranges, notes)
			}

			for _, name := range []string{"sheets_named_ranges_update", "sheets_named_ranges_delete"} {
				for _, selector := range []string{"missing", "duplicate"} {
					before := len(events)

					args := map[string]any{"spreadsheet_id": "selected-sheet", "name_or_id": selector}
					if name == "sheets_named_ranges_update" {
						args["name"] = "Unused"
					}

					result := callSheetsDetail(t, harness, name, args, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = true })
					if result.OK || !slices.Equal(events[before:], []string{"GET"}) {
						t.Fatalf("unresolved range mutated: %s events%v", result.Body, events[before:])
					}
				}
			}
			denials := []func(*ToolAuthorization){func(a *ToolAuthorization) { a.AllowWrite = false }, func(a *ToolAuthorization) {
				a.Scopes = []string{"https://www.googleapis.com/auth/spreadsheets.readonly"}
			}, func(a *ToolAuthorization) { a.Resource = nil }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-sheet" }, func(a *ToolAuthorization) { a.AllowedTools = []string{"sheets_metadata"} }}

			valid := map[string]map[string]any{"sheets_named_ranges_add": {"spreadsheet_id": "selected-sheet", "name": "Denied", "range": "First!A1:B2"}, "sheets_named_ranges_update": {"spreadsheet_id": "selected-sheet", "name_or_id": "range-1", "name": "Denied"}, "sheets_named_ranges_delete": {"spreadsheet_id": "selected-sheet", "name_or_id": "range-1"}, "sheets_update_note": {"spreadsheet_id": "selected-sheet", "range": "First!A1:B2", "note": "Denied"}}
			for name, args := range valid {
				for _, change := range denials {
					before := len(events)

					result := callSheetsDetail(t, harness, name, args, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = true; change(a) })
					if result.OK || len(events) != before {
						t.Fatalf("unauthorized%s reached Google: %s", name, result.Body)
					}
				}
			}

			for name, args := range map[string]map[string]any{"sheets_named_ranges_delete": valid["sheets_named_ranges_delete"], "sheets_update_note": {"spreadsheet_id": "selected-sheet", "range": "First!A1:B2", "note": ""}} {
				before := len(events)

				result := callSheetsDetail(t, harness, name, args, scope, func(a *ToolAuthorization) { a.AllowWrite = true; a.AllowDestructive = false })
				if result.OK || len(events) != before {
					t.Fatalf("destructive%s reached Google: %s", name, result.Body)
				}
			}
		})
	}
}

func TestSheetsRangeNoteExactMethodGuard(t *testing.T) {
	root := "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet"
	tests := []struct {
		method, url string
		allowed     bool
	}{
		{http.MethodGet, root, true},
		{http.MethodPost, root + ":batchUpdate", true},
		{http.MethodPost, root, false},
		{http.MethodGet, root + ":batchUpdate", false},
		{http.MethodDelete, root, false},
		{http.MethodGet, root + "/values/A1", false},
		{http.MethodPost, "https://sheets.googleapis.com/v4/spreadsheets/foreign-sheet:batchUpdate", false},
		{http.MethodPost, "https://foreign.test/v4/spreadsheets/selected-sheet:batchUpdate", false},
	}

	for _, name := range []string{"sheets_named_ranges_add", "sheets_named_ranges_update", "sheets_named_ranges_delete", "sheets_update_note"} {
		invocation := &ToolInvocation{Name: name, Arguments: map[string]any{"spreadsheet_id": "selected-sheet"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-sheet"}}}
		guard := nativeRequestGuard(invocation)

		for _, test := range tests {
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
