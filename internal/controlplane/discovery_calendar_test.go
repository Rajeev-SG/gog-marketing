package controlplane

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

var errUnexpectedCalendarProvider = errors.New("unexpected provider call")

type calendarDiscoveryTransport func(*http.Request) (*http.Response, error)

func (f calendarDiscoveryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCalendarDiscoveryPaginationAndNoPartialInventoryOnFailure(t *testing.T) {
	for _, failure := range []string{"", "page-error", "loop"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			discoverer := EngineDiscoverer{CalendarFactory: func(ctx context.Context, _ string) (*calendar.Service, error) {
				transport := calendarDiscoveryTransport(func(r *http.Request) (*http.Response, error) {
					calls++

					if r.URL.Host != "www.googleapis.com" || r.URL.Path != "/calendar/v3/users/me/calendarList" || r.Method != http.MethodGet {
						t.Errorf("unexpected Calendar request: %s", r.URL)
					}
					body := `{"items":[{"id":"owner+beta@example.test","summary":"Primary"}],"nextPageToken":"next"}`

					if r.URL.Query().Get("pageToken") == "next" {
						if failure == "page-error" {
							return &http.Response{StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"synthetic forbidden"}}`)), Request: r}, nil
						}

						body = `{"items":[{"id":"owner+beta@example.test","summary":"Duplicate"},{"id":"en.uk#holiday@group.v.calendar.google.com","summary":"Holidays"},{"id":"deleted@example.test","deleted":true}],"nextPageToken":""}`
						if failure == "loop" {
							body = `{"items":[],"nextPageToken":"next"}`
						}
					}

					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})

				return calendar.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
			}}

			report, err := discoverer.DiscoverReport(t.Context(), Connection{ID: "connection", OrganizationID: "tenant", GoogleEmail: "owner@example.test", Services: []string{"calendar", "unknown"}}, OAuthToken{AccessToken: "synthetic"})
			if err != nil {
				t.Fatal(err)
			}

			if calls != 2 {
				t.Fatalf("unexpected page count: %d", calls)
			}

			if report.Statuses["unknown"].State != DiscoveryServiceUnsupported {
				t.Fatal("unrelated service failure was hidden")
			}

			if failure != "" {
				if len(report.Resources) != 0 || report.Statuses["calendar"].State != DiscoveryServiceError {
					t.Fatalf("partial inventory was published: %+v", report)
				}

				return
			}

			if len(report.Resources) != 2 || report.Statuses["calendar"].ResourceCount != 2 {
				t.Fatalf("bad inventory: %+v", report)
			}

			for _, resource := range report.Resources {
				if resource.Enabled || resource.Service != "calendar" || resource.ResourceType != "calendar" || resource.ConnectionID != "connection" || resource.OrganizationID != "tenant" {
					t.Fatalf("bad grant: %+v", resource)
				}
			}
		})
	}
}

func TestCalendarDiscoveryRejectsUnexpectedRequestsBeforeProvider(t *testing.T) {
	discoverer := EngineDiscoverer{CalendarFactory: func(ctx context.Context, _ string) (*calendar.Service, error) {
		client := &http.Client{Transport: googleapi.GuardedTransport(ctx, calendarDiscoveryTransport(func(*http.Request) (*http.Response, error) {
			t.Error("foreign endpoint reached provider")
			return nil, errUnexpectedCalendarProvider
		}))}

		return calendar.NewService(ctx, option.WithHTTPClient(client), option.WithEndpoint("https://foreign.example.test/"))
	}}

	report, err := discoverer.DiscoverReport(t.Context(), Connection{GoogleEmail: "owner@example.test", Services: []string{"calendar"}}, OAuthToken{AccessToken: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}

	if report.Statuses["calendar"].State != DiscoveryServiceError || len(report.Resources) != 0 {
		t.Fatalf("foreign discovery wasn't denied: %+v", report)
	}
}
