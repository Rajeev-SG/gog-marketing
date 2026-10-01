package controlplane

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPostgresStoreMigrationsAndRestart(t *testing.T) {
	databaseURL := os.Getenv("CONTROL_PLANE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTROL_PLANE_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()

	store, storeErr := OpenPostgresStore(ctx, databaseURL)
	if storeErr != nil {
		t.Fatal(storeErr)
	}

	if rollbackErr := store.RollBackMigrations(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}

	if migrateErr := store.Migrate(ctx); migrateErr != nil {
		t.Fatal(migrateErr)
	}

	owner, organization, bootstrapErr := store.BootstrapOwner(ctx, User{
		Email: "postgres-owner@example.com", DisplayName: "Owner",
	}, Organization{Name: "Postgres", Slug: "postgres"}, "owner")
	if bootstrapErr != nil {
		t.Fatal(bootstrapErr)
	}
	actor := Actor{UserID: owner.ID, OrganizationID: organization.ID, Role: "owner"}

	secrets, secretErr := NewFileSecretStore(filepath.Join(t.TempDir(), "secrets.json"), []byte("0123456789abcdef0123456789abcdef"))
	if secretErr != nil {
		t.Fatal(secretErr)
	}

	service := &Service{
		Store: store, Secrets: secrets,
		OAuth: &fakeOAuth{token: OAuthToken{
			AccessToken: "postgres-access-secret", RefreshToken: "postgres-refresh-secret",
			Expiry: time.Now().Add(time.Hour), Subject: "postgres-subject", Email: "postgres-owner@example.com",
		}},
		RedirectURI: "http://example.test/oauth/google/callback",
	}

	connection, createErr := service.CreateConnection(ctx, actor, "gmail", []string{"analytics"})
	if createErr != nil {
		t.Fatal(createErr)
	}

	start, startErr := service.BeginOAuth(ctx, actor, connection.ID, false)
	if startErr != nil {
		t.Fatal(startErr)
	}

	if _, completeErr := service.CompleteOAuth(ctx, start.State, "code"); completeErr != nil {
		t.Fatal(completeErr)
	}

	if closeErr := store.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	reopened, reopenErr := OpenPostgresStore(ctx, databaseURL)
	if reopenErr != nil {
		t.Fatal(reopenErr)
	}
	defer func() { _ = reopened.Close() }()

	rebootedOwner, rebootedOrganization, rebootErr := reopened.BootstrapOwner(ctx, User{
		Email: "postgres-owner@example.com", DisplayName: "Owner",
	}, Organization{Name: "Postgres", Slug: "postgres"}, "owner")
	if rebootErr != nil {
		t.Fatal(rebootErr)
	}

	if rebootedOwner.ID != owner.ID || rebootedOrganization.ID != organization.ID {
		t.Fatalf("bootstrap IDs changed across restart: user %q -> %q, org %q -> %q", owner.ID, rebootedOwner.ID, organization.ID, rebootedOrganization.ID)
	}

	rebootedActor := Actor{UserID: rebootedOwner.ID, OrganizationID: rebootedOrganization.ID, Role: "owner"}

	connections, listErr := reopened.ListConnections(ctx, rebootedActor.OrganizationID)
	if listErr != nil {
		t.Fatal(listErr)
	}

	if len(connections) != 1 || connections[0].ID != connection.ID {
		t.Fatalf("connections did not survive bootstrap restart: %+v", connections)
	}

	persisted, getErr := reopened.GetConnection(ctx, rebootedActor.OrganizationID, connection.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}

	if persisted.Status != ConnectionHealthy || persisted.GoogleEmail != "postgres-owner@example.com" || persisted.SecretRef == "" {
		t.Fatalf("connection did not persist across reopen: %+v", persisted)
	}

	if rollbackErr := reopened.RollBackMigrations(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}

	if remigrateErr := reopened.Migrate(ctx); remigrateErr != nil {
		t.Fatal(remigrateErr)
	}

	if _, listErr := reopened.ListConnections(ctx, actor.OrganizationID); listErr != nil {
		t.Fatal(listErr)
	}
}

func TestPostgresRediscoveryPreservesSelections(t *testing.T) {
	databaseURL := os.Getenv("CONTROL_PLANE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated CONTROL_PLANE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()

	store, err := OpenPostgresStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = store.Close() }()

	_, org, err := store.BootstrapOwner(ctx, User{Email: "rediscovery@example.test", DisplayName: "Development regression"}, Organization{Name: "Rediscovery regression", Slug: "rediscovery-regression"}, "owner")
	if err != nil {
		t.Fatal(err)
	}

	connection, err := store.CreateConnection(ctx, Connection{OrganizationID: org.ID, Name: fmt.Sprintf("google-%d", time.Now().UnixNano()), Services: []string{"analytics"}})
	if err != nil {
		t.Fatal(err)
	}

	for _, initial := range []bool{false, true} {
		resourceID := map[bool]string{false: "properties/disabled", true: "properties/enabled"}[initial]

		grant := ResourceGrant{OrganizationID: org.ID, ConnectionID: connection.ID, Service: "analytics", ResourceType: "property", ResourceID: resourceID, DisplayName: "Old name", Enabled: initial}
		if _, grantErr := store.UpsertResourceGrant(ctx, grant); grantErr != nil {
			t.Fatal(grantErr)
		}
		grant.Enabled = !initial
		grant.DisplayName = "New name"

		saved, grantErr := store.UpsertResourceGrant(ctx, grant)
		if grantErr != nil || saved.Enabled != initial || saved.DisplayName != "New name" {
			t.Fatalf("rediscovery did not preserve choice/update metadata: %v", grantErr)
		}
	}
}

func TestPostgresSetResourceEnabledPersistsAcrossReopen(t *testing.T) {
	databaseURL := os.Getenv("CONTROL_PLANE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated CONTROL_PLANE_TEST_DATABASE_URL not set")
	}

	ctx := context.Background()

	store, err := OpenPostgresStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	_, org, err := store.BootstrapOwner(ctx, User{
		Email: "selection-" + suffix + "@example.test", DisplayName: "Development selection regression",
	}, Organization{Name: "Selection regression", Slug: "selection-" + suffix}, "owner")
	if err != nil {
		t.Fatal(err)
	}

	connection, err := store.CreateConnection(ctx, Connection{
		OrganizationID: org.ID, Name: "google-" + suffix, Services: []string{"analytics"},
	})
	if err != nil {
		t.Fatal(err)
	}

	resourceID := "properties/selection-" + suffix
	if _, grantErr := store.UpsertResourceGrant(ctx, ResourceGrant{
		OrganizationID: org.ID, ConnectionID: connection.ID,
		Service: "analytics", ResourceType: "property", ResourceID: resourceID,
		DisplayName: "Development selection", Enabled: false,
	}); grantErr != nil {
		t.Fatal(grantErr)
	}

	if _, toggleErr := store.SetResourceEnabled(ctx, org.ID, connection.ID, resourceID, true); toggleErr != nil {
		t.Fatal(toggleErr)
	}

	if closeErr := store.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	reopened, reopenErr := OpenPostgresStore(ctx, databaseURL)
	if reopenErr != nil {
		t.Fatal(reopenErr)
	}
	defer func() { _ = reopened.Close() }()

	saved, getErr := reopened.GetResourceGrant(ctx, org.ID, connection.ID, resourceID)
	if getErr != nil || !saved.Enabled {
		t.Fatalf("Postgres selection did not survive reopen: %+v, %v", saved, getErr)
	}
}

