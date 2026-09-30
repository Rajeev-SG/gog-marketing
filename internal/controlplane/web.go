package controlplane

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Authenticator interface {
	Login(ctx context.Context, email, credential string) (Actor, error)
}

type OwnerAuthenticator struct {
	Email string
	Token string
	Actor Actor
}

func (a OwnerAuthenticator) Login(_ context.Context, email, credential string) (Actor, error) {
	if normalizeEmail(email) != normalizeEmail(a.Email) {
		return Actor{}, ErrForbidden
	}

	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(credential)), []byte(strings.TrimSpace(a.Token))) != 1 {
		return Actor{}, ErrForbidden
	}

	return a.Actor, nil
}

type sessionPayload struct {
	UserID    string `json:"u"`
	OrgID     string `json:"o"`
	Role      string `json:"r"`
	CSRF      string `json:"c"`
	ExpiresAt int64  `json:"e"`
	Admin     bool   `json:"a,omitempty"`
}

type SessionManager struct {
	key    []byte
	ttl    time.Duration
	secure bool
}

func NewSessionManager(key []byte, ttl time.Duration, secure bool) (*SessionManager, error) {
	if len(key) < 16 {
		return nil, ErrSessionKey
	}

	if ttl <= 0 {
		ttl = 12 * time.Hour
	}

	return &SessionManager{key: key, ttl: ttl, secure: secure}, nil
}

func (m *SessionManager) New(actor Actor) (string, sessionPayload, error) {
	return m.newSession(actor, true)
}

func (m *SessionManager) NewProduct(actor Actor) (string, sessionPayload, error) {
	return m.newSession(actor, false)
}

func (m *SessionManager) newSession(actor Actor, admin bool) (string, sessionPayload, error) {
	csrf, err := randomToken(24)
	if err != nil {
		return "", sessionPayload{}, wrapControlPlaneError(err)
	}
	payload := sessionPayload{
		UserID: actor.UserID, OrgID: actor.OrganizationID, Role: actor.Role,
		CSRF: csrf, ExpiresAt: time.Now().Add(m.ttl).Unix(), Admin: admin,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", sessionPayload{}, wrapControlPlaneError(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	sig := m.sign(encoded)

	return encoded + "." + sig, payload, nil
}

func (m *SessionManager) FromRequest(r *http.Request) (sessionPayload, bool) {
	cookie, err := r.Cookie("gog_control_plane_session")
	if err != nil {
		return sessionPayload{}, false
	}

	return m.decodeSession(cookie.Value)
}

func (m *SessionManager) FromProductRequest(r *http.Request) (sessionPayload, bool) {
	cookie, err := r.Cookie("gog_marketing_product_session")
	if err != nil {
		return sessionPayload{}, false
	}

	payload, ok := m.decodeSession(cookie.Value)

	return payload, ok && !payload.Admin
}

func (m *SessionManager) decodeSession(value string) (sessionPayload, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(m.sign(parts[0])), []byte(parts[1])) {
		return sessionPayload{}, false
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionPayload{}, false
	}

	var payload sessionPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.ExpiresAt < time.Now().Unix() {
		return sessionPayload{}, false
	}

	return payload, true
}

func (m *SessionManager) Cookie(value string) *http.Cookie {
	//nolint:gosec // Secure is configurable so the documented local HTTP smoke run works.
	return &http.Cookie{
		Name: "gog_control_plane_session", Value: value, Path: "/", HttpOnly: true,
		Secure: m.secure, SameSite: http.SameSiteStrictMode, MaxAge: int(m.ttl.Seconds()),
	}
}

// ProductCookie permits the top-level Google callback redirect chain. Product
// mutations still require CSRF tokens; OAuth additionally verifies state and PKCE.
func (m *SessionManager) ProductCookie(value string) *http.Cookie {
	cookie := m.Cookie(value) //nolint:gosec // The tested factory retains Secure/HttpOnly; Lax is required for Google callbacks and mutations remain CSRF-protected.
	cookie.Name = "gog_marketing_product_session"
	cookie.SameSite = http.SameSiteLaxMode

	return cookie
}

