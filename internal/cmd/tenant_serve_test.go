package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/tenants"
)

func tenantFakeGog(t *testing.T) string {
	t.Helper()

	// Re-exec the test binary in TestMain TENANT_FAKE_GOG mode: it records
	// env and argv, prints one JSON line, and exits 0. Hermetic without shell quoting.
	return os.Args[0]
}

func tenantSeed(t *testing.T, tenantsSeed []tenants.Tenant) *tenantServeHandler {
	t.Helper()

	// The hosted child re-execs the test binary; flip it into fake-gog mode.
	t.Setenv("TENANT_FAKE_GOG", "1")

	configDir := t.TempDir()
	store := tenants.NewStore(configDir)
	for _, tenant := range tenantsSeed {
		if err := store.Set(tenant); err != nil {
			t.Fatalf("seed tenant: %v", err)
		}
	}
	masterKey, err := loadTenantMasterKey("")
	if err != nil {
		t.Fatalf("load master key: %v", err)
	}
	handler := &tenantServeHandler{
		store:     store,
		self:      tenantFakeGog(t),
		timeout:   10 * time.Second,
		masterKey: masterKey,
	}
	return handler
}

func tenantPost(t *testing.T, handler *tenantServeHandler, path string, body any) (int, map[string]any) {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, bytes.NewReader(encoded))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	var payload map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &payload)
	return recorder.Code, payload
}

func TestTenantServeListAndAllowlistDeny(t *testing.T) {
	handler := tenantSeed(t, []tenants.Tenant{
		{Name: "personal", Account: "rajeev.sgill@gmail.com", ReadOnly: true},
	})

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tenants", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var listPayload struct {
		Tenants []map[string]any `json:"tenants"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("tenants json: %v", err)
	}
	if len(listPayload.Tenants) != 1 || listPayload.Tenants[0]["name"] != "personal" {
		t.Fatalf("unexpected tenants: %#v", listPayload.Tenants)
	}

	// Read tool on a read-only tenant is allowed.
	status, payload := tenantPost(t, handler, "/tenants/personal/call", map[string]any{
		"tool": "gmail_search",
		"arguments": map[string]any{
			"query": "newer_than:7d",
		},
	})
	if status != http.StatusOK {
		t.Fatalf("expected allowed read call, got %d %v", status, payload)
	}

	// Write-risk tool without an allowlist is denied.
	status, payload = tenantPost(t, handler, "/tenants/personal/call", map[string]any{
		"tool": "gmail_create_label",
	})
	if status != http.StatusForbidden {
		t.Fatalf("expected denied write call, got %d %v", status, payload)
	}

	// Unknown tool is denied.
	status, payload = tenantPost(t, handler, "/tenants/personal/call", map[string]any{
		"tool": "nonexistent",
	})
	if status != http.StatusForbidden {
		t.Fatalf("expected denied unknown tool, got %d %v", status, payload)
	}
}

func TestTenantServeCallRunsIsolatedChild(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "child.log")
	t.Setenv("TENANT_TEST_LOG", logPath)
	t.Setenv("GOG_HOME", "/should-be-overridden")
	t.Setenv("GOG_ACCOUNT", "/should-be-overridden")

	handler := tenantSeed(t, []tenants.Tenant{
		{Name: "personal", Account: "rajeev.sgill@gmail.com", ReadOnly: true},
	})
	status, payload := tenantPost(t, handler, "/tenants/personal/call", map[string]any{
		"tool":      "gmail_search",
		"arguments": map[string]any{"query": "newer_than:7d", "max": 5},
	})
	if status != http.StatusOK {
		t.Fatalf("call: %d %v", status, payload)
	}
	if payload["exit_code"] != float64(0) {
		t.Fatalf("unexpected exit code: %#v", payload["exit_code"])
	}

	child, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read child log: %v", err)
	}
	childText := string(child)
	if !strings.Contains(childText, "--readonly") {
		t.Fatalf("expected --readonly for readonly tenant: %s", childText)
	}
	home := handler.store.Home("personal")
	if !strings.Contains(childText, "GOG_HOME:"+home) {
		t.Fatalf("expected tenant home env: %s", childText)
	}
	if !strings.Contains(childText, "GOG_ACCOUNT:rajeev.sgill@gmail.com") {
		t.Fatalf("expected pinned account: %s", childText)
	}

	// The call must leave an audit entry inside the tenant home.
	audit := tenants.NewAuditLog(home)
	entries, err := audit.Tail(10)
	if err != nil || len(entries) != 1 || entries[0].Decision != "execute" {
		t.Fatalf("unexpected audit: %#v err=%v", entries, err)
	}
}

func TestTenantServeExplicitAllowlistAndMasterKey(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "child.log")
	t.Setenv("TENANT_TEST_LOG", logPath)
	t.Setenv("GOG_TENANTS_MASTER_KEY", "op-secret")

	handler := tenantSeed(t, []tenants.Tenant{
		{Name: "singulyr", Account: "rajeev@singulyr.com", AllowTools: []string{"gmail_search"}},
	})

	// Allowlisted read tool runs and derives a tenant keyring password.
	status, _ := tenantPost(t, handler, "/tenants/singulyr/call", map[string]any{
		"tool":      "gmail_search",
		"arguments": map[string]any{"query": "x"},
	})
	if status != http.StatusOK {
		t.Fatalf("allowlisted call: %d", status)
	}
	child, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read child log: %v", err)
	}
	if !strings.Contains(string(child), "GOG_KEYRING:file") {
		t.Fatalf("expected file keyring with master key: %s", child)
	}
	if !strings.Contains(string(child), fmt.Sprintf("GOG_HOME:%s", handler.store.Home("singulyr"))) {
		t.Fatalf("expected tenant home: %s", child)
	}

	// A known-but-unallowlisted tool is refused.
	status, payload := tenantPost(t, handler, "/tenants/singulyr/call", map[string]any{
		"tool": "gmail_create_label",
	})
	if status != http.StatusForbidden || !strings.Contains(fmt.Sprintf("%v", payload["error"]), "not allowlisted") {
		t.Fatalf("expected allowlist denial, got %d %v", status, payload)
	}

	// Master key derivation must not be shared across tenants: derive and compare.
	one, _ := tenants.DeriveKeyringPassword([]byte("op-secret"), "singulyr")
	two, _ := tenants.DeriveKeyringPassword([]byte("op-secret"), "other")
	if string(one) == string(two) {
		t.Fatal("tenant passwords must differ")
	}
}
