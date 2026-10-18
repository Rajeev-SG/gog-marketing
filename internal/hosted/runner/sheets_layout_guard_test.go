package runner

import (
	"context"
	"net/http"
	"testing"
)

func TestNativeSheetsLayoutGuardsPinMethodAndSelectedPath(t *testing.T) {
	for _, name := range []string{"sheets_freeze", "sheets_reorder_tab"} {
		t.Run(name, func(t *testing.T) {
			args := map[string]any{"spreadsheet_id": "selected-sheet", "sheet": "Sheet1", "rows": 0}
			if name == "sheets_reorder_tab" {
				args = map[string]any{"spreadsheet_id": "selected-sheet", "tab": "Sheet1", "to": 0}
			}

			guard := nativeRequestGuard(&ToolInvocation{
				Name: name, Arguments: args,
				Authorization: sheetsLocalAuthorization(name, sheetsWriteScope, true, false),
			})

			root := "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet"
			for _, request := range []struct {
				method string
				url    string
				allow  bool
			}{
				{http.MethodGet, root, true},
				{http.MethodPost, root + ":batchUpdate", true},
				{http.MethodPost, root, false},
				{http.MethodGet, root + ":batchUpdate", false},
				{http.MethodDelete, root, false},
				{http.MethodGet, "https://sheets.googleapis.com/v4/spreadsheets/foreign-sheet", false},
				{http.MethodPost, "https://sheets.googleapis.com/v4/spreadsheets/foreign-sheet:batchUpdate", false},
				{http.MethodGet, "https://example.test/v4/spreadsheets/selected-sheet", false},
			} {
				req, err := http.NewRequestWithContext(context.Background(), request.method, request.url, nil)
				if err != nil {
					t.Fatal(err)
				}

				if err := guard(req); (err == nil) != request.allow {
					t.Errorf("%s %s: allow=%t, got %v", request.method, request.url, request.allow, err)
				}
			}
		})
	}
}
