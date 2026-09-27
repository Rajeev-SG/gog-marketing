package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	handler := &tenantServeHandler{
		store:         store,
		self:          tenantFakeGog(t),
		timeout:       10 * time.Second,
		masterKey:     []byte("test-master"),
		serveToken:    "test-token",
		errOut:        io.Discard,
		port:          8086,
		childEnvExtra: []string{"TENANT_FAKE_GOG=1"},
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
	request.Host = "127.0.0.1:8086"
	request.Header.Set("Authorization", "Bearer test-token")
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
	request.Host = "127.0.0.1:8086"
	request.Header.Set("Authorization", "Bearer test-token")
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

	// SEC: requests without a bearer token are rejected; bad Host and browser
	// origins are rejected before tenant data is touched.
	unauthorized := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tenants", nil)
	unauthorized.Host = "127.0.0.1:8086"
	unauthorizedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", unauthorizedRecorder.Code)
	}

	rebind := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tenants", nil)
	rebind.Host = "evil.example.com:8086"
	rebind.Header.Set("Authorization", "Bearer test-token")
	rebindRecorder := httptest.NewRecorder()
	handler.ServeHTTP(rebindRecorder, rebind)
	if rebindRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unexpected Host, got %d", rebindRecorder.Code)
	}

	origin := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tenants", nil)
	origin.Host = "127.0.0.1:8086"
	origin.Header.Set("Authorization", "Bearer test-token")
	origin.Header.Set("Origin", "https://attacker.example.com")
	originRecorder := httptest.NewRecorder()
	handler.ServeHTTP(originRecorder, origin)
	if originRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for browser Origin, got %d", originRecorder.Code)
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
	// Poison the operator environment: the minimal child env must drop it.
	t.Setenv("GOG_ACCESS_TOKEN", "poisoned-op-token")

	handler := tenantSeed(t, []tenants.Tenant{
		{Name: "personal", Account: "rajeev.sgill@gmail.com", ReadOnly: true},
	})
	handler.childEnvExtra = append(handler.childEnvExtra, "TENANT_TEST_LOG="+logPath)
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
	home, homeErr := handler.store.Home("personal")
	if homeErr != nil {
		t.Fatalf("tenant home: %v", homeErr)
	}
	if !strings.Contains(childText, "GOG_HOME:"+home) {
		t.Fatalf("expected tenant home env: %s", childText)
	}
	if !strings.Contains(childText, "GOG_ACCOUNT:rajeev.sgill@gmail.com") {
		t.Fatalf("expected pinned account: %s", childText)
	}
	if strings.Contains(childText, "GOG_ACCESS_TOKEN_SET:true") {
		t.Fatalf("child must not inherit the operator access token: %s", childText)
	}

	// The keyring password must travel by a temporary file, not by env, and
	// must be removed before the call result is returned.
	homeForEnv, homeForEnvErr := handler.tenantHome("personal")
	if homeForEnvErr != nil {
		t.Fatal(homeForEnvErr)
	}
	childEnv, cleanupChildEnv, childEnvErr := handler.tenantChildEnv("personal", tenants.Tenant{Name: "personal", Account: "x@y.com"})
	if childEnvErr != nil {
		t.Fatalf("tenantChildEnv: %v", childEnvErr)
	}
	for _, entry := range childEnv {
		if strings.HasPrefix(entry, "GOG_KEYRING_PASSWORD=") {
			t.Fatalf("keyring password must not be in child env: %s", entry)
		}
	}
	if !strings.Contains(strings.Join(childEnv, "\n"), "GOG_KEYRING_PASSWORD_FILE=") {
		t.Fatal("expected GOG_KEYRING_PASSWORD_FILE in child env")
	}
	passwordFile := filepath.Join(homeForEnv, "keys", "keyring-password")
	if _, statErr := os.Stat(passwordFile); statErr != nil {
		t.Fatalf("temporary keyring password file should exist while the child runs: %v", statErr)
	}
	cleanupChildEnv()
	if _, statErr := os.Stat(passwordFile); !os.IsNotExist(statErr) {
		t.Fatalf("temporary keyring password file must be removed after cleanup: %v", statErr)
	}

	// The call must leave an audit entry inside the tenant home.
	audit := tenants.NewAuditLog(home)
	entries, err := audit.Tail(10)
	if err != nil || len(entries) != 1 || entries[0].Decision != "execute" {
		t.Fatalf("unexpected audit: %#v err=%v", entries, err)
	}
	passwordFileAfter := filepath.Join(home, "keys", "keyring-password")
	if _, statErr := os.Stat(passwordFileAfter); !os.IsNotExist(statErr) {
		t.Fatalf("keyring password file must be removed after handleCall: %v", statErr)
	}
}

