package controlplane

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProductReadDeniesBeforeCredentialsOrGoogle(t *testing.T) {
	for _, tc := range []struct {
		name, resource, organization string
		enabled                      bool
		status                       ConnectionStatus
	}{
		{"disabled", "properties/123", "org-1", false, ConnectionHealthy},
		{"unknown", "properties/999", "org-1", true, ConnectionHealthy},
		{"foreign organization", "properties/123", "foreign-org", true, ConnectionHealthy},
		{"disconnected", "properties/123", "org-1", true, ConnectionDisconnected},
		{"reconnect", "properties/123", "org-1", true, ConnectionNeedsReconnect},
		{"parent is not property", "accounts/123", "org-1", true, ConnectionHealthy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewMemoryStore()

			connection, err := store.CreateConnection(context.Background(), Connection{OrganizationID: "org-1", Name: "google", Services: []string{"analytics"}, Status: tc.status})
			if err != nil {
				t.Fatal(err)
			}

			if _, grantErr := store.UpsertResourceGrant(context.Background(), ResourceGrant{OrganizationID: "org-1", ConnectionID: connection.ID, Service: "analytics", ResourceType: "property", ResourceID: "properties/123", Enabled: tc.enabled}); grantErr != nil {
				t.Fatal(grantErr)
			}
			// Nil credentials/provider make any accidental credential or refresh call fail.
			service := &Service{Store: store}

			actor := Actor{UserID: "user-1", OrganizationID: tc.organization, Role: "owner"}
			if _, readErr := service.ReadAnalyticsProperty(context.Background(), actor, connection.ID, tc.resource); !errors.Is(readErr, ErrForbidden) && !errors.Is(readErr, ErrInvalid) {
				t.Fatalf("read was not denied: %v", readErr)
			}

			audit, err := store.ListAudit(context.Background(), tc.organization, 10)
			if err != nil || len(audit) != 1 || audit[0].Result != "deny" || !strings.Contains(audit[0].Detail, "google_api_requests=0") {
				t.Fatal("missing deny audit", err)
			}
		})
	}
}

func TestProductReadAPIRequiresProductSession(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	server, client := newProductTestHandler(t, service, actor)
	resp := productGet(t, client, server.URL, "/api/connections/unknown/analytics/property?resource=properties/123", nil)

	body := readProductBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized || body != "{\"error\":\"sign_in_required\"}\n" {
		t.Fatalf("unexpected unauthenticated response: %d %s", resp.StatusCode, body)
	}
	cookies := productSessionCookies(t, client, server)
	resp = productGet(t, client, server.URL, "/api/connections/unknown/analytics/property?resource=properties/123", cookies)

	body = readProductBody(t, resp)
	if resp.StatusCode != http.StatusForbidden || body != "{\"error\":\"access_denied\"}\n" {
		t.Fatalf("unknown connection accepted: %d %s", resp.StatusCode, body)
	}
}

func TestProductSessionCookieAllowsGoogleRedirectWithoutChangingAdmin(t *testing.T) {
	sessions, err := NewSessionManager([]byte("0123456789abcdef0123456789abcdef"), time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	product := sessions.ProductCookie("signed-product-session")

	admin := sessions.Cookie("signed-admin-session")
	if product.SameSite != http.SameSiteLaxMode || admin.SameSite != http.SameSiteStrictMode || product.Name == admin.Name || !product.Secure || !product.HttpOnly {
		t.Fatal("cookie isolation or redirect safety regressed")
	}
}

type unavailableGrantStore struct{ *MemoryStore }

func (s unavailableGrantStore) GetResourceGrant(context.Context, string, string, string) (ResourceGrant, error) {
	return ResourceGrant{}, ErrReadUnavailable
}

func TestProductReadStorageFailureIsNotPermissionDenial(t *testing.T) {
	store := NewMemoryStore()

	connection, err := store.CreateConnection(context.Background(), Connection{OrganizationID: "org-1", Name: "google", Services: []string{"analytics"}, Status: ConnectionHealthy})
	if err != nil {
		t.Fatal(err)
	}

	service := &Service{Store: unavailableGrantStore{MemoryStore: store}}
	if _, readErr := service.ReadAnalyticsProperty(context.Background(), Actor{UserID: "user-1", OrganizationID: "org-1", Role: "owner"}, connection.ID, "properties/123"); !errors.Is(readErr, ErrReadUnavailable) {
		t.Fatalf("storage outage was misreported: %v", readErr)
	}
}
