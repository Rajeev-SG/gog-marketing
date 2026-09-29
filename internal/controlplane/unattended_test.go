package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	errTestInvalidGoogleGrant = errors.New("oauth2: \"invalid_grant\" \"Bad Request\"")
	errTestUpdateConnection   = errors.New("update connection failed")
)

type failingUpdateStore struct {
	*MemoryStore
}

func (failingUpdateStore) UpdateConnection(context.Context, Connection) (Connection, error) {
	return Connection{}, errTestUpdateConnection
}

type recordingSecretStore struct {
	SecretStore
	lastReference string
	lastValue     []byte
}

func (r *recordingSecretStore) Put(ctx context.Context, organizationID string, value []byte) (string, error) {
	reference, err := r.SecretStore.Put(ctx, organizationID, value)
	r.lastReference = reference

	r.lastValue = append([]byte(nil), value...)

	if err != nil {
		return "", fmt.Errorf("store secret: %w", err)
	}

	return reference, nil
}

func TestSaveTokenUsesCallerOrganization(t *testing.T) {
	ctx := context.Background()
	service, store, secrets := testService(t)
	actor := ownerActor(t, store)

	connection, err := store.CreateConnection(ctx, Connection{
		ID: "connection-1", OrganizationID: "wrong-org", Name: "gmail",
		Status: ConnectionHealthy,
	})
	if err != nil {
		t.Fatal(err)
	}

	token := OAuthToken{
		AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour),
		GrantedScopes: []string{"https://www.googleapis.com/auth/analytics.readonly"},
	}
	if saveErr := service.saveToken(ctx, actor, &connection, token); saveErr != nil {
		t.Fatal(saveErr)
	}

	if connection.SecretRef == "" {
		t.Fatal("saveToken did not set a secret reference")
	}

	raw, err := secrets.Get(ctx, actor.OrganizationID, connection.SecretRef)
	if err != nil {
		t.Fatalf("token not stored under caller organization: %v", err)
	}

	if _, decodeErr := unmarshalToken(raw); decodeErr != nil {
		t.Fatal(decodeErr)
	}
}

func TestSaveTokenRetainsNewSecretWhenConnectionUpdateFails(t *testing.T) {
	ctx := context.Background()
	_, _, secrets := testService(t)
	recording := &recordingSecretStore{SecretStore: secrets}
	service := &Service{Store: failingUpdateStore{}, Secrets: recording}
	connection := Connection{ID: "connection-1", OrganizationID: "connection-org"}
	token := OAuthToken{
		AccessToken: "access", RefreshToken: "rotated-refresh", Expiry: time.Now().Add(time.Hour),
		GrantedScopes: []string{"https://www.googleapis.com/auth/analytics.readonly"},
	}

	err := service.saveToken(ctx, Actor{OrganizationID: "caller-org"}, &connection, token)
	if err == nil {
		t.Fatal("saveToken unexpectedly succeeded")
	}

	if recording.lastReference == "" {
		t.Fatal("saveToken did not store a rotated secret")
	}

	raw, getErr := recording.Get(ctx, "caller-org", recording.lastReference)
	if getErr != nil {
		t.Fatalf("rotated secret was discarded after update failure: %v", getErr)
	}

	if _, decodeErr := unmarshalToken(raw); decodeErr != nil {
		t.Fatal(decodeErr)
	}

	if connection.SecretRef != "" {
		t.Fatalf("failed update left an uncommitted secret ref on the caller connection: %q", connection.SecretRef)
	}
}

