package runner

import (
	"net/http"
	"regexp"
	"strings"
)

// analyticsAccountResourcePattern pins GA4 account resources to canonical
// accounts/<numeric id> selectors.
var analyticsAccountResourcePattern = regexp.MustCompile(`^accounts/[0-9]+$`)

// Native GA4 inventory is read-only. analytics_accounts lists account
// summaries under an account-level exact tool grant and requires no asset
// grant; analytics_properties_list lists properties under one granted account
// through a fixed parent filter. The native empty-filter fallback to the
// global account-summary listing is never permitted for the selected account.
func nativeAnalyticsInventoryBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{
			Name:    "analytics_accounts",
			Service: "analytics",
			Write:   false,
			hosts:   []string{"analyticsadmin.googleapis.com"},
			scopes:  []string{"analytics.readonly", "analytics.edit"},
			methods: []string{http.MethodGet},
			paths:   exactPaths("", ""),
			argumentPaths: func(map[string]any) ([]string, error) {
				return []string{"/v1beta/accountSummaries"}, nil
			},
		},
		{
			Name:             "analytics_properties_list",
			Service:          "analytics",
			ResourceType:     "account",
			ResourceArgument: "account",
			Write:            false,
			hosts:            []string{"analyticsadmin.googleapis.com"},
			scopes:           []string{"analytics.readonly", "analytics.edit"},
			methods:          []string{http.MethodGet},
			paths:            exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				account, _ := args["account"].(string)
				if !analyticsAccountResourcePattern.MatchString(account) {
					return nil, invalidf("invalid analytics account selector")
				}

				return []string{"/v1beta/properties"}, nil
			},
		},
	}
}

// admitAnalyticsPropertyInventory is an additional, analytics-specific query
// guard that nativeRequestGuard applies to analytics_properties_list after its
// common host, scope, method and grant checks. It pins the outgoing request to
// exactly one parent:<granted account> filter and rejects method, path,
// billing-project and access_token overrides.
func admitAnalyticsPropertyInventory(invocation *ToolInvocation, request *http.Request) error {
	if invocation == nil {
		return invalidf("analytics property inventory requires a signed invocation")
	}

	if invocation.Authorization.Service != "analytics" {
		return invalidf("analytics property inventory requires the analytics service, got %q", invocation.Authorization.Service)
	}

	resource := invocation.Authorization.Resource
	if resource == nil {
		return invalidf("analytics property inventory requires an account resource grant")
	}

	if resource.ResourceType != "account" {
		return invalidf("analytics property inventory requires an account resource grant, got %q", resource.ResourceType)
	}

	account := strings.TrimSpace(resource.ResourceID)
	if !analyticsAccountResourcePattern.MatchString(account) {
		return invalidf("invalid analytics account resource grant %q", resource.ResourceID)
	}

	if request == nil || request.URL == nil {
		return invalidf("missing request")
	}

	argument, ok := invocation.Arguments["account"].(string)
	if !ok || argument != account || !resource.Enabled || resource.Service != "analytics" {
		return invalidf("invalid selected account authority")
	}

	if request.Method != http.MethodGet {
		return invalidf("analytics property inventory is read-only: %s", request.Method)
	}

	if request.URL.Path != "/v1beta/properties" {
		return invalidf("unexpected analytics property inventory path %q", request.URL.Path)
	}

	if request.Header.Get("x-goog-user-project") != "" {
		return invalidf("billing project override is not permitted")
	}

	query := request.URL.Query()
	for key := range query {
		switch key {
		case "filter", "pageSize", "pageToken", "alt":
		case "prettyPrint":
			values := query[key]
			if len(values) != 1 || values[0] != "false" {
				return invalidf("invalid SDK formatting query")
			}
		default:
			return invalidf("unexpected analytics property inventory query parameter %q", key)
		}
	}

	filters := query["filter"]
	if len(filters) != 1 {
		return invalidf("analytics property inventory requires exactly one parent filter, got %d", len(filters))
	}

	if filters[0] != "parent:"+account {
		return invalidf("analytics property inventory filter %q does not match the granted account %q", filters[0], account)
	}

	return nil
}
