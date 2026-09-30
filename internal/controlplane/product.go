package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	templateErrorField           = "Error"
	productConnectionName        = "google"
	productSigninStatePrefix     = "signin_"
	productSigninStateCookie     = "gog_product_signin_oauth"
	productConnectionStateCookie = "gog_product_connection_oauth"
)

type ProductIdentity struct {
	Email         string
	EmailVerified bool
	Subject       string
}

type ProductAuthenticator interface {
	AuthorizationURL(state, codeVerifier string) string
	Complete(ctx context.Context, state, code, codeVerifier string) (ProductIdentity, error)
	Actor(ctx context.Context, identity ProductIdentity) (Actor, error)
}

type GoogleProductAuthenticator struct {
	Config      oauth2.Config
	HTTPClient  *http.Client
	UserInfoURL string
	OwnerEmail  string
	OwnerActor  Actor
}

func (a *GoogleProductAuthenticator) userInfoEndpoint() (*url.URL, error) {
	parsed, err := url.Parse(a.UserInfoURL)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}

	if parsed.Scheme != "https" || parsed.Host == "" {
		return nil, ErrGoogleIdentityHTTP
	}

	return parsed, nil
}

func NewGoogleProductAuthenticator(clientID, clientSecret, redirectURI, ownerEmail string, actor Actor) *GoogleProductAuthenticator {
	return &GoogleProductAuthenticator{
		Config: oauth2.Config{
			ClientID:     strings.TrimSpace(clientID),
			ClientSecret: strings.TrimSpace(clientSecret),
			RedirectURL:  strings.TrimSpace(redirectURI),
			Endpoint:     google.Endpoint,
			Scopes:       []string{"openid", "email", "profile"},
		},
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
		UserInfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
		OwnerEmail:  normalizeEmail(ownerEmail),
		OwnerActor:  actor,
	}
}

func (a *GoogleProductAuthenticator) AuthorizationURL(state, codeVerifier string) string {
	return a.Config.AuthCodeURL(state, oauth2.AccessTypeOnline, oauth2.S256ChallengeOption(codeVerifier))
}

