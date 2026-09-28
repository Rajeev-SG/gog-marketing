package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeOAuth struct {
	token              OAuthToken
	err                error
	authorizationCalls int
	refreshCalls       int
}

func (f *fakeOAuth) AuthorizationURL(input OAuthStartInput) string {
	f.authorizationCalls++
	return "https://accounts.example.test/auth?state=" + input.State
}

func (f *fakeOAuth) Exchange(_ context.Context, _ OAuthStartInput, _ string) (OAuthToken, error) {
	if f.err != nil {
		return OAuthToken{}, f.err
	}

	return f.token, nil
}

func (f *fakeOAuth) Refresh(_ context.Context, token OAuthToken) (OAuthToken, error) {
	f.refreshCalls++
	if f.err != nil {
		return OAuthToken{}, f.err
	}

	out := f.token
	if out.RefreshToken == "" {
		out.RefreshToken = token.RefreshToken
	}

	return out, nil
}

type fakeDiscoverer struct {
	resources []ResourceGrant
	err       error
}

func (f fakeDiscoverer) Discover(_ context.Context, connection Connection, _ OAuthToken) ([]ResourceGrant, error) {
	if f.err != nil {
		return nil, f.err
	}

	out := make([]ResourceGrant, 0, len(f.resources))
	for _, resource := range f.resources {
		resource.ConnectionID = connection.ID
		resource.OrganizationID = connection.OrganizationID
		out = append(out, resource)
	}

	return out, nil
}