func (m *SessionManager) ClearCookie() *http.Cookie {
	//nolint:gosec // Secure is configurable so the documented local HTTP smoke run works.
	return &http.Cookie{
		Name: "gog_control_plane_session", Value: "", Path: "/", HttpOnly: true,
		Secure: m.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1,
	}
}

func (m *SessionManager) ClearProductCookie() *http.Cookie {
	cookie := m.ClearCookie() //nolint:gosec // Retains the tested clear-cookie attributes while isolating the product cookie name.
	cookie.Name = "gog_marketing_product_session"
	cookie.SameSite = http.SameSiteLaxMode

	return cookie
}

func (m *SessionManager) sign(value string) string {
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(value))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *SessionManager) EncodeSigned(value string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(value))
	return encoded + "." + m.sign(encoded)
}

func (m *SessionManager) DecodeSigned(value string) (string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(m.sign(parts[0])), []byte(parts[1])) {
		return "", false
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}

	return string(raw), true
}

type WebConfig struct {
	Service         *Service
	Sessions        *SessionManager
	Authenticator   Authenticator
	OwnerEmail      string
	DisplayName     string
	ExternalBaseURL string
	BasePath        string
}

type WebHandler struct {
	config    WebConfig
	mux       *http.ServeMux
	templates *template.Template
}

func NewWebHandler(config WebConfig) (*WebHandler, error) {
	if config.Service == nil || config.Sessions == nil || config.Authenticator == nil {
		return nil, ErrWebDependencies
	}

	basePath := strings.TrimRight(strings.TrimSpace(config.BasePath), "/")
	config.BasePath = basePath

	templates, err := template.New("root").Funcs(template.FuncMap{"adminPath": func(path string) string { return basePath + path }}).Parse(webTemplates)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}
	handler := &WebHandler{config: config, mux: http.NewServeMux(), templates: templates}
	handler.routes()

	return handler, nil
}

func (h *WebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *WebHandler) path(path string) string {
	return strings.TrimRight(h.config.BasePath, "/") + path
}

func (h *WebHandler) routes() {
	base := strings.TrimRight(h.config.BasePath, "/")
	h.mux.HandleFunc("GET "+base+"/{$}", h.handleConnections)
	h.mux.HandleFunc("GET "+base+"/login", h.handleLogin)
	h.mux.HandleFunc("POST "+base+"/login", h.handleLoginSubmit)
	h.mux.HandleFunc("POST "+base+"/logout", h.handleLogout)
	h.mux.HandleFunc("GET "+base+"/connections", h.handleConnections)
	h.mux.HandleFunc("POST "+base+"/connections", h.handleCreateConnection)
	h.mux.HandleFunc("GET "+base+"/connections/{id}", h.handleConnection)
	h.mux.HandleFunc("POST "+base+"/connections/{id}/rename", h.handleRename)
	h.mux.HandleFunc("POST "+base+"/connections/{id}/reconnect", h.handleReconnect)
	h.mux.HandleFunc("POST "+base+"/connections/{id}/disconnect", h.handleDisconnect)
	h.mux.HandleFunc("POST "+base+"/connections/{id}/discover", h.handleDiscover)
	h.mux.HandleFunc("POST "+base+"/connections/{id}/refresh", h.handleRefresh)
	h.mux.HandleFunc("POST "+base+"/connections/{id}/resources/{resourceID...}", h.handleResourceToggle)
	h.mux.HandleFunc("GET "+base+"/oauth/google/start", h.handleOAuthStart)
	h.mux.HandleFunc("GET "+base+"/oauth/google/callback", h.handleOAuthCallback)
}

func (h *WebHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, "login", map[string]any{"OwnerEmail": h.config.OwnerEmail})
}

