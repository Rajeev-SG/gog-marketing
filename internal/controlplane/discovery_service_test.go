package controlplane

import (
	"context"
	"errors"
	"strings"
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

	if _, discoverErr := discoverer.Discover(context.Background(), connection, OAuthToken{}); discoverErr == nil {
		t.Fatal("legacy Discover should retain fatal single-service behavior")
	}

	discoverer.GoogleAdsDeveloperToken = ""

	report, err = discoverer.DiscoverReport(context.Background(), connection, OAuthToken{})
	if err != nil || report.Statuses["googleads"].State != DiscoveryServiceUnavailable {
		t.Fatal("unconfigured Google Ads was not represented as unavailable", err)
	}
}
