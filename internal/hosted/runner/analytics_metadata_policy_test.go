package runner

import (
	"net/http"
	"testing"
)

func TestAnalyticsMetadataPolicy(t *testing.T) {
	binding, ok := nativeBinding("analytics_metadata_get")
	if !ok || binding.Write || binding.ResourceType != "property" || binding.ResourceArgument != "property_id" {
		t.Fatalf("binding = %+v/%t", binding, ok)
	}
	if len(binding.methods) != 1 || binding.methods[0] != http.MethodGet {
		t.Fatalf("methods = %#v", binding.methods)
	}
	if len(binding.hosts) != 1 || binding.hosts[0] != "analyticsdata.googleapis.com" {
		t.Fatalf("hosts = %#v", binding.hosts)
	}
	if fmtSprintStrings(binding.scopes) != fmtSprintStrings([]string{"analytics.readonly", "analytics"}) {
		t.Fatalf("scopes = %#v", binding.scopes)
	}
	paths, err := binding.argumentPaths(map[string]any{"property_id": "123"})
	if err != nil || fmtSprintStrings(paths) != fmtSprintStrings([]string{"/v1beta/properties/123/metadata"}) {
		t.Fatalf("argumentPaths = %#v/%v", paths, err)
	}
	if _, err := binding.argumentPaths(map[string]any{"property_id": "properties/123"}); err == nil {
		t.Fatal("foreign selector accepted")
	}
}

func TestAnalyticsMetadataInvocationAndRequestGuard(t *testing.T) {
	auth := ToolAuthorization{Service: ServiceAnalytics, AllowedTools: []string{"analytics_metadata_get"}, Scopes: []string{"https://www.googleapis.com/auth/analytics.readonly"}, Resource: &ResourceGrantInput{Enabled: true, Service: ServiceAnalytics, ResourceType: "property", ResourceID: "123"}}
	invocation := ToolInvocation{Name: "analytics_metadata_get", Arguments: map[string]any{"property_id": "123"}, Authorization: auth}
	if err := validateToolInvocation(&invocation); err != nil {
		t.Fatalf("valid invocation rejected: %v", err)
	}
	guard := nativeRequestGuard(&invocation)
	request, _ := http.NewRequest(http.MethodGet, "https://analyticsdata.googleapis.com/v1beta/properties/123/metadata", nil)
	if err := guard(request); err != nil {
		t.Fatalf("valid request denied: %v", err)
	}
	for _, tc := range []struct{ method, host, path string }{
		{http.MethodPost, "analyticsdata.googleapis.com", "/v1beta/properties/123/metadata"},
		{http.MethodGet, "analyticsadmin.googleapis.com", "/v1beta/properties/123"},
		{http.MethodGet, "analyticsdata.googleapis.com", "/v1beta/properties/456/metadata"},
		{http.MethodGet, "analyticsdata.googleapis.com", "/v1beta/properties/123/runReport"},
	} {
		bad, _ := http.NewRequest(tc.method, "https://"+tc.host+tc.path, nil)
		if err := guard(bad); err == nil {
			t.Fatalf("accepted %s %s%s", tc.method, tc.host, tc.path)
		}
	}
	unauthorized := invocation
	unauthorized.Authorization.Scopes = []string{"https://www.googleapis.com/auth/analytics.edit"}
	if err := validateToolInvocation(&unauthorized); err == nil {
		t.Fatal("edit-only scope accepted")
	}
}

func fmtSprintStrings(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for _, value := range values[1:] {
		out += "," + value
	}
	return out
}
