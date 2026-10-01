package controlplane

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeToolReader struct {
	calls []string
}

func (f *fakeToolReader) Read(_ context.Context, _ Connection, _ OAuthToken, tool string) (ToolReadResult, error) {
	f.calls = append(f.calls, tool)
	return ToolReadResult{ResultCount: 1}, nil
}

func TestProductCatalogGroupsWorkspaceAndMarketing(t *testing.T) {
	groups := map[string]bool{}
	for _, service := range ProductServices() {
		groups[service.Category] = true
	}

	if !groups[ProductCategoryWorkspace] || !groups[ProductCategoryMarketing] {
		t.Fatalf("catalog categories = %#v", groups)
	}

	for _, want := range []string{"gmail", "calendar", "drive", "analytics", "googleads", "tagmanager", "searchconsole", "bigquery"} {
		found := false

		for _, service := range ProductServices() {
			if service.Service == want {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("catalog missing %q", want)
		}
	}
}

func TestAddConnectionServicesAndToolPolicy(t *testing.T) {
	ctx := context.Background()
	service, store, _ := testService(t)
	actor := ownerActor(t, store)
	service.ToolReader = &fakeToolReader{}
	service.OAuth.(*fakeOAuth).token.GrantedScopes = []string{"openid", "email", "https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/gmail.readonly"}

	connection, err := service.CreateProductConnection(ctx, actor, "google", []string{"gmail"})
	if err != nil {
		t.Fatal(err)
	}

	start, err := service.BeginOAuth(ctx, actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, completeErr := service.CompleteOAuth(ctx, start.State, "code"); completeErr != nil {
		t.Fatal(completeErr)
	}

	updated, err := service.AddConnectionServices(ctx, actor, connection.ID, []string{"calendar", "drive"})
	if err != nil {
		t.Fatal(err)
	}

	if !containsString(updated.Services, "gmail") || !containsString(updated.Services, "calendar") || !containsString(updated.Services, "drive") {
		t.Fatalf("services = %#v", updated.Services)
	}

	if !containsString(updated.RequestedScopes, "https://www.googleapis.com/auth/calendar.readonly") || !containsString(updated.RequestedScopes, "https://www.googleapis.com/auth/drive.readonly") {
		t.Fatalf("incremental scopes = %#v", updated.RequestedScopes)
	}

	if _, toolErr := service.SetToolEnabled(ctx, actor, connection.ID, "gmail", "gmail_search", true); toolErr != nil {
		t.Fatal(toolErr)
	}

	service.OAuth.(*fakeOAuth).token.GrantedScopes = append(service.OAuth.(*fakeOAuth).token.GrantedScopes,
		"https://www.googleapis.com/auth/calendar.readonly", "https://www.googleapis.com/auth/drive.readonly")

	reconnect, err := service.BeginOAuth(ctx, actor, connection.ID, true)
	if err != nil {
		t.Fatal(err)
	}

	requested := service.OAuth.(*fakeOAuth).authorizationScopes
	if !containsString(requested, "https://www.googleapis.com/auth/gmail.readonly") || !containsString(requested, "https://www.googleapis.com/auth/calendar.readonly") || !containsString(requested, "https://www.googleapis.com/auth/drive.readonly") {
		t.Fatalf("reconnect scopes = %#v", requested)
	}

	reconnected, err := service.CompleteOAuth(ctx, reconnect.State, "code-2")
	if err != nil {
		t.Fatal(err)
	}

	if !containsString(reconnected.GrantedScopes, "https://www.googleapis.com/auth/gmail.readonly") || !containsString(reconnected.GrantedScopes, "https://www.googleapis.com/auth/calendar.readonly") || !containsString(reconnected.GrantedScopes, "https://www.googleapis.com/auth/drive.readonly") {
		t.Fatalf("merged scopes = %#v", reconnected.GrantedScopes)
	}

	preserved, err := service.ListToolGrants(ctx, actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	if !containsToolGrant(preserved, "gmail", "gmail_search") {
		t.Fatalf("curated grant did not survive reconnect: %#v", preserved)
	}

	reader := service.ToolReader.(*fakeToolReader)
	if _, err := service.RunTool(ctx, actor, connection.ID, "gmail", "gmail_search"); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(reader.calls, []string{"gmail_search"}) {
		t.Fatalf("tool calls = %#v", reader.calls)
	}

	if _, err := service.RunTool(ctx, actor, connection.ID, "calendar", "calendar_events"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("disabled tool error = %v", err)
	}

	if _, err := service.RunTool(ctx, actor, connection.ID, "analytics", "analytics_properties_list"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("resource-backed tool error = %v", err)
	}

	if err := (Policy{Store: store}).AllowTool(ctx, actor, connection.ID, "gmail", "gmail_search"); err != nil {
		t.Fatal(err)
	}
}

func TestToolPolicyDoesNotInventResourceGrants(t *testing.T) {
	ctx := context.Background()
	service, store, _ := testService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateProductConnection(ctx, actor, "google", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetToolEnabled(ctx, actor, connection.ID, "analytics", "*", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("resource service tool grant error = %v", err)
	}

	if _, err := service.SetToolEnabled(ctx, actor, connection.ID, "gmail", "gmail_send", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown tool grant error = %v", err)
	}

	if _, err := service.SetToolEnabled(ctx, actor, connection.ID, "gmail", "*", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wildcard tool grant error = %v", err)
	}

	if err := (Policy{Store: store}).AllowTool(ctx, actor, connection.ID, "analytics", "analytics_properties_list"); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("resource service tool policy error = %v", err)
	}
}

func TestLegacyWildcardToolGrantIsDenied(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	actor := ownerActor(t, store)

	connection, err := store.CreateConnection(ctx, Connection{
		OrganizationID: actor.OrganizationID,
		Name:           "google",
		Services:       []string{"gmail"},
		ToolGrants: map[string]ToolGrant{
			"gmail/*": {Service: "gmail", Tool: "*", Enabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := (Policy{Store: store}).AllowTool(ctx, actor, connection.ID, "gmail", "gmail_search"); err == nil {
		t.Fatal("legacy wildcard grant allowed a tool")
	}
}

func TestUncuratedServicesRejectToolAccess(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	actor := ownerActor(t, store)
	services := []string{}
	grants := map[string]ToolGrant{}

	for _, definition := range ProductServices() {
		if definition.Tool != "" {
			continue
		}
		services = append(services, definition.Service)
		grants[toolGrantKey(definition.Service, "*")] = ToolGrant{Service: definition.Service, Tool: "*", Enabled: true}
	}

	connection, err := store.CreateConnection(ctx, Connection{OrganizationID: actor.OrganizationID, Name: "google", Services: services, ToolGrants: grants})
	if err != nil {
		t.Fatal(err)
	}

	for _, definition := range ProductServices() {
		if definition.Tool != "" {
			continue
		}

		if err := (Policy{Store: store}).AllowTool(ctx, actor, connection.ID, definition.Service, "*"); err == nil {
			t.Fatalf("uncurated service %q accepted wildcard tool access", definition.Service)
		}
	}
}

func TestToolGrantsForServicesAreCuratedAndDeterministic(t *testing.T) {
	got := toolGrantsForServices([]string{"gmail", "calendar", "drive", "docs", "analytics", "admin"})
	if len(got) != 3 {
		t.Fatalf("tool grants = %#v", got)
	}

	for _, key := range []string{"gmail/gmail_search", "calendar/calendar_events", "drive/drive_search"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing curated grant %q: %#v", key, got)
		}
	}

	for key := range got {
		if strings.Contains(key, "/*") {
			t.Fatalf("wildcard grant provisioned: %#v", got)
		}
	}
}

func containsToolGrant(grants []ToolGrant, service, tool string) bool {
	for _, grant := range grants {
		if grant.Service == service && grant.Tool == tool && grant.Enabled {
			return true
		}
	}

	return false
}
