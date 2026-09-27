package cmd

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/tenants"
	"github.com/openclaw/gogcli/internal/ui"
)

const (
	tenantMaxOutputBytes      = 4 << 20
	tenantMaxRequestBodyBytes = 1 << 20
	tenantMaxConcurrentCalls  = 8
	tenantRequestTimeout      = 3 * time.Minute
)

type TenantServeCmd struct {
	Port           int    `name:"port" help:"HTTP port (binds 127.0.0.1 only)"`
	ServeToken     string `name:"serve-token" help:"Bearer token required on every request (generated and printed if unset)" env:"GOG_TENANTS_SERVE_TOKEN"`
	ServeTokenFile string `name:"serve-token-file" type:"path" help:"File holding the bearer token for the hosted API"`
	MasterKeyFile  string `name:"master-key-file" type:"path" help:"File holding the master secret; enables per-tenant encrypted file keyrings (required)"`
	TimeoutSeconds int    `name:"timeout-seconds" help:"Per-call child timeout in seconds"`
}

func (c *TenantServeCmd) Run(ctx context.Context, flags *RootFlags) error {
	store, err := commandTenantStore(ctx)
	if err != nil {
		return err
	}

	masterKey, err := loadTenantMasterKey(c.MasterKeyFile)
	if err != nil {
		return err
	}
	if len(masterKey) == 0 {
		return usage("tenant serving requires a master secret (--master-key-file or GOG_TENANTS_MASTER_KEY) so tenant OAuth tokens are always encrypted at rest")
	}

	serveToken, generated, err := loadTenantServeToken(c.ServeToken, c.ServeTokenFile)
	if err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	timeout := time.Duration(c.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		return usage("--timeout-seconds must be greater than zero")
	}

	handler := &tenantServeHandler{
		store:      store,
		self:       self,
		masterKey:  masterKey,
		serveToken: serveToken,
		timeout:    timeout,
		errOut:     os.Stderr,
		callSlots:  make(chan struct{}, tenantMaxConcurrentCalls),
		port:       c.Port,
	}

	addr := fmt.Sprintf("127.0.0.1:%d", c.Port)
	ui.FromContext(ctx).Err().Linef("gog tenant API listening on http://%s", addr)
	ui.FromContext(ctx).Err().Linef("%s", tenantTokenNotice(serveToken, generated))

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return httpServer.ListenAndServe()
}

func loadTenantMasterKey(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return []byte(strings.TrimSpace(os.Getenv("GOG_TENANTS_MASTER_KEY"))), nil
	}
	raw, err := os.ReadFile(path) //nolint:gosec // operator-provided key/token file path
	if err != nil {
		return nil, fmt.Errorf("read --master-key-file: %w", err)
	}
	return []byte(strings.TrimSpace(string(raw))), nil
}

func tenantTokenNotice(token string, generated bool) string {
	if generated {
		return fmt.Sprintf("hosted API bearer token: %s (give it only to authorized clients)", token)
	}
	return "hosted API bearer token is configured; value withheld from logs"
}

// loadTenantServeToken resolves or generates the hosted API bearer token.
// Operators may pin one via --serve-token(-file); otherwise a random token is
// generated per run and printed to stderr for authorized clients.
func loadTenantServeToken(token, tokenFile string) (string, bool, error) {
	if strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token), false, nil
	}
	if strings.TrimSpace(tokenFile) != "" {
		raw, err := os.ReadFile(strings.TrimSpace(tokenFile))
		if err != nil {
			return "", false, fmt.Errorf("read --serve-token-file: %w", err)
		}
		return strings.TrimSpace(string(raw)), false, nil
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", false, fmt.Errorf("generate serve token: %w", err)
	}
	return hex.EncodeToString(buf), true, nil
}

type tenantServeHandler struct {
	store      *tenants.Store
	self       string
	masterKey  []byte
	serveToken string
	timeout    time.Duration
	errOut     io.Writer
	callSlots  chan struct{}
	port       int
	// childEnvExtra is appended to every tenant child environment. Hosted
	// operators can use it for managed settings; tests use it to re-exec the
	// test binary in a controlled fake-child mode.
	childEnvExtra []string
}