func (a *GoogleProductAuthenticator) Complete(ctx context.Context, _ string, code, codeVerifier string) (ProductIdentity, error) {
	token, err := a.Config.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return ProductIdentity{}, wrapControlPlaneError(err)
	}

	userInfoURL, err := a.userInfoEndpoint()
	if err != nil {
		return ProductIdentity{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL.String(), nil) //nolint:gosec // endpoint is restricted to HTTPS
	if err != nil {
		return ProductIdentity{}, wrapControlPlaneError(err)
	}

	token.SetAuthHeader(req)

	resp, err := a.HTTPClient.Do(req) //nolint:gosec // endpoint is restricted to HTTPS
	if err != nil {
		return ProductIdentity{}, wrapControlPlaneError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProductIdentity{}, ErrGoogleIdentityHTTP
	}

	var identity struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Sub           string `json:"sub"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return ProductIdentity{}, wrapControlPlaneError(err)
	}

	if identity.Sub == "" {
		return ProductIdentity{}, ErrGoogleIdentitySubject
	}

	return ProductIdentity{
		Email:         normalizeEmail(identity.Email),
		EmailVerified: identity.EmailVerified,
		Subject:       identity.Sub,
	}, nil
}

func (a *GoogleProductAuthenticator) Actor(_ context.Context, identity ProductIdentity) (Actor, error) {
	if !identity.EmailVerified || identity.Subject == "" || a.OwnerEmail == "" || normalizeEmail(identity.Email) != a.OwnerEmail {
		return Actor{}, ErrForbidden
	}

	return a.OwnerActor, nil
}

type ProductConfig struct {
	Service         *Service
	Sessions        *SessionManager
	Auth            ProductAuthenticator
	DisplayName     string
	DefaultServices []string
}

type ProductHandler struct {
	config    ProductConfig
	mux       *http.ServeMux
	templates *template.Template
}

type productOAuthState struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier,omitempty"`
	ConnectionID string    `json:"connection_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type productAsset struct {
	Service     string
	ServiceName string
	Kind        string
	Name        string
	ResourceID  string
	Parent      string
	Enabled     bool
}

type productAssetGroup struct {
	ServiceName  string
	Assets       []productAsset
	Count        int
	EnabledCount int
}

func NewProductHandler(config ProductConfig) (*ProductHandler, error) {
	if config.Service == nil || config.Sessions == nil || config.Auth == nil {
		return nil, ErrWebDependencies
	}

	t, err := template.New("product").Funcs(template.FuncMap{
		"serviceLabel": serviceLabel,
		"kindLabel":    kindLabel,
	}).Parse(productTemplates)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}
	h := &ProductHandler{config: config, mux: http.NewServeMux(), templates: t}
	h.routes()

	return h, nil
}

func (h *ProductHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

func (h *ProductHandler) routes() {
	h.mux.HandleFunc("GET /{$}", h.home)
	h.mux.HandleFunc("GET /signin", h.signin)
	h.mux.HandleFunc("GET /auth/google/start", h.authStart)
	h.mux.HandleFunc("GET /oauth/google/callback", h.oauthCallback)
	h.mux.HandleFunc("POST /logout", h.logout)
	h.mux.HandleFunc("POST /connect/google", h.connect)
	h.mux.HandleFunc("GET /assets/{id}", h.assets)
	h.mux.HandleFunc("POST /assets/{id}/save", h.saveAssets)
	h.mux.HandleFunc("POST /assets/{id}/discover", h.discover)
	h.mux.HandleFunc("POST /assets/{id}/reconnect", h.reconnect)
}

func (h *ProductHandler) actor(w http.ResponseWriter, r *http.Request) (Actor, sessionPayload, bool) {
	session, ok := h.config.Sessions.FromRequest(r)
	if !ok || session.Admin {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return Actor{}, sessionPayload{}, false
	}

	return Actor{UserID: session.UserID, OrganizationID: session.OrgID, Role: session.Role}, session, true
}

func (h *ProductHandler) csrfOK(w http.ResponseWriter, r *http.Request, session sessionPayload) bool {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return false
	}

	if !constantTimeEqual(r.FormValue("csrf"), session.CSRF) {
		http.Error(w, "invalid CSRF token", http.StatusBadRequest)
		return false
	}

	return true
}

func (h *ProductHandler) signin(w http.ResponseWriter, r *http.Request) {
	h.render(w, "signin", map[string]any{templateErrorField: r.URL.Query().Get("error")})
}

func (h *ProductHandler) authStart(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken(32)
	if err != nil {
		http.Error(w, "Sign-in is temporarily unavailable.", http.StatusInternalServerError)

		return
	}
	state = productSigninStatePrefix + state

	codeVerifier, err := randomToken(48)
	if err != nil {
		http.Error(w, "Sign-in is temporarily unavailable.", http.StatusInternalServerError)

		return
	}

	h.setProductOAuthCookie(w, productSigninStateCookie, productOAuthState{
		State: state, CodeVerifier: codeVerifier, ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	http.Redirect(w, r, h.config.Auth.AuthorizationURL(state, codeVerifier), http.StatusSeeOther)
}

func (h *ProductHandler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if strings.HasPrefix(state, productSigninStatePrefix) {
		h.authCallback(w, r, state)

		return
	}

	h.googleCallback(w, r)
}

func (h *ProductHandler) authCallback(w http.ResponseWriter, r *http.Request, state string) {
	stored, ok := h.takeProductOAuthCookie(r, w, productSigninStateCookie)
	if !ok || stored.State != state {
		http.Redirect(w, r, "/signin?error="+url.QueryEscape("This sign-in link has expired."), http.StatusSeeOther)

		return
	}

	codeVerifier := stored.CodeVerifier

	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, "/signin?error="+url.QueryEscape("Google sign-in was not completed."), http.StatusSeeOther)

		return
	}

	identity, err := h.config.Auth.Complete(r.Context(), state, r.URL.Query().Get("code"), codeVerifier)
	if err != nil {
		http.Redirect(w, r, "/signin?error="+url.QueryEscape("Google sign-in was not completed."), http.StatusSeeOther)

		return
	}

	actor, err := h.config.Auth.Actor(r.Context(), identity)
	if err != nil {
		http.Redirect(w, r, "/signin?error="+url.QueryEscape("This account does not have workspace access."), http.StatusSeeOther)

		return
	}

	token, _, err := h.config.Sessions.NewProduct(actor)
	if err != nil {
		http.Error(w, "Sign-in is temporarily unavailable.", http.StatusInternalServerError)

		return
	}

	http.SetCookie(w, h.config.Sessions.Cookie(token))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *ProductHandler) logout(w http.ResponseWriter, r *http.Request) {
	_, session, ok := h.actor(w, r)
	if !ok || !h.csrfOK(w, r, session) {
		return
	}

	http.SetCookie(w, h.config.Sessions.ClearCookie())
	http.Redirect(w, r, "/signin", http.StatusSeeOther)
}

func (h *ProductHandler) connect(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok || !h.csrfOK(w, r, session) {
		return
	}

	connection, err := h.productConnection(r.Context(), actor)
	if err != nil {
		http.Redirect(w, r, "/?error="+url.QueryEscape("We couldn't start Google onboarding."), http.StatusSeeOther)

		return
	}

	if productConnectionState(connection) == "connected" {
		if _, discoverErr := h.config.Service.Discover(r.Context(), actor, connection.ID); discoverErr != nil {
			message, needsReconnect := productFailureMessage(discoverErr)
			if needsReconnect {
				http.Redirect(w, r, "/?error="+url.QueryEscape(message), http.StatusSeeOther)
				return
			}

			http.Redirect(w, r, "/assets/"+url.PathEscape(connection.ID)+"?error="+url.QueryEscape(message), http.StatusSeeOther)

			return
		}

		http.Redirect(w, r, "/assets/"+url.PathEscape(connection.ID), http.StatusSeeOther)

		return
	}

	start, err := h.config.Service.BeginOAuth(r.Context(), actor, connection.ID, connection.Status != ConnectionNeedsConnect)
	if err != nil {
		http.Redirect(w, r, "/?error="+url.QueryEscape("Google needs you to reconnect this account."), http.StatusSeeOther)

		return
	}

	h.setProductOAuthCookie(w, productConnectionStateCookie, productOAuthState{
		State: start.State, ConnectionID: connection.ID, ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	http.Redirect(w, r, start.URL, http.StatusSeeOther)
}

func (h *ProductHandler) googleCallback(w http.ResponseWriter, r *http.Request) {
	session, ok := h.config.Sessions.FromRequest(r)
	if !ok || session.Admin {
		http.Redirect(w, r, "/signin?error="+url.QueryEscape("Your sign-in session expired. Sign in and choose Connect Google again."), http.StatusSeeOther)
		return
	}

	actor := Actor{UserID: session.UserID, OrganizationID: session.OrgID, Role: session.Role}

	stored, ok := h.takeProductOAuthCookie(r, w, productConnectionStateCookie)
	if !ok || stored.State != r.URL.Query().Get("state") || stored.ConnectionID == "" {
		http.Redirect(w, r, "/?error="+url.QueryEscape("Google needs you to reconnect this account."), http.StatusSeeOther)
		return
	}

	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, "/?error="+url.QueryEscape("Google needs you to reconnect this account."), http.StatusSeeOther)

		return
	}

	connection, err := h.config.Service.CompleteOAuth(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if err != nil || connection.ID != stored.ConnectionID {
		http.Redirect(w, r, "/?error="+url.QueryEscape("Google needs you to reconnect this account."), http.StatusSeeOther)

		return
	}

	discoverActor := Actor{UserID: actor.UserID, OrganizationID: connection.OrganizationID, Role: actor.Role}
	if _, err := h.config.Service.Discover(r.Context(), discoverActor, connection.ID); err != nil {
		message, needsReconnect := productFailureMessage(err)
		if needsReconnect {
			http.Redirect(w, r, "/?error="+url.QueryEscape(message), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, "/assets/"+url.PathEscape(connection.ID)+"?error="+url.QueryEscape(message), http.StatusSeeOther)

		return
	}

	http.Redirect(w, r, "/assets/"+url.PathEscape(connection.ID), http.StatusSeeOther)
}

func (h *ProductHandler) home(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok {
		return
	}

	connection, err := h.existingProductConnection(r.Context(), actor)
	if err != nil {
		h.render(w, "home", map[string]any{
			"DisplayName":      h.config.DisplayName,
			"State":            "disconnected",
			"StateLabel":       "Disconnected",
			templateErrorField: "We couldn't load your Google data.",
			"CSRF":             session.CSRF,
		})

		return
	}

	state := productConnectionState(connection)
	assets := []productAsset{}

	errorMessage := r.URL.Query().Get("error")
	if connection.ID != "" {
		if grants, listErr := h.config.Service.ListResources(r.Context(), actor, connection.ID); listErr == nil {
			assets = productAssets(grants)
		}
	}

	h.render(w, "home", map[string]any{
		"DisplayName":      h.config.DisplayName,
		"Connection":       connection,
		"State":            state,
		"StateLabel":       productStateLabel(state),
		"Assets":           assets,
		"AssetCount":       len(assets),
		"ServiceCount":     len(productAssetGroups(assets, "")),
		"NeedsReconnect":   state == "needs_attention",
		"Message":          r.URL.Query().Get("message"),
		templateErrorField: errorMessage,
		"CSRF":             session.CSRF,
	})
}

func (h *ProductHandler) assets(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")

	connection, err := h.productConnectionByID(r.Context(), actor, id)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)

		return
	}

	grants, err := h.config.Service.ListResources(r.Context(), actor, id)
	if err != nil {
		h.render(w, "assets", map[string]any{
			"Connection": connection, "State": productConnectionState(connection), "StateLabel": productStateLabel(productConnectionState(connection)),
			templateErrorField: "We couldn't load your Google assets.", "CSRF": session.CSRF,
		})

		return
	}
	assets := productAssets(grants)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	state := productConnectionState(connection)
	h.render(w, "assets", map[string]any{
		"Connection":       connection,
		"State":            state,
		"StateLabel":       productStateLabel(state),
		"Assets":           assets,
		"Groups":           productAssetGroups(assets, query),
		"Query":            query,
		"HasAssets":        len(productAssetGroups(assets, query)) > 0,
		"NeedsReconnect":   state == "needs_attention",
		"CanRetry":         state == "connected",
		templateErrorField: r.URL.Query().Get("error"),
		"CSRF":             session.CSRF,
	})
}

func (h *ProductHandler) saveAssets(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok || !h.csrfOK(w, r, session) {
		return
	}

	id := r.PathValue("id")
	if _, err := h.productConnectionByID(r.Context(), actor, id); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)

		return
	}

	grants, err := h.config.Service.ListResources(r.Context(), actor, id)
	if err != nil {
		h.redirectAssetsError(w, r, id, "We couldn't load the current access selection.")
		return
	}

	if service := r.FormValue("select_all"); service != "" {
		h.setServiceEnabled(w, r, actor, id, grants, service, true)
		return
	}

	if service := r.FormValue("select_none"); service != "" {
		h.setServiceEnabled(w, r, actor, id, grants, service, false)
		return
	}

	// Derive the editable set from scoped grants using the same filter as the page.
	// Hidden assets retain their current access, including an empty result set.
	selected := make(map[string]bool, len(grants))
	for _, grant := range grants {
		selected[grant.ResourceID] = grant.Enabled
	}
	editable := make(map[string]bool)

	for _, group := range productAssetGroups(productAssets(grants), r.FormValue("q")) {
		for _, asset := range group.Assets {
			editable[asset.ResourceID] = true
			selected[asset.ResourceID] = false
		}
	}

	for _, value := range r.Form["resource"] {
		if !editable[value] {
			h.redirectAssetsError(w, r, id, "The selection no longer matches the displayed assets. Reload and try again.")
			return
		}
		selected[value] = true
	}

	if err := h.applyResourceEnabled(r.Context(), actor, id, grants, selected); err != nil {
		h.redirectAssetsError(w, r, id, "Some access changes could not be saved. Review the selection and try again.")
		return
	}

	http.Redirect(w, r, "/?message="+url.QueryEscape("Access saved."), http.StatusSeeOther)
}

func (h *ProductHandler) setServiceEnabled(w http.ResponseWriter, r *http.Request, actor Actor, id string, grants []ResourceGrant, serviceName string, enabled bool) {
	service := serviceFromLabel(serviceName)

	desired := make(map[string]bool, len(grants))
	for _, grant := range grants {
		desired[grant.ResourceID] = grant.Enabled
		if grant.Service == service {
			desired[grant.ResourceID] = enabled
		}
	}

	if err := h.applyResourceEnabled(r.Context(), actor, id, grants, desired); err != nil {
		h.redirectAssetsError(w, r, id, "Some access changes could not be saved. Review the selection and try again.")
		return
	}

	http.Redirect(w, r, "/assets/"+url.PathEscape(id), http.StatusSeeOther)
}

func (h *ProductHandler) applyResourceEnabled(ctx context.Context, actor Actor, id string, grants []ResourceGrant, desired map[string]bool) error {
	for _, grant := range grants {
		want := desired[grant.ResourceID]
		if grant.Enabled == want {
			continue
		}

		if _, err := h.config.Service.SetResourceEnabled(ctx, actor, id, grant.ResourceID, want); err != nil {
			return err
		}
	}

	return nil
}

func (h *ProductHandler) redirectAssetsError(w http.ResponseWriter, r *http.Request, id, message string) {
	location := "/assets/" + url.PathEscape(id)

	values := url.Values{}
	if query := strings.TrimSpace(r.FormValue("q")); query != "" {
		values.Set("q", query)
	}

	values.Set("error", message)
	location += "?" + values.Encode()

	http.Redirect(w, r, location, http.StatusSeeOther)
}

func (h *ProductHandler) discover(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok || !h.csrfOK(w, r, session) {
		return
	}

	id := r.PathValue("id")
	if _, err := h.productConnectionByID(r.Context(), actor, id); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)

		return
	}

	if _, err := h.config.Service.Discover(r.Context(), actor, id); err != nil {
		message, needsReconnect := productFailureMessage(err)
		if needsReconnect {
			http.Redirect(w, r, "/?error="+url.QueryEscape(message), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, "/assets/"+url.PathEscape(id)+"?error="+url.QueryEscape(message), http.StatusSeeOther)

		return
	}

	http.Redirect(w, r, "/assets/"+url.PathEscape(id), http.StatusSeeOther)
}

func (h *ProductHandler) reconnect(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok || !h.csrfOK(w, r, session) {
		return
	}
	id := r.PathValue("id")

	connection, err := h.productConnectionByID(r.Context(), actor, id)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)

		return
	}

	start, err := h.config.Service.BeginOAuth(r.Context(), actor, connection.ID, true)
	if err != nil {
		http.Redirect(w, r, "/?error="+url.QueryEscape("Google needs you to reconnect this account."), http.StatusSeeOther)

		return
	}

	h.setProductOAuthCookie(w, productConnectionStateCookie, productOAuthState{
		State: start.State, ConnectionID: connection.ID, ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	http.Redirect(w, r, start.URL, http.StatusSeeOther)
}

func (h *ProductHandler) render(w http.ResponseWriter, name string, data any) {
	var body bytes.Buffer
	if err := h.templates.ExecuteTemplate(&body, name, data); err != nil {
		http.Error(w, "product page unavailable", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

func (h *ProductHandler) existingProductConnection(ctx context.Context, actor Actor) (Connection, error) {
	connections, err := h.config.Service.ListConnections(ctx, actor)
	if err != nil {
		return Connection{}, err
	}

	return findProductConnection(connections), nil
}

func (h *ProductHandler) productConnection(ctx context.Context, actor Actor) (Connection, error) {
	connections, err := h.config.Service.ListConnections(ctx, actor)
	if err != nil {
		return Connection{}, err
	}

	connection := findProductConnection(connections)
	if connection.ID != "" {
		return connection, nil
	}

	return h.config.Service.CreateConnection(ctx, actor, productConnectionName, h.config.DefaultServices)
}

func (h *ProductHandler) productConnectionByID(ctx context.Context, actor Actor, id string) (Connection, error) {
	connection, err := h.config.Service.GetConnection(ctx, actor, id)
	if err != nil || connection.Name != productConnectionName {
		return Connection{}, ErrNotFound
	}

	return connection, nil
}

func (h *ProductHandler) setProductOAuthCookie(w http.ResponseWriter, name string, state productOAuthState) {
	raw, err := json.Marshal(state)
	if err != nil {
		http.Error(w, "product authentication unavailable", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is configurable for documented local HTTP smoke runs.
		Name: name, Value: h.config.Sessions.EncodeSigned(string(raw)), Path: "/",
		HttpOnly: true, Secure: h.config.Sessions.secure, SameSite: http.SameSiteLaxMode, MaxAge: 600,
	})
}

func (h *ProductHandler) takeProductOAuthCookie(r *http.Request, w http.ResponseWriter, name string) (productOAuthState, bool) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return productOAuthState{}, false
	}

	raw, ok := h.config.Sessions.DecodeSigned(cookie.Value)
	if !ok {
		return productOAuthState{}, false
	}

	var state productOAuthState
	if err := json.Unmarshal([]byte(raw), &state); err != nil || state.ExpiresAt.Before(time.Now()) {
		return productOAuthState{}, false
	}

	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is configurable for documented local HTTP smoke runs.
		Name: name, Value: "", Path: "/", HttpOnly: true, Secure: h.config.Sessions.secure,
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})

	return state, true
}

func findProductConnection(items []Connection) Connection {
	for _, item := range items {
		if item.Name == productConnectionName {
			return item
		}
	}

	return Connection{}
}

func productConnectionState(connection Connection) string {
	switch {
	case connection.ID == "":
		return "disconnected"
	case connection.Status == ConnectionHealthy && connection.SecretRef != "":
		return "connected"
	case connection.Status == ConnectionNeedsReconnect || connection.Status == ConnectionExpired:
		return "needs_attention"
	default:
		return "disconnected"
	}
}

func productStateLabel(state string) string {
	switch state {
	case "connected":
		return "Connected"
	case "needs_attention":
		return "Needs attention"
	default:
		return "Disconnected"
	}
}

func productFailureMessage(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	if errors.Is(err, ErrDiscovererNotConfigured) || errors.Is(err, ErrGoogleAdsUnavailable) {
		return "Some Google services are not available yet. Your other connected data is still ready.", false
	}

	switch AuthFailureCategoryFor(err) {
	case AuthFailureInvalidGrant, AuthFailureSessionControl, AuthFailureScopeMismatch, AuthFailureOAuthClient, AuthFailurePermission:
		return "Google needs you to reconnect this account.", true
	default:
		return "We couldn't refresh Google data right now. Please try again.", false
	}
}

func productAssets(grants []ResourceGrant) []productAsset {
	out := make([]productAsset, 0, len(grants))
	for _, g := range grants {
		out = append(out, productAsset{
			Service:     g.Service,
			ServiceName: serviceLabel(g.Service),
			Kind:        kindLabel(g.ResourceType),
			Name:        g.DisplayName,
			ResourceID:  g.ResourceID,
			Parent:      g.Parent,
			Enabled:     g.Enabled,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ServiceName != out[j].ServiceName {
			return out[i].ServiceName < out[j].ServiceName
		}

		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}

		return out[i].ResourceID < out[j].ResourceID
	})

	return out
}

func productAssetGroups(assets []productAsset, query string) []productAssetGroup {
	query = strings.ToLower(strings.TrimSpace(query))
	groups := make([]productAssetGroup, 0, 5)
	index := make(map[string]int)

	for _, asset := range assets {
		if query != "" && !strings.Contains(strings.ToLower(asset.Name+" "+asset.ServiceName+" "+asset.Kind+" "+asset.Parent), query) {
			continue
		}

		position, ok := index[asset.ServiceName]
		if !ok {
			position = len(groups)
			index[asset.ServiceName] = position
			groups = append(groups, productAssetGroup{ServiceName: asset.ServiceName})
		}

		groups[position].Assets = append(groups[position].Assets, asset)

		groups[position].Count++
		if asset.Enabled {
			groups[position].EnabledCount++
		}
	}

	return groups
}

func serviceLabel(service string) string {
	switch strings.ToLower(service) {
	case "analytics":
		return "Google Analytics"
	case "googleads":
		return "Google Ads"
	case "tagmanager":
		return "Google Tag Manager"
	case "searchconsole":
		return "Search Console"
	case "bigquery":
		return "BigQuery"
	default:
		return "Google service"
	}
}

func serviceFromLabel(label string) string {
	switch strings.TrimSpace(label) {
	case "Google Analytics":
		return "analytics"
	case "Google Ads":
		return "googleads"
	case "Google Tag Manager":
		return "tagmanager"
	case "Search Console":
		return "searchconsole"
	case "BigQuery":
		return "bigquery"
	default:
		return ""
	}
}

func kindLabel(kind string) string {
	switch strings.ToLower(kind) {
	case "property":
		return "Property"
	case "account":
		return "Account"
	case "container":
		return "Container"
	case "customer":
		return "Customer"
	case "site":
		return "Site"
	case "project":
		return "Project"
	default:
		return "Asset"
	}
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	var different byte
	for i := range a {
		different |= a[i] ^ b[i]
	}

	return different == 0
}