func TestTenantServeMalformedBodySurfacesAuditFailure(t *testing.T) {
	handler := tenantSeed(t, []tenants.Tenant{
		{Name: "personal", Account: "rajeev.sgill@gmail.com", ReadOnly: true},
	})
	var errOut bytes.Buffer
	handler.errOut = &errOut

	home, homeErr := handler.tenantHome("personal")
	if homeErr != nil {
		t.Fatal(homeErr)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "audit"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/tenants/personal/call", strings.NewReader("{"))
	request.Host = "127.0.0.1:8086"
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed body to be rejected, got %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(errOut.String(), "AUDIT append failed") {
		t.Fatalf("malformed-body audit failure must be surfaced, got stderr=%q", errOut.String())
	}
}

func TestTenantTokenNoticeRedactsConfiguredTokens(t *testing.T) {
	generated := tenantTokenNotice("generated-secret", true)
	configured := tenantTokenNotice("configured-secret", false)
	if !strings.Contains(generated, "generated-secret") {
		t.Fatalf("generated token notice should include the token: %q", generated)
	}
	if strings.Contains(configured, "configured-secret") {
		t.Fatalf("configured token notice must not leak the token: %q", configured)
	}
}

func TestTenantServeExplicitAllowlistAndMasterKey(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "child.log")
	t.Setenv("TENANT_TEST_LOG", logPath)
	t.Setenv("GOG_TENANTS_MASTER_KEY", "op-secret")

	handler := tenantSeed(t, []tenants.Tenant{
		{Name: "singulyr", Account: "rajeev@singulyr.com", AllowTools: []string{"gmail_search"}},
	})
	handler.childEnvExtra = append(handler.childEnvExtra, "TENANT_TEST_LOG="+logPath)

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
	singulyrHome, singulyrErr := handler.store.Home("singulyr")
	if singulyrErr != nil {
		t.Fatalf("tenant home: %v", singulyrErr)
	}
	if !strings.Contains(string(child), fmt.Sprintf("GOG_HOME:%s", singulyrHome)) {
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

func TestTenantServeOversizedBodyRejected(t *testing.T) {
	handler := tenantSeed(t, []tenants.Tenant{
		{Name: "personal", Account: "rajeev.sgill@gmail.com", ReadOnly: true},
	})

	huge := strings.Repeat("x", tenantMaxRequestBodyBytes+1024)
	status, payload := tenantPost(t, handler, "/tenants/personal/call", map[string]any{
		"tool":      "gmail_search",
		"arguments": map[string]any{"query": huge},
	})
	if status != http.StatusBadRequest && status != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected oversized body to be rejected, got %d %v", status, payload)
	}
}

func TestTenantServeAuditFailureIsVisible(t *testing.T) {
	handler := tenantSeed(t, []tenants.Tenant{
		{Name: "personal", Account: "rajeev.sgill@gmail.com", ReadOnly: true},
	})

	var errOut bytes.Buffer
	handler.errOut = &errOut

	// Make the audit dir unwritable: a file where the dir must go.
	home, homeErr := handler.tenantHome("personal")
	if homeErr != nil {
		t.Fatal(homeErr)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "audit"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	status, _ := tenantPost(t, handler, "/tenants/personal/call", map[string]any{
		"tool":      "gmail_search",
		"arguments": map[string]any{"query": "x"},
	})
	if status != http.StatusOK {
		t.Fatalf("call itself should still run: %d", status)
	}
	if !strings.Contains(errOut.String(), "AUDIT append failed") {
		t.Fatalf("audit failure must be surfaced to the operator, got stderr=%q", errOut.String())
	}
}

func TestTenantServeHealthRemainsAvailableWhileCallsAreBusy(t *testing.T) {
	handler := tenantSeed(t, []tenants.Tenant{
		{Name: "personal", Account: "rajeev.sgill@gmail.com", ReadOnly: true},
	})
	handler.callSlots = make(chan struct{}, tenantMaxConcurrentCalls)
	for i := 0; i < tenantMaxConcurrentCalls; i++ {
		handler.callSlots <- struct{}{}
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	request.Host = "127.0.0.1:8086"
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("healthz must remain unthrottled while call slots are busy: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestTenantServeRejectsUnsafeTenantNamesBeforeLookup(t *testing.T) {
	handler := tenantSeed(t, nil)
	for _, path := range []string{"/tenants/..%2F..%2F/call", "/tenants/a%2Fb/call", "/tenants/..%2F..%2F/tools"} {
		request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader("{}"))
		request.Host = "127.0.0.1:8086"
		request.Header.Set("Authorization", "Bearer test-token")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s should reject unsafe tenant name with 400, got %d %s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestTenantHomeRejectsUnsafeNames(t *testing.T) {
	configDir := t.TempDir()
	store := tenants.NewStore(configDir)

	for _, name := range []string{"..", ".", "a/b", "has space", ""} {
		if _, err := store.Home(name); err == nil {
			t.Fatalf("expected Home(%q) to be rejected", name)
		}
	}
	handler := &tenantServeHandler{store: store}
	if _, err := handler.tenantHome("a/b"); err == nil {
		t.Fatal("handler tenantHome must propagate unsafe-name errors")
	}
	if _, statErr := os.Stat(filepath.Join(configDir, "tenants", "_invalid")); !os.IsNotExist(statErr) {
		t.Fatalf("unsafe names must not create an _invalid fallback directory: %v", statErr)
	}
}
