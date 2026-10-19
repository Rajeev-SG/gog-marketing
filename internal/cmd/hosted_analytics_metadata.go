package cmd

import (
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var hostedAnalyticsMetadataPropertyID = regexp.MustCompile(`^[1-9][0-9]*$`)

func hostedAnalyticsMetadataTools() []mcpToolSpec {
	propertyID := mcp.WithString("property", mcp.Required(), mcp.Description("GA4 property in properties/ID form with a positive numeric ID"))
	return []mcpToolSpec{
		{
			Name: "analytics_metadata_get", Service: "analytics", Risk: mcpRiskRead,
			Description: "Read GA4 Analytics Data API dimension and metric metadata for one explicitly enabled property. Requires exact tool/property authority and analytics.readonly; broader analytics is outside the hosted allowlist. No account-wide access, raw HTTP, alternate hosts or retries.",
			Options:     []mcp.ToolOption{propertyID},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				value, err := requireMCPString(req, "property")
				if err != nil {
					return nil, err
				}
				propertyID := strings.TrimPrefix(value, "properties/")
				if propertyID == value || !hostedAnalyticsMetadataPropertyID.MatchString(propertyID) {
					return nil, usage("a canonical properties/ID selector with a positive numeric GA4 property ID is required")
				}
				return []string{"analytics", "metadata", "get", propertyID}, nil
			},
		},
	}
}
