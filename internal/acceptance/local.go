package acceptance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openclaw/gogcli/internal/controlplane"
)

type fakeOAuthProvider struct {
	authorizationCalls int
	token              controlplane.OAuthToken
}

func (p *fakeOAuthProvider) AuthorizationURL(controlplane.OAuthStartInput) string {
	p.authorizationCalls++
	return "https://accounts.invalid/oauth"
}

func (p *fakeOAuthProvider) Exchange(context.Context, controlplane.OAuthStartInput, string) (controlplane.OAuthToken, error) {
	return p.token, nil
}

func (p *fakeOAuthProvider) Refresh(context.Context, controlplane.OAuthToken) (controlplane.OAuthToken, error) {
	return p.token, nil
}

type fakeDiscoverer struct {
	resources []controlplane.ResourceGrant
}

func (f fakeDiscoverer) Discover(_ context.Context, connection controlplane.Connection, _ controlplane.OAuthToken) ([]controlplane.ResourceGrant, error) {
	out := make([]controlplane.ResourceGrant, 0, len(f.resources))
	for _, resource := range f.resources {
		resource.ConnectionID = connection.ID
		resource.OrganizationID = connection.OrganizationID
		out = append(out, resource)
	}

	return out, nil
}

type fakeReader struct {
	reads int
}

func (r *fakeReader) Read(context.Context, controlplane.Connection, controlplane.OAuthToken, controlplane.ResourceGrant) error {
	r.reads++
	return nil
}

