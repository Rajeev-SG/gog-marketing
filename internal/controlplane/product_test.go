package controlplane

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakeProductAuth struct{ actor Actor }

type failingResourceStore struct {
	*MemoryStore
	failAt int
	calls  int
}

func (s *failingResourceStore) SetResourceEnabled(ctx context.Context, organizationID, connectionID, resourceID string, enabled bool) (ResourceGrant, error) {
	s.calls++
	if s.calls >= s.failAt {
		return ResourceGrant{}, errTestUpdateConnection
	}

	return s.MemoryStore.SetResourceEnabled(ctx, organizationID, connectionID, resourceID, enabled)
}

func (fakeProductAuth) AuthorizationURL(state, codeVerifier string) string {
	return "https://accounts.example.test/auth?state=" + url.QueryEscape(state) + "&code_verifier=" + url.QueryEscape(codeVerifier)
}

func (fakeProductAuth) Complete(context.Context, string, string, string) (ProductIdentity, error) {
	return ProductIdentity{Email: "owner@example.com", EmailVerified: true, Subject: "subject-1"}, nil
}

func (f fakeProductAuth) Actor(context.Context, ProductIdentity) (Actor, error) {
	return f.actor, nil
}

func productTestService(t *testing.T) (*Service, *MemoryStore) {
	t.Helper()
	service, store, _ := testService(t)
	oauth := service.OAuth.(*fakeOAuth)
	oauth.token.GrantedScopes = append(oauth.token.GrantedScopes, "https://www.googleapis.com/auth/tagmanager.readonly", "https://www.googleapis.com/auth/bigquery.readonly")
	service.Discoverer = fakeDiscoverer{resources: []ResourceGrant{
		{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", DisplayName: "Example GA4", Parent: "accounts/456"},
		{Service: "analytics", ResourceType: "property", ResourceID: "properties/124", DisplayName: "Other GA4", Parent: "accounts/456"},
		{Service: "bigquery", ResourceType: "project", ResourceID: "projects/data", DisplayName: "Data project"},
	}}

	return service, store
}

func newProductTestHandler(t *testing.T, service *Service, actor Actor) (*httptest.Server, *http.Client) {
	t.Helper()

	sessions, err := NewSessionManager([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}

	return newProductTestHandlerWithSessions(t, service, actor, sessions)
}

func newProductTestHandlerWithSessions(t *testing.T, service *Service, actor Actor, sessions *SessionManager) (*httptest.Server, *http.Client) {
	t.Helper()

	handler, err := NewProductHandler(ProductConfig{
		Service: service, Sessions: sessions, Auth: fakeProductAuth{actor: actor},
		DisplayName: "Test workspace", DefaultServices: []string{"analytics", "tagmanager", "bigquery"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	return server, client
}

func productGetWithCSRF(t *testing.T, client *http.Client, serverURL, path string, cookies []*http.Cookie, csrf string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	req.Header.Set("Sec-Fetch-Site", "same-origin")

	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}

func productGet(t *testing.T, client *http.Client, serverURL, path string, cookies []*http.Cookie) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}

func readProductBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	return string(body)
}

func productSessionCookies(t *testing.T, client *http.Client, server *httptest.Server) []*http.Cookie {
	t.Helper()
	resp := productGet(t, client, server.URL, "/auth/google/start", nil)

	resp.Body.Close()
	stateCookies := resp.Cookies()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-in start status = %d", resp.StatusCode)
	}
	stateURL := resp.Header.Get("Location")

	parsed, err := url.Parse(stateURL)
	if err != nil {
		t.Fatal(err)
	}
	state := parsed.Query().Get("state")
	resp = productGet(t, client, server.URL, "/oauth/google/callback?state="+url.QueryEscape(state)+"&code=code", stateCookies)

	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-in callback status = %d", resp.StatusCode)
	}

	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("sign-in did not create a product session")
	}

	return cookies
}

