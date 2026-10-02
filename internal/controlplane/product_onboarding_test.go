package controlplane

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestFreshConnectRedirectsToOnboardingAndReconnectReturnsToPicker(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)

	setFakeGoogleIdentity(t, service, "fresh-subject", "fresh@example.test")
	connectProduct(t, client, server, cookies, csrf)

	connections, listErr := service.ListConnections(context.Background(), actor)
	if listErr != nil {
		t.Fatal(listErr)
	}

	var fresh Connection
	for _, connection := range connections {
		if connection.GoogleEmail == "fresh@example.test" {
			fresh = connection
		}
	}
	if fresh.ID == "" {
		t.Fatal("fresh connection missing")
	}

	homeResp := productGet(t, client, server.URL, "/", cookies) //nolint:bodyclose // readProductBody closes this response
	if !strings.Contains(readProductBody(t, homeResp), "fresh@example.test") {
		t.Fatal("home missing fresh account")
	}

	// Reconnect routes back to the picker, not onboarding.
	reconnectStart := postProduct(t, client, server, "/assets/"+fresh.ID+"/reconnect", url.Values{"csrf": {csrf}}, cookies)
	defer reconnectStart.Body.Close()
	if reconnectStart.StatusCode != http.StatusSeeOther {
		t.Fatalf("reconnect start = %d", reconnectStart.StatusCode)
	}
	oauthCookies := append(append([]*http.Cookie{}, cookies...), reconnectStart.Cookies()...)
	parsedStart, parseErr := url.Parse(reconnectStart.Header.Get("Location"))
	if parseErr != nil {
		t.Fatal(parseErr)
	}

	setFakeGoogleIdentity(t, service, "fresh-subject", "fresh@example.test")
	callback := productGet(t, client, server.URL, "/oauth/google/callback?state="+url.QueryEscape(parsedStart.Query().Get("state"))+"&code=code", oauthCookies)
	defer callback.Body.Close()

	if callback.StatusCode != http.StatusSeeOther {
		t.Fatalf("reconnect callback = %d", callback.StatusCode)
	}
	if strings.HasPrefix(callback.Header.Get("Location"), "/onboarding/") {
		t.Fatalf("reconnect must not return to onboarding: %q", callback.Header.Get("Location"))
	}
}

func TestOnboardingRouteRedirectsUnknownConnection(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)

	resp := productGet(t, client, server.URL, "/onboarding/does-not-exist", cookies)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("unknown onboarding id = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}
