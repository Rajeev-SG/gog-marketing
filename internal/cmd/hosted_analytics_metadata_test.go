package cmd

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedAnalyticsMetadataArgs(t *testing.T) {
	spec := findHostedAnalyticsMetadataTool(t, "analytics_metadata_get")
	args, err := spec.BuildArgs(hostedAnalyticsMetadataTestRequest(map[string]any{"property": "properties/123"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 4 || args[0] != "analytics" || args[3] != "123" {
		t.Fatalf("args = %#v", args)
	}
}

func TestHostedAnalyticsMetadataRejectsSelectors(t *testing.T) {
	spec := findHostedAnalyticsMetadataTool(t, "analytics_metadata_get")
	for _, property := range []string{"123", "properties/0", "properties/-1", "properties/abc", "properties/123/../456"} {
		if _, err := spec.BuildArgs(hostedAnalyticsMetadataTestRequest(map[string]any{"property": property})); err == nil {
			t.Fatalf("property %q accepted", property)
		}
	}
}

func hostedAnalyticsMetadataTestRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

func findHostedAnalyticsMetadataTool(t *testing.T, name string) mcpToolSpec {
	t.Helper()
	for _, tool := range hostedAnalyticsMetadataTools() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing %s tool", name)
	return mcpToolSpec{}
}