func testService(t *testing.T) (*Service, *MemoryStore, *FileSecretStore) {
	t.Helper()
	store := NewMemoryStore()

	secrets, err := NewFileSecretStore(filepath.Join(t.TempDir(), "secrets.json"), []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	oauth := &fakeOAuth{token: OAuthToken{
		AccessToken: "access-secret-value", RefreshToken: "refresh-secret-value",
		Expiry: time.Now().Add(time.Hour), Subject: "subject-1", Email: "owner@example.com",
		GrantedScopes: []string{"openid", "email", "https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/analytics.readonly"},
	}}
	service := &Service{
		Store: store, Secrets: secrets, OAuth: oauth, RedirectURI: "http://example.test/oauth/google/callback",
		Discoverer: fakeDiscoverer{resources: []ResourceGrant{{
			Service: "analytics", ResourceType: "property", ResourceID: "properties/123",
			DisplayName: "Example GA4", Parent: "accounts/456",
		}}},
	}

	return service, store, secrets
}

func ownerActor(t *testing.T, store *MemoryStore) Actor {
	t.Helper()

	_, _, err := store.BootstrapOwner(context.Background(), User{
		ID: "user-1", Email: "owner@example.com", DisplayName: "Owner",
	}, Organization{ID: "org-1", Name: "Default", Slug: "default"}, "owner")
	if err != nil {
		t.Fatal(err)
	}

	return Actor{UserID: "user-1", OrganizationID: "org-1", Role: "owner"}
}

func TestConnectionOAuthDiscoveryPolicyAndSecretIsolation(t *testing.T) {
	ctx := context.Background()
	service, store, secrets := testService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	if connection.Status != ConnectionNeedsConnect {
		t.Fatalf("status = %q", connection.Status)
	}

	start, err := service.BeginOAuth(ctx, actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if start.URL == "" || start.State == "" {
		t.Fatal("missing OAuth URL/state")
	}

	connected, err := service.CompleteOAuth(ctx, start.State, "auth-code")
	if err != nil {
		t.Fatal(err)
	}

	if connected.Status != ConnectionHealthy || connected.GoogleEmail != "owner@example.com" || connected.SecretRef == "" {
		t.Fatalf("unexpected connection: %+v", connected)
	}

	rawSecrets, err := os.ReadFile(filepath.Join(filepath.Dir(secrets.path), "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(rawSecrets), "refresh-secret-value") || strings.Contains(string(rawSecrets), "access-secret-value") {
		t.Fatal("secret store contains token plaintext")
	}

	if _, stateErr := service.CompleteOAuth(ctx, start.State, "auth-code"); stateErr == nil {
		t.Fatal("OAuth state should be single-use")
	}

	reconnect, err := service.BeginOAuth(ctx, actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, reconnectErr := service.CompleteOAuth(ctx, reconnect.State, "auth-code-2"); reconnectErr != nil {
		t.Fatal(err)
	}

	connections, err := service.ListConnections(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}

	if len(connections) != 1 {
		t.Fatalf("reconnect duplicated connection: %d", len(connections))
	}

	resources, err := service.Discover(ctx, actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(resources) != 1 || resources[0].ResourceID != "properties/123" || resources[0].Parent != "accounts/456" {
		t.Fatalf("unexpected normalized resources: %+v", resources)
	}

	policy := Policy{Store: store}
	if allowErr := policy.Allow(ctx, actor, connection.ID, "analytics", "properties/123"); allowErr == nil {
		t.Fatal("disabled resource must be denied")
	}

	if _, grantErr := service.SetResourceEnabled(ctx, actor, connection.ID, "properties/123", true); grantErr != nil {
		t.Fatal(err)
	}

	if allowErr := policy.Allow(ctx, actor, connection.ID, "analytics", "properties/123"); allowErr != nil {
		t.Fatalf("enabled resource denied: %v", err)
	}

	if allowErr := policy.Allow(ctx, actor, connection.ID, "analytics", "properties/unknown"); allowErr == nil {
		t.Fatal("unknown resource must be denied")
	}

	if _, _, bootstrapErr := store.BootstrapOwner(ctx, User{ID: "user-2", Email: "other@example.com"}, Organization{ID: "org-2", Name: "Other", Slug: "other"}, "owner"); bootstrapErr != nil {
		t.Fatal(err)
	}

	other := Actor{UserID: "user-2", OrganizationID: "org-2", Role: "owner"}
	if allowErr := policy.Allow(ctx, other, connection.ID, "analytics", "properties/123"); allowErr == nil {
		t.Fatal("cross-organisation resource must be denied")
	}

	if disconnectErr := service.Disconnect(ctx, actor, connection.ID); disconnectErr != nil {
		t.Fatal(err)
	}

	disconnected, err := service.GetConnection(ctx, actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	if disconnected.Status != ConnectionDisconnected || disconnected.SecretRef != "" {
		t.Fatalf("disconnect did not remove credential: %+v", disconnected)
	}

	if _, err := secrets.Get(ctx, actor.OrganizationID, connected.SecretRef); err == nil {
		t.Fatal("deleted secret remained readable")
	}
}

func TestOAuthDeniedAndInvalidState(t *testing.T) {
	ctx := context.Background()
	service, store, _ := testService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(ctx, actor, "singulyr", []string{"searchconsole"})
	if err != nil {
		t.Fatal(err)
	}

	if _, stateErr := service.CompleteOAuth(ctx, "missing-state", "code"); stateErr == nil {
		t.Fatal("invalid state was accepted")
	}
	service.OAuth = &fakeOAuth{err: http.ErrHandlerTimeout}

	start, err := service.BeginOAuth(ctx, actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, stateErr := service.CompleteOAuth(ctx, start.State, "code"); stateErr == nil {
		t.Fatal("denied exchange was accepted")
	}

	updated, err := service.GetConnection(ctx, actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	if updated.Status != ConnectionNeedsReconnect || updated.LastError == "" {
		t.Fatalf("OAuth failure was not surfaced safely: %+v", updated)
	}
}

func TestFileSecretStoreScopesOrganizations(t *testing.T) {
	store, err := NewFileSecretStore(filepath.Join(t.TempDir(), "secrets.json"), []byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	ref, err := store.Put(ctx, "org-1", []byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}

	if got, err := store.Get(ctx, "org-1", ref); err != nil || string(got) != "secret-value" {
		t.Fatalf("get = %q, %v", got, err)
	}

	if _, err := store.Get(ctx, "org-2", ref); err == nil {
		t.Fatal("cross-organisation secret read succeeded")
	}

	if err := store.Delete(ctx, "org-2", ref); err == nil {
		t.Fatal("cross-organisation secret delete succeeded")
	}

	if err := store.Delete(ctx, "org-1", ref); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get(ctx, "org-1", ref); err == nil {
		t.Fatal("deleted secret remained readable")
	}
}

func TestWebRequiresSessionAndNeverRendersTokens(t *testing.T) {
	ctx := context.Background()
	service, store, _ := testService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	start, err := service.BeginOAuth(ctx, actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, stateErr := service.CompleteOAuth(ctx, start.State, "code"); stateErr != nil {
		t.Fatal(err)
	}

	if _, discoverErr := service.Discover(ctx, actor, connection.ID); discoverErr != nil {
		t.Fatal(err)
	}

	sessions, err := NewSessionManager([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}

	handler, err := NewWebHandler(WebConfig{
		Service: service, Sessions: sessions,
		Authenticator: OwnerAuthenticator{Email: "owner@example.com", Token: "admin-token-0123456789abcdef", Actor: actor},
		OwnerEmail:    "owner@example.com", DisplayName: "Owner",
	})
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	request, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/connections", nil)
	if requestErr != nil {
		t.Fatal(requestErr)
	}

	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("unauthenticated response = %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	form := url.Values{"email": {"owner@example.com"}, "token": {"admin-token-0123456789abcdef"}}

	request, requestErr = http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/login", strings.NewReader(form.Encode()))
	if requestErr != nil {
		t.Fatal(requestErr)
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d", resp.StatusCode)
	}

	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not set session cookie")
	}

	req, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/connections/"+connection.ID, nil)
	if requestErr != nil {
		t.Fatal(requestErr)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := make([]byte, 1<<20)
	n, _ := resp.Body.Read(body)
	html := string(body[:n])

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d", resp.StatusCode)
	}

	if strings.Contains(html, "refresh-secret-value") || strings.Contains(html, "access-secret-value") {
		t.Fatal("web response contained token material")
	}

	if !strings.Contains(html, "properties/123") {
		t.Fatal("web response did not contain discovered resource")
	}
}

func TestMigrationContract(t *testing.T) {
	up := strings.ToLower(migration001Up)
	down := strings.ToLower(migration001Down)

	for _, table := range []string{"users", "organizations", "memberships", "google_connections", "resource_grants", "audit_events", "oauth_states"} {
		if !strings.Contains(up, "create table if not exists "+table) {
			t.Fatalf("migration missing %s", table)
		}

		if !strings.Contains(down, "drop table if exists "+table) {
			t.Fatalf("rollback missing %s", table)
		}
	}

	if strings.Contains(up, "refresh_token") || strings.Contains(up, "access_token") {
		t.Fatal("connection schema contains token plaintext columns")
	}

	if !strings.Contains(up, "token_secret_ref text") {
		t.Fatal("connection schema missing token secret reference")
	}
}

func TestOwnerAuthenticatorRequiresCredential(t *testing.T) {
	authenticator := OwnerAuthenticator{
		Email: "owner@example.com", Token: "admin-token-0123456789abcdef",
		Actor: Actor{UserID: "user-1", OrganizationID: "org-1", Role: "owner"},
	}
	if _, err := authenticator.Login(context.Background(), "owner@example.com", "wrong"); err == nil {
		t.Fatal("wrong admin token minted an actor")
	}

	if _, err := authenticator.Login(context.Background(), "other@example.com", authenticator.Token); err == nil {
		t.Fatal("unknown email minted an actor")
	}

	actor, err := authenticator.Login(context.Background(), "owner@example.com", authenticator.Token)
	if err != nil || actor.UserID != "user-1" {
		t.Fatalf("valid credential rejected: %+v, %v", actor, err)
	}
}
