package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var (
	sheetsCreateScopeSpreadsheets = []string{"https://www.googleapis.com/auth/spreadsheets"}
	sheetsCreateScopeDriveFile    = []string{"https://www.googleapis.com/auth/drive.file"}
)

// TestSignedNativeSheetsCreateBlankOnlyTitleWithoutResourceGrant proves the
// fixed sheets_create_blank tool issues exactly one POST /v4/spreadsheets
// with a properties.title-only body under either write scope, denies missing
// tool/write/scope grants, unsupported parent/template/sheets arguments,
// malformed titles and foreign tenants before Google, and never turns the
// created spreadsheet into an automatic read grant.
func TestSignedNativeSheetsCreateBlankOnlyTitleWithoutResourceGrant(t *testing.T) {
	const (
		createdSheetID = "new-blank-sheet"
		createdTitle   = "Quarterly plan"
	)

	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices = app.Services{Sheets: func(ctx context.Context, account string) (*sheets.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
				t.Error("trusted execution identity was lost")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodPost || r.URL.Hostname() != "sheets.googleapis.com" || r.URL.Path != "/v4/spreadsheets" {
					t.Errorf("unexpected native Sheets create request: %s %s", r.Method, r.URL)
				}

				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}

				if len(body) != 1 {
					t.Errorf("create body carried fields beyond properties: %v", body)
				}

				var properties map[string]json.RawMessage
				if err := json.Unmarshal(body["properties"], &properties); err != nil {
					t.Error(err)
				}

				if len(properties) != 1 {
					t.Errorf("create properties carried fields beyond title: %v", properties)
				}

				var title string
				if err := json.Unmarshal(properties["title"], &title); err != nil {
					t.Error(err)
				}

				if title != createdTitle {
					t.Errorf("unexpected create title: %q", title)
				}

				response := `{"spreadsheetId":"` + createdSheetID + `","properties":{"title":"` + createdTitle + `"},"spreadsheetUrl":"https://docs.google.com/spreadsheets/d/` + createdSheetID + `"}`

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
			})

			return sheets.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}}
	})

	call := func(args map[string]any, scopes []string, allowedTools []string, allowWrite bool) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{
				Name:      "sheets_create_blank",
				Arguments: args,
				Authorization: ToolAuthorization{
					Service:      "sheets",
					AllowedTools: allowedTools,
					Scopes:       scopes,
					AllowWrite:   allowWrite,
				},
			}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	created := call(map[string]any{"title": createdTitle}, sheetsCreateScopeSpreadsheets, []string{"sheets_create_blank"}, true)
	if !created.OK || calls.Load() != 1 {
		t.Fatalf("native blank create failed: %s", created.Body)
	}

	if !strings.Contains(string(created.Result), createdSheetID) {
		t.Fatalf("create result missing spreadsheet id: %s", created.Result)
	}

	driveFile := call(map[string]any{"title": createdTitle}, sheetsCreateScopeDriveFile, []string{"sheets_create_blank"}, true)
	if !driveFile.OK || calls.Load() != 2 {
		t.Fatalf("native blank create under drive.file failed: %s", driveFile.Body)
	}

	before := calls.Load()

	denials := []struct {
		label  string
		args   map[string]any
		scopes []string
		tools  []string
		write  bool
	}{
		{label: "missing tool grant", args: map[string]any{"title": createdTitle}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_metadata"}, write: true},
		{label: "missing write permission", args: map[string]any{"title": createdTitle}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_create_blank"}, write: false},
		{label: "missing write scope", args: map[string]any{"title": createdTitle}, scopes: []string{"https://www.googleapis.com/auth/spreadsheets.readonly"}, tools: []string{"sheets_create_blank"}, write: true},
		{label: "unknown parent argument", args: map[string]any{"title": createdTitle, "parent": "folder-1"}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_create_blank"}, write: true},
		{label: "template argument", args: map[string]any{"title": createdTitle, "template": "template-1"}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_create_blank"}, write: true},
		{label: "sheets argument", args: map[string]any{"title": createdTitle, "sheets": "Tab A,Tab B"}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_create_blank"}, write: true},
		{label: "unicode control title", args: map[string]any{"title": "Quarterly\u0085plan"}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_create_blank"}, write: true},
		{label: "whitespace title", args: map[string]any{"title": "   "}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_create_blank"}, write: true},
		{label: "empty title", args: map[string]any{"title": ""}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_create_blank"}, write: true},
		{label: "oversized title", args: map[string]any{"title": strings.Repeat("a", 513)}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_create_blank"}, write: true},
		{label: "control character title", args: map[string]any{"title": "Quarterly\x07plan"}, scopes: sheetsCreateScopeSpreadsheets, tools: []string{"sheets_create_blank"}, write: true},
	}
	for _, denial := range denials {
		result := call(denial.args, denial.scopes, denial.tools, denial.write)
		if result.OK || calls.Load() != before {
			t.Fatalf("%s was not denied before Google: %s", denial.label, result.Body)
		}
	}

	foreignBody := nativeToolBody(t, func(req *Request) {
		req.TenantID = "tenant-b"
		req.Tool = &ToolInvocation{
			Name:      "sheets_create_blank",
			Arguments: map[string]any{"title": createdTitle},
			Authorization: ToolAuthorization{
				Service:      "sheets",
				AllowedTools: []string{"sheets_create_blank"},
				Scopes:       sheetsCreateScopeSpreadsheets,
				AllowWrite:   true,
			},
		}
	})

	foreign := postExecute(t, harness.server, signInvocation(t, foreignBody, func(claims map[string]any) { claims["operation"] = OperationToolCall }), foreignBody)
	if foreign.OK || calls.Load() != before {
		t.Fatalf("foreign tenant invocation was not denied before Google: %s", foreign.Body)
	}

	readBody := nativeToolBody(t, func(req *Request) {
		req.Tool = &ToolInvocation{
			Name:      "sheets_metadata",
			Arguments: map[string]any{"spreadsheet_id": createdSheetID},
			Authorization: ToolAuthorization{
				Service:      "sheets",
				AllowedTools: []string{"sheets_metadata"},
				Scopes:       []string{"https://www.googleapis.com/auth/spreadsheets.readonly"},
			},
		}
	})

	read := postExecute(t, harness.server, signInvocation(t, readBody, func(claims map[string]any) { claims["operation"] = OperationToolCall }), readBody)
	if read.OK || calls.Load() != before {
		t.Fatalf("created spreadsheet became read authority without a grant: %s", read.Body)
	}
}

func TestSheetsBlankCreateGuardRejectsOtherEndpointsAndMethods(t *testing.T) {
	guard := nativeRequestGuard(&ToolInvocation{
		Name: "sheets_create_blank", Arguments: map[string]any{"title": "Blank"},
		Authorization: ToolAuthorization{Service: "sheets", AllowedTools: []string{"sheets_create_blank"}, Scopes: sheetsCreateScopeSpreadsheets, AllowWrite: true},
	})

	for _, entry := range []struct {
		method  string
		url     string
		allowed bool
	}{
		{http.MethodPost, "https://sheets.googleapis.com/v4/spreadsheets", true},
		{http.MethodGet, "https://sheets.googleapis.com/v4/spreadsheets", false},
		{http.MethodPatch, "https://sheets.googleapis.com/v4/spreadsheets", false},
		{http.MethodPost, "http://sheets.googleapis.com/v4/spreadsheets", false},
		{http.MethodPost, "https://sheets.googleapis.com/v4/spreadsheets/foreign", false},
		{http.MethodPost, "https://sheets.googleapis.com/v4/spreadsheets/foreign:batchUpdate", false},
		{http.MethodPost, "https://sheets.googleapis.com/v4/spreadsheets?access_token=foreign", false},
	} {
		parsed, err := url.Parse(entry.url)
		if err != nil {
			t.Fatal(err)
		}

		allowed := guard(&http.Request{Method: entry.method, URL: parsed, Header: http.Header{}}) == nil
		if allowed != entry.allowed {
			t.Errorf("%s %s: allowed=%v, want %v", entry.method, entry.url, allowed, entry.allowed)
		}
	}
}
