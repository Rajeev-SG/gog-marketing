package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeAnalyticsInventoryListsExactPagedEndpoints(t *testing.T) {
	tests := []struct {
		name         string
		tool         string
		args         map[string]any
		resource     *ResourceGrantInput
		path         string
		filter       string
		firstNeedle  string
		secondNeedle string
		firstToken   string
		nextToken    string
		firstBody    string
		nextBody     string
	}{
		{
			name: "accounts", tool: "analytics_accounts",
			args:        map[string]any{"max": 2},
			path:        "/v1beta/accountSummaries",
			firstNeedle: "accounts/111", secondNeedle: "accounts/333",
			firstToken: "accounts-page-2", nextToken: "accounts-page-3",
			firstBody: `{"accountSummaries":[{"account":"accounts/111","displayName":"One","propertySummaries":[{"property":"properties/222","displayName":"P"}]}],"nextPageToken":"accounts-page-2"}`,
			nextBody:  `{"accountSummaries":[{"account":"accounts/333","displayName":"Three"}]}`,
		},
		{
			name: "properties", tool: "analytics_properties_list",
			args:        map[string]any{"account": "accounts/111", "max": 2},
			resource:    &ResourceGrantInput{Service: "analytics", ResourceType: "account", ResourceID: "accounts/111", Enabled: true},
			path:        "/v1beta/properties",
			filter:      "parent:accounts/111",
			firstNeedle: "properties/222", secondNeedle: "properties/444",
			firstToken: "properties-page-2", nextToken: "properties-page-3",
			firstBody: `{"properties":[{"name":"properties/222","displayName":"P","parent":"accounts/111"}],"nextPageToken":"properties-page-2"}`,
			nextBody:  `{"properties":[{"name":"properties/444","displayName":"Q","parent":"accounts/111"}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.AnalyticsAdmin = func(ctx context.Context, account string) (*analyticsadmin.Service, error) {
					if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted inventory execution identity was lost")
					}

					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						number := calls.Add(1)

						if r.Method != http.MethodGet || r.URL.Path != test.path || r.URL.Hostname() != "analyticsadmin.googleapis.com" {
							t.Errorf("foreign or mutating inventory request reached Google: %s %s", r.Method, r.URL)
						}

						if r.URL.Query().Get("pageSize") != "2" {
							t.Errorf("native page size was lost: %q", r.URL.Query().Get("pageSize"))
						}

						if test.filter == "" {
							if _, ok := r.URL.Query()["filter"]; ok {
								t.Errorf("accounts inventory carried a filter: %v", r.URL.Query()["filter"])
							}
						} else if got := r.URL.Query().Get("filter"); got != test.filter {
							t.Errorf("native filter: got %q want %q", got, test.filter)
						}

						token := r.URL.Query().Get("pageToken")
						var body string

						switch number {
						case 1, 3:
							if token != "" {
								t.Errorf("first page carried %q", token)
							}
							body = test.firstBody
						case 2, 4:
							if token != test.firstToken {
								t.Errorf("second page token: got %q want %q", token, test.firstToken)
							}
							body = test.nextBody
						default:
							t.Errorf("unexpected inventory request: %s %s", r.Method, r.URL)
						}

						return &http.Response{
							StatusCode: 200,
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body:       io.NopCloser(strings.NewReader(body)),
							Request:    r,
						}, nil
					})

					return analyticsadmin.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			call := func(tool string, scope string, args map[string]any, resource *ResourceGrantInput) capturedResponse {
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{
						Name: tool, Arguments: args,
						Authorization: ToolAuthorization{
							Service: "analytics", AllowedTools: []string{tool},
							Scopes:   []string{"https://www.googleapis.com/auth/" + scope},
							Resource: resource,
						},
					}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) {
					claims["operation"] = OperationToolCall
				}), body)
			}

			first := call(test.tool, "analytics.readonly", test.args, test.resource)
			if !first.OK || !strings.Contains(string(first.Result), test.firstNeedle) {
				t.Fatalf("first %s page failed: %s", test.name, first.Body)
			}

			secondArgs := map[string]any{}
			for key, value := range test.args {
				secondArgs[key] = value
			}
			secondArgs["page"] = test.firstToken
			secondArgs["fail_empty"] = true

			second := call(test.tool, "analytics.edit", secondArgs, test.resource)
			if !second.OK || !strings.Contains(string(second.Result), test.secondNeedle) {
				t.Fatalf("second %s page failed: %s", test.name, second.Body)
			}

			before := calls.Load()

			if test.resource != nil {
				for _, bad := range []string{
					"accounts/../foreign",
					"accounts/../../foreign",
					"--account=foreign@example.com",
					"accounts/abc",
					"properties/111",
				} {
					denied := call(test.tool, "analytics.readonly", map[string]any{"account": bad, "max": 2}, test.resource)
					if denied.OK || calls.Load() != before {
						t.Fatalf("malformed selector or foreign account flag reached Google: %s", denied.Body)
					}
				}

				denied := call(test.tool, "analytics.readonly", test.args, nil)
				if denied.OK || calls.Load() != before {
					t.Fatal("missing resource grant reached Google")
				}
			}

			denied := call(test.tool, "analytics.manage.users", test.args, test.resource)
			if denied.OK || calls.Load() != before {
				t.Fatal("unauthorized inventory scope reached Google")
			}

			otherTool := "analytics_properties_list"
			if test.tool == otherTool {
				otherTool = "analytics_accounts"
			}

			deniedBody := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{
					Name: test.tool, Arguments: test.args,
					Authorization: ToolAuthorization{
						Service: "analytics", AllowedTools: []string{otherTool},
						Scopes:   []string{"https://www.googleapis.com/auth/analytics.readonly"},
						Resource: test.resource,
					},
				}
			})

			denied = postExecute(t, harness.server, signInvocation(t, deniedBody, func(claims map[string]any) {
				claims["operation"] = OperationToolCall
			}), deniedBody)
			if denied.OK || calls.Load() != before {
				t.Fatal("disabled tool grant reached Google")
			}

			foreignTenant := nativeToolBody(t, func(req *Request) {
				req.TenantID = "tenant-b"
				req.Tool = &ToolInvocation{
					Name: test.tool, Arguments: test.args,
					Authorization: ToolAuthorization{
						Service: "analytics", AllowedTools: []string{test.tool},
						Scopes:   []string{"https://www.googleapis.com/auth/analytics.readonly"},
						Resource: test.resource,
					},
				}
			})

			foreign := postExecute(t, harness.server, signInvocation(t, foreignTenant, func(claims map[string]any) {
				claims["operation"] = OperationToolCall
			}), foreignTenant)
			if foreign.OK || foreign.Status != http.StatusForbidden || calls.Load() != before {
				t.Fatal("foreign tenant reached inventory execution")
			}
		})
	}
}

func TestNativeAnalyticsInventoryBindingsStayReadOnly(t *testing.T) {
	for _, name := range []string{"analytics_accounts", "analytics_properties_list"} {
		binding, ok := nativeBinding(name)
		if !ok || binding.Write || binding.destructive || len(binding.methods) != 1 || binding.methods[0] != http.MethodGet {
			t.Fatalf("%s has an unsafe binding: %+v", name, binding)
		}

		if fmt.Sprint(binding.hosts) != fmt.Sprint([]string{"analyticsadmin.googleapis.com"}) {
			t.Fatalf("%s has an unexpected host: %v", name, binding.hosts)
		}

		if fmt.Sprint(binding.scopes) != fmt.Sprint([]string{"analytics.readonly", "analytics.edit"}) {
			t.Fatalf("%s has unexpected scopes: %v", name, binding.scopes)
		}
	}

	accounts, ok := nativeBinding("analytics_accounts")
	if !ok {
		t.Fatal("analytics_accounts is not registered as a native binding")
	}

	if accounts.ResourceType != "" || accounts.ResourceArgument != "" {
		t.Fatal("analytics_accounts must require no asset grant")
	}

	paths, err := accounts.argumentPaths(nil)
	if err != nil || len(paths) != 1 || paths[0] != "/v1beta/accountSummaries" {
		t.Fatalf("analytics_accounts argumentPaths: %v (%v)", paths, err)
	}

	properties, ok := nativeBinding("analytics_properties_list")
	if !ok {
		t.Fatal("analytics_properties_list is not registered as a native binding")
	}

	if properties.ResourceType != "account" || properties.ResourceArgument != "account" {
		t.Fatalf("analytics_properties_list has an unexpected resource binding: %+v", properties)
	}

	paths, err = properties.argumentPaths(map[string]any{"account": "accounts/111"})
	if err != nil || len(paths) != 1 || paths[0] != "/v1beta/properties" {
		t.Fatalf("analytics_properties_list argumentPaths: %v (%v)", paths, err)
	}

	if _, err := properties.argumentPaths(map[string]any{"account": "accounts/../foreign"}); err == nil {
		t.Fatal("malformed account selector accepted")
	}
}

func TestAdmitAnalyticsPropertyInventoryQueryGuard(t *testing.T) {
	granted := &ResourceGrantInput{Service: "analytics", ResourceType: "account", ResourceID: "accounts/111", Enabled: true}

	invocation := func(resource *ResourceGrantInput) *ToolInvocation {
		return &ToolInvocation{
			Name:      "analytics_properties_list",
			Arguments: map[string]any{"account": "accounts/111"},
			Authorization: ToolAuthorization{
				Service:      "analytics",
				AllowedTools: []string{"analytics_properties_list"},
				Resource:     resource,
			},
		}
	}

	request := func(rawQuery string) *http.Request {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://analyticsadmin.googleapis.com/v1beta/properties?"+rawQuery, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}

		return req
	}

	if err := admitAnalyticsPropertyInventory(invocation(granted), request("filter=parent:accounts/111&pageSize=50&alt=json")); err != nil {
		t.Fatalf("canonical request denied: %v", err)
	}

	if err := admitAnalyticsPropertyInventory(invocation(granted), request("pageToken=properties-page-2&filter=parent:accounts/111")); err != nil {
		t.Fatalf("paged request denied: %v", err)
	}

	denials := []struct {
		name     string
		resource *ResourceGrantInput
		rawQuery string
		method   string
		path     string
		header   string
	}{
		{name: "duplicate filter", resource: granted, rawQuery: "filter=parent:accounts/111&filter=parent:accounts/111"},
		{name: "or filter", resource: granted, rawQuery: "filter=parent:accounts/111%20OR%20parent:accounts/222"},
		{name: "ancestor filter", resource: granted, rawQuery: "filter=ancestor:accounts/111"},
		{name: "global empty filter", resource: granted, rawQuery: "filter="},
		{name: "foreign account filter", resource: granted, rawQuery: "filter=parent:accounts/222"},
		{name: "unknown filter", resource: granted, rawQuery: "filter=display_name:foo"},
		{name: "missing filter", resource: granted, rawQuery: "pageSize=50"},
		{name: "access_token override", resource: granted, rawQuery: "filter=parent:accounts/111&access_token=evil"},
		{name: "unknown query parameter", resource: granted, rawQuery: "filter=parent:accounts/111&unexpected=false"},
		{name: "mutating method", resource: granted, rawQuery: "filter=parent:accounts/111", method: http.MethodPost},
		{name: "foreign path", resource: granted, rawQuery: "pageSize=50", path: "/v1beta/accountSummaries"},
		{name: "missing resource grant", rawQuery: "filter=parent:accounts/111"},
		{name: "wrong resource type", resource: &ResourceGrantInput{ResourceType: "property", ResourceID: "properties/111"}, rawQuery: "filter=parent:accounts/111"},
		{name: "malformed resource id", resource: &ResourceGrantInput{ResourceType: "account", ResourceID: "accounts/abc"}, rawQuery: "filter=parent:accounts/111"},
		{name: "foreign resource id path", resource: &ResourceGrantInput{ResourceType: "account", ResourceID: "accounts/111/extra"}, rawQuery: "filter=parent:accounts/111"},
		{name: "billing override", resource: granted, rawQuery: "filter=parent:accounts/111", header: "billing-1"},
	}

	for _, denial := range denials {
		t.Run(denial.name, func(t *testing.T) {
			req := request(denial.rawQuery)
			if denial.method != "" {
				req.Method = denial.method
			}

			if denial.path != "" {
				req.URL.Path = denial.path
			}

			if denial.header != "" {
				req.Header.Set("x-goog-user-project", denial.header)
			}

			if err := admitAnalyticsPropertyInventory(invocation(denial.resource), req); err == nil {
				t.Fatal("denial case was admitted")
			}
		})
	}

	if err := admitAnalyticsPropertyInventory(nil, request("filter=parent:accounts/111")); err == nil {
		t.Fatal("unsigned invocation admitted")
	}

	wrongService := invocation(granted)

	wrongService.Authorization.Service = "calendar"
	if err := admitAnalyticsPropertyInventory(wrongService, request("filter=parent:accounts/111")); err == nil {
		t.Fatal("foreign service admitted")
	}
}
