package controlplane

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	bigqueryapi "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

type bigQueryDiscoveryTransport func(*http.Request) (*http.Response, error)

func (f bigQueryDiscoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHostedBigQueryProjectDiscoveryPaginatesExcludesPlatformAndStartsDisabled(t *testing.T) {
	calls := 0
	d := EngineDiscoverer{BigQueryProjectsFactory: func(ctx context.Context, account string) (*bigqueryapi.Service, error) {
		if account != "owner@example.com" {
			t.Fatalf("wrong account: %s", account)
		}
		transport := bigQueryDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
			calls++

			if r.Method != http.MethodGet || r.URL.Host != "bigquery.googleapis.com" || r.URL.Path != "/bigquery/v2/projects" {
				t.Errorf("unexpected discovery: %s %s", r.Method, r.URL)
			}

			body := `{"projects":[{"projectReference":{"projectId":"gog-marketing-prod"}},{"friendlyName":"Customer project","projectReference":{"projectId":"customer-project"}}],"nextPageToken":"next-page"}`
			if r.URL.Query().Get("pageToken") == "next-page" {
				body = `{"projects":[{"projectReference":{"projectId":"customer-project"}},{"projectReference":{"projectId":"second-project"}},{"id":"malformed-without-reference"}]}`
			}

			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})

		return bigqueryapi.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
	}}

	resources, err := d.Discover(googleapi.WithReadOnly(context.Background(), true), Connection{ID: "connection-a", OrganizationID: "tenant-a", GoogleEmail: "owner@example.com", Services: []string{"bigquery"}}, OAuthToken{AccessToken: "fixture-token"})
	if err != nil || calls != 2 || len(resources) != 2 {
		t.Fatalf("discovery = %#v, calls=%d err=%v", resources, calls, err)
	}

	for i, id := range []string{"customer-project", "second-project"} {
		r := resources[i]
		if r.ResourceID != id || r.ResourceType != "project" || r.Service != "bigquery" || r.Enabled || r.ConnectionID != "connection-a" || r.OrganizationID != "tenant-a" {
			t.Fatalf("unsafe/wrong project grant: %#v", r)
		}
	}
}

func TestLocalBigQueryDiscoveryWithoutConfiguredProjectsDoesNotUseHostedFactory(t *testing.T) {
	resources, err := (EngineDiscoverer{}).Discover(context.Background(), Connection{GoogleEmail: "owner@example.com", Services: []string{"bigquery"}}, OAuthToken{AccessToken: "fixture-token"})
	if err != nil || len(resources) != 0 {
		t.Fatalf("local empty project semantics changed: %#v %v", resources, err)
	}
}
