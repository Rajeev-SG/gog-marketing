package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const sheetsDetailFixture = `{"spreadsheetId":"selected-sheet","namedRanges":[{"namedRangeId":"range-1","name":"RevenueRange","range":{"sheetId":2,"startRowIndex":0,"endRowIndex":3,"startColumnIndex":0,"endColumnIndex":2}},{"namedRangeId":"range-2","name":"Duplicate"},{"namedRangeId":"range-3","name":"DUPLICATE"}],"sheets":[{"properties":{"sheetId":2,"title":"Revenue Tab","gridProperties":{"rowCount":10,"columnCount":10}},"charts":[{"chartId":73,"spec":{"title":"Revenue","basicChart":{"chartType":"COLUMN"}}},{"chartId":99,"spec":{"title":"Other"}}],"tables":[{"tableId":"table-1","name":"RevenueTable","range":{"sheetId":2,"startRowIndex":0,"endRowIndex":3,"startColumnIndex":0,"endColumnIndex":2},"columnProperties":[{"columnIndex":0,"columnName":"Revenue","columnType":"TEXT"}]},{"tableId":"table-2","name":"Duplicate"},{"tableId":"table-3","name":"DUPLICATE"}]}]}`

func newSheetsDetailHarness(t *testing.T, fields string, calls *int) *testHarness {
	t.Helper()

	return newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Sheets = func(ctx context.Context, account string) (*sheets.Service, error) {
			if account != "owner@example.com" {
				t.Errorf("account = %q, want owner@example.com", account)
			}

			if got := authclient.AccessTokenFromContext(ctx); got != "synthetic-ephemeral-token" {
				t.Errorf("token = %q, want synthetic-ephemeral-token", got)
			}

			if !googleapi.NoInputFromContext(ctx) {
				t.Error("expected no-input context for native sheets service")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				*calls++

				if r.Method != http.MethodGet || r.URL.Hostname() != "sheets.googleapis.com" || r.URL.Path != "/v4/spreadsheets/selected-sheet" {
					t.Errorf("unexpected request: %s %s%s", r.Method, r.Host, r.URL.Path)
				}

				if got := r.URL.Query().Get("fields"); got != fields {
					t.Errorf("fields = %q, want %q", got, fields)
				}

				return &http.Response{
					StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(sheetsDetailFixture)), Request: r,
				}, nil
			})

			return sheets.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
}

func callSheetsDetail(t *testing.T, h *testHarness, name string, args map[string]any, scope string, change func(*ToolAuthorization)) capturedResponse {
	t.Helper()

	authorization := ToolAuthorization{
		Service: "sheets", AllowedTools: []string{name},
		Scopes:   []string{"https://www.googleapis.com/auth/" + scope},
		Resource: &ResourceGrantInput{Service: "sheets", ResourceType: "file", ResourceID: "selected-sheet", Enabled: true},
	}
	if change != nil {
		change(&authorization)
	}
	body := nativeToolBody(t, func(req *Request) {
		req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: authorization}
	})

	return postExecute(t, h.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
}

func runNativeSheetsCase(t *testing.T, fields, name string, args map[string]any, wantOK bool, check func(*testing.T, capturedResponse)) {
	t.Helper()
	calls := 0
	h := newSheetsDetailHarness(t, fields, &calls)

	res := callSheetsDetail(t, h, name, args, "spreadsheets.readonly", nil)
	if res.OK != wantOK {
		t.Fatalf("OK = %v, want %v; body = %s", res.OK, wantOK, res.Body)
	}

	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}

	if check != nil {
		check(t, res)
	}
}

