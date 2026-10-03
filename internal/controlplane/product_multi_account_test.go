package controlplane

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func setFakeGoogleIdentity(t *testing.T, service *Service, subject, email string) {
	t.Helper()

	oauth, ok := service.OAuth.(*fakeOAuth)
	if !ok {
		t.Fatal("expected fake OAuth provider")
	}
	token := oauth.token
	token.Subject = subject
	token.Email = email
	token.Expiry = time.Now().Add(time.Hour)
	oauth.token = token
}

func TestProductManagesIndependentGoogleAccounts(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)

	setFakeGoogleIdentity(t, service, "personal-subject", "personal@example.test")
	connectProduct(t, client, server, cookies, csrf)
	setFakeGoogleIdentity(t, service, "singulyr-subject", "singulyr@example.test")
	connectProduct(t, client, server, cookies, csrf)

	connections, err := service.ListConnections(context.Background(), actor)
	if err != nil {
		t.Fatal(err)
	}
	productConnections := make([]Connection, 0, 2)

	for _, connection := range connections {
		if isProductConnection(connection) {
			productConnections = append(productConnections, connection)
		}
	}

	if len(productConnections) != 2 || productConnections[0].ID == productConnections[1].ID {
		t.Fatalf("expected two stable product connections: %+v", productConnections)
	}

	for _, connection := range productConnections {
		if !connection.ProductManaged {
			t.Fatalf("connection was not product managed: %+v", connection)
		}
	}

	if productConnections[0].GoogleEmail == productConnections[1].GoogleEmail ||
		productConnections[0].GoogleSubject == productConnections[1].GoogleSubject {
		t.Fatal("account identity was not independent")
	}

	homeResp := productGet(t, client, server.URL, "/", cookies) //nolint:bodyclose // readProductBody closes this response

	home := readProductBody(t, homeResp)
	for _, want := range []string{"personal@example.test", "singulyr@example.test", "Manage access", "Disconnect", "Connect Google account"} {
		if !strings.Contains(home, want) {
			t.Fatalf("account list missing %q: %s", want, home)
		}
	}

	if strings.Contains(home, "Reconnect Google") {
		t.Fatalf("healthy accounts must not show reconnect actions: %s", home)
	}

	first := productConnections[0]
	second := productConnections[1]

	firstGrants, err := service.ListResources(context.Background(), actor, first.ID)
	if err != nil || len(firstGrants) == 0 {
		t.Fatal("first account has no independent grants", err)
	}

	if _, enableErr := service.SetResourceEnabled(context.Background(), actor, first.ID, firstGrants[0].ResourceID, true); enableErr != nil {
		t.Fatal(enableErr)
	}

	secondGrants, err := service.ListResources(context.Background(), actor, second.ID)
	if err != nil || len(secondGrants) == 0 {
		t.Fatal("second account has no independent grants", err)
	}

	for _, grant := range secondGrants {
		if grant.Enabled {
			t.Fatal("first-account selection leaked into second account")
		}
	}

	firstEmail := first.GoogleEmail
	resp := postProduct(t, client, server, "/assets/"+first.ID+"/disconnect", url.Values{"csrf": {csrf}}, cookies)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("disconnect status = %d", resp.StatusCode)
	}

	firstAfter, err := service.GetConnection(context.Background(), actor, first.ID)
	if err != nil || firstAfter.Status != ConnectionDisconnected || firstAfter.GoogleEmail != "" || firstAfter.SecretRef != "" {
		t.Fatalf("first account did not disconnect cleanly: %+v, %v", firstAfter, err)
	}

	secondAfter, err := service.GetConnection(context.Background(), actor, second.ID)
	if err != nil || secondAfter.Status != ConnectionHealthy || secondAfter.GoogleEmail != "singulyr@example.test" || secondAfter.SecretRef == "" {
		t.Fatalf("second account changed during first disconnect: %+v, %v", secondAfter, err)
	}

	homeResp = productGet(t, client, server.URL, "/", cookies) //nolint:bodyclose // readProductBody closes this response

	home = readProductBody(t, homeResp)
	if strings.Contains(home, firstEmail) || strings.Contains(home, "/assets/"+first.ID+"/reconnect") {
		t.Fatal("disconnected account remained visible for reconnect")
	}

	if !strings.Contains(home, "Connect Google account") {
		t.Fatal("disconnected account lost the add/reconnect product path")
	}
}

