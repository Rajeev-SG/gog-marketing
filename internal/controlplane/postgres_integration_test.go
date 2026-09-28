package controlplane

import (
	"context"
	"os"
	"path/filepath"
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

	if rollbackErr := store.RollBackMigration001(ctx); rollbackErr != nil {
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

	start, startErr := service.BeginOAuth(ctx, actor, connection.ID)
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

	if rollbackErr := reopened.RollBackMigration001(ctx); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}

	if remigrateErr := reopened.Migrate(ctx); remigrateErr != nil {
		t.Fatal(remigrateErr)
	}

	if _, listErr := reopened.ListConnections(ctx, actor.OrganizationID); listErr != nil {
		t.Fatal(listErr)
	}
}
