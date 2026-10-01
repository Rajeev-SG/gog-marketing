package cmd

import (
	"reflect"
	"testing"
)

func TestCLIProductSurfaceParity(t *testing.T) {
	typ := reflect.TypeOf(CLI{})
	for _, name := range []string{
		"Gmail", "Calendar", "Drive", "Docs", "Sheets", "Slides", "Chat", "Contacts", "Tasks", "People", "Forms", "Meet", "Classroom", "AppScript", "Admin",
		"Analytics", "GoogleAds", "TagManager", "SearchConsole", "BigQuery",
	} {
		if _, ok := typ.FieldByName(name); !ok {
			t.Fatalf("command tree lost %s", name)
		}
	}
	tools := map[string]bool{}
	for _, tool := range mcpAllTools() {
		tools[tool.Name] = true
	}
	for _, name := range []string{"gmail_search", "calendar_events", "drive_search", "analytics_properties_list", "googleads_customers_list", "tagmanager_accounts_list", "searchconsole_sites_list", "bigquery_datasets_list"} {
		if !tools[name] {
			t.Fatalf("tool catalogue lost %s", name)
		}
	}
}