func TestOAuthIdentityCannotOverwriteAnotherConnection(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)

	first, err := service.CreateProductConnection(context.Background(), actor, "google", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	second, err := service.CreateProductConnection(context.Background(), actor, "google-2", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	setFakeGoogleIdentity(t, service, "shared-subject", "shared@example.test")

	start, err := service.BeginOAuth(context.Background(), actor, first.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, completeErr := service.CompleteOAuth(context.Background(), start.State, "code"); completeErr != nil {
		t.Fatal(completeErr)
	}

	start, err = service.BeginOAuth(context.Background(), actor, second.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, completeErr := service.CompleteOAuth(context.Background(), start.State, "code"); !errors.Is(completeErr, ErrConflict) {
		t.Fatal("duplicate Google identity was accepted", completeErr)
	}

	if _, lookupErr := service.GetConnection(context.Background(), actor, second.ID); !errors.Is(lookupErr, ErrNotFound) {
		t.Fatalf("failed duplicate stub was not removed: %+v", lookupErr)
	}

	start, err = service.BeginOAuth(context.Background(), actor, first.ID, true)
	if err != nil {
		t.Fatal(err)
	}

	if _, completeErr := service.CompleteOAuth(context.Background(), start.State, "code"); completeErr != nil {
		t.Fatal("same connection could not reconnect", completeErr)
	}
}

func TestProductReconnectNeedsAttentionConnection(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	setFakeGoogleIdentity(t, service, "attention-subject", "attention@example.test")

	connection, err := service.CreateProductConnection(context.Background(), actor, "google", []string{"analytics"})
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

	connection.Status = ConnectionNeedsReconnect
	connection.LastError = "development invalid grant"

	connection.LastErrorCategory = AuthFailureInvalidGrant
	if _, updateErr := store.UpdateConnection(context.Background(), connection); updateErr != nil {
		t.Fatal(updateErr)
	}

	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)
	resp := postProduct(t, client, server, "/assets/"+connection.ID+"/reconnect", url.Values{"csrf": {csrf}}, cookies)
	location := resp.Header.Get("Location")
	oauthCookies := append(append([]*http.Cookie{}, cookies...), resp.Cookies()...)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(location, "accounts.example.test") {
		t.Fatalf("needs-attention reconnect did not start OAuth: %d %q", resp.StatusCode, location)
	}

	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}

	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatal("reconnect OAuth state missing")
	}
	callback := productGet(t, client, server.URL, "/oauth/google/callback?state="+url.QueryEscape(state)+"&code=code", oauthCookies)
	callback.Body.Close()

	if callback.StatusCode != http.StatusSeeOther || callback.Header.Get("Location") != "/assets/"+connection.ID {
		t.Fatalf("reconnect state was not bound to the intended connection: %d %q", callback.StatusCode, callback.Header.Get("Location"))
	}

	after, err := service.GetConnection(context.Background(), actor, connection.ID)
	if err != nil || after.Status != ConnectionHealthy || after.GoogleSubject != "attention-subject" || after.SecretRef == "" {
		t.Fatalf("needs-attention reconnect did not restore the same connection: %+v, %v", after, err)
	}
}

func TestOAuthRequiresVerifiedGoogleIdentity(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateProductConnection(context.Background(), actor, "google", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	token := service.OAuth.(*fakeOAuth).token
	token.Subject = ""
	token.Email = ""
	service.OAuth.(*fakeOAuth).token = token

	start, err := service.BeginOAuth(context.Background(), actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, completeErr := service.CompleteOAuth(context.Background(), start.State, "code"); !errors.Is(completeErr, ErrInvalid) {
		t.Fatal("identity-less OAuth token was accepted", completeErr)
	}
}

func TestProductRejectsSecretOnlyOperatorConnection(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(context.Background(), actor, "operator", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}
	connection.GoogleEmail = "operator@example.test"
	connection.GoogleSubject = "operator-subject"

	connection.SecretRef = "operator-secret"
	if _, updateErr := store.UpdateConnection(context.Background(), connection); updateErr != nil {
		t.Fatal(updateErr)
	}

	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	resp := productGet(t, client, server.URL, "/assets/"+connection.ID, cookies)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("operator connection was product-manageable: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	homeResp := productGet(t, client, server.URL, "/", cookies) //nolint:bodyclose // readProductBody closes this response

	home := readProductBody(t, homeResp)
	if strings.Contains(home, "operator@example.test") || strings.Contains(home, "Manage access") {
		t.Fatalf("secret-only operator connection leaked into product UI: %s", home)
	}
}

func TestFailedConnectAttemptsReuseOneHiddenStub(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)

	for i := 0; i < 2; i++ {
		resp := postProduct(t, client, server, "/connect/google", url.Values{"csrf": {csrf}}, cookies)
		resp.Body.Close()

		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("connect attempt %d status = %d", i, resp.StatusCode)
		}
	}

	connections, err := service.ListConnections(context.Background(), actor)
	if err != nil {
		t.Fatal(err)
	}
	stubs := 0

	for _, connection := range connections {
		if isProductConnection(connection) {
			stubs++
		}
	}

	if stubs != 1 {
		t.Fatalf("failed attempts accumulated %d product connections", stubs)
	}

	homeResp := productGet(t, client, server.URL, "/", cookies) //nolint:bodyclose // readProductBody closes this response

	home := readProductBody(t, homeResp)
	if strings.Contains(home, "Manage access") || strings.Contains(home, "Google account pending") {
		t.Fatalf("unconnected stub leaked into account list: %s", home)
	}
}

func TestProductReconnectHealthyConnection(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	setFakeGoogleIdentity(t, service, "healthy-subject", "healthy@example.test")

	connection, err := service.CreateProductConnection(context.Background(), actor, "google", []string{"analytics"})
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

	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)
	resp := postProduct(t, client, server, "/assets/"+connection.ID+"/reconnect", url.Values{"csrf": {csrf}}, cookies)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "accounts.example.test") {
		t.Fatalf("healthy reconnect did not start OAuth: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	after, err := service.GetConnection(context.Background(), actor, connection.ID)
	if err != nil || after.Status != ConnectionHealthy || after.GoogleSubject != "healthy-subject" || after.SecretRef == "" {
		t.Fatalf("healthy reconnect changed connection state: %+v, %v", after, err)
	}
}
