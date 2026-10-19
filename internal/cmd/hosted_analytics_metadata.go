package cmd

import (
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
)

var hostedAnalyticsMetadataPropertyID = regexp.MustCompile(`^[1-9][0-9]*$`)

func hostedAnalyticsMetadataTools() []mcpToolSpec {
	propertyID := mcp.WithString("property_id", mcp.Required(), mcp.Description("Positive numeric GA4 property ID"))
	return []mcpToolSpec{
		{
			Name: "analytics_metadata_get", Service: "analytics", Risk: mcpRiskRead,
			Description: "Read GA4 Analytics Data API dimension and metric metadata for one explicitly enabled property. Requires exact tool/property authority and analytics.readonly or analytics. No account-wide access, raw HTTP, alternate hosts or retries.",
			Options:     []mcp.ToolOption{propertyID},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				value, err := requireMCPString(req, "property_id")
				if err != nil {
					return nil, err
				}
				if !hostedAnalyticsMetadataPropertyID.MatchString(value) {
					return nil, usage("a positive numeric GA4 property_id is required")
				}
				return []string{"analytics", "metadata", "get", value}, nil
			},
		},
	}
}