func (h *WebHandler) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	actor, err := h.config.Authenticator.Login(r.Context(), r.FormValue("email"), r.FormValue("token"))
	if err != nil {
		h.render(w, http.StatusUnauthorized, "login", map[string]any{templateErrorField: "This identity is not authorized.", "OwnerEmail": h.config.OwnerEmail})
		return
	}

	token, _, err := h.config.Sessions.New(actor)
	if err != nil {
		http.Error(w, "session unavailable", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, h.config.Sessions.Cookie(token))
	http.Redirect(w, r, h.path("/connections"), http.StatusSeeOther)
}

func (h *WebHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, h.config.Sessions.ClearCookie())
	http.Redirect(w, r, h.path("/login"), http.StatusSeeOther)
}

func (h *WebHandler) actor(w http.ResponseWriter, r *http.Request) (Actor, sessionPayload, bool) {
	session, ok := h.config.Sessions.FromRequest(r)
	if !ok || !session.Admin {
		http.Redirect(w, r, h.path("/login"), http.StatusSeeOther)
		return Actor{}, sessionPayload{}, false
	}

	return Actor{UserID: session.UserID, OrganizationID: session.OrgID, Role: session.Role}, session, true
}

func (h *WebHandler) csrfOK(w http.ResponseWriter, r *http.Request, session sessionPayload) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}

	if !hmac.Equal([]byte(r.FormValue("csrf")), []byte(session.CSRF)) {
		http.Error(w, "invalid CSRF token", http.StatusBadRequest)
		return false
	}

	return true
}

func (h *WebHandler) handleConnections(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok {
		return
	}

	connections, err := h.config.Service.ListConnections(r.Context(), actor)
	if err != nil {
		h.render(w, http.StatusInternalServerError, "connections", map[string]any{templateErrorField: safeOAuthError(err)})
		return
	}

	h.render(w, http.StatusOK, "connections", map[string]any{
		"Actor": actor, "CSRF": session.CSRF, "Connections": connections,
		"DisplayName": h.config.DisplayName,
	})
}

