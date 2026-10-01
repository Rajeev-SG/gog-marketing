package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	errTestDiscoveryService   = errors.New("development discovery service failure")
	errTestGoogleAdsDiscovery = errors.New("development Google Ads failure")
)

type fakeReportingDiscoverer struct {
	report DiscoveryReport
	err    error
}

func (f fakeReportingDiscoverer) Discover(context.Context, Connection, OAuthToken) ([]ResourceGrant, error) {
	return f.report.Resources, f.err
}

func (f fakeReportingDiscoverer) DiscoverReport(context.Context, Connection, OAuthToken) (DiscoveryReport, error) {
	return f.report, f.err
}

func TestDiscoveryKeepsSuccessesAndSelectionsWhenOneServiceFails(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)

	connection, err := service.CreateConnection(context.Background(), actor, "google", []string{"analytics", "tagmanager", "bigquery"})
	if err != nil {
		t.Fatal(err)
	}

	start, err := service.BeginOAuth(context.Background(), actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, callbackErr := service.CompleteOAuth(context.Background(), start.State, "code"); callbackErr != nil {
		t.Fatal(callbackErr)
	}

	for _, initial := range []ResourceGrant{
		{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", Enabled: true},
		{Service: "analytics", ResourceType: "property", ResourceID: "properties/124", Enabled: true},
		{Service: "tagmanager", ResourceType: "container", ResourceID: "containers/1", Enabled: true},
		{Service: "bigquery", ResourceType: "project", ResourceID: "projects/data", Enabled: true},
	} {
		initial.OrganizationID = actor.OrganizationID

		initial.ConnectionID = connection.ID
		if _, grantErr := store.UpsertResourceGrant(context.Background(), initial); grantErr != nil {
			t.Fatal(grantErr)
		}
	}

	service.Discoverer = fakeReportingDiscoverer{report: DiscoveryReport{
		Resources: []ResourceGrant{
			{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", DisplayName: "Kept property"},
			{Service: "analytics", ResourceType: "property", ResourceID: "properties/125", DisplayName: "New property"},
		},
		Statuses: map[string]DiscoveryServiceStatus{
			"analytics":  {State: DiscoveryServiceOK, ResourceCount: 2, CheckedAt: time.Now().UTC()},
			"tagmanager": {State: DiscoveryServiceError, Detail: string(AuthFailureUnknown), CheckedAt: time.Now().UTC(), err: errTestDiscoveryService},
			"bigquery":   {State: DiscoveryServiceUnavailable, Detail: "safe unavailability", CheckedAt: time.Now().UTC()},
		},
	}}

	discovered, err := service.Discover(context.Background(), actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	for _, grant := range discovered {
		want := grant.ResourceID == "properties/123"
		if grant.Enabled != want {
			t.Fatalf("returned grant %s enabled=%v want=%v", grant.ResourceID, grant.Enabled, want)
		}
	}

	grants, err := service.ListResources(context.Background(), actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	for _, grant := range grants {
		want := grant.ResourceID == "properties/123" || grant.ResourceID == "containers/1" || grant.ResourceID == "projects/data"
		if grant.Enabled != want {
			t.Fatalf("partial discovery changed %s to %v", grant.ResourceID, grant.Enabled)
		}
	}

	persisted, err := service.GetConnection(context.Background(), actor, connection.ID)
	if err != nil {
		t.Fatal(err)
	}

	if persisted.DiscoveryStatus["analytics"].State != DiscoveryServiceOK ||
		persisted.DiscoveryStatus["tagmanager"].State != DiscoveryServiceError ||
		persisted.DiscoveryStatus["bigquery"].State != DiscoveryServiceUnavailable {
		t.Fatalf("service statuses were not persisted: %+v", persisted.DiscoveryStatus)
	}

	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	resp := productGet(t, client, server.URL, "/assets/"+connection.ID, cookies) //nolint:bodyclose // readProductBody closes this response

	body := readProductBody(t, resp)
	for _, want := range []string{"Service discovery", "Google Analytics: Ready", "Google Tag Manager: Needs attention", "BigQuery: Unavailable"} {
		if !strings.Contains(body, want) {
			t.Fatalf("assets page missing %q: %s", want, body)
		}
	}
}

func TestEngineDiscoveryReportsServiceFailuresWithoutAbortingOtherServices(t *testing.T) {
	discoverer := EngineDiscoverer{
		GoogleAdsDeveloperToken: "development-token",
		GoogleAdsDiscover: func(context.Context, Connection, OAuthToken) ([]ResourceGrant, error) {
			return nil, errTestGoogleAdsDiscovery
		},
	}
	connection := Connection{ID: "connection", OrganizationID: "org", GoogleEmail: "owner@example.com", Services: []string{"googleads"}}

	report, err := discoverer.DiscoverReport(context.Background(), connection, OAuthToken{})
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Resources) != 0 || report.Statuses["googleads"].State != DiscoveryServiceError {
		t.Fatalf("unexpected report: %+v", report)
	}

	_, discoverErr := discoverer.Discover(context.Background(), connection, OAuthToken{})
	if discoverErr == nil || !errors.Is(discoverErr, errTestGoogleAdsDiscovery) || !strings.HasPrefix(discoverErr.Error(), "control plane: ") {
		t.Fatal("legacy Discover did not retain the wrapped error contract", discoverErr)
	}

	discoverer.GoogleAdsDeveloperToken = ""

	report, err = discoverer.DiscoverReport(context.Background(), connection, OAuthToken{})
	if err != nil || report.Statuses["googleads"].State != DiscoveryServiceUnavailable {
		t.Fatal("unconfigured Google Ads was not represented as unavailable", err)
	}
}

func TestDiscoveryStatusSurvivesConcurrentConnectionUpdate(t *testing.T) {
	store := NewMemoryStore()

	_, org, err := store.BootstrapOwner(context.Background(), User{Email: "concurrent@example.test"}, Organization{Name: "Concurrent", Slug: "concurrent"}, "owner")
	if err != nil {
		t.Fatal(err)
	}

	connection, err := store.CreateConnection(context.Background(), Connection{OrganizationID: org.ID, Name: "google", Services: []string{"analytics"}})
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

		for i := 0; i < 50; i++ {
			if statusErr := store.UpdateDiscoveryStatus(context.Background(), org.ID, connection.ID, statuses); statusErr != nil {
				t.Error(statusErr)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()

		for i := 0; i < 50; i++ {
			update := connection

			update.Name = fmt.Sprintf("google-%d", i)
			if _, updateErr := store.UpdateConnection(context.Background(), update); updateErr != nil {
				t.Error(updateErr)
				return
			}
		}
	}()

	wg.Wait()

	persisted, err := store.GetConnection(context.Background(), org.ID, connection.ID)
	if err != nil || persisted.DiscoveryStatus["analytics"].State != DiscoveryServiceError {
		t.Fatalf("concurrent connection update clobbered discovery status: %+v, %v", persisted.DiscoveryStatus, err)
	}
}

func TestDiscoveryStatusSurfaceOmitsRemovedAndRejectsUnknownServices(t *testing.T) {
	discoverer := EngineDiscoverer{}

	report, err := discoverer.DiscoverReport(context.Background(), Connection{
		ID: "connection", OrganizationID: "org", GoogleEmail: "owner@example.com",
		Services: []string{"analytics", "typo-service"},
	}, OAuthToken{})
	if err != nil {
		t.Fatal(err)
	}

	if report.Statuses["typo-service"].State != DiscoveryServiceUnsupported {
		t.Fatalf("unknown service was not marked unsupported: %+v", report.Statuses)
	}

	if len(report.Resources) != 0 {
		t.Fatal("unknown service unexpectedly returned resources")
	}

	surface := productDiscoveryStatuses(Connection{
		Services: []string{"analytics", "typo-service"},
		DiscoveryStatus: map[string]DiscoveryServiceStatus{
			"analytics":    {State: DiscoveryServiceOK, ResourceCount: 1},
			"typo-service": {State: DiscoveryServiceError},
			"bigquery":     {State: DiscoveryServiceUnavailable},
		},
	})
	if len(surface) != 2 {
		t.Fatalf("removed service was still surfaced: %+v", surface)
	}

	for _, status := range surface {
		if status.ServiceName == "BigQuery" {
			t.Fatal("removed service was rendered")
		}

		if status.ServiceName == "Google service" && status.Detail == "Ready" {
			t.Fatal("unknown service was rendered as ready")
		}
	}
}

func TestLegacyDiscoveryErrorUsesConfiguredServiceOrder(t *testing.T) {
	discoverer := EngineDiscoverer{
		GoogleAdsDeveloperToken: "development-token",
		GoogleAdsDiscover: func(context.Context, Connection, OAuthToken) ([]ResourceGrant, error) {
			return nil, errTestGoogleAdsDiscovery
		},
	}

	connection := Connection{ID: "connection", OrganizationID: "org", GoogleEmail: "owner@example.com", Services: []string{"googleads", "typo-service"}}
	for i := 0; i < 25; i++ {
		_, err := discoverer.Discover(context.Background(), connection, OAuthToken{})
		if !errors.Is(err, errTestGoogleAdsDiscovery) {
			t.Fatalf("legacy error was nondeterministic at iteration %d: %v", i, err)
		}
	}
}
