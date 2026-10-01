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

	if _, err := service.SetToolEnabled(ctx, actor, connection.ID, "gmail", "gmail_search", true); err != nil {
		t.Fatal(err)
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