func TestProductShellSignsInWithoutAdminCredentialAndOnboardsWithConnectGoogle(t *testing.T) {
	service, store := productTestService(t)
	server, client := newProductTestHandler(t, service, ownerActor(t, store))
	cookies := productSessionCookies(t, client, server)

	signinResp := productGet(t, client, server.URL, "/signin", nil) //nolint:bodyclose // readProductBody closes this response

	signin := readProductBody(t, signinResp)
	if !strings.Contains(signin, "Continue with Google") || strings.Contains(strings.ToLower(signin), "admin access token") {
		t.Fatalf("sign-in page is not marketer-first: %s", signin)
	}

	homeResp := productGet(t, client, server.URL, "/", cookies)

	defer homeResp.Body.Close()

	home := readProductBody(t, homeResp)
	if !strings.Contains(home, "Connect Google") || strings.Contains(strings.ToLower(home), "gcp") || strings.Contains(strings.ToLower(home), "oauth") {
		t.Fatalf("home leaked implementation concepts: %s", home)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/connect/google", strings.NewReader("csrf="+url.QueryEscape(productCSRF(t, server, client, cookies))))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "accounts.example.test") {
		t.Fatalf("connect redirect = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func productCSRF(t *testing.T, server *httptest.Server, client *http.Client, cookies []*http.Cookie) string {
	t.Helper()
	bodyResp := productGet(t, client, server.URL, "/", cookies)

	defer bodyResp.Body.Close()
	body := readProductBody(t, bodyResp)

	start := strings.Index(body, `name="csrf" value="`)
	if start < 0 {
		t.Fatalf("missing CSRF token: %s", body)
	}
	rest := body[start+len(`name="csrf" value="`):]

	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatal("unterminated CSRF token")
	}

	return rest[:end]
}

func TestProductShellDiscoversGroupsSelectsAndPersistsAssets(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	service.Discoverer = fakeDiscoverer{resources: []ResourceGrant{
		{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", DisplayName: "Example GA4", Parent: "accounts/456"},
		{Service: "analytics", ResourceType: "property", ResourceID: "properties/124", DisplayName: "Other GA4", Parent: "accounts/456"},
		{Service: "bigquery", ResourceType: "project", ResourceID: "projects/data", DisplayName: "Data project"},
	}}
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)

	connectProduct(t, client, server, cookies, csrf)

	connections, err := service.ListConnections(context.Background(), actor)
	if err != nil || len(connections) != 1 {
		t.Fatalf("product connection not created: %+v, %v", connections, err)
	}

	connection := connections[0]
	if connection.Name != productConnectionName {
		t.Fatalf("product connection name = %q", connection.Name)
	}
	resp := productGet(t, client, server.URL, "/assets/"+connection.ID, cookies) //nolint:bodyclose // readProductBody closes this response

	assets := readProductBody(t, resp)
	for _, want := range []string{"Google Analytics", "BigQuery", "Allow all and save", "Remove all and save", "Example GA4", "Data project"} {
		if !strings.Contains(assets, want) {
			t.Fatalf("asset page missing %q: %s", want, assets)
		}
	}

	form := url.Values{"csrf": {csrf}, "resource": {"properties/123"}}
	postProduct(t, client, server, "/assets/"+connection.ID+"/save", form, cookies) //nolint:bodyclose // caller closes this response

	grants, err := service.ListResources(context.Background(), actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	for _, grant := range grants {
		want := grant.ResourceID == "properties/123"
		if grant.Enabled != want {
			t.Fatalf("grant %s enabled = %v, want %v", grant.ResourceID, grant.Enabled, want)
		}
	}
}

func TestProductShellCSRFAndReconnectRecovery(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)
	connectProduct(t, client, server, cookies, csrf)

	connection, err := service.ListConnections(context.Background(), actor)
	if err != nil || len(connection) != 1 {
		t.Fatal("missing product connection")
	}

	resp := postProduct(t, client, server, "/assets/"+connection[0].ID+"/save", url.Values{"resource": {"properties/123"}}, cookies)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing CSRF status = %d", resp.StatusCode)
	}

	resp.Body.Close()

	start, err := service.BeginOAuth(context.Background(), actor, connection[0].ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, completeErr := service.CompleteOAuth(context.Background(), start.State, "code"); completeErr != nil {
		t.Fatal(completeErr)
	}

	current, token, err := service.FreshToken(context.Background(), actor, connection[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	token.Expiry = time.Now().Add(-time.Hour)
	if err := service.saveToken(context.Background(), actor, &current, token); err != nil {
		t.Fatal(err)
	}

	service.OAuth.(*fakeOAuth).err = errTestInvalidGoogleGrant
	if _, err := service.Refresh(context.Background(), actor, connection[0].ID); err == nil {
		t.Fatal("expected refresh failure")
	}
	homeResp := productGet(t, client, server.URL, "/", cookies)

	defer homeResp.Body.Close()

	home := readProductBody(t, homeResp)
	if !strings.Contains(home, "Needs attention") || !strings.Contains(home, "Reconnect Google") {
		t.Fatalf("reconnect recovery missing from home: %s", home)
	}
}

func connectProduct(t *testing.T, client *http.Client, server *httptest.Server, cookies []*http.Cookie, csrf string) {
	t.Helper()
	resp := postProduct(t, client, server, "/connect/google", url.Values{"csrf": {csrf}}, cookies)

	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("connect status = %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if !strings.Contains(location, "accounts.example.test") {
		t.Fatalf("connect did not start OAuth: %q", location)
	}

	oauthCookies := append(append([]*http.Cookie{}, cookies...), resp.Cookies()...)

	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	state := parsed.Query().Get("state")
	callback := productGet(t, client, server.URL, "/oauth/google/callback?state="+url.QueryEscape(state)+"&code=code", oauthCookies)
	callback.Body.Close()

	if callback.StatusCode != http.StatusSeeOther {
		t.Fatalf("callback status = %d", callback.StatusCode)
	}
}

func postProduct(t *testing.T, client *http.Client, server *httptest.Server, path string, form url.Values, cookies []*http.Cookie) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}

func TestProductAuthStateIsSingleUse(t *testing.T) {
	service, store := productTestService(t)
	server, client := newProductTestHandler(t, service, ownerActor(t, store))
	resp := productGet(t, client, server.URL, "/auth/google/start", nil)

	resp.Body.Close()
	stateCookies := resp.Cookies()

	parsed, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := parsed.Query().Get("state")

	first := productGet(t, client, server.URL, "/oauth/google/callback?state="+url.QueryEscape(state)+"&code=code", stateCookies)
	first.Body.Close()
	second := productGet(t, client, server.URL, "/oauth/google/callback?state="+url.QueryEscape(state)+"&code=code", nil)

	secondBody := readProductBody(t, second)
	if second.StatusCode != http.StatusSeeOther || !strings.Contains(second.Header.Get("Location"), "/signin") || !strings.Contains(secondBody, "") {
		t.Fatalf("replayed state was accepted: %d %q", second.StatusCode, second.Header.Get("Location"))
	}
}

func TestProductAndAdminSessionsCannotCrossSurfaces(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)

	resp := productGet(t, client, server.URL, "/", cookies)

	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("product session home status = %d", resp.StatusCode)
	}

	sessions, err := NewSessionManager([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}

	adminToken, _, err := sessions.New(actor)
	if err != nil {
		t.Fatal(err)
	}
	adminCookie := sessions.Cookie(adminToken)
	resp = productGet(t, client, server.URL, "/", []*http.Cookie{adminCookie})

	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/signin" {
		t.Fatalf("admin session opened product surface: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	adminHandler, err := NewWebHandler(WebConfig{
		Service: service, Sessions: sessions,
		Authenticator: OwnerAuthenticator{Email: "owner@example.com", Token: "admin-token-0123456789abcdef", Actor: actor},
		BasePath:      "/admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	adminServer := httptest.NewServer(adminHandler)
	t.Cleanup(adminServer.Close)
	adminClient := adminServer.Client()
	adminClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	resp = productGet(t, adminClient, adminServer.URL, "/admin/connections", []*http.Cookie{adminCookie})

	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin session rejected by admin surface: %d", resp.StatusCode)
	}
	resp = productGet(t, adminClient, adminServer.URL, "/admin/connections", cookies)

	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/admin/login" {
		t.Fatalf("product session opened admin surface: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestGoogleProductAuthenticatorUsesPKCEAndRequiresVerifiedEmail(t *testing.T) {
	auth := NewGoogleProductAuthenticator("client-id", "client-secret", "https://example.test/oauth/google/callback", "owner@example.com", Actor{UserID: "user-1", OrganizationID: "org-1", Role: "owner"})

	authURL := auth.AuthorizationURL("state-value", "verifier-value")
	if !strings.Contains(authURL, "code_challenge=") || !strings.Contains(authURL, "code_challenge_method=S256") {
		t.Fatalf("sign-in URL is missing PKCE: %s", authURL)
	}

	if strings.Contains(authURL, "verifier-value") {
		t.Fatalf("sign-in URL leaked code verifier: %s", authURL)
	}

	if _, err := auth.Actor(context.Background(), ProductIdentity{Email: "owner@example.com", Subject: "subject-1"}); err == nil {
		t.Fatal("unverified email accepted")
	}

	if _, err := auth.Actor(context.Background(), ProductIdentity{Email: "owner@example.com", EmailVerified: true, Subject: "subject-1"}); err != nil {
		t.Fatalf("verified owner rejected: %v", err)
	}
}

func TestProductAssetIDsAreAdvancedDisclosureOnly(t *testing.T) {
	assets := productAssets([]ResourceGrant{{Service: "analytics", ResourceType: "property", ResourceID: "properties/private-id", DisplayName: "Example GA4"}})
	if len(assets) != 1 || assets[0].ResourceID != "properties/private-id" {
		t.Fatal("asset model did not retain grant identity")
	}
}

func TestProductHomeDoesNotCreateConnectionOnGET(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)

	resp := productGet(t, client, server.URL, "/", cookies) //nolint:bodyclose // readProductBody closes this response

	_ = readProductBody(t, resp)

	connections, err := service.ListConnections(context.Background(), actor)
	if err != nil {
		t.Fatal(err)
	}

	if len(connections) != 0 {
		t.Fatalf("GET home created %d connections", len(connections))
	}
}

func TestSigninRendersErrorsAndSingleOwnerPolicy(t *testing.T) {
	service, store := productTestService(t)
	server, client := newProductTestHandler(t, service, ownerActor(t, store))
	signinResp := productGet(t, client, server.URL, "/signin?error=Sign-in+failed", nil) //nolint:bodyclose // readProductBody closes this response

	signin := readProductBody(t, signinResp)
	if !strings.Contains(signin, "Sign-in failed") {
		t.Fatalf("sign-in error did not render: %s", signin)
	}

	if strings.Contains(strings.ToLower(signin), "your team") {
		t.Fatalf("single-user product copy overpromises team access: %s", signin)
	}

	auth := NewGoogleProductAuthenticator("client-id", "client-secret", "https://example.test/oauth/google/callback", "owner@example.com", Actor{UserID: "user-1", OrganizationID: "org-1", Role: "owner"})
	if _, err := auth.Actor(context.Background(), ProductIdentity{Email: "other@example.com", EmailVerified: true, Subject: "subject-2"}); err == nil {
		t.Fatal("non-owner identity was accepted")
	}
}

func TestProductAuthStateSurvivesHandlerReconstruction(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)

	sessions, err := NewSessionManager([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}

	firstServer, firstClient := newProductTestHandlerWithSessions(t, service, actor, sessions)
	startResp := productGet(t, firstClient, firstServer.URL, "/auth/google/start", nil)
	startResp.Body.Close()
	stateURL := startResp.Header.Get("Location")

	parsed, err := url.Parse(stateURL)
	if err != nil {
		t.Fatal(err)
	}

	state := parsed.Query().Get("state")
	stateCookies := startResp.Cookies()
	secondServer, secondClient := newProductTestHandlerWithSessions(t, service, actor, sessions)
	callbackResp := productGet(t, secondClient, secondServer.URL, "/oauth/google/callback?state="+url.QueryEscape(state)+"&code=code", stateCookies)
	callbackResp.Body.Close()

	if callbackResp.StatusCode != http.StatusSeeOther || len(callbackResp.Cookies()) == 0 {
		t.Fatalf("stateless sign-in state did not survive handler reconstruction: %d", callbackResp.StatusCode)
	}

	for i := 0; i < 128; i++ {
		resp := productGet(t, firstClient, firstServer.URL, "/auth/google/start", nil)

		resp.Body.Close()

		if len(resp.Cookies()) != 1 || len(resp.Cookies()[0].Value) > 2048 {
			t.Fatalf("unbounded auth state cookie at iteration %d", i)
		}
	}
}

func TestProductSaveReportsPartialFailureAndSkipsUnchangedGrants(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(context.Background(), actor, productConnectionName, []string{"analytics", "bigquery"})
	if err != nil {
		t.Fatal(err)
	}

	start, err := service.BeginOAuth(context.Background(), actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, completeErr := service.CompleteOAuth(context.Background(), start.State, "code"); completeErr != nil {
		t.Fatal(completeErr)
	}

	if _, discoverErr := service.Discover(context.Background(), actor, connection.ID); discoverErr != nil {
		t.Fatal(discoverErr)
	}

	service.Store = &failingResourceStore{MemoryStore: store, failAt: 2}
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)
	form := url.Values{"csrf": {csrf}, "resource": {"properties/123", "properties/124"}}
	resp := postProduct(t, client, server, "/assets/"+connection.ID+"/save", form, cookies)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "error=") {
		t.Fatalf("partial save did not return a user-visible error redirect: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	assetsResp := productGet(t, client, server.URL, resp.Header.Get("Location"), cookies) //nolint:bodyclose // readProductBody closes this response

	assets := readProductBody(t, assetsResp)
	if !strings.Contains(assets, "Some access changes could not be saved") {
		t.Fatalf("partial save error was not rendered: %s", assets)
	}

	service.Store = store

	grants, err := service.ListResources(context.Background(), actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	for _, grant := range grants {
		if grant.ResourceID == "properties/123" && !grant.Enabled {
			t.Fatal("first changed grant was not persisted before partial failure")
		}
	}
}

func TestConnectCallbackRequiresProductSessionAndBoundState(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)
	resp := postProduct(t, client, server, "/connect/google", url.Values{"csrf": {csrf}}, cookies)
	resp.Body.Close()
	stateURL := resp.Header.Get("Location")

	parsed, err := url.Parse(stateURL)
	if err != nil {
		t.Fatal(err)
	}

	state := parsed.Query().Get("state")
	connectionCookies := resp.Cookies()
	sessionless := productGet(t, client, server.URL, "/oauth/google/callback?state="+url.QueryEscape(state)+"&code=code", connectionCookies)
	sessionless.Body.Close()

	if sessionless.StatusCode != http.StatusSeeOther || !strings.HasPrefix(sessionless.Header.Get("Location"), "/signin") {
		t.Fatalf("sessionless connect callback was accepted: %d %q", sessionless.StatusCode, sessionless.Header.Get("Location"))
	}

	unbound := productGet(t, client, server.URL, "/oauth/google/callback?state="+url.QueryEscape(state)+"&code=code", cookies)
	unbound.Body.Close()

	if unbound.StatusCode != http.StatusSeeOther || !strings.Contains(unbound.Header.Get("Location"), "error=") {
		t.Fatalf("unbound connect callback was accepted: %d %q", unbound.StatusCode, unbound.Header.Get("Location"))
	}

	signinPrefixed := productGet(t, client, server.URL, "/oauth/google/callback?state=signin_wrong&code=code", connectionCookies)
	signinPrefixed.Body.Close()

	if signinPrefixed.StatusCode != http.StatusSeeOther || !strings.HasPrefix(signinPrefixed.Header.Get("Location"), "/signin") {
		t.Fatalf("signin-prefixed state leaked into connect callback: %d %q", signinPrefixed.StatusCode, signinPrefixed.Header.Get("Location"))
	}
}

func TestOAuthStateCookiesAllowTopLevelCallback(t *testing.T) {
	service, store := productTestService(t)
	server, client := newProductTestHandler(t, service, ownerActor(t, store))
	startResp := productGet(t, client, server.URL, "/auth/google/start", nil)
	startResp.Body.Close()

	for _, cookie := range startResp.Cookies() {
		if cookie.Name == productSigninStateCookie && cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("sign-in state cookie SameSite = %v, want Lax", cookie.SameSite)
		}
	}
}

func TestProductFilteredSavePreservesHiddenGrants(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)
	connectProduct(t, client, server, cookies, csrf)

	connections, err := service.ListConnections(context.Background(), actor)
	if err != nil || len(connections) != 1 {
		t.Fatal("missing connection", err)
	}

	id := connections[0].ID
	for _, resource := range []string{"properties/123", "properties/124", "projects/data"} {
		if _, err := service.SetResourceEnabled(context.Background(), actor, id, resource, true); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		query          string
		resources      []string
		visibleEnabled bool
	}{
		{"Example GA4", []string{"properties/123"}, true},
		{"Example GA4", nil, false},
		{"no matching assets", nil, false},
	} {
		resp := postProduct(t, client, server, "/assets/"+id+"/save", url.Values{"csrf": {csrf}, "q": {tc.query}, "resource": tc.resources}, cookies)
		resp.Body.Close()

		grants, err := service.ListResources(context.Background(), actor, id)
		if err != nil {
			t.Fatal(err)
		}

		for _, grant := range grants {
			want := true
			if grant.ResourceID == "properties/123" {
				want = tc.visibleEnabled
			}

			if grant.Enabled != want {
				t.Fatalf("query %q: %s enabled=%v want=%v", tc.query, grant.ResourceID, grant.Enabled, want)
			}
		}
	}

	// Reload the unfiltered page to verify hidden selections remain visible as saved.
	resp := productGet(t, client, server.URL, "/assets/"+id, cookies) //nolint:bodyclose // readProductBody closes the response
	body := readProductBody(t, resp)

	if !strings.Contains(body, `name="resource" value="properties/124" type="checkbox" checked`) {
		t.Fatal("hidden grant was not selected after reload")
	}
}