func TestRefreshAndDiscoverNeverAuthorize(t *testing.T) {
	ctx := context.Background()
	service, store, _ := testService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	oauth := service.OAuth.(*fakeOAuth)
	if _, completeErr := service.CompleteOAuth(ctx, mustOAuthState(t, service, actor, connection.ID), "code"); completeErr != nil {
		t.Fatal(err)
	}
	before := oauth.authorizationCalls

	if _, err := service.Refresh(ctx, actor, connection.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Discover(ctx, actor, connection.ID); err != nil {
		t.Fatal(err)
	}

	if oauth.authorizationCalls != before {
		t.Fatalf("routine refresh/discover invoked AuthorizationURL %d times", oauth.authorizationCalls-before)
	}
}

func TestInvalidGrantMarksNeedsReconnectWithoutBrowser(t *testing.T) {
	ctx := context.Background()
	service, store, _ := testService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	oauth := service.OAuth.(*fakeOAuth)
	if _, completeErr := service.CompleteOAuth(ctx, mustOAuthState(t, service, actor, connection.ID), "code"); completeErr != nil {
		t.Fatal(err)
	}
	oauth.err = errTestInvalidGoogleGrant
	token, _ := unmarshalToken(mustToken(t, service, actor, connection.ID))

	token.Expiry = time.Now().Add(-time.Minute)
	if saveErr := service.saveToken(ctx, actor, &connection, token); saveErr != nil {
		t.Fatal(err)
	}
	before := oauth.authorizationCalls

	if _, refreshErr := service.Refresh(ctx, actor, connection.ID); refreshErr == nil {
		t.Fatal("invalid_grant refresh succeeded")
	}

	updated, err := service.GetConnection(ctx, actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	if updated.Status != ConnectionNeedsReconnect || updated.LastErrorCategory != AuthFailureInvalidGrant {
		t.Fatalf("unexpected invalid_grant state: %+v", updated)
	}

	if oauth.authorizationCalls != before {
		t.Fatal("invalid_grant opened an authorization URL")
	}
}

func TestScopeDriftMarksNeedsReconnect(t *testing.T) {
	ctx := context.Background()
	service, store, _ := testService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	oauth := service.OAuth.(*fakeOAuth)
	if _, completeErr := service.CompleteOAuth(ctx, mustOAuthState(t, service, actor, connection.ID), "code"); completeErr != nil {
		t.Fatal(err)
	}
	oauth.token.GrantedScopes = []string{"openid"}

	stale, _ := unmarshalToken(mustToken(t, service, actor, connection.ID))

	stale.Expiry = time.Now().Add(-time.Minute)
	if saveErr := service.saveToken(ctx, actor, &connection, stale); saveErr != nil {
		t.Fatal(err)
	}

	if _, refreshErr := service.Refresh(ctx, actor, connection.ID); refreshErr == nil {
		t.Fatal("scope mismatch refresh succeeded")
	}

	updated, err := service.GetConnection(ctx, actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	if updated.Status != ConnectionNeedsReconnect || updated.LastErrorCategory != AuthFailureScopeMismatch {
		t.Fatalf("unexpected scope state: %+v", updated)
	}
}

func TestAuthorizationURLForcesConsentOnlyWhenRequested(t *testing.T) {
	provider := NewGoogleOAuthProvider("client", "secret", "http://example.test/callback")
	input := OAuthStartInput{State: "state", CodeVerifier: "verifier", Nonce: "nonce", RedirectURI: "http://example.test/callback", Scopes: []string{"openid"}}

	url := provider.AuthorizationURL(input)
	if strings.Contains(url, "prompt=consent") {
		t.Fatal("normal authorization URL forced consent")
	}
	input.ForceConsent = true

	url = provider.AuthorizationURL(input)
	if !strings.Contains(url, "prompt=consent") {
		t.Fatal("explicit reconnect URL did not force consent")
	}
}

func mustOAuthState(t *testing.T, service *Service, actor Actor, id string) string {
	t.Helper()

	start, err := service.BeginOAuth(context.Background(), actor, id, false)
	if err != nil {
		t.Fatal(err)
	}

	return start.State
}

func mustToken(t *testing.T, service *Service, actor Actor, id string) []byte {
	t.Helper()

	raw, err := service.Secrets.Get(context.Background(), actor.OrganizationID, serviceSecretRef(t, service, actor, id))
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func serviceSecretRef(t *testing.T, service *Service, actor Actor, id string) string {
	t.Helper()

	connection, err := service.Store.GetConnection(context.Background(), actor.OrganizationID, id)
	if err != nil {
		t.Fatal(err)
	}

	return connection.SecretRef
}

func TestRefreshedTokenRemainsReadableByCallerOrganization(t *testing.T) {
	ctx := context.Background()
	service, store, _ := testService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	oauth := service.OAuth.(*fakeOAuth)
	if _, completeErr := service.CompleteOAuth(ctx, mustOAuthState(t, service, actor, connection.ID), "code"); completeErr != nil {
		t.Fatal(completeErr)
	}

	connection, err = service.GetConnection(ctx, actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	oauth.authorizationCalls = 0

	// Exercise the import/bootstrap path: store a token under the actor organization key,
	// force a refresh that replaces the secret, then load it for discovery.
	token, err := unmarshalToken(mustToken(t, service, actor, connection.ID))
	if err != nil {
		t.Fatal(err)
	}
	token.Expiry = time.Now().Add(-time.Minute)
	token.Email = "imported@example.com"

	if saveErr := service.saveToken(ctx, actor, &connection, token); saveErr != nil {
		t.Fatal(saveErr)
	}

	if _, refreshErr := service.Refresh(ctx, actor, connection.ID); refreshErr != nil {
		t.Fatal(refreshErr)
	}

	if _, discoverErr := service.Discover(ctx, actor, connection.ID); discoverErr != nil {
		t.Fatal(discoverErr)
	}

	if oauth.authorizationCalls != 0 {
		t.Fatalf("refresh/discover called AuthorizationURL %d times", oauth.authorizationCalls)
	}

	ref := serviceSecretRef(t, service, actor, connection.ID)
	if raw, loadErr := service.Secrets.Get(ctx, actor.OrganizationID, ref); loadErr != nil {
		t.Fatalf("refreshed token not readable under caller organization: %v", loadErr)
	} else if _, decodeErr := unmarshalToken(raw); decodeErr != nil {
		t.Fatalf("refreshed token does not decode: %v", decodeErr)
	}
}

func TestConcurrentRefreshPersistsOnce(t *testing.T) {
	ctx := context.Background()
	service, store, _ := testService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	oauth := service.OAuth.(*fakeOAuth)
	if _, completeErr := service.CompleteOAuth(ctx, mustOAuthState(t, service, actor, connection.ID), "code"); completeErr != nil {
		t.Fatal(err)
	}

	stale, _ := unmarshalToken(mustToken(t, service, actor, connection.ID))

	stale.Expiry = time.Now().Add(-time.Minute)
	if saveErr := service.saveToken(ctx, actor, &connection, stale); saveErr != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = service.Refresh(ctx, actor, connection.ID)
		}()
	}

	wg.Wait()

	if oauth.refreshCalls != 1 {
		t.Fatalf("concurrent refresh calls = %d", oauth.refreshCalls)
	}
}

func TestMissingSecretMarksNeedsReconnect(t *testing.T) {
	ctx := context.Background()
	service, store, secrets := testService(t)
	actor := ownerActor(t, store)

	connection, createErr := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if createErr != nil {
		t.Fatal(createErr)
	}

	if _, completeErr := service.CompleteOAuth(ctx, mustOAuthState(t, service, actor, connection.ID), "code"); completeErr != nil {
		t.Fatal(completeErr)
	}

	stale, _ := unmarshalToken(mustToken(t, service, actor, connection.ID))
	stale.Expiry = time.Now().Add(-time.Minute)

	if err := service.saveToken(ctx, actor, &connection, stale); err != nil {
		t.Fatal(err)
	}

	current, getErr := service.GetConnection(ctx, actor, connection.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}

	if deleteErr := secrets.Delete(ctx, actor.OrganizationID, current.SecretRef); deleteErr != nil {
		t.Fatal(deleteErr)
	}

	if _, refreshErr := service.Refresh(ctx, actor, connection.ID); refreshErr == nil {
		t.Fatal("missing secret refresh succeeded")
	}

	updated, finalErr := service.GetConnection(ctx, actor, connection.ID)
	if finalErr != nil {
		t.Fatal(finalErr)
	}

	if updated.Status != ConnectionNeedsReconnect || updated.LastErrorCategory != AuthFailureInvalidGrant {
		t.Fatalf("missing secret state: %+v", updated)
	}
}