func (h *WebHandler) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok || !h.csrfOK(w, r, session) {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	services := strings.Split(r.FormValue("services"), ",")
	if _, err := h.config.Service.CreateConnection(r.Context(), actor, r.FormValue("name"), services); err != nil {
		http.Redirect(w, r, h.path("/connections")+"?error="+url.QueryEscape(safeOAuthError(err)), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, h.path("/connections"), http.StatusSeeOther)
}

func (h *WebHandler) handleConnection(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")

	connection, err := h.config.Service.GetConnection(r.Context(), actor, id)
	if err != nil {
		h.render(w, http.StatusNotFound, "connection", map[string]any{templateErrorField: "Connection not found."})
		return
	}

	resources, err := h.config.Service.ListResources(r.Context(), actor, id)
	if err != nil {
		h.render(w, http.StatusInternalServerError, "connection", map[string]any{templateErrorField: safeOAuthError(err)})
		return
	}

	h.render(w, http.StatusOK, "connection", map[string]any{
		"Actor": actor, "CSRF": session.CSRF, "Connection": connection,
		"Resources": resources, "Message": r.URL.Query().Get("message"), templateErrorField: r.URL.Query().Get("error"),
	})
}

func (h *WebHandler) mutate(w http.ResponseWriter, r *http.Request, action func(context.Context, Actor) error) {
	actor, session, ok := h.actor(w, r)
	if !ok || !h.csrfOK(w, r, session) {
		return
	}

	id := r.PathValue("id")
	if err := action(r.Context(), actor); err != nil {
		http.Redirect(w, r, h.path("/connections/")+url.PathEscape(id)+"?error="+url.QueryEscape(safeOAuthError(err)), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, h.path("/connections/")+url.PathEscape(id), http.StatusSeeOther)
}

func (h *WebHandler) handleRename(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id := r.PathValue("id")
	h.mutate(w, r, func(ctx context.Context, actor Actor) error {
		_, renameErr := h.config.Service.RenameConnection(ctx, actor, id, r.FormValue("name"))
		if renameErr != nil {
			return fmt.Errorf("rename connection: %w", renameErr)
		}

		return nil
	})
}

func (h *WebHandler) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.mutate(w, r, func(ctx context.Context, actor Actor) error {
		return h.config.Service.Disconnect(ctx, actor, id)
	})
}

func (h *WebHandler) handleDiscover(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.mutate(w, r, func(ctx context.Context, actor Actor) error {
		_, discoverErr := h.config.Service.Discover(ctx, actor, id)
		if discoverErr != nil {
			return fmt.Errorf("discover resources: %w", discoverErr)
		}

		return nil
	})
}

func (h *WebHandler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.mutate(w, r, func(ctx context.Context, actor Actor) error {
		_, refreshErr := h.config.Service.Refresh(ctx, actor, id)
		if refreshErr != nil {
			return fmt.Errorf("refresh connection: %w", refreshErr)
		}

		return nil
	})
}

func (h *WebHandler) handleResourceToggle(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id := r.PathValue("id")
	resourceID := r.PathValue("resourceID")
	enabled := r.FormValue("enabled") == "true"
	h.mutate(w, r, func(ctx context.Context, actor Actor) error {
		_, grantErr := h.config.Service.SetResourceEnabled(ctx, actor, id, resourceID, enabled)
		if grantErr != nil {
			return fmt.Errorf("set resource grant: %w", grantErr)
		}

		return nil
	})
}

func (h *WebHandler) handleReconnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	actor, session, ok := h.actor(w, r)
	if !ok || !h.csrfOK(w, r, session) {
		return
	}

	start, err := h.config.Service.BeginOAuth(r.Context(), actor, id, true)
	if err != nil {
		http.Redirect(w, r, h.path("/connections/")+url.PathEscape(id)+"?error="+url.QueryEscape(safeOAuthError(err)), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, start.URL, http.StatusSeeOther)
}

func (h *WebHandler) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("connection")

	start, err := h.config.Service.BeginOAuth(r.Context(), actor, id, false)
	if err != nil {
		http.Redirect(w, r, h.path("/connections")+"?error="+url.QueryEscape(safeOAuthError(err)), http.StatusSeeOther)
		return
	}
	_ = session

	http.Redirect(w, r, start.URL, http.StatusSeeOther)
}