func TestProductFilteredSaveRejectsOutOfSelectionResources(t *testing.T) {
	for _, resource := range []string{"properties/124", "properties/foreign", "properties/unknown"} {
		t.Run(resource, func(t *testing.T) {
			service, store := productTestService(t)
			actor := ownerActor(t, store)
			server, client := newProductTestHandler(t, service, actor)
			cookies := productSessionCookies(t, client, server)
			csrf := productCSRF(t, server, client, cookies)
			connectProduct(t, client, server, cookies, csrf)

			connections, err := service.ListConnections(context.Background(), actor)
			if err != nil || len(connections) != 1 {
				t.Fatal("missing connection", err)
			}

			id := connections[0].ID

			foreign, createErr := store.CreateConnection(context.Background(), Connection{OrganizationID: "foreign-org", Name: "google"})
			if createErr != nil {
				t.Fatal(createErr)
			}

			if _, grantErr := store.UpsertResourceGrant(context.Background(), ResourceGrant{OrganizationID: "foreign-org", ConnectionID: foreign.ID, Service: "analytics", ResourceID: "properties/foreign", Enabled: true}); grantErr != nil {
				t.Fatal(grantErr)
			}

			if _, enableErr := service.SetResourceEnabled(context.Background(), actor, id, "properties/123", true); enableErr != nil {
				t.Fatal(enableErr)
			}
			resp := postProduct(t, client, server, "/assets/"+id+"/save", url.Values{"csrf": {csrf}, "q": {"Example GA4"}, "resource": {resource}}, cookies)
			resp.Body.Close()

			if !strings.Contains(resp.Header.Get("Location"), "error=") {
				t.Fatal("invalid selection accepted")
			}

			grants, err := service.ListResources(context.Background(), actor, id)
			if err != nil {
				t.Fatal(err)
			}

			for _, grant := range grants {
				if grant.Enabled != (grant.ResourceID == "properties/123") {
					t.Fatal("invalid selection changed grants")
				}
			}
		})
	}
}

func TestProductServiceSelectionIgnoresSearchAndPreservesOtherServices(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)
	connectProduct(t, client, server, cookies, csrf)

	connections, err := service.ListConnections(context.Background(), actor)
	if err != nil || len(connections) != 1 {
		t.Fatal("missing connection", err)
	}

	id := connections[0].ID
	if _, err := service.SetResourceEnabled(context.Background(), actor, id, "projects/data", true); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"select_all", "select_none"} {
		resp := postProduct(t, client, server, "/assets/"+id+"/save", url.Values{"csrf": {csrf}, "q": {"Example GA4"}, action: {"Google Analytics"}}, cookies)
		resp.Body.Close()

		grants, err := service.ListResources(context.Background(), actor, id)
		if err != nil {
			t.Fatal(err)
		}

		for _, grant := range grants {
			want := grant.Service != "analytics" || action == "select_all"
			if grant.Enabled != want {
				t.Fatalf("%s: unexpected permission on %s", action, grant.ResourceID)
			}
		}
	}
}
