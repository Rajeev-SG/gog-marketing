package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/googleauth"
)

type httpRefreshOAuth struct {
	OAuthProvider
	tokenURL string
}

func (o httpRefreshOAuth) Refresh(ctx context.Context, token OAuthToken) (OAuthToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.tokenURL, nil)
	if err != nil {
		return OAuthToken{}, fmt.Errorf("mock refresh HTTP: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return OAuthToken{}, fmt.Errorf("mock refresh HTTP: %w", err)
	}

	resp.Body.Close()

	refreshed, refreshErr := o.OAuthProvider.Refresh(ctx, token)
	if refreshErr != nil {
		return OAuthToken{}, fmt.Errorf("mock refresh token: %w", refreshErr)
	}

	return refreshed, nil
}

// Mock transport coverage is development evidence only, never live acceptance.
func TestProductReadSuccessUsesCentralTokenAndCountsOnlyAPI(t *testing.T) {
	for _, refresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh token", true: "refresh excluded"}[refresh], func(t *testing.T) {
			service, store := productTestService(t)
			actor := ownerActor(t, store)

			connection, err := service.CreateConnection(context.Background(), actor, productConnectionName, []string{"analytics"})
			if err != nil {
				t.Fatal(err)
			}

			start, err := service.BeginOAuth(context.Background(), actor, connection.ID, false)
			if err != nil {
				t.Fatal(err)
			}

			connection, err = service.CompleteOAuth(context.Background(), start.State, "code")
			if err != nil {
				t.Fatal(err)
			}

			if _, discoverErr := service.Discover(context.Background(), actor, connection.ID); discoverErr != nil {
				t.Fatal(discoverErr)
			}

			if _, enableErr := service.SetResourceEnabled(context.Background(), actor, connection.ID, "properties/123", true); enableErr != nil {
				t.Fatal(enableErr)
			}
			var apiCalls, tokenCalls atomic.Int64
			token := service.OAuth.(*fakeOAuth).token

			google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					tokenCalls.Add(1)
					w.WriteHeader(http.StatusOK)

					return
				}

				apiCalls.Add(1)

				if r.Method != http.MethodGet || r.URL.Path != "/v1beta/properties/123" || r.Header.Get("Authorization") != "Bearer "+token.AccessToken {
					t.Errorf("incorrect fixed resource or central token request")
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"properties/123","displayName":"Development fixture","currencyCode":"GBP","timeZone":"Europe/London","industryCategory":"TECHNOLOGY"}`))
			}))
			defer google.Close()

			if refresh {
				stale := token

				stale.Expiry = time.Now().Add(-time.Hour)
				if saveErr := service.saveToken(context.Background(), actor, &connection, stale); saveErr != nil {
					t.Fatal(saveErr)
				}
				service.OAuth = httpRefreshOAuth{OAuthProvider: service.OAuth, tokenURL: google.URL + "/token"}
			}
			service.AnalyticsAdminFactory = func(ctx context.Context, email string) (*analyticsadmin.Service, error) {
				if email != connection.GoogleEmail || authclient.AccessTokenFromContext(ctx) != token.AccessToken || !googleapi.ReadOnly(ctx) || !googleapi.NoInputFromContext(ctx) {
					t.Error("central account or read-only/no-input context not propagated")
				}

				client, clientErr := googleapi.NewHTTPClient(ctx, googleauth.ServiceAnalytics, email)
				if clientErr != nil {
					return nil, fmt.Errorf("mock GA4 transport: %w", clientErr)
				}

				sdk, sdkErr := analyticsadmin.NewService(ctx, option.WithHTTPClient(client), option.WithEndpoint(google.URL+"/"))
				if sdkErr != nil {
					return nil, fmt.Errorf("mock GA4 client: %w", sdkErr)
				}

				return sdk, nil
			}
			server, client := newProductTestHandler(t, service, actor)
			cookies := productSessionCookies(t, client, server)
			resp := productGetWithCSRF(t, client, server.URL, "/api/connections/"+connection.ID+"/analytics/property?resource=properties/123", cookies, productCSRF(t, server, client, cookies))

			body := readProductBody(t, resp)
			if resp.StatusCode != http.StatusOK || !strings.Contains(body, "analytics.property.get") || strings.Contains(body, "industryCategory") {
				t.Fatalf("fixed response contract failed: %d %s", resp.StatusCode, body)
			}

			var result struct {
				Property AnalyticsPropertyData `json:"property"`
			}
			if jsonErr := json.Unmarshal([]byte(body), &result); jsonErr != nil || result.Property.Name != "properties/123" {
				t.Fatal("wrong property result", jsonErr)
			}

			if apiCalls.Load() != 1 || (refresh && tokenCalls.Load() != 1) {
				t.Fatal("incorrect API/refresh request counts")
			}

			audit, auditErr := store.ListAudit(context.Background(), actor.OrganizationID, 30)
			if auditErr != nil {
				t.Fatal(auditErr)
			}
			found := false

			for _, event := range audit {
				if event.Action == "agent.analytics.property.get" && event.Result == "ok" && strings.Contains(event.Detail, "google_api_requests=1") {
					found = true
				}
			}

			if !found {
				t.Fatal("success audit did not isolate the single API request")
			}
		})
	}
}

func TestProductReadRejectsCrossOriginBeforePolicyOrCredentials(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)

	sessions, err := NewSessionManager([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}

	token, payload, err := sessions.NewProduct(actor)
	if err != nil {
		t.Fatal(err)
	}

	server, client := newProductTestHandlerWithSessions(t, service, actor, sessions)
	for _, site := range []string{"cross-site", "same-site", ""} {
		req, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/connections/unknown/analytics/property?resource=properties/123", nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}

		req.AddCookie(sessions.ProductCookie(token))
		req.Header.Set("Sec-Fetch-Site", site)

		if site != "" {
			req.Header.Set("X-CSRF-Token", payload.CSRF)
		}

		resp, responseErr := client.Do(req)
		if responseErr != nil {
			t.Fatal(responseErr)
		}

		body := readProductBody(t, resp)
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "same_origin_required") {
			t.Fatalf("origin %q was not blocked: %d", site, resp.StatusCode)
		}
	}

	audit, err := store.ListAudit(context.Background(), actor.OrganizationID, 10)
	if err != nil || len(audit) != 0 {
		t.Fatal("cross-origin request reached the resource policy/read path", err)
	}
}

func TestProductAndAdminSessionsCoexistAndRejectRoleSwap(t *testing.T) {
	sessions, err := NewSessionManager([]byte("0123456789abcdef0123456789abcdef"), time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	actor := Actor{UserID: "owner", OrganizationID: "org", Role: "owner"}

	admin, _, err := sessions.New(actor)
	if err != nil {
		t.Fatal(err)
	}

	product, _, err := sessions.NewProduct(actor)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "https://product.localhost/", nil)
	req.AddCookie(sessions.Cookie(admin))
	req.AddCookie(sessions.ProductCookie(product))
	adminPayload, adminOK := sessions.FromRequest(req)

	productPayload, productOK := sessions.FromProductRequest(req)
	if !adminOK || !adminPayload.Admin || !productOK || productPayload.Admin {
		t.Fatal("admin/product sessions did not coexist")
	}
	swapped := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "https://product.localhost/", nil)
	swapped.AddCookie(sessions.ProductCookie(admin))

	if _, ok := sessions.FromProductRequest(swapped); ok {
		t.Fatal("admin token accepted as a product session")
	}

	if sessions.ClearProductCookie().Name == sessions.ClearCookie().Name {
		t.Fatal("product logout would clear admin session")
	}
}
