package cmd

import "github.com/mark3labs/mcp-go/mcp"

func hostedSitesTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 2)
	for _, operation := range []string{"get", "url"} {
		options := []mcp.ToolOption{mcp.WithString("site_id", mcp.Required())}
		if operation == "get" {
			options = append(options, mcp.WithString("fields"))
		}
		result = append(result, mcpToolSpec{
			Name: "sites_" + operation, Service: "sites", Risk: mcpRiskRead, Description: "Read an explicitly enabled Picker-selected Google Site using native Drive-backed metadata with drive.file. No global Sites inventory/search or site editing is exposed.", Options: options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "site_id")
				if err != nil {
					return nil, err
				}
				args := []string{"sites", operation}
				if operation == "get" {
					if fields := req.GetString("fields", ""); fields != "" {
						args = append(args, "--fields="+fields)
					}
				}
				return append(args, "--", id), nil
			},
		})
	}
	return result
}
