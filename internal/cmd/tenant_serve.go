package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/tenants"
	"github.com/openclaw/gogcli/internal/ui"
)

type TenantServeCmd struct {
	Port           int    `name:"port" help:"HTTP port (binds 127.0.0.1 only)"`
	MasterKeyFile  string `name:"master-key-file" type:"path" help:"File holding the master secret; enables per-tenant encrypted file keyrings"`
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
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	timeout := time.Duration(c.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		return usage("--timeout-seconds must be greater than zero")
	}

	handler := &tenantServeHandler{
		store:     store,
		self:      self,
		masterKey: masterKey,
		timeout:   timeout,
	}
	addr := fmt.Sprintf("127.0.0.1:%d", c.Port)
	ui.FromContext(ctx).Out().Linef("gog tenant API listening on http://%s (tenants: /tenants/<name>/tools, /tenants/<name>/call)", addr)
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
	raw, err := os.ReadFile(path) //nolint:gosec // operator-provided key file path
	if err != nil {
		return nil, fmt.Errorf("read --master-key-file: %w", err)
	}
	return []byte(strings.TrimSpace(string(raw))), nil
}

type tenantServeHandler struct {
	store     *tenants.Store
	self      string
	masterKey []byte
	timeout   time.Duration
}

func (h *tenantServeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	switch {
	case r.Method == http.MethodGet && path == "healthz":
		writeTenantJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case r.Method == http.MethodGet && path == "tenants":
		h.handleList(w)
	case r.Method == http.MethodPost && strings.Count(path, "/") == 2 && strings.HasSuffix(path, "/tools"):
		h.handleListTools(w, tenantNameFromPath(path))
	case r.Method == http.MethodPost && strings.Count(path, "/") == 2 && strings.HasSuffix(path, "/call"):
		h.handleCall(w, r, tenantNameFromPath(path))
	default:
		writeTenantJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func tenantNameFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "tenants" {
		return ""
	}
	return parts[1]
}

func (h *tenantServeHandler) handleList(w http.ResponseWriter) {
	tenantsList, err := h.store.List()
	if err != nil {
		writeTenantJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(tenantsList))
	for _, tenant := range tenantsList {
		out = append(out, map[string]any{
			"name":     tenant.Name,
			"account":  tenant.Account,
			"readonly": tenant.ReadOnly,
		})
	}
	writeTenantJSON(w, http.StatusOK, map[string]any{"tenants": out})
}

// allowedTools applies the tenant policy: an explicit allow-tools list wins,
// and with no list only read-risk tools are exposed. Write tools require an
// explicit allowlist and a non-readonly tenant.
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

func (h *tenantServeHandler) handleListTools(w http.ResponseWriter, name string) {
	tenant, ok, err := h.store.Get(name)
	if err != nil || !ok {
		writeTenantJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("tenant %q not found", name)})
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

func (h *tenantServeHandler) handleCall(w http.ResponseWriter, r *http.Request, name string) {
	tenant, ok, err := h.store.Get(name)
	if err != nil || !ok {
		writeTenantJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("tenant %q not found", name)})
		return
	}

	var body tenantToolRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&body); decodeErr != nil {
		writeTenantJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("decode body: %v", decodeErr)})
		return
	}

	audit := tenants.NewAuditLog(h.store.Home(tenant.Name))
	spec, allowed := tenantFindTool(tenant, body.Tool)
	if spec == nil {
		reason := fmt.Sprintf("unknown tool %q", body.Tool)
		if allowed == tenantAllowKnown {
			reason = fmt.Sprintf("tool %q is not allowlisted for this tenant", body.Tool)
		}
		writeTenantJSON(w, http.StatusForbidden, map[string]any{"error": reason})
		_ = audit.Append(tenants.AuditEntry{Tenant: tenant.Name, Action: body.Tool, Decision: "deny", Detail: reason})
		return
	}

	request := mcpCallRequest(body.Tool, body.Arguments)
	childArgs, err := spec.BuildArgs(request)
	if err != nil {
		writeTenantJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		_ = audit.Append(tenants.AuditEntry{Tenant: tenant.Name, Action: body.Tool, Decision: "deny", Detail: err.Error()})
		return
	}

	childEnv, err := h.tenantChildEnv(tenant.Name, tenant)
	if err != nil {
		writeTenantJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	result := h.runTenantTool(r.Context(), tenant, spec, childArgs, childEnv)
	detail := result.Stderr
	if detail == "" && result.ExitCode == 0 {
		detail = "ok"
	}
	_ = audit.Append(tenants.AuditEntry{
		Tenant:   tenant.Name,
		Action:   body.Tool,
		Decision: "execute",
		ExitCode: result.ExitCode,
		Detail:   detail,
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

func tenantFindTool(tenant tenants.Tenant, toolName string) (*mcpToolSpec, tenantAllowState) {
	for index, spec := range allowedTools(tenant) {
		if spec.Name == toolName {
			return &allowedTools(tenant)[index], tenantAllowExplicit
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

type tenantAllowState int

const (
	tenantAllowUnknown tenantAllowState = iota
	tenantAllowKnown
	tenantAllowExplicit
)

func mcpCallRequest(tool string, arguments map[string]any) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Name = tool
	if arguments == nil {
		arguments = map[string]any{}
	}
	request.Params.Arguments = arguments
	return request
}

// tenantChildEnv builds the child process environment. The tenant home
// isolates config, tokens, cache, and state; the per-tenant file keyring
// password (derived with HKDF from the operator master secret) keeps OAuth
// tokens encrypted at rest per tenant.
func (h *tenantServeHandler) tenantChildEnv(tenantName string, tenant tenants.Tenant) ([]string, error) {
	env := append(os.Environ(),
		"GOG_HOME="+h.store.Home(tenant.Name),
		"GOG_ACCOUNT="+tenant.Account,
	)
	if len(h.masterKey) > 0 {
		password, err := tenants.DeriveKeyringPassword(h.masterKey, tenantName)
		if err != nil {
			return nil, err
		}
		env = append(env,
			"GOG_KEYRING_BACKEND=file",
			"GOG_KEYRING_PASSWORD="+hex.EncodeToString(password),
		)
	}
	return env, nil
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

const tenantMaxOutputBytes = 4 << 20

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
