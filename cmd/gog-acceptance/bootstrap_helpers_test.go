package main

import (
	"context"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/acceptance"

	"github.com/openclaw/gogcli/internal/controlplane"
)

func TestBootstrapReauthorizeArgumentsMatchStableCLI(t *testing.T) {
	got := reauthorizeBootstrapTokenArgs("user@example.test", "/tmp/token.json")
	want := [][]string{
		{
			"auth", "add", "user@example.test",
			"--client", "personal-owned",
			"--services", "analytics,searchconsole,bigquery,ads",
			"--extra-scopes", "https://www.googleapis.com/auth/tagmanager.readonly",
			"--force-consent",
		},
		{
			"auth", "tokens", "export", "user@example.test",
			"--client", "personal-owned",
			"--out", "/tmp/token.json",
			"--overwrite",
			"--no-input",
		},
	}

	if len(got) != len(want) {
		t.Fatalf("command count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("command %d = %#v, want %#v", i, got[i], want[i])
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("command %d arg %d = %q, want %q", i, j, got[i][j], want[i][j])
			}
		}
	}
}

type bootstrapFakeOAuth struct {
	authorizationCalls int
}

func (bootstrapFakeOAuth) AuthorizationURL(controlplane.OAuthStartInput) string {
	return "https://accounts.example.test/auth"
}

func (bootstrapFakeOAuth) Exchange(context.Context, controlplane.OAuthStartInput, string) (controlplane.OAuthToken, error) {
	return controlplane.OAuthToken{}, nil
}

func (f *bootstrapFakeOAuth) Refresh(_ context.Context, token controlplane.OAuthToken) (controlplane.OAuthToken, error) {
	return controlplane.OAuthToken{
		AccessToken:  "refreshed-access",
		RefreshToken: token.RefreshToken,
		Expiry:       time.Now().Add(time.Hour),
		Subject:      token.Subject,
		Email:        token.Email,
		GrantedScopes: []string{
			"https://www.googleapis.com/auth/analytics.readonly",
		},
	}, nil
}

type bootstrapFakeDiscoverer struct{}

func (bootstrapFakeDiscoverer) Discover(context.Context, controlplane.Connection, controlplane.OAuthToken) ([]controlplane.ResourceGrant, error) {
	return []controlplane.ResourceGrant{{
		Service:      "analytics",
		ResourceType: "property",
		ResourceID:   "properties/test",
		DisplayName:  "Test property",
		Enabled:      true,
	}}, nil
}

func TestInstallBootstrapTokenRefreshesAndDiscoversWithPersistedIdentity(t *testing.T) {
	ctx := context.Background()
	store := controlplane.NewMemoryStore()
	secrets, err := controlplane.NewFileSecretStore(t.TempDir()+"/secrets.json", []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	oauth := &bootstrapFakeOAuth{}
	service := &controlplane.Service{
		Store:      store,
		Secrets:    secrets,
		OAuth:      oauth,
		Discoverer: bootstrapFakeDiscoverer{},
	}
	owner, org, ownerErr := store.BootstrapOwner(ctx, controlplane.User{Email: "owner@example.test"}, controlplane.Organization{Name: "Acceptance", Slug: "acceptance"}, "owner")
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	actor := controlplane.Actor{
		UserID:         owner.ID,
		OrganizationID: org.ID,
		Role:           "owner",
	}

	connection, err := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	legacyRef, err := secrets.Put(ctx, "postgres-org-uuid", []byte(`{"refresh_token":"legacy","granted_scopes":["https://www.googleapis.com/auth/analytics.readonly"]}`))
	if err != nil {
		t.Fatal(err)
	}
	connection.SecretRef = legacyRef
	if _, updateErr := store.UpdateConnection(ctx, connection); updateErr != nil {
		t.Fatal(updateErr)
	}

	token := controlplane.OAuthToken{
		RefreshToken: "imported-refresh",
		Expiry:       time.Now().Add(-time.Minute),
		Subject:      "subject-1",
		Email:        "gmail@example.test",
		GrantedScopes: []string{
			"https://www.googleapis.com/auth/analytics.readonly",
		},
	}
	if installErr := installBootstrapToken(ctx, time.Second, store, secrets, service, actor, connection, token); installErr != nil {
		t.Fatal(installErr)
	}

	if _, refreshErr := service.Refresh(ctx, actor, connection.ID); refreshErr != nil {
		t.Fatal(refreshErr)
	}

	if _, discoverErr := service.Discover(ctx, actor, connection.ID); discoverErr != nil {
		t.Fatal(discoverErr)
	}
	if oauth.authorizationCalls != 0 {
		t.Fatalf("bootstrap import opened OAuth %d times", oauth.authorizationCalls)
	}

	updated, err := service.GetConnection(ctx, actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := secrets.Get(ctx, org.ID, updated.SecretRef)
	if err != nil {
		t.Fatalf("imported token is not readable under acceptance organization: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("imported token is empty")
	}
}

func TestBootstrapOwnerIdentityIsStableAcrossRepeatedRuns(t *testing.T) {
	ctx := context.Background()
	store := controlplane.NewMemoryStore()
	root := t.TempDir()
	paths := acceptance.Paths{
		Root:       root,
		Config:     root + "/config.json",
		MasterKey:  root + "/master.key",
		Secrets:    root + "/secrets.json",
		OutputRoot: root + "/out",
	}
	if err := acceptance.EnsureProfile(paths, acceptance.Profile{Version: 1, DatabaseURL: "postgres://example", OwnerEmail: "owner@example.test", OrganizationSlug: "acceptance"}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		owner, org, err := store.BootstrapOwner(ctx, controlplane.User{Email: "owner@example.test"}, controlplane.Organization{Name: "Acceptance", Slug: "acceptance"}, "owner")
		if err != nil {
			t.Fatal(err)
		}
		if identityErr := acceptance.SetControlPlaneIdentity(paths, owner.ID, org.ID); identityErr != nil {
			t.Fatal(identityErr)
		}
		loaded, _, err := acceptance.LoadProfile(paths)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.UserID != owner.ID || loaded.OrganizationID != org.ID {
			t.Fatalf("iteration %d identity mismatch: profile=%+v owner=%s org=%s", i, loaded, owner.ID, org.ID)
		}
	}
}
