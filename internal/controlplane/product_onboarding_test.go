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

	freshConnect := postProduct(t, client, server, "/connect/google", url.Values{"csrf": {csrf}}, cookies)
	defer freshConnect.Body.Close()

	if freshConnect.StatusCode != http.StatusSeeOther {
		t.Fatalf("connect = %d", freshConnect.StatusCode)
	}
	freshOAuth := append(append([]*http.Cookie{}, cookies...), freshConnect.Cookies()...)

	freshStart, parseFresh := url.Parse(freshConnect.Header.Get("Location"))
	if parseFresh != nil {
		t.Fatal(parseFresh)
	}

	freshCallback := productGet(t, client, server.URL, "/oauth/google/callback?state="+url.QueryEscape(freshStart.Query().Get("state"))+"&code=code", freshOAuth)
	defer freshCallback.Body.Close()

	if freshCallback.StatusCode != http.StatusSeeOther {
		t.Fatalf("fresh callback = %d", freshCallback.StatusCode)
	}

	if want := "/onboarding/"; !strings.HasPrefix(freshCallback.Header.Get("Location"), want) {
		t.Fatalf("fresh callback Location = %q, want prefix %q", freshCallback.Header.Get("Location"), want)
	}

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

	// First save (from the onboarding pick) always routes to the completion
	// screen so the final step resolves from real server state.
	firstSave := postProduct(t, client, server, "/assets/"+fresh.ID+"/save", url.Values{"csrf": {csrf}, "resource": {"properties/123"}}, cookies)
	defer firstSave.Body.Close()

	if want := "/onboarding/" + fresh.ID + "?message=Access+saved."; firstSave.Header.Get("Location") != want || firstSave.StatusCode != http.StatusSeeOther {
		t.Fatalf("save redirect = %d %q, want %q", firstSave.StatusCode, firstSave.Header.Get("Location"), want)
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

	if want := "/assets/" + fresh.ID; callback.Header.Get("Location") != want {
		t.Fatalf("reconnect callback Location = %q, want %q", callback.Header.Get("Location"), want)
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

func TestProductTemplatesSecondRender(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)

	for _, path := range []string{"/", "/signin"} {
		resp := productGet(t, client, server.URL, path, cookies)
		defer resp.Body.Close()

		body := readProductBody(t, resp)
		if !strings.Contains(body, "shell-top") || !strings.Contains(body, "</html>") {
			t.Fatalf("GET %s missing full shell render", path)
		}
	}
}

func TestProductTemplatesRenderShellPaths(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)

	for _, path := range []string{"/", "/signin"} {
		resp := productGet(t, client, server.URL, path, cookies)
		defer resp.Body.Close()

		body := readProductBody(t, resp)
		if !strings.Contains(body, "shell-top") || !strings.Contains(body, "</html>") {
			t.Fatalf("GET %s missing full shell render", path)
		}
	}
}