func (h *tenantServeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Local-host hardening: DNS rebinding / cross-origin browser calls are
	// rejected before any tenant data is touched.
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		writeTenantJSON(w, http.StatusForbidden, map[string]any{"error": "browser-origin requests are not allowed on the hosted API"})
		return
	}
	if !tenantHostAllowed(r.Host, h.port) {
		writeTenantJSON(w, http.StatusForbidden, map[string]any{"error": "unexpected Host header"})
		return
	}
	if !h.authorized(r) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeTenantJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing or invalid bearer token"})
		return
	}

	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, tenantMaxRequestBodyBytes)
	}

	path := strings.Trim(r.URL.EscapedPath(), "/")
	switch {
	case r.Method == http.MethodGet && path == "healthz":
		// Liveness must stay cheap and unthrottled so operators can detect a
		// wedged or busy worker without the endpoint becoming a DoS lever.
		writeTenantJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case r.Method == http.MethodGet && path == "tenants":
		h.handleList(w)
	case r.Method == http.MethodPost && strings.Count(path, "/") == 2 && strings.HasSuffix(path, "/tools"):
		name, ok := tenantNameFromPath(path)
		if !ok {
			writeTenantJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid tenant name"})
			return
		}
		h.handleListTools(w, name)
	case r.Method == http.MethodPost && strings.Count(path, "/") == 2 && strings.HasSuffix(path, "/call"):
		name, ok := tenantNameFromPath(path)
		if !ok {
			writeTenantJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid tenant name"})
			return
		}
		h.handleCallWithLimit(w, r, name)
	default:
		writeTenantJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func tenantHostAllowed(host string, port int) bool {
	return host == fmt.Sprintf("127.0.0.1:%d", port) || host == fmt.Sprintf("localhost:%d", port)
}

// tenantNameFromPath parses /tenants/<name>[/(tools|call)] and rejects names
// outside the same slug contract used by the registry and home resolver.
func tenantNameFromPath(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "tenants" {
		return "", false
	}
	name := parts[1]
	normalized, err := tenants.NormalizeName(name)
	if err != nil || normalized != name {
		return "", false
	}
	return name, true
}

// allowedTools applies the tenant policy: an explicit allow-tools list is the
// tenant's exact tool surface; without a list only read-risk tools are exposed.
func allowedTools(tenant tenants.Tenant) []mcpToolSpec {
	allow := make(map[string]bool, len(tenant.AllowTools))
	explicit := len(tenant.AllowTools) > 0
	for _, name := range tenant.AllowTools {
		allow[name] = true
	}

	out := make([]mcpToolSpec, 0, len(mcpAllTools()))
	for _, spec := range mcpAllTools() {
		if explicit {
			if allow[spec.Name] {
				out = append(out, spec)
			}
			continue
		}
		if spec.Risk == mcpRiskRead {
			out = append(out, spec)
		}
	}
	return out
}

// authorized checks the bearer token with a constant-time compare.
func (h *tenantServeHandler) authorized(r *http.Request) bool {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.serveToken)) == 1
}

func (h *tenantServeHandler) tenantErr(err error) {
	_, _ = fmt.Fprintf(h.errOut, "tenant API error: %v\n", err)
}

func (h *tenantServeHandler) handleList(w http.ResponseWriter) {
	tenantsList, err := h.store.List()
	if err != nil {
		h.tenantErr(err)
		writeTenantJSON(w, http.StatusInternalServerError, map[string]any{"error": "tenant registry is unavailable"})
		return
	}

	out := make([]map[string]any, 0, len(tenantsList))
	for _, tenant := range tenantsList {
		out = append(out, map[string]any{
			"name":     tenant.Name,
			"readonly": tenant.ReadOnly,
		})
	}
	writeTenantJSON(w, http.StatusOK, map[string]any{"tenants": out})
}

func (h *tenantServeHandler) resolveTenant(w http.ResponseWriter, name string) (tenants.Tenant, bool) {
	tenant, ok, err := h.store.Get(name)
	if err != nil {
		h.tenantErr(err)
		writeTenantJSON(w, http.StatusInternalServerError, map[string]any{"error": "tenant registry is unavailable"})
		return tenants.Tenant{}, false
	}
	if !ok {
		writeTenantJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("tenant %q not found", name)})
		return tenants.Tenant{}, false
	}
	return tenant, true
}

func (h *tenantServeHandler) tenantAudit(tenant tenants.Tenant) (*tenants.AuditLog, error) {
	home, err := h.tenantHome(tenant.Name)
	if err != nil {
		return nil, err
	}
	return tenants.NewAuditLog(home), nil
}

func (h *tenantServeHandler) tenantHome(name string) (string, error) {
	home, err := h.store.Home(name)
	if err != nil {
		return "", fmt.Errorf("resolve tenant home for %q: %w", name, err)
	}
	return home, nil
}

