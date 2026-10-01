package controlplane

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestProductServicesPageShowsWorkspaceAndMarketingGrants(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateProductConnection(context.Background(), actor, "google", []string{"gmail"})
	if err != nil {
		t.Fatal(err)
	}
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	resp := productGet(t, client, server.URL, "/assets/"+connection.ID+"/services", cookies) //nolint:bodyclose // readProductBody closes this response

	body := readProductBody(t, resp)
	for _, want := range []string{"Workspace", "Marketing", "Enable service", "Allow read tool", "Curated read-tool access", "Excluded from hosted tool path", "Gmail API", "Analytics Admin API"} {
		if !strings.Contains(body, want) {
			t.Fatalf("services page missing %q: %s", want, body)
		}
	}

	assets := productGet(t, client, server.URL, "/assets/"+connection.ID, cookies) //nolint:bodyclose // readProductBody closes this response
	if body := readProductBody(t, assets); !strings.Contains(body, "/services") {
		t.Fatalf("assets page missing services link: %s", body)
	}
	service.ToolReader = &fakeToolReader{}
	service.OAuth.(*fakeOAuth).token.GrantedScopes = []string{"openid", "email", "https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/gmail.readonly"}

	start, err := service.BeginOAuth(context.Background(), actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, completeErr := service.CompleteOAuth(context.Background(), start.State, "code"); completeErr != nil {
		t.Fatal(completeErr)
	}

	if _, toolErr := service.SetToolEnabled(context.Background(), actor, connection.ID, "gmail", "gmail_search", true); toolErr != nil {
		t.Fatal(toolErr)
	}

	csrf := productCSRF(t, server, client, cookies)

	toolResp := productGetWithCSRF(t, client, server.URL, "/api/connections/"+connection.ID+"/tool?service=gmail&tool=gmail_search", cookies, csrf) //nolint:bodyclose // readProductBody closes this response
	if body := readProductBody(t, toolResp); !strings.Contains(body, `"operation":"tool.read"`) {
		t.Fatalf("tool API response = %s", body)
	}
	resp = postProduct(t, client, server, "/assets/"+connection.ID+"/services/save", url.Values{"csrf": []string{csrf}}, cookies)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save services status = %d", resp.StatusCode)
	}

	grants, err := service.ListToolGrants(context.Background(), actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	for _, grant := range grants {
		if grant.Tool == "gmail_search" && grant.Enabled {
			t.Fatal("unchecked tool grant remained enabled")
		}
	}
}
