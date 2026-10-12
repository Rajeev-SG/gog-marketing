package cmd

import (
	"encoding/json"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedAnalyticsAdminTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 25)
	for _, family := range []struct{ command, api string }{{"datastreams", "dataStreams"}, {"keyevents", "keyEvents"}, {"custom-dimensions", "customDimensions"}, {"custom-metrics", "customMetrics"}, {"googleads-links", "googleAdsLinks"}} {
		operations := []string{"list", "get", "create", "update", "delete"}
		if family.api == "customDimensions" || family.api == "customMetrics" {
			operations[4] = "archive"
		}
		for _, operation := range operations {
			risk := mcpRiskRead
			options := []mcp.ToolOption{mcp.WithString("property", mcp.Required())}
			item := operation == "get" || operation == "update" || (operation == "delete" || operation == "archive")
			if item {
				options = append(options, mcp.WithString("resource_id", mcp.Required(), mcp.Description("Child resource ID within the selected property")))
			}
			if operation == "create" || operation == "update" {
				risk = mcpRiskWrite
				options = append(options, mcp.WithString("body_json", mcp.Required(), mcp.Description("Inline resource JSON object only; no files or @file references")))
			}
			if operation == "delete" || operation == "archive" {
				risk = mcpRiskWrite
			}
			if operation == "update" {
				options = append(options, mcp.WithString("update_mask", mcp.Required(), mcp.Description("Required snake_case Google field mask, e.g. display_name or counting_method")))
			}
			if operation == "list" {
				options = append(options, mcp.WithInteger("max", mcp.Min(1), mcp.Max(200)), mcp.WithString("page"), mcp.WithBoolean("all"), mcp.WithBoolean("fail_empty"))
			}
			result = append(result, mcpToolSpec{
				Name: "analytics_" + family.command + "_" + operation, Service: "analytics", Risk: risk,
				Description: "Run native GA4 Admin " + family.command + " " + operation + " within an explicitly enabled property. Writes require analytics.edit; deletion requires destructive permission.", Options: options,
				BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
					property, err := requireMCPString(req, "property")
					if err != nil {
						return nil, err
					}
					target := property
					if item {
						id, err := requireMCPString(req, "resource_id")
						if err != nil {
							return nil, err
						}
						target += "/" + family.api + "/" + id
					}
					args := []string{"analytics", family.command, operation}
					if operation == "create" || operation == "update" {
						raw, err := requireMCPString(req, "body_json")
						if err != nil {
							return nil, err
						}
						var body map[string]any
						if json.Unmarshal([]byte(raw), &body) != nil || body == nil {
							return nil, ErrHostedToolInput
						}
						if name, present := body["name"]; present && name != "" && name != target {
							return nil, ErrHostedToolInput
						}
						args = append(args, "--json-file="+raw)
					}
					if operation == "update" {
						mask, err := requireMCPString(req, "update_mask")
						if err != nil {
							return nil, err
						}
						args = append(args, "--update-mask="+mask)
					}
					if operation == "list" {
						args = append(args, "--page-size="+strconv.Itoa(req.GetInt("max", 50)))
						if page := req.GetString("page", ""); page != "" {
							args = append(args, "--page-token="+page)
						}
						for _, key := range []string{"all", "fail_empty"} {
							if value, present := req.GetArguments()[key]; present {
								enabled, _ := value.(bool)
								flag := key
								if key == "fail_empty" {
									flag = "fail-empty"
								}
								args = append(args, "--"+flag+"="+strconv.FormatBool(enabled))
							}
						}
					}
					if family.api == "googleAdsLinks" && operation == "get" {
						return append(args, "--", property, target), nil
					}
					return append(args, "--", target), nil
				},
			})
		}
	}
	return result
}
