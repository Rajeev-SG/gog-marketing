package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeAnalyticsPropertiesCreateRequiresGrantedAccount(t *testing.T) {
	var requests atomic.Int64

	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AnalyticsAdmin = func(ctx context.Context, _ string) (*analyticsadmin.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)

				if r.Method != http.MethodPost || r.URL.Path != "/v1beta/properties" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}

				if r.URL.Query().Get("parent") != "" {
					t.Errorf("parent must not be a query parameter: %s", r.URL.RawQuery)
				}

				var body struct {
					Parent       string `json:"parent"`
					DisplayName  string `json:"displayName"`
					TimeZone     string `json:"timeZone"`
					CurrencyCode string `json:"currencyCode"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}

				if body.Parent != "accounts/999" || body.DisplayName != "Paid Media Decoded" || body.TimeZone != "Europe/London" || body.CurrencyCode != "GBP" {
					t.Errorf("unexpected body: %+v", body)
				}

				data, _ := json.Marshal(map[string]any{"name": "properties/321", "parent": "accounts/999"})

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
			})

			return analyticsadmin.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(scope string, allowWrite bool, args map[string]any) capturedResponse {
		name := "analytics_properties_create"
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{
				Service: "analytics", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: allowWrite,
				Resource: &ResourceGrantInput{Service: "analytics", ResourceType: "account", ResourceID: "accounts/999", Enabled: true},
			}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	args := map[string]any{
		"account":      "accounts/999",
		"display_name": "Paid Media Decoded",
		"time_zone":    "Europe/London",
		"currency":     "GBP",
	}

	if denied := call("analytics.readonly", true, args); denied.OK || requests.Load() != 0 {
		t.Fatal("read scope authorized property creation")
	}

	if denied := call("analytics.edit", false, args); denied.OK || requests.Load() != 0 {
		t.Fatal("missing write grant authorized property creation")
	}

	foreign := map[string]any{"account": "accounts/111", "display_name": "Paid Media Decoded", "time_zone": "Europe/London", "currency": "GBP"}
	if denied := call("analytics.edit", true, foreign); denied.OK || requests.Load() != 0 {
		t.Fatal("foreign account reached Google")
	}

	response := call("analytics.edit", true, args)
	if !response.OK {
		t.Fatalf("native analytics_properties_create failed: %s", response.Body)
	}

	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}

func TestAdmitAnalyticsPropertyCreateRejectsForeignBodyParent(t *testing.T) {
	invocation := &ToolInvocation{
		Name:      "analytics_properties_create",
		Arguments: map[string]any{"account": "accounts/999"},
		Authorization: ToolAuthorization{
			Service: "analytics", AllowWrite: true,
			Resource: &ResourceGrantInput{Service: "analytics", ResourceType: "account", ResourceID: "accounts/999", Enabled: true},
		},
	}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://analyticsadmin.googleapis.com/v1beta/properties?alt=json&prettyPrint=false", strings.NewReader(`{"parent":"accounts/111","displayName":"X","timeZone":"Europe/London","currencyCode":"GBP"}`))
	if err != nil {
		t.Fatal(err)
	}

	if admitAnalyticsPropertyCreate(invocation, request) == nil {
		t.Fatal("foreign body parent was admitted")
	}

	// The body is restored for the real request after inspection.
	request.Body = io.NopCloser(strings.NewReader(`{"parent":"accounts/999","displayName":"X"}`))
	if err := admitAnalyticsPropertyCreate(invocation, request); err != nil {
		t.Fatalf("granted body parent rejected: %v", err)
	}
}

func TestNativeAnalyticsRealtimeUsesSelectedProperty(t *testing.T) {
	var requests atomic.Int64

	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AnalyticsData = func(ctx context.Context, _ string) (*analyticsdata.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)

				if r.Method != http.MethodPost || r.URL.Path != "/v1beta/properties/123:runRealtimeReport" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}

				var body struct {
					Dimensions []struct {
						Name string `json:"name"`
					} `json:"dimensions"`
					Metrics []struct {
						Name string `json:"name"`
					} `json:"metrics"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}

				if len(body.Dimensions) != 1 || body.Dimensions[0].Name != "eventName" || len(body.Metrics) != 1 || body.Metrics[0].Name != "eventCount" {
					t.Errorf("unexpected realtime body: %+v", body)
				}

				data, _ := json.Marshal(map[string]any{"rows": []any{map[string]any{"dimensionValues": []any{map[string]any{"value": "page_view"}}}}})

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
			})

			return analyticsdata.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(args map[string]any, resourceID string) capturedResponse {
		name := "analytics_realtime"
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{
				Service: "analytics", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/analytics.readonly"},
				Resource: &ResourceGrantInput{Service: "analytics", ResourceType: "property", ResourceID: resourceID, Enabled: true},
			}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	args := map[string]any{"property": "properties/123", "dimensions": "eventName", "metrics": "eventCount", "max": 25}

	if denied := call(args, "properties/456"); denied.OK || requests.Load() != 0 {
		t.Fatal("foreign property reached the realtime endpoint")
	}

	response := call(args, "properties/123")
	if !response.OK {
		t.Fatalf("native analytics_realtime failed: %s", response.Body)
	}

	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}
