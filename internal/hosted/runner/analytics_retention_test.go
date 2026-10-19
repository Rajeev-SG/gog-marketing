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
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeAnalyticsRetentionAuthorityAndPartialUpdates(t *testing.T) {
	var calls atomic.Int64
	state := map[string]any{"name": "properties/123/dataRetentionSettings", "eventDataRetention": "FOURTEEN_MONTHS", "userDataRetention": "TWO_MONTHS", "resetUserDataOnNewActivity": true}
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AnalyticsAdmin = func(ctx context.Context, account string) (*analyticsadmin.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
				t.Error("retention identity lost")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.URL.Path != "/v1beta/properties/123/dataRetentionSettings" {
					t.Errorf("unexpected retention path %s", r.URL)
				}

				switch r.Method {
				case http.MethodGet:
				case http.MethodPatch:
					var patch map[string]any
					if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
						t.Fatal(err)
					}

					if r.URL.Query().Get("updateMask") != "event_data_retention,reset_user_data_on_new_activity" || len(patch) != 3 || patch["eventDataRetention"] != "TWO_MONTHS" || patch["resetUserDataOnNewActivity"] != false {
						t.Errorf("retention patch mask/body %s %+v", r.URL, patch)
					}
					state["eventDataRetention"] = patch["eventDataRetention"]
					state["resetUserDataOnNewActivity"] = patch["resetUserDataOnNewActivity"]
				default:
					t.Errorf("undeclared retention method %s", r.Method)
				}

				raw, err := json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
			})

			return analyticsadmin.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	call := func(name, scope string, args map[string]any, write, destructive bool, change func(*ToolAuthorization)) capturedResponse {
		auth := ToolAuthorization{Service: "analytics", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: write, AllowDestructive: destructive, Resource: &ResourceGrantInput{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", Enabled: true}}
		if change != nil {
			change(&auth)
		}
		body := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: auth} })

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	get := map[string]any{"property": "properties/123"}
	for _, scope := range []string{"analytics.readonly", "analytics.edit"} {
		if result := call("analytics_retention_get", scope, get, false, false, nil); !result.OK {
			t.Fatalf("read %s %s", scope, result.Body)
		}
	}
	update := map[string]any{"property": "properties/123", "event_data_retention": "TWO_MONTHS", "reset_user_data_on_new_activity": false}

	result := call("analytics_retention_update", "analytics.edit", update, true, true, nil)
	if !result.OK || state["userDataRetention"] != "TWO_MONTHS" || state["resetUserDataOnNewActivity"] != false || !strings.Contains(string(result.Result), "TWO_MONTHS") {
		t.Fatalf("partial retention update failed %s", result.Body)
	}
	before := calls.Load()

	for _, change := range []func(*ToolAuthorization){
		func(a *ToolAuthorization) { a.AllowWrite = false }, func(a *ToolAuthorization) { a.AllowDestructive = false },
		func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/analytics.readonly"} },
		func(a *ToolAuthorization) { a.Resource = nil }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.Resource.ResourceID = "properties/456" },
		func(a *ToolAuthorization) { a.AllowedTools = []string{"analytics_retention_get"} }, func(a *ToolAuthorization) { a.Service = "docs" },
	} {
		if result := call("analytics_retention_update", "analytics.edit", update, true, true, change); result.OK {
			t.Fatal("unauthorized retention mutation admitted")
		}
	}

	for _, args := range []map[string]any{
		{"property": "properties/123"},
		{"property": "properties/123", "user_data_retention": "FIFTY_MONTHS"},
		{"property": "properties/123", "event_data_retention": "RETENTION_DURATION_UNSPECIFIED"},
		{"property": "properties/123", "reset_user_data_on_new_activity": "false"},
		{"property": "properties/123/../456", "event_data_retention": "TWO_MONTHS"},
	} {
		if result := call("analytics_retention_update", "analytics.edit", args, true, true, nil); result.OK {
			t.Fatalf("invalid retention mutation admitted %v", args)
		}
	}

	if calls.Load() != before {
		t.Fatal("denied retention operation reached Google")
	}
}

func TestAnalyticsRetentionExactHTTPGuard(t *testing.T) {
	for _, name := range []string{"analytics_retention_get", "analytics_retention_update"} {
		guard := nativeRequestGuard(&ToolInvocation{Name: name, Arguments: map[string]any{"property": "properties/123"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "properties/123"}}})

		path := "https://analyticsadmin.googleapis.com/v1beta/properties/123/dataRetentionSettings"
		for _, tc := range []struct {
			method, url string
			allow       bool
		}{
			{http.MethodGet, path, name == "analytics_retention_get"},
			{http.MethodPatch, path, name == "analytics_retention_update"},
			{http.MethodPost, path, false},
			{http.MethodDelete, path, false},
			{http.MethodGet, path + "/extra", false},
			{http.MethodPatch, "https://analyticsadmin.googleapis.com/v1beta/properties/456/dataRetentionSettings", false},
			{http.MethodGet, "https://foreign.example/v1beta/properties/123/dataRetentionSettings", false},
		} {
			request, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err := guard(request); (err == nil) != tc.allow {
				t.Errorf("%s %s %s allowed=%t err=%v", name, tc.method, tc.url, tc.allow, err)
			}
		}
	}
}

func TestNativeRetentionUncertainWriteSingleAttemptThroughGuard(t *testing.T) {
	var effects atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.AnalyticsAdmin = func(ctx context.Context, _ string) (*analyticsadmin.Service, error) {
			base := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				effects.Add(1)
				return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":503,"message":"uncertain"}}`)), Request: r}, nil
			})
			retry := googleapi.NewRetryTransport(base)
			retry.MaxRetries5xx = 1

			return analyticsadmin.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, retry)}))
		}
	})
	body := nativeToolBody(t, func(req *Request) {
		req.Tool = &ToolInvocation{Name: "analytics_retention_update", Arguments: map[string]any{"property": "properties/123", "event_data_retention": "TWO_MONTHS"}, Authorization: ToolAuthorization{Service: "analytics", AllowedTools: []string{"analytics_retention_update"}, Scopes: []string{"https://www.googleapis.com/auth/analytics.edit"}, Resource: &ResourceGrantInput{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", Enabled: true}, AllowWrite: true, AllowDestructive: true}}
	})

	result := postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	if result.OK || effects.Load() != 1 {
		t.Fatalf("uncertain patch repeated effects=%d response=%s", effects.Load(), result.Body)
	}
}
