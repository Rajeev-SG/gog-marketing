package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/googleauth"
)

func TestHostedExistingToolUsesNativeSheetsReadWriteHandlers(t *testing.T) {
	var calls atomic.Int64
	value := "before"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer synthetic-ephemeral-token" {
			t.Error("guarded transport did not use the provided ephemeral token")
		}
		if !strings.Contains(r.URL.Path, "/spreadsheets/selected-sheet/values/A1") {
			t.Errorf("unexpected native request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			var body struct {
				Values [][]string `json:"values"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			value = body.Values[0][0]
			_, _ = w.Write([]byte(`{"updatedRange":"A1","updatedCells":1}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"range": "A1", "values": [][]string{{value}}})
	}))
	t.Cleanup(api.Close)
	options := HostedToolOptions{
		Account: "owner@example.com", AccessToken: "synthetic-ephemeral-token",
		Services: app.Services{Sheets: func(ctx context.Context, account string) (*sheets.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
				t.Error("trusted account/token/no-input context was not preserved")
			}
			client, err := googleapi.NewHTTPClient(ctx, googleauth.ServiceSheets, account)
			if err != nil {
				return nil, err
			}
			return sheets.NewService(ctx, option.WithEndpoint(api.URL+"/"), option.WithHTTPClient(client))
		}},
	}
	writeArgs := map[string]any{"spreadsheet_id": "selected-sheet", "range": "A1", "values_json": `[["after"]]`}
	if _, err := ExecuteHostedExistingTool(t.Context(), "sheets_update_range", writeArgs, options); !errors.Is(err, ErrHostedToolDenied) {
		t.Fatalf("write without authority: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("denied write reached Google handler")
	}
	options.AllowWrite = true
	if _, err := ExecuteHostedExistingTool(t.Context(), "sheets_update_range", writeArgs, options); err != nil {
		t.Fatalf("native write failed: %v", err)
	}
	options.AllowWrite = false
	result, err := ExecuteHostedExistingTool(t.Context(), "sheets_read_range", map[string]any{"spreadsheet_id": "selected-sheet", "range": "A1"}, options)
	if err != nil {
		t.Fatalf("native read failed: %v", err)
	}
	encoded, _ := json.Marshal(result)
	if !strings.Contains(string(encoded), "after") || calls.Load() != 2 {
		t.Fatalf("native handlers did not persist/read the value: %s calls=%d", encoded, calls.Load())
	}
}

func TestHostedExistingToolRejectsEscapeHatchesBeforeHandler(t *testing.T) {
	var calls atomic.Int64
	options := HostedToolOptions{
		Account: "owner@example.com", AccessToken: "synthetic-token", AllowWrite: true,
		Services: app.Services{Sheets: func(context.Context, string) (*sheets.Service, error) {
			calls.Add(1)
			return nil, errors.New("must not reach service")
		}},
	}
	for _, name := range []string{"gog_exec", "gmail_search", "googleads_query", "drive_search", "auth_add", "api_call"} {
		if _, err := ExecuteHostedExistingTool(t.Context(), name, nil, options); !errors.Is(err, ErrHostedToolDenied) {
			t.Errorf("%s was not denied: %v", name, err)
		}
	}
	for _, args := range []map[string]any{
		{"spreadsheet_id": "x", "range": "A1", "account": "foreign@example.com"},
		{"spreadsheet_id": "x", "range": "A1", "access_token": "foreign"},
		{"spreadsheet_id": "x", "range": "A1", "argv": []string{"auth", "add"}},
		{"spreadsheet_id": 123, "range": "A1"},
		{"range": "A1"},
	} {
		if _, err := ExecuteHostedExistingTool(t.Context(), "sheets_read_range", args, options); !errors.Is(err, ErrHostedToolInput) {
			t.Errorf("invalid typed input was not rejected: %v", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("schema rejection reached native handler")
	}
}

func TestHostedExistingToolCatalogPreservesLocalMCP(t *testing.T) {
	local := mcpAllTools()
	localHasGmail := false
	for _, spec := range local {
		localHasGmail = localHasGmail || spec.Service == "gmail"
	}
	if !localHasGmail {
		t.Fatal("local CLI MCP Gmail was removed")
	}
	for _, hosted := range HostedExistingToolCatalog() {
		if hosted.Service == "gmail" || hosted.Service == "googleads" || hosted.Tool.Name == "drive_search" {
			t.Fatalf("excluded hosted capability: %s", hosted.Tool.Name)
		}
	}
}