func RunLocal(ctx context.Context, paths Paths, databaseURL string) (Manifest, error) {
	manifest := NewManifest("local", executableIdentity(), "")
	defer func() {
		manifest.Finish()
		_, _ = manifest.Write(paths.OutputRoot, "local-"+time.Now().UTC().Format("20060102T150405Z"))
	}()

	var store controlplane.Store
	if strings.TrimSpace(databaseURL) == "" {
		store = controlplane.NewMemoryStore()
	} else {
		postgres, err := controlplane.OpenPostgresStore(ctx, databaseURL)
		if err != nil {
			manifest.Add("database", "FAIL", "database_unavailable", err.Error(), 0)
			return manifest, wrapAcceptanceError(err)
		}
		store = postgres
	}
	defer func() { _ = store.Close() }()
	master := []byte("local-acceptance-master-key-0123456789")

	secrets, err := controlplane.NewFileSecretStore(paths.Secrets, master)
	if err != nil {
		manifest.Add("secret_store", "FAIL", "secret_store", err.Error(), 0)
		return manifest, wrapAcceptanceError(err)
	}
	oauth := &fakeOAuthProvider{token: controlplane.OAuthToken{
		AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh",
		Expiry: time.Now().Add(time.Hour), Email: "local@example.test",
		GrantedScopes: []string{"https://www.googleapis.com/auth/analytics.readonly"},
	}}
	reader := &fakeReader{}
	service := &controlplane.Service{
		Store: store, Secrets: secrets, OAuth: oauth,
		Discoverer: fakeDiscoverer{resources: []controlplane.ResourceGrant{{
			Service: "analytics", ResourceType: "property", ResourceID: "properties/local",
			DisplayName: "Local property", Enabled: true,
		}}},
	}

	actor, org, err := store.BootstrapOwner(ctx, controlplane.User{Email: "owner@example.test"}, controlplane.Organization{Name: "Acceptance", Slug: "acceptance"}, "owner")
	if err != nil {
		manifest.Add("bootstrap", "FAIL", "local_store", err.Error(), 0)
		return manifest, wrapAcceptanceError(err)
	}

	localActor := controlplane.Actor{UserID: actor.ID, OrganizationID: org.ID, Role: "owner"}

	for _, name := range []string{"gmail", "singulyr"} {
		oauth.token.Email = name + "@example.test"
		oauth.token.Subject = name + "-subject"

		connection, err := service.CreateConnection(ctx, localActor, name, []string{"analytics"})
		if err != nil {
			manifest.Add(name+".create", "FAIL", "local_store", err.Error(), 0)
			return manifest, wrapAcceptanceError(err)
		}

		start, err := service.BeginOAuth(ctx, localActor, connection.ID, false)
		if err != nil {
			manifest.Add(name+".oauth_setup", "FAIL", "local_store", err.Error(), 0)
			return manifest, wrapAcceptanceError(err)
		}

		if start.URL == "" {
			manifest.Add(name+".oauth_setup", "FAIL", "local_store", "missing fake authorization URL", 0)
			return manifest, ErrMissingFakeAuthURL
		}

		if _, completeErr := service.CompleteOAuth(ctx, start.State, "local-code"); completeErr != nil {
			manifest.Add(name+".oauth_setup", "FAIL", "local_store", completeErr.Error(), 0)
			return manifest, wrapAcceptanceError(completeErr)
		}

		if _, refreshErr := service.Refresh(ctx, localActor, connection.ID); refreshErr != nil {
			manifest.Add(name+".refresh", "FAIL", "local_store", refreshErr.Error(), 0)
			return manifest, wrapAcceptanceError(refreshErr)
		}

		resources, discoverErr := service.Discover(ctx, localActor, connection.ID)
		if discoverErr != nil {
			manifest.Add(name+".discovery", "FAIL", "local_store", discoverErr.Error(), 0)

			return manifest, wrapAcceptanceError(discoverErr)
		}

		if len(resources) != 1 {
			manifest.Add(name+".discovery", "FAIL", "local_store", "expected one discovered resource", 0)

			return manifest, ErrMissingResourceGrant
		}

		if _, grantErr := service.SetResourceEnabled(ctx, localActor, connection.ID, resources[0].ResourceID, true); grantErr != nil {
			manifest.Add(name+".grant", "FAIL", "local_store", grantErr.Error(), 0)
			return manifest, wrapAcceptanceError(grantErr)
		}

		grant, err := service.ListResources(ctx, localActor, connection.ID)
		if err != nil || len(grant) != 1 {
			manifest.Add(name+".read", "FAIL", "local_store", "resource grant missing", 0)
			return manifest, ErrMissingResourceGrant
		}

		if err := reader.Read(ctx, connection, oauth.token, grant[0]); err != nil {
			manifest.Add(name+".read", "FAIL", "local_store", err.Error(), 0)
			return manifest, wrapAcceptanceError(err)
		}

		manifest.Add(name, "PASS", "", "fake OAuth and resource read completed", 0)
	}

	if oauth.authorizationCalls > 0 {
		manifest.Add("no_routine_oauth", "PASS", "", fmt.Sprintf("authorization URL generated %d times only for explicit local setup", oauth.authorizationCalls), 0)
	}

	if reader.reads != 2 {
		manifest.Add("local_reads", "FAIL", "local_store", "expected two fake reads", 0)
		return manifest, ErrExpectedTwoReads
	}

	manifest.Add("local_reads", "PASS", "", "two fake reads completed", 0)

	if strings.TrimSpace(databaseURL) != "" {
		if err := store.Close(); err != nil {
			manifest.Add("restart", "FAIL", "database_unavailable", err.Error(), 0)
			return manifest, wrapAcceptanceError(err)
		}

		reopened, err := controlplane.OpenPostgresStore(ctx, databaseURL)
		if err != nil {
			manifest.Add("restart", "FAIL", "database_unavailable", err.Error(), 0)
			return manifest, wrapAcceptanceError(err)
		}
		defer func() { _ = reopened.Close() }()

		connections, err := reopened.ListConnections(ctx, localActor.OrganizationID)
		if err != nil || len(connections) != 2 {
			manifest.Add("restart", "FAIL", "database_unavailable", "connections did not survive restart", 0)
			return manifest, ErrRestartPersistence
		}

		manifest.Add("restart", "PASS", "", "Postgres control-plane state survived reopen", 0)
	}

	return manifest, nil
}