func (h *WebHandler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if oauthError := r.URL.Query().Get("error"); oauthError != "" {
		http.Redirect(w, r, h.path("/connections")+"?error="+url.QueryEscape(oauthError), http.StatusSeeOther)
		return
	}

	connection, err := h.config.Service.CompleteOAuth(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if err != nil {
		http.Redirect(w, r, h.path("/connections")+"?error="+url.QueryEscape(safeOAuthError(err)), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, h.path("/connections/")+url.PathEscape(connection.ID)+"?message="+url.QueryEscape("Google connection is ready."), http.StatusSeeOther)
}

func (h *WebHandler) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = h.templates.ExecuteTemplate(w, name, data)
}

const webTemplates = `
{{define "style"}}
<style>
:root{color-scheme:light dark;font-family:Inter,ui-sans-serif,system-ui,sans-serif}
body{margin:0;background:#f5f7fb;color:#172033}
header{background:#fff;border-bottom:1px solid #d9deea;padding:16px 24px;display:flex;justify-content:space-between;align-items:center;gap:16px}
main{max-width:1120px;margin:28px auto;padding:0 20px}
h1{font-size:26px;margin:0 0 18px} h2{font-size:19px}
a{color:#225bd6;text-decoration:none}
table{width:100%;border-collapse:collapse;background:#fff;border:1px solid #d9deea}
th,td{text-align:left;padding:12px;border-bottom:1px solid #e6eaf2;vertical-align:top}
th{font-size:12px;text-transform:uppercase;color:#5d6880}
button,.button{border:1px solid #b8c2d8;background:#fff;color:#172033;border-radius:6px;padding:8px 11px;cursor:pointer;font:inherit}
button.primary,.button.primary{background:#225bd6;border-color:#225bd6;color:#fff}
form.inline{display:inline}
.panel{background:#fff;border:1px solid #d9deea;padding:18px;margin:14px 0}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}
.badge{display:inline-block;border-radius:999px;padding:3px 8px;background:#e8edf8;font-size:12px}
.error{background:#fff0f0;border:1px solid #efb6b6;padding:10px;margin:10px 0;color:#8b1e1e}
.message{background:#eefaf1;border:1px solid #aad8b7;padding:10px;margin:10px 0;color:#17622c}
input,select{width:100%;box-sizing:border-box;padding:9px;border:1px solid #b8c2d8;border-radius:6px;background:#fff;color:#172033}
label{font-size:13px;color:#4b5872;display:block;margin:8px 0 4px}
.muted{color:#66728a;font-size:13px}.stack{display:flex;gap:8px;flex-wrap:wrap}
@media(max-width:720px){table,thead,tbody,tr,th,td{display:block}thead{display:none}td{border:0;border-bottom:1px solid #e6eaf2}tr{margin:10px 0;border:1px solid #d9deea;background:#fff}}
</style>
{{end}}

{{define "login"}}
<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign in · gog-marketing</title>{{template "style" .}}</head>
<body><header><strong>gog-marketing control plane</strong></header><main>
<div class="panel" style="max-width:520px;margin:60px auto">
<h1>Sign in</h1>
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
<form method="post" action="{{adminPath "/login"}}">
<label for="email">Owner or admin email</label>
<input id="email" name="email" type="email" required autocomplete="email" value="{{.OwnerEmail}}">
<label for="token">Admin access token</label>
<input id="token" name="token" type="password" required autocomplete="current-password">
<button class="primary" type="submit">Continue</button>
</form>
<p class="muted">Control-plane access is separate from Google marketing consent.</p>
</div></main></body></html>
{{end}}

{{define "connections"}}
<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Connections · gog-marketing</title>{{template "style" .}}</head>
<body><header><strong>gog-marketing control plane</strong><span>{{.DisplayName}}</span>
<form class="inline" method="post" action="{{adminPath "/logout"}}"><button>Sign out</button></form></header><main>
<h1>Connections</h1>
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
<div class="panel"><h2>Add connection</h2>
<form method="post" action="{{adminPath "/connections"}}">
<input type="hidden" name="csrf" value="{{.CSRF}}">
<div class="grid"><div><label>Name</label><input name="name" required placeholder="gmail"></div>
<div><label>Services</label><input name="services" value="analytics,tagmanager,googleads,searchconsole,bigquery"></div></div>
<button class="primary" type="submit">Add connection</button>
</form></div>
<table><thead><tr><th>Connection</th><th>Google account</th><th>Services</th><th>Status</th><th>Resources</th><th></th></tr></thead><tbody>
{{range .Connections}}<tr>
<td><a href="{{adminPath "/connections"}}/{{.ID}}"><strong>{{.Name}}</strong></a></td>
<td>{{if .GoogleEmail}}{{.GoogleEmail}}{{else}}<span class="muted">Not connected</span>{{end}}</td>
<td>{{range .Services}}<span class="badge">{{.}}</span> {{end}}</td>
<td><span class="badge">{{.Status}}</span></td>
<td>{{if .LastValidatedAt}}validated {{.LastValidatedAt.Format "2006-01-02 15:04"}}{{else}}—{{end}}</td>
<td><a class="button" href="{{adminPath "/connections"}}/{{.ID}}">Open</a></td>
</tr>{{else}}<tr><td colspan="6">No connections yet.</td></tr>{{end}}
</tbody></table>
</main></body></html>
{{end}}

{{define "connection"}}
<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Connection.Name}} · gog-marketing</title>{{template "style" .}}</head>
<body><header><strong>gog-marketing control plane</strong><a href="{{adminPath "/connections"}}">All connections</a>
<form class="inline" method="post" action="{{adminPath "/logout"}}"><button>Sign out</button></form></header><main>
<h1>{{.Connection.Name}}</h1>
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
{{if .Message}}<div class="message">{{.Message}}</div>{{end}}
<div class="grid">
<div class="panel"><h2>Google identity</h2>
<p>{{if .Connection.GoogleEmail}}{{.Connection.GoogleEmail}}{{else}}Not connected{{end}}</p>
<p><span class="badge">{{.Connection.Status}}</span></p>
{{if .Connection.LastValidatedAt}}<p class="muted">Last validated {{.Connection.LastValidatedAt.Format "2006-01-02 15:04 UTC"}}</p>{{end}}
{{if .Connection.LastError}}<div class="error">{{.Connection.LastError}}</div>{{end}}
<div class="stack">
<form class="inline" method="get" action="{{adminPath "/oauth/google/start"}}"><input type="hidden" name="connection" value="{{.Connection.ID}}"><button class="primary">Connect Google</button></form>
<form class="inline" method="post" action="{{adminPath "/connections"}}/{{.Connection.ID}}/reconnect"><input type="hidden" name="csrf" value="{{.CSRF}}"><button>Reconnect</button></form>
<form class="inline" method="post" action="{{adminPath "/connections"}}/{{.Connection.ID}}/refresh"><input type="hidden" name="csrf" value="{{.CSRF}}"><button>Validate</button></form>
</div></div>
<div class="panel"><h2>Services and scopes</h2>
<p>{{range .Connection.Services}}<span class="badge">{{.}}</span> {{end}}</p>
<details><summary>Granted OAuth scopes</summary><ul>{{range .Connection.GrantedScopes}}<li class="muted">{{.}}</li>{{else}}<li class="muted">None yet</li>{{end}}</ul></details>
<form method="post" action="{{adminPath "/connections"}}/{{.Connection.ID}}/rename"><input type="hidden" name="csrf" value="{{.CSRF}}">
<label>Rename</label><input name="name" value="{{.Connection.Name}}" required><button type="submit">Save</button></form>
</div></div>
<div class="panel"><h2>Approved resources</h2>
<form class="inline" method="post" action="{{adminPath "/connections"}}/{{.Connection.ID}}/discover"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="primary">Refresh and discover</button></form>
<table><thead><tr><th>Service</th><th>Type</th><th>Resource</th><th>Parent</th><th>Expose</th></tr></thead><tbody>
{{range .Resources}}<tr><td>{{.Service}}</td><td>{{.ResourceType}}</td><td><strong>{{.DisplayName}}</strong><br><span class="muted">{{.ResourceID}}</span></td><td>{{.Parent}}</td>
<td><form class="inline" method="post" action="{{adminPath "/connections"}}/{{$.Connection.ID}}/resources/{{urlquery .ResourceID}}"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="enabled" value="{{if .Enabled}}false{{else}}true{{end}}"><button>{{if .Enabled}}Disable{{else}}Enable{{end}}</button></form></td></tr>
{{else}}<tr><td colspan="5">No discovered resources.</td></tr>{{end}}
</tbody></table></div>
<div class="panel"><h2>Disconnect</h2><p class="muted">Deletes the stored encrypted refresh token and revokes it when Google supports revocation.</p>
<form class="inline" method="post" action="{{adminPath "/connections"}}/{{.Connection.ID}}/disconnect"><input type="hidden" name="csrf" value="{{.CSRF}}"><button>Disconnect</button></form></div>
</main></body></html>
{{end}}
`
