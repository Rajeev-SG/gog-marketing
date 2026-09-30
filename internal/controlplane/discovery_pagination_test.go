package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	"google.golang.org/api/option"
	"google.golang.org/api/tagmanager/v2"
)

// These mocked pages are regression checks, not live-account acceptance.
func TestAnalyticsDiscoveryPagesDeduplicatesAndRejectsPartialFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "all pages", true: "later page fails"}[fail], func(t *testing.T) {
			var requests atomic.Int64

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")

				if r.URL.Query().Get("pageToken") == "next" {
					if fail {
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"error":{"code":403,"message":"development permission failure"}}`))

						return
					}
					_, _ = w.Write([]byte(`{"accountSummaries":[{"account":"accounts/1","displayName":"One","propertySummaries":[{"property":"properties/1","displayName":"First"}]},{"account":"accounts/2","displayName":"Two","propertySummaries":[{"property":"properties/2","displayName":"Second"}]}]}`))

					return
				}
				_, _ = w.Write([]byte(`{"nextPageToken":"next","accountSummaries":[{"account":"accounts/1","displayName":"One","propertySummaries":[{"property":"properties/1","displayName":"First"}]}]}`))
			}))
			defer server.Close()

			client, err := analyticsadmin.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
			if err != nil {
				t.Fatal(err)
			}
			resources, err := discoverAnalyticsPages(context.Background(), client, Connection{ID: "connection", OrganizationID: "org"}, OAuthToken{})

			if requests.Load() != 2 {
				t.Fatalf("page requests = %d", requests.Load())
			}

			if fail {
				if err == nil || resources != nil {
					t.Fatal("later failure returned a truncated successful inventory")
				}

				return
			}

			if err != nil || len(resources) != 4 {
				t.Fatalf("complete deduplicated inventory = %d, %v", len(resources), err)
			}
		})
	}
}

func TestTagManagerDiscoveryPaginatesAccountsAndContainers(t *testing.T) {
	var accounts, containers atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/accounts") {
			accounts.Add(1)

			if r.URL.Query().Get("pageToken") == "accounts-next" {
				_, _ = w.Write([]byte(`{"account":[{"path":"accounts/1","name":"One"},{"path":"accounts/2","name":"Two"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"nextPageToken":"accounts-next","account":[{"path":"accounts/1","name":"One"}]}`))

			return
		}

		containers.Add(1)

		if strings.Contains(r.URL.Path, "accounts/2/") {
			_, _ = w.Write([]byte(`{"container":[{"path":"accounts/2/containers/3","name":"Third"}]}`))
			return
		}

		if r.URL.Query().Get("pageToken") == "containers-next" {
			_, _ = w.Write([]byte(`{"container":[{"path":"accounts/1/containers/1","name":"First"},{"path":"accounts/1/containers/2","name":"Second"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"nextPageToken":"containers-next","container":[{"path":"accounts/1/containers/1","name":"First"}]}`))
	}))
	defer server.Close()

	client, err := tagmanager.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}

	resources, err := discoverTagManagerPages(context.Background(), client, Connection{ID: "connection", OrganizationID: "org"}, OAuthToken{})
	if err != nil || len(resources) != 5 || accounts.Load() != 2 || containers.Load() != 3 {
		t.Fatalf("inventory=%d accounts=%d containers=%d error=%v", len(resources), accounts.Load(), containers.Load(), err)
	}
}

func TestTagManagerLaterContainerFailureDoesNotReturnPartialInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/accounts") {
			_, _ = w.Write([]byte(`{"account":[{"path":"accounts/1","name":"One"}]}`))
			return
		}

		if r.URL.Query().Get("pageToken") == "next" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"message":"development failure"}}`))

			return
		}
		_, _ = w.Write([]byte(`{"nextPageToken":"next","container":[{"path":"accounts/1/containers/1","name":"First"}]}`))
	}))
	defer server.Close()

	client, err := tagmanager.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}

	resources, err := discoverTagManagerPages(context.Background(), client, Connection{ID: "connection", OrganizationID: "org"}, OAuthToken{})
	if err == nil || resources != nil {
		t.Fatal("later container failure produced partial successful inventory")
	}
}