func (h *tenantServeHandler) handleListTools(w http.ResponseWriter, name string) {
	tenant, ok := h.resolveTenant(w, name)
	if !ok {
		return
	}
	names := make([]string, 0, 8)
	for _, spec := range allowedTools(tenant) {
		names = append(names, spec.Name)
	}
	writeTenantJSON(w, http.StatusOK, map[string]any{"tenant": tenant.Name, "tools": names})
}

type tenantToolRequest struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

func (h *tenantServeHandler) handleCallWithLimit(w http.ResponseWriter, r *http.Request, name string) {
	callCtx, cancel := context.WithTimeout(r.Context(), tenantRequestTimeout)
	defer cancel()

	if h.callSlots == nil {
		h.callSlots = make(chan struct{}, tenantMaxConcurrentCalls)
	}
	select {
	case h.callSlots <- struct{}{}:
		defer func() { <-h.callSlots }()
	case <-callCtx.Done():
		writeTenantJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "hosted API is busy"})
		return
	}

	request := r.WithContext(callCtx)
	h.handleCall(w, request, name)
}

func (h *tenantServeHandler) handleCall(w http.ResponseWriter, r *http.Request, name string) {
	tenant, ok := h.resolveTenant(w, name)
	if !ok {
		return
	}
	audit, auditErr := h.tenantAudit(tenant)
	if auditErr != nil {
		h.tenantErr(auditErr)
		writeTenantJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not resolve tenant audit log"})
		return
	}

	var body tenantToolRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&body); decodeErr != nil {
		writeTenantJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("decode body: %v", decodeErr)})
		h.recordAudit(audit, tenants.AuditEntry{Tenant: tenant.Name, Action: body.Tool, Decision: "deny", Detail: decodeErr.Error()})
		return
	}

	spec, allowState := tenantFindTool(tenant, body.Tool)
	if spec == nil {
		reason := fmt.Sprintf("unknown tool %q", body.Tool)
		if allowState == tenantAllowKnown {
			reason = fmt.Sprintf("tool %q is not allowlisted for this tenant", body.Tool)
		}
		writeTenantJSON(w, http.StatusForbidden, map[string]any{"error": reason})
		h.recordAudit(audit, tenants.AuditEntry{Tenant: tenant.Name, Action: body.Tool, Decision: "deny", Detail: reason})
		return
	}

	request := mcpCallRequest(body.Tool, body.Arguments)
	childArgs, buildErr := spec.BuildArgs(request)
	if buildErr != nil {
		writeTenantJSON(w, http.StatusBadRequest, map[string]any{"error": buildErr.Error()})
		h.recordAudit(audit, tenants.AuditEntry{Tenant: tenant.Name, Action: body.Tool, Decision: "deny", Detail: buildErr.Error()})
		return
	}

	childEnv, cleanupChildEnv, envErr := h.tenantChildEnv(tenant.Name, tenant)
	if envErr != nil {
		h.tenantErr(envErr)
		writeTenantJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not prepare the tenant environment"})
		return
	}
	defer cleanupChildEnv()

	result := h.runTenantTool(r.Context(), tenant, spec, childArgs, childEnv)
	h.recordAudit(audit, tenants.AuditEntry{
		Tenant:   tenant.Name,
		Action:   body.Tool,
		Decision: "execute",
		ExitCode: result.ExitCode,
		Detail:   tenantAuditDetail(result),
	})
	writeTenantJSON(w, http.StatusOK, map[string]any{
		"tenant":    tenant.Name,
		"tool":      result.Tool,
		"service":   result.Service,
		"risk":      result.Risk,
		"exit_code": result.ExitCode,
		"stdout":    result.Stdout,
		"stderr":    result.Stderr,
	})
}

// recordAudit never swallows audit failures: execute-path failures are logged
// loudly so the operator knows an action went unrecorded.
func (h *tenantServeHandler) recordAudit(audit *tenants.AuditLog, entry tenants.AuditEntry) {
	if err := audit.Append(entry); err != nil {
		h.tenantErr(fmt.Errorf("AUDIT append failed for tenant %s action %s: %w", entry.Tenant, entry.Action, err))
	}
}

func tenantAuditDetail(result mcpCommandResult) string {
	if result.Stderr != "" {
		return result.Stderr
	}
	return "ok"
}

type tenantAllowState int

const (
	tenantAllowUnknown tenantAllowState = iota
	tenantAllowKnown
	tenantAllowExplicit
)

