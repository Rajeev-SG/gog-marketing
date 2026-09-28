package controlplane

import (
	"context"
	"testing"
)

func TestEngineDiscoverySkipsUnavailableGoogleAds(t *testing.T) {
	discoverer := EngineDiscoverer{}

	resources, err := discoverer.Discover(context.Background(), Connection{
		ID: "connection", OrganizationID: "org", GoogleEmail: "owner@example.com",
		Services: []string{"googleads"},
	}, OAuthToken{AccessToken: "synthetic-access"})
	if err != nil {
		t.Fatalf("discovery should skip unavailable Google Ads configuration: %v", err)
	}

	if len(resources) != 0 {
		t.Fatalf("unexpected resources: %+v", resources)
	}
}

func TestEngineDiscoveryTreatsConfiguredGoogleAdsFailureAsFatal(t *testing.T) {
	discoverer := EngineDiscoverer{
		GoogleAdsDeveloperToken: "configured-developer-token",
		GoogleAdsDiscover: func(context.Context, Connection, OAuthToken) ([]ResourceGrant, error) {
			return nil, ErrGoogleAdsUnavailable
		},
	}

	if _, err := discoverer.Discover(context.Background(), Connection{
		ID: "connection", OrganizationID: "org", GoogleEmail: "owner@example.com",
		Services: []string{"googleads"},
	}, OAuthToken{AccessToken: "synthetic-access"}); err == nil {
		t.Fatal("configured Google Ads discovery failure was ignored")
	}
}
