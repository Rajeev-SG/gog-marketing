package cmd

import (
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func findHostedAnalyticsInventoryTool(t *testing.T, name string) mcpToolSpec {
	t.Helper()
	for _, tool := range hostedAnalyticsInventoryTools() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing %s tool", name)
	return mcpToolSpec{}
}

func buildHostedAnalyticsArgs(t *testing.T, spec mcpToolSpec, args map[string]any) ([]string, error) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = spec.Name
	req.Params.Arguments = args
	return spec.BuildArgs(req)
}

func TestHostedAnalyticsInventoryToolsAreReadOnlyAndBounded(t *testing.T) {
	tools := hostedAnalyticsInventoryTools()
	if len(tools) != 2 {
		t.Fatalf("expected exactly two inventory tools, got %d", len(tools))
	}

	for _, name := range []string{"analytics_accounts", "analytics_properties_list"} {
		tool := findHostedAnalyticsInventoryTool(t, name)
		if tool.Service != "analytics" || tool.Risk != mcpRiskRead {
			t.Fatalf("%s has an unsafe spec: %+v", name, tool)
		}
	}
}

func TestHostedAnalyticsInventorySchemasNeverExposeFilter(t *testing.T) {
	schema := func(spec mcpToolSpec) map[string]any {
		return mcp.NewTool(spec.Name, spec.Options...).InputSchema.Properties
	}

	accounts := schema(findHostedAnalyticsInventoryTool(t, "analytics_accounts"))
	if _, ok := accounts["account"]; ok {
		t.Fatal("analytics_accounts must not take an account selector")
	}
	if _, ok := accounts["filter"]; ok {
		t.Fatal("analytics_accounts must not expose filter")
	}
	for _, key := range []string{"max", "page", "all", "fail_empty"} {
		if _, ok := accounts[key]; !ok {
			t.Fatalf("analytics_accounts is missing the %s option", key)
		}
	}

	properties := schema(findHostedAnalyticsInventoryTool(t, "analytics_properties_list"))
	if _, ok := properties["filter"]; ok {
		t.Fatal("analytics_properties_list must not expose filter")
	}
	for _, key := range []string{"account", "max", "page", "all", "fail_empty"} {
		if _, ok := properties[key]; !ok {
			t.Fatalf("analytics_properties_list is missing the %s option", key)
		}
	}
}

func TestHostedAnalyticsAccountsBuildArgs(t *testing.T) {
	spec := findHostedAnalyticsInventoryTool(t, "analytics_accounts")

	args, err := buildHostedAnalyticsArgs(t, spec, map[string]any{})
	if err != nil || !reflect.DeepEqual(args, []string{"analytics", "accounts", "--max=50"}) {
		t.Fatalf("default accounts args: %v (%v)", args, err)
	}

	args, err = buildHostedAnalyticsArgs(t, spec, map[string]any{"max": 2, "page": "tok", "all": true, "fail_empty": true})
	if err != nil || !reflect.DeepEqual(args, []string{"analytics", "accounts", "--max=2", "--page=tok", "--all", "--fail-empty"}) {
		t.Fatalf("paged accounts args: %v (%v)", args, err)
	}

	if _, err := buildHostedAnalyticsArgs(t, spec, map[string]any{"max": 0}); err == nil {
		t.Fatal("unbounded page size was accepted")
	}
	if _, err := buildHostedAnalyticsArgs(t, spec, map[string]any{"max": 201}); err == nil {
		t.Fatal("unbounded page size was accepted")
	}
}

func TestHostedAnalyticsPropertiesListBuildArgs(t *testing.T) {
	spec := findHostedAnalyticsInventoryTool(t, "analytics_properties_list")

	args, err := buildHostedAnalyticsArgs(t, spec, map[string]any{"account": "accounts/111"})
	if err != nil || !reflect.DeepEqual(args, []string{"analytics", "properties", "list", "--page-size=50", "--filter=parent:accounts/111"}) {
		t.Fatalf("default properties args: %v (%v)", args, err)
	}

	args, err = buildHostedAnalyticsArgs(t, spec, map[string]any{"account": " accounts/111 ", "max": 2, "page": "tok", "all": true, "fail_empty": true})
	if err != nil || !reflect.DeepEqual(args, []string{"analytics", "properties", "list", "--page-size=2", "--filter=parent:accounts/111", "--page-token=tok", "--all", "--fail-empty"}) {
		t.Fatalf("paged properties args: %v (%v)", args, err)
	}

	for _, bad := range []any{
		"111", "accounts/abc", "accounts/", "accounts/111/extra", "properties/111",
		"accounts/../foreign", "ACCOUNTS/111", "accounts/111 or accounts/222", "",
	} {
		if _, err := buildHostedAnalyticsArgs(t, spec, map[string]any{"account": bad}); err == nil {
			t.Fatalf("invalid account selector %q was accepted", bad)
		}
	}

	if _, err := buildHostedAnalyticsArgs(t, spec, map[string]any{}); err == nil {
		t.Fatal("missing account selector was accepted")
	}

	if _, err := buildHostedAnalyticsArgs(t, spec, map[string]any{"account": "accounts/111", "filter": "ancestor:accounts/111"}); err == nil {
		t.Fatal("filter override was accepted")
	}
}
