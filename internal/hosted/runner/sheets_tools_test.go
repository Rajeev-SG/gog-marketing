package runner

import (
	"context"
	"encoding/json"
	"fmt"
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

const sheetsValuesRange = "Sheet1!A1:C"

var (
	sheetsWriteScope = []string{"https://www.googleapis.com/auth/spreadsheets"}
	sheetsReadScope  = []string{"https://www.googleapis.com/auth/spreadsheets.readonly"}
)

func sheetsLocalAuthorization(name string, scopes []string, allowWrite, allowDestructive bool) ToolAuthorization {
	return ToolAuthorization{
		Service:          "sheets",
		AllowedTools:     []string{name},
		Scopes:           scopes,
		AllowWrite:       allowWrite,
		AllowDestructive: allowDestructive,
		Resource:         &ResourceGrantInput{Service: "sheets", ResourceType: "file", ResourceID: "selected-sheet", Enabled: true},
	}
}

func signSheetsInvocation(t *testing.T, name string, args map[string]any, auth ToolAuthorization) []byte {
	t.Helper()

	return nativeToolBody(t, func(req *Request) {
		req.Tool.Name = name
		req.Tool.Arguments = args
		req.Tool.Authorization = auth
	})
}

func TestSignedNativeSheetsAppendReadClearAndMetadataUnderSelectedFileGrant(t *testing.T) {
	var calls atomic.Int64
	rows := [][]any{{"existing"}}
	rawCell := "=IMPORTRANGE(\\\"other-sheet\\\",\\\"A1\\\")"
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices = app.Services{Sheets: func(ctx context.Context, account string) (*sheets.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
				t.Error("trusted execution identity was lost")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				path := r.URL.Path
				if r.URL.Hostname() != "sheets.googleapis.com" || !strings.HasPrefix(path, "/v4/spreadsheets/selected-sheet") {
					t.Errorf("unexpected native Sheets request: %s", r.URL)
				}

				body := "{}"

				switch {
				case r.Method == http.MethodGet && path == "/v4/spreadsheets/selected-sheet":
					body = `{"spreadsheetId":"selected-sheet","properties":{"title":"Selected sheet","locale":"en_US","timeZone":"Etc/UTC"},"sheets":[{"properties":{"sheetId":0,"title":"Sheet1"}}]}`
				case r.Method == http.MethodGet && path == "/v4/spreadsheets/selected-sheet/values/"+sheetsValuesRange:
					if len(rows) == 0 {
						body = `{"range":"` + sheetsValuesRange + `"}`
					} else {
						encoded, marshalErr := json.Marshal(map[string]any{"range": sheetsValuesRange, "values": rows})
						if marshalErr != nil {
							t.Error(marshalErr)
						}
						body = string(encoded)
					}
				case r.Method == http.MethodPost && strings.HasSuffix(path, "/"+sheetsValuesRange+":append"):
					var input struct {
						Values [][]any `json:"values"`
					}
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}
					rows = append(rows, input.Values...)

					cells := 0
					for i := range input.Values {
						cells += len(input.Values[i])
					}
					body = `{"updates":{"updatedRange":"` + sheetsValuesRange + `","updatedRows":` + fmt.Sprint(len(input.Values)) + `,"updatedColumns":` + fmt.Sprint(cells) + `,"updatedCells":` + fmt.Sprint(cells) + `}}`
				case r.Method == http.MethodPost && strings.HasSuffix(path, "/"+sheetsValuesRange+":clear"):
					rows = nil
					body = `{"clearedRange":"` + sheetsValuesRange + `"}`
				default:
					t.Errorf("restricted/undeclared Sheets endpoint: %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return sheets.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}}
	})

	writeAuth := sheetsLocalAuthorization("sheets_append_range", sheetsWriteScope, true, false)
	appendArgs := func(input, values string) map[string]any {
		return map[string]any{"spreadsheet_id": "selected-sheet", "range": sheetsValuesRange, "values_json": values, "input": input}
	}
	call := func(name string, args map[string]any, auth ToolAuthorization) capturedResponse {
		body := signSheetsInvocation(t, name, args, auth)

		return postExecute(t, harness.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
	}
	read := func() capturedResponse {
		return call("sheets_read_range", map[string]any{"spreadsheet_id": "selected-sheet", "range": sheetsValuesRange}, sheetsLocalAuthorization("sheets_read_range", sheetsReadScope, false, false))
	}

	response := call("sheets_append_range", appendArgs("USER_ENTERED", `[["hello","one"]]`), writeAuth)
	if response.Status != 200 || !response.OK {
		t.Fatalf("native append failed: %d %s", response.Status, response.Body)
	}

	// USER_ENTERED formulas that import from an unselected file are blocked
	// before any network request is made.
	before := calls.Load()

	imported := call("sheets_append_range", appendArgs("USER_ENTERED", `[["=IMPORTRANGE(\"other-sheet\",\"A1\")"]]`), writeAuth)
	if imported.OK || imported.Status != 400 || imported.Error.Code != "invalid_request" {
		t.Fatalf("foreign import formula was not rejected: %d %s", imported.Status, imported.Body)
	}

	if calls.Load() != before {
		t.Fatal("blocked append reached Google")
	}

	// The same foreign import appended with RAW input is stored as literal text.
	raw := call("sheets_append_range", appendArgs("RAW", `[["=IMPORTRANGE(\"other-sheet\",\"A1\")","literal"]]`), writeAuth)
	if raw.Status != 200 || !raw.OK {
		t.Fatalf("native RAW append failed: %d %s", raw.Status, raw.Body)
	}

	if stored := read(); stored.Status != 200 || !stored.OK || !strings.Contains(string(stored.Result), "hello") || !strings.Contains(string(stored.Result), rawCell) {
		t.Fatalf("native read did not persist appended values: %d %s", stored.Status, stored.Result)
	}

	if metadata := call("sheets_metadata", map[string]any{"spreadsheet_id": "selected-sheet"}, sheetsLocalAuthorization("sheets_metadata", sheetsReadScope, false, false)); metadata.Status != 200 || !metadata.OK || !strings.Contains(string(metadata.Result), "Selected sheet") {
		t.Fatalf("native metadata read failed: %d %s", metadata.Status, metadata.Result)
	}

	// Clearing values is a destructive write that is disabled by default.
	clearArgs := map[string]any{"spreadsheet_id": "selected-sheet", "range": sheetsValuesRange}
	clearAuth := sheetsLocalAuthorization("sheets_clear_range", sheetsWriteScope, true, true)

	denied := call("sheets_clear_range", clearArgs, sheetsLocalAuthorization("sheets_clear_range", sheetsWriteScope, true, false))
	if denied.OK || denied.Status != 400 || denied.Error.Code != "invalid_request" {
		t.Fatalf("clear without destructive permission was not rejected: %d %s", denied.Status, denied.Body)
	}

	permitted := call("sheets_clear_range", clearArgs, clearAuth)
	if permitted.Status != 200 || !permitted.OK {
		t.Fatalf("native clear failed: %d %s", permitted.Status, permitted.Body)
	}

	if emptied := read(); emptied.Status != 200 || !emptied.OK || strings.Contains(string(emptied.Result), "hello") {
		t.Fatalf("cleared values were still readable: %d %s", emptied.Status, emptied.Result)
	}
}

func TestSignedNativeSheetsDenialsNeverReachGoogle(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Sheets = func(context.Context, string) (*sheets.Service, error) {
			calls.Add(1)
			return nil, errUnexpectedNativeFactory
		}
	})
	appendArgs := map[string]any{"spreadsheet_id": "selected-sheet", "range": sheetsValuesRange, "values_json": `[["new"]]`, "input": "USER_ENTERED"}
	clearArgs := map[string]any{"spreadsheet_id": "selected-sheet", "range": sheetsValuesRange}
	call := func(name string, args map[string]any, grant ToolAuthorization) capturedResponse {
		body := signSheetsInvocation(t, name, args, grant)

		return postExecute(t, harness.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
	}

	denials := []struct {
		name string
		tool string
		args map[string]any
		auth ToolAuthorization
	}{
		{"wrong file", "sheets_append_range", map[string]any{"spreadsheet_id": "unselected-sheet", "range": sheetsValuesRange, "values_json": `[["new"]]`, "input": "USER_ENTERED"}, sheetsLocalAuthorization("sheets_append_range", sheetsWriteScope, true, false)},
		{"disabled tool", "sheets_append_range", appendArgs, func() ToolAuthorization {
			auth := sheetsLocalAuthorization("sheets_append_range", sheetsWriteScope, true, false)
			auth.AllowedTools = nil

			return auth
		}()},
		{"disabled resource", "sheets_append_range", appendArgs, func() ToolAuthorization {
			auth := sheetsLocalAuthorization("sheets_append_range", sheetsWriteScope, true, false)
			auth.Resource.Enabled = false

			return auth
		}()},
		{"write disabled", "sheets_append_range", appendArgs, sheetsLocalAuthorization("sheets_append_range", sheetsWriteScope, false, false)},
		{"readonly scope for clear", "sheets_clear_range", clearArgs, sheetsLocalAuthorization("sheets_clear_range", sheetsReadScope, true, true)},
		{"readonly scope for append", "sheets_append_range", appendArgs, sheetsLocalAuthorization("sheets_append_range", sheetsReadScope, true, false)},
	}
	for _, test := range denials {
		t.Run(test.name, func(t *testing.T) {
			if response := call(test.tool, test.args, test.auth); response.OK || response.Status != 400 || response.Error.Code != "invalid_request" {
				t.Fatalf("unsafe invocation allowed: %d %s", response.Status, response.Body)
			}
		})
	}

	if calls.Load() != 0 {
		t.Fatal("denied invocation reached native handler")
	}
}

func TestNativeSheetsRequestGuardAdmitsOnlyExactMethodsAndSuffixes(t *testing.T) {
	base := "https://sheets.googleapis.com"

	tests := []struct {
		name   string
		tool   string
		path   string
		method string
		admit  bool
	}{
		{"append exact suffix", "sheets_append_range", "/v4/spreadsheets/selected-sheet/values/" + sheetsValuesRange + ":append", http.MethodPost, true},
		{"append wrong method", "sheets_append_range", "/v4/spreadsheets/selected-sheet/values/" + sheetsValuesRange + ":append", http.MethodGet, false},
		{"append missing action suffix", "sheets_append_range", "/v4/spreadsheets/selected-sheet/values/" + sheetsValuesRange, http.MethodPost, false},
		{"append wrong action suffix", "sheets_append_range", "/v4/spreadsheets/selected-sheet/values/" + sheetsValuesRange + ":clear", http.MethodPost, false},
		{"append wrong spreadsheet", "sheets_append_range", "/v4/spreadsheets/unselected-sheet/values/" + sheetsValuesRange + ":append", http.MethodPost, false},
		{"clear exact suffix", "sheets_clear_range", "/v4/spreadsheets/selected-sheet/values/" + sheetsValuesRange + ":clear", http.MethodPost, true},
		{"clear wrong method", "sheets_clear_range", "/v4/spreadsheets/selected-sheet/values/" + sheetsValuesRange + ":clear", http.MethodDelete, false},
		{"clear missing action suffix", "sheets_clear_range", "/v4/spreadsheets/selected-sheet/values/" + sheetsValuesRange, http.MethodPost, false},
		{"metadata exact path", "sheets_metadata", "/v4/spreadsheets/selected-sheet", http.MethodGet, true},
		{"metadata wrong method", "sheets_metadata", "/v4/spreadsheets/selected-sheet", http.MethodPost, false},
		{"metadata values path", "sheets_metadata", "/v4/spreadsheets/selected-sheet/values/" + sheetsValuesRange, http.MethodGet, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation := &ToolInvocation{
				Name: test.tool, Arguments: map[string]any{"spreadsheet_id": "selected-sheet", "range": sheetsValuesRange},
				Authorization: sheetsLocalAuthorization(test.tool, sheetsWriteScope, true, true),
			}
			guard := nativeRequestGuard(invocation)

			parsed, err := url.Parse(base + test.path)
			if err != nil {
				t.Fatal(err)
			}
			result := guard(&http.Request{Method: test.method, URL: parsed, Header: http.Header{}})

			if test.admit && result != nil {
				t.Fatalf("expected exact admission, got %v", result)
			}

			if !test.admit && result == nil {
				t.Fatal("expected denial")
			}
		})
	}
}