func tenantFindTool(tenant tenants.Tenant, toolName string) (*mcpToolSpec, tenantAllowState) {
	tools := allowedTools(tenant)
	for index, spec := range tools {
		if spec.Name == toolName {
			return &tools[index], tenantAllowExplicit
		}
	}

	if len(tenant.AllowTools) > 0 {
		for _, spec := range mcpAllTools() {
			if spec.Name == toolName {
				return nil, tenantAllowKnown
			}
		}
	}
	return nil, tenantAllowUnknown
}

func mcpCallRequest(tool string, arguments map[string]any) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Name = tool
	if arguments == nil {
		arguments = map[string]any{}
	}
	request.Params.Arguments = arguments
	return request
}

// tenantChildEnv builds a minimal, tenant-scoped child environment. Operator
// secrets and unrelated GOG_* overrides are NOT inherited. The derived keyring
// password is materialized only for the duration of one child invocation and
// removed before the result is returned.
func (h *tenantServeHandler) tenantChildEnv(tenantName string, tenant tenants.Tenant) ([]string, func(), error) {
	password, err := tenants.DeriveKeyringPassword(h.masterKey, tenantName)
	if err != nil {
		return nil, nil, err
	}

	home, homeErr := h.tenantHome(tenant.Name)
	if homeErr != nil {
		return nil, nil, homeErr
	}
	passwordFile := filepath.Join(home, "keys", "keyring-password")
	if writeErr := os.MkdirAll(filepath.Dir(passwordFile), 0o700); writeErr != nil {
		return nil, nil, writeErr
	}
	if writeErr := os.WriteFile(passwordFile, []byte(hex.EncodeToString(password)+"\n"), 0o600); writeErr != nil {
		return nil, nil, fmt.Errorf("write tenant keyring password file: %w", writeErr)
	}
	cleanup := func() { _ = os.Remove(passwordFile) }

	env := make([]string, 0, 10+len(h.childEnvExtra))
	env = append(env,
		"PATH="+os.Getenv("PATH"),
		"HOME="+os.Getenv("HOME"),
		"TERM="+os.Getenv("TERM"),
		"LANG="+os.Getenv("LANG"),
		"LC_ALL="+os.Getenv("LC_ALL"),
		"TMPDIR="+os.Getenv("TMPDIR"),
		"GOG_HOME="+home,
		"GOG_ACCOUNT="+tenant.Account,
		"GOG_KEYRING_BACKEND=file",
		"GOG_KEYRING_PASSWORD_FILE="+passwordFile,
	)
	env = append(env, h.childEnvExtra...)
	return env, cleanup, nil
}

func (h *tenantServeHandler) runTenantTool(callCtx context.Context, tenant tenants.Tenant, spec *mcpToolSpec, childArgs []string, childEnv []string) mcpCommandResult {
	ctx, cancel := context.WithTimeout(callCtx, h.timeout)
	defer cancel()

	baseArgs := []string{"--account", tenant.Account, "--json"}
	if tenant.ReadOnly {
		baseArgs = append(baseArgs, "--readonly")
	}

	args := append(append([]string{}, baseArgs...), childArgs...)
	//nolint:gosec // argv comes from typed tool schemas, not model-supplied shell text.
	cmd := exec.CommandContext(ctx, h.self, args...)
	cmd.Env = childEnv
	stdoutBuf := &tenantLimitedBuffer{max: tenantMaxOutputBytes}
	stderrBuf := &tenantLimitedBuffer{max: tenantMaxOutputBytes}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	if ctx.Err() == context.DeadlineExceeded {
		exitCode = 124
	}

	return mcpCommandResult{
		Tool:     spec.Name,
		Service:  spec.Service,
		Risk:     string(spec.Risk),
		ExitCode: exitCode,
		Stdout:   parseMCPStdout(stdoutBuf.String()),
		Stderr:   stderrBuf.String(),
	}
}

func writeTenantJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type tenantLimitedBuffer struct {
	buf  []byte
	max  int
	caps bool
}

func (b *tenantLimitedBuffer) Write(p []byte) (int, error) {
	if !b.caps {
		remaining := b.max - len(b.buf)
		if len(p) <= remaining {
			b.buf = append(b.buf, p...)
			return len(p), nil
		}
		b.buf = append(b.buf, p[:remaining]...)
		b.caps = true
	}
	return len(p), nil
}

func (b *tenantLimitedBuffer) String() string {
	return string(b.buf)
}