func TestNativeSheetsDetailReads(t *testing.T) {
	scopes := []string{"spreadsheets.readonly", "spreadsheets", "drive.file"}

	tools := []struct {
		name, fields string
		args         map[string]any
		check        func(*testing.T, capturedResponse)
	}{
		{
			"sheets_chart_get", "sheets(charts,properties(sheetId,title))",
			map[string]any{"spreadsheet_id": "selected-sheet", "chart_id": 73},
			func(t *testing.T, res capturedResponse) {
				t.Helper()

				chart := string(res.Result)
				if !strings.Contains(chart, `"chartId":73`) || !strings.Contains(chart, "Revenue") {
					t.Errorf("chart 73/Revenue missing: %s", chart)
				}

				if strings.Contains(chart, `"chartId":99`) || strings.Contains(chart, "Other") {
					t.Errorf("unrelated chart 99 leaked: %s", chart)
				}
			},
		},
		{
			"sheets_named_ranges_get", "sheets(properties(sheetId,title,index,gridProperties(rowCount,columnCount))),namedRanges(namedRangeId,name,range)",
			map[string]any{"spreadsheet_id": "selected-sheet", "name_or_id": "range-1"},
			func(t *testing.T, res capturedResponse) {
				t.Helper()

				var got struct {
					NamedRange struct {
						NamedRangeID string `json:"namedRangeId"`
						Name         string `json:"name"`
						SheetTitle   string `json:"sheetTitle"`
						A1           string `json:"a1"`
					} `json:"namedRange"`
				}
				if err := json.Unmarshal(res.Result, &got); err != nil {
					t.Fatalf("decode: %v", err)
				}

				nr := got.NamedRange
				if nr.NamedRangeID != "range-1" || nr.Name != "RevenueRange" || nr.SheetTitle != "Revenue Tab" || nr.A1 != "'Revenue Tab'!A1:B3" {
					t.Errorf("named range = %+v", nr)
				}
			},
		},
		{
			"sheets_table_get", "sheets(properties(sheetId,title),tables(tableId,name,range,rowsProperties(footerColorStyle),columnProperties(columnIndex,columnName,columnType,dataValidationRule)))",
			map[string]any{"spreadsheet_id": "selected-sheet", "table_id": "table-1"},
			func(t *testing.T, res capturedResponse) {
				t.Helper()

				var got struct {
					Table struct {
						TableID    string `json:"tableId"`
						Name       string `json:"name"`
						SheetTitle string `json:"sheetTitle"`
						A1         string `json:"a1"`
						Columns    []struct {
							ColumnName string `json:"columnName"`
							ColumnType string `json:"columnType"`
						} `json:"columns"`
					} `json:"table"`
				}
				if err := json.Unmarshal(res.Result, &got); err != nil {
					t.Fatalf("decode: %v", err)
				}

				tb := got.Table
				if tb.TableID != "table-1" || tb.Name != "RevenueTable" || tb.SheetTitle != "Revenue Tab" || tb.A1 != "'Revenue Tab'!A1:B3" {
					t.Errorf("table = %+v", tb)
				}

				if len(tb.Columns) != 1 || tb.Columns[0].ColumnName != "Revenue" || tb.Columns[0].ColumnType != "TEXT" {
					t.Errorf("columns = %+v", tb.Columns)
				}
			},
		},
	}
	for _, scope := range scopes {
		for _, tc := range tools {
			t.Run(scope+"/"+tc.name, func(t *testing.T) {
				calls := 0
				h := newSheetsDetailHarness(t, tc.fields, &calls)

				res := callSheetsDetail(t, h, tc.name, tc.args, scope, nil)
				if !res.OK {
					t.Fatalf("unexpected failure: %s", res.Body)
				}

				tc.check(t, res)

				if calls != 1 {
					t.Errorf("calls = %d, want 1", calls)
				}
			})
		}
	}

	for _, tc := range tools[1:] {
		key, name := "name_or_id", "revenuerange"
		if tc.name == "sheets_table_get" {
			key, name = "table_id", "revenuetable"
		}

		for _, selector := range []struct {
			value string
			ok    bool
		}{{name, true}, {"missing", false}, {"duplicate", false}} {
			t.Run(tc.name+"/"+selector.value, func(t *testing.T) {
				runNativeSheetsCase(t, tc.fields, tc.name, map[string]any{"spreadsheet_id": "selected-sheet", key: selector.value}, selector.ok, nil)
			})
		}
	}

	t.Run("chart-999-not-found", func(t *testing.T) {
		runNativeSheetsCase(t, tools[0].fields, "sheets_chart_get", map[string]any{"spreadsheet_id": "selected-sheet", "chart_id": 999}, false, nil)
	})

	denials := []struct {
		label  string
		change func(name string, a *ToolAuthorization)
	}{
		{"nil-resource", func(_ string, a *ToolAuthorization) { a.Resource = nil }},
		{"disabled-resource", func(_ string, a *ToolAuthorization) { a.Resource.Enabled = false }},
		{"foreign-resource", func(_ string, a *ToolAuthorization) { a.Resource.ResourceID = "other-sheet" }},
		{"wrong-tool", func(name string, a *ToolAuthorization) {
			a.AllowedTools = []string{"sheets_chart_get"}
			if name == "sheets_chart_get" {
				a.AllowedTools = []string{"sheets_named_ranges_get"}
			}
		}},
		{"broad-scope", func(_ string, a *ToolAuthorization) {
			a.Scopes = []string{"https://www.googleapis.com/auth/drive.readonly"}
		}},
	}
	for _, tc := range tools {
		for _, d := range denials {
			t.Run(tc.name+"/"+d.label, func(t *testing.T) {
				calls := 0
				h := newSheetsDetailHarness(t, tc.fields, &calls)

				res := callSheetsDetail(t, h, tc.name, tc.args, "spreadsheets.readonly", func(a *ToolAuthorization) { d.change(tc.name, a) })
				if res.OK {
					t.Errorf("denial %s unexpectedly succeeded", d.label)
				}

				if calls != 0 {
					t.Errorf("calls = %d, want 0", calls)
				}
			})
		}
	}
}

func TestSheetsDetailExactGuard(t *testing.T) {
	for _, name := range []string{"sheets_chart_get", "sheets_named_ranges_get", "sheets_table_get"} {
		invocation := &ToolInvocation{
			Name: name, Arguments: map[string]any{"spreadsheet_id": "selected-sheet"},
			Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-sheet"}},
		}
		guard := nativeRequestGuard(invocation)

		cases := []struct {
			label, method, url string
			want               bool
		}{
			{"get-root-allowed", http.MethodGet, "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet", true},
			{"head-root-denied", http.MethodHead, "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet", false},
			{"post-root-denied", http.MethodPost, "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet", false},
			{"post-batchupdate-denied", http.MethodPost, "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet:batchUpdate", false},
			{"get-values-denied", http.MethodGet, "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet/values/A1", false},
			{"foreign-sheet-denied", http.MethodGet, "https://sheets.googleapis.com/v4/spreadsheets/other-sheet", false},
			{"foreign-host-denied", http.MethodGet, "https://evil.example.com/v4/spreadsheets/selected-sheet", false},
		}
		for _, tc := range cases {
			t.Run(tc.label, func(t *testing.T) {
				req, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
				if err != nil {
					t.Fatalf("build request: %v", err)
				}

				if allowed := guard(req) == nil; allowed != tc.want {
					t.Errorf("guard allowed = %v, want %v", allowed, tc.want)
				}
			})
		}
	}
}
