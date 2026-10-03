package controlplane

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestProductNoResultsOffersClearSearchWithoutRediscovery(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	csrf := productCSRF(t, server, client, cookies)
	connectProduct(t, client, server, cookies, csrf)

	connections, err := service.ListConnections(context.Background(), actor)
	if err != nil {
		t.Fatal(err)
	}
	var id string

	for _, connection := range connections {
		if isProductConnection(connection) {
			id = connection.ID
		}
	}

	if id == "" {
		t.Fatal("missing connected account")
	}

	if _, enableErr := service.SetResourceEnabled(context.Background(), actor, id, "properties/123", true); enableErr != nil {
		t.Fatal(enableErr)
	}
	response := productGet(t, client, server.URL, "/assets/"+id+"?q="+url.QueryEscape("no matching property"), cookies) //nolint:bodyclose // readProductBody closes the response.

	body := readProductBody(t, response)
	for _, text := range []string{"No matching assets", "Clear search", "Your existing selections are unchanged."} {
		if !strings.Contains(body, text) {
			t.Fatalf("no-results feedback missing %q", text)
		}
	}

	if strings.Contains(body, "No assets found yet") || strings.Contains(body, ">Try again</button>") {
		t.Fatal("search miss incorrectly suggests discovery failure")
	}

	grants, err := service.ListResources(context.Background(), actor, id)
	if err != nil {
		t.Fatal(err)
	}

	for _, grant := range grants {
		if grant.ResourceID == "properties/123" && !grant.Enabled {
			t.Fatal("search changed the existing selection")
		}
	}
	clearResponse := productGet(t, client, server.URL, "/assets/"+id, cookies) //nolint:bodyclose // readProductBody closes the response.

	cleared := readProductBody(t, clearResponse)
	if !strings.Contains(cleared, "Example GA4") {
		t.Fatal("clearing search did not restore discovered assets")
	}
}
