package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeSheetsInventoryPolicySignedExecution(t *testing.T) {
	const fixtureBody = `{"spreadsheetId":"selected-sheet","properties":{"title":"Revenue workbook","spreadsheetId":"selected-sheet"},"namedRanges":[{"namedRangeId":"range-1","name":"RevenueRange","range":{"sheetId":0,"startRowIndex":0,"endRowIndex":3,"startColumnIndex":0,"endColumnIndex":2}}],"sheets":[{"properties":{"sheetId":0,"title":"Sheet1","index":0,"gridProperties":{"rowCount":10,"columnCount":10}},"charts":[{"chartId":73,"spec":{"title":"RevenueChart","basicChart":{"chartType":"COLUMN"}},"position":{"overlayPosition":{"anchorCell":{"sheetId":0,"rowIndex":0,"columnIndex":0}}}}],"tables":[{"tableId":"table-1","name":"RevenueTable","range":{"sheetId":0,"startRowIndex":0,"endRowIndex":3,"startColumnIndex":0,"endColumnIndex":2}}],"bandedRanges":[{"bandedRangeId":74,"range":{"sheetId":0,"startRowIndex":0,"endRowIndex":3,"startColumnIndex":0,"endColumnIndex":2}}],"conditionalFormats":[{"ranges":[{"sheetId":0,"startRowIndex":0,"endRowIndex":3,"startColumnIndex":0,"endColumnIndex":2}],"booleanRule":{"condition":{"type":"TEXT_CONTAINS","values":[{"userEnteredValue":"Ready"}]},"format":{"backgroundColor":{"red":1}}}}]}]}`

	tests := []struct {
		name    string
		tool    string
		fields  string
		needles []string
	}{
		{
			name:    "sheets_chart_list",
			tool:    "sheets_chart_list",
			fields:  "sheets(charts(chartId,spec(title,basicChart(chartType)),position(overlayPosition(anchorCell))),properties(sheetId,title))",
			needles: []string{`"chartId":73`, `"title":"RevenueChart"`, `"type":"COLUMN"`},
		},
		{
			name:    "sheets_named_ranges_list",
			tool:    "sheets_named_ranges_list",
			fields:  "sheets(properties(sheetId,title,index,gridProperties(rowCount,columnCount))),namedRanges(namedRangeId,name,range)",
			needles: []string{`"namedRangeId":"range-1"`, `"name":"RevenueRange"`},
		},
		{
			name:    "sheets_table_list",
			tool:    "sheets_table_list",
			fields:  "sheets(properties(sheetId,title),tables(tableId,name,range,rowsProperties(footerColorStyle),columnProperties(columnIndex,columnName,columnType,dataValidationRule)))",
			needles: []string{`"tableId":"table-1"`, `"name":"RevenueTable"`},
		},
		{
			name:    "sheets_banding_list",
			tool:    "sheets_banding_list",
			fields:  "sheets(properties(sheetId,title),bandedRanges)",
			needles: []string{`"bandedRangeId":74`},
		},
		{
			name:    "sheets_conditional_format_list",
			tool:    "sheets_conditional_format_list",
			fields:  "sheets(properties(sheetId,title),conditionalFormats)",
			needles: []string{"Ready", "TEXT_CONTAINS"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64

			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Sheets = func(ctx context.Context, account string) (*sheets.Service, error) {
					if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
						t.Error("trusted sheets execution identity was lost")
					}

					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)

						if r.Method != http.MethodGet || r.URL.Hostname() != "sheets.googleapis.com" || r.URL.Path != "/v4/spreadsheets/selected-sheet" {
							t.Errorf("foreign or mutating sheets request reached Google: %s %s", r.Method, r.URL)
						}

						if got := r.URL.Query().Get("fields"); got != test.fields {
							t.Errorf("native fields: got %q want %q", got, test.fields)
						}

						return &http.Response{
							StatusCode: 200,
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body:       io.NopCloser(strings.NewReader(fixtureBody)),
							Request:    r,
						}, nil
					})

					return sheets.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			call := func(tool string, scope string, args map[string]any, resource *ResourceGrantInput, allowedTool string) capturedResponse {
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{
						Name: tool, Arguments: args,
						Authorization: ToolAuthorization{
							Service: "sheets", AllowedTools: []string{allowedTool},
							Scopes:   []string{"https://www.googleapis.com/auth/" + scope},
							Resource: resource,
						},
					}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) {
					claims["operation"] = OperationToolCall
				}), body)
			}

			args := map[string]any{"spreadsheet_id": "selected-sheet"}
			resource := &ResourceGrantInput{Service: "sheets", ResourceType: "file", ResourceID: "selected-sheet", Enabled: true}
			scopes := []string{"spreadsheets.readonly", "spreadsheets", "drive.file"}

			for _, scope := range scopes {
				ok := call(test.tool, scope, args, resource, test.tool)
				if !ok.OK {
					t.Fatalf("%s under %s failed: %s", test.tool, scope, ok.Body)
				}

				for _, needle := range test.needles {
					if !strings.Contains(string(ok.Result), needle) {
						t.Fatalf("%s result missing %q: %s", test.tool, needle, ok.Result)
					}
				}
			}

			before := calls.Load()

			for _, denial := range []struct {
				name     string
				args     map[string]any
				resource *ResourceGrantInput
				scope    string
				tool     string
				allowed  string
			}{
				{name: "missing resource", args: args, resource: nil, scope: "spreadsheets.readonly", tool: test.tool, allowed: test.tool},
				{name: "disabled resource", args: args, resource: &ResourceGrantInput{Service: "sheets", ResourceType: "file", ResourceID: "selected-sheet", Enabled: false}, scope: "spreadsheets.readonly", tool: test.tool, allowed: test.tool},
				{name: "foreign spreadsheet id", args: map[string]any{"spreadsheet_id": "foreign-sheet"}, resource: resource, scope: "spreadsheets.readonly", tool: test.tool, allowed: test.tool},
				{name: "wrong tool grant", args: args, resource: resource, scope: "spreadsheets.readonly", tool: test.tool, allowed: "sheets_metadata"},
				{name: "wrong scope", args: args, resource: resource, scope: "documents.readonly", tool: test.tool, allowed: test.tool},
				{name: "unsupported option", args: map[string]any{"spreadsheet_id": "selected-sheet", "unsupported_option": true}, resource: resource, scope: "spreadsheets.readonly", tool: test.tool, allowed: test.tool},
			} {
				t.Run("denied/"+denial.name, func(t *testing.T) {
					denied := call(denial.tool, denial.scope, denial.args, denial.resource, denial.allowed)
					if denied.OK || calls.Load() != before {
						t.Fatalf("%s reached Google: %s", denial.name, denied.Body)
					}
				})
			}

			foreignTenant := nativeToolBody(t, func(req *Request) {
				req.TenantID = "tenant-b"
				req.Tool = &ToolInvocation{
					Name: test.tool, Arguments: args,
					Authorization: ToolAuthorization{
						Service: "sheets", AllowedTools: []string{test.tool},
						Scopes:   []string{"https://www.googleapis.com/auth/spreadsheets.readonly"},
						Resource: resource,
					},
				}
			})

			foreign := postExecute(t, harness.server, signInvocation(t, foreignTenant, func(claims map[string]any) {
				claims["operation"] = OperationToolCall
			}), foreignTenant)
			if foreign.OK || foreign.Status != http.StatusForbidden || calls.Load() != before {
				t.Fatal("foreign tenant reached sheets execution")
			}
		})
	}
}
