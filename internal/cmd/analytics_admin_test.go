package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAnalyticsPropertiesListDefaultsToAccountSummaries(t *testing.T) {
	propertiesCalled := false
	svc := newAnalyticsAdminTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1beta/properties") {
			propertiesCalled = true
			http.Error(w, "filter must not be empty", http.StatusBadRequest)
			return
		}
		if !strings.Contains(r.URL.Path, "/v1beta/accountSummaries") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accountSummaries": []map[string]any{
				{
					"account":     "accounts/123",
					"displayName": "Demo Account",
					"propertySummaries": []map[string]any{
						{"property": "properties/999", "displayName": "Main Property", "parent": "accounts/123", "canEdit": true, "propertyType": "PROPERTY_TYPE_ORDINARY"},
					},
				},
			},
		})
	}))
	result := executeWithAnalyticsAdminTestService(t, []string{"--account", "a@b.com", "--json", "analytics", "properties", "list"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if propertiesCalled {
		t.Fatal("bare properties list must use account summaries instead of Properties.List")
	}
	if !strings.Contains(result.stdout, "properties/999") || !strings.Contains(result.stdout, "Main Property") {
		t.Fatalf("unexpected output: %s", result.stdout)
	}
	assertAnalyticsPropertyListSchema(t, result.stdout, "account_summaries")
}

func TestAnalyticsPropertiesListPassesFilter(t *testing.T) {
	svc := newAnalyticsAdminTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1beta/properties") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("filter"); got != "ancestor:accounts/123" {
			t.Fatalf("filter = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"properties": []map[string]any{
				{"name": "properties/999", "displayName": "Main Property"},
			},
		})
	}))
	result := executeWithAnalyticsAdminTestService(t, []string{"--account", "a@b.com", "--json", "analytics", "properties", "list", "--filter", "ancestor:accounts/123"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stdout, "properties/999") {
		t.Fatalf("unexpected output: %s", result.stdout)
	}
	assertAnalyticsPropertyListSchema(t, result.stdout, "properties_list")
}

func assertAnalyticsPropertyListSchema(t *testing.T, output, source string) {
	t.Helper()
	var payload struct {
		Properties []map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode properties output: %v\n%s", err, output)
	}
	if len(payload.Properties) != 1 {
		t.Fatalf("properties = %#v", payload.Properties)
	}
	for _, key := range []string{"name", "display_name", "parent", "can_edit", "property_type", "create_time", "update_time", "source"} {
		if _, ok := payload.Properties[0][key]; !ok {
			t.Fatalf("property schema missing %q: %#v", key, payload.Properties[0])
		}
	}
	if payload.Properties[0]["source"] != source {
		t.Fatalf("source = %#v, want %q", payload.Properties[0]["source"], source)
	}
}