func TestPostgresDiscoveryStatusPersistsAcrossReopen(t *testing.T) {
	databaseURL := os.Getenv("CONTROL_PLANE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated CONTROL_PLANE_TEST_DATABASE_URL not set")
	}

	ctx := context.Background()

	store, err := OpenPostgresStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	_, org, err := store.BootstrapOwner(ctx, User{
		Email: "discovery-status-" + suffix + "@example.test", DisplayName: "Development status regression",
	}, Organization{Name: "Discovery status regression", Slug: "discovery-status-" + suffix}, "owner")
	if err != nil {
		t.Fatal(err)
	}

	connection, err := store.CreateConnection(ctx, Connection{
		OrganizationID: org.ID, Name: "google-" + suffix, Services: []string{"analytics", "tagmanager"}, ProductManaged: true,
		DiscoveryStatus: map[string]DiscoveryServiceStatus{
			"analytics":  {State: DiscoveryServiceOK, ResourceCount: 2, CheckedAt: time.Now().UTC()},
			"tagmanager": {State: DiscoveryServiceUnavailable, Detail: "google_ads_unconfigured", CheckedAt: time.Now().UTC()},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	connection.Name = "renamed-" + suffix
	if _, updateErr := store.UpdateConnection(ctx, connection); updateErr != nil {
		t.Fatal(updateErr)
	}

	if closeErr := store.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	reopened, reopenErr := OpenPostgresStore(ctx, databaseURL)
	if reopenErr != nil {
		t.Fatal(reopenErr)
	}
	defer func() { _ = reopened.Close() }()

	saved, getErr := reopened.GetConnection(ctx, org.ID, connection.ID)
	if getErr != nil || !saved.ProductManaged || saved.DiscoveryStatus["analytics"].ResourceCount != 2 || saved.DiscoveryStatus["tagmanager"].State != DiscoveryServiceUnavailable {
		t.Fatalf("discovery statuses did not survive reopen: %+v, %v", saved.DiscoveryStatus, getErr)
	}
}

func TestPostgresDiscoveryStatusConcurrentConnectionUpdates(t *testing.T) {
	databaseURL := os.Getenv("CONTROL_PLANE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated CONTROL_PLANE_TEST_DATABASE_URL not set")
	}

	ctx := context.Background()

	store, err := OpenPostgresStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = store.Close() }()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	_, org, err := store.BootstrapOwner(ctx, User{
		Email: "status-concurrency-" + suffix + "@example.test", DisplayName: "Development concurrency regression",
	}, Organization{Name: "Status concurrency regression", Slug: "status-concurrency-" + suffix}, "owner")
	if err != nil {
		t.Fatal(err)
	}

	connection, err := store.CreateConnection(ctx, Connection{
		OrganizationID: org.ID, Name: "google-" + suffix, Services: []string{"analytics"},
	})
	if err != nil {
		t.Fatal(err)
	}

	statuses := map[string]DiscoveryServiceStatus{
		"analytics": {State: DiscoveryServiceError, Detail: string(AuthFailureUnknown), CheckedAt: time.Now().UTC()},
	}
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()

		for i := 0; i < 25; i++ {
			if statusErr := store.UpdateDiscoveryStatus(ctx, org.ID, connection.ID, statuses); statusErr != nil {
				t.Error(statusErr)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()

		for i := 0; i < 25; i++ {
			update := connection

			update.Name = fmt.Sprintf("google-%d-%s", i, suffix)
			if _, updateErr := store.UpdateConnection(ctx, update); updateErr != nil {
				t.Error(updateErr)
				return
			}
		}
	}()

	wg.Wait()

	saved, err := store.GetConnection(ctx, org.ID, connection.ID)
	if err != nil || saved.DiscoveryStatus["analytics"].State != DiscoveryServiceError {
		t.Fatalf("Postgres connection update clobbered discovery status: %+v, %v", saved.DiscoveryStatus, err)
	}
}
