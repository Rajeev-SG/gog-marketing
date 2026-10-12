package cmd

import "github.com/mark3labs/mcp-go/mcp"

func hostedAppScriptProjectTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 3)
	for _, operation := range []string{"get", "content", "create"} {
		risk := mcpRiskRead
		options := []mcp.ToolOption{mcp.WithString("script_id", mcp.Required())}
		if operation == "create" {
			risk = mcpRiskWrite
			options = []mcp.ToolOption{mcp.WithString("title", mcp.Required())}
		}
		result = append(result, mcpToolSpec{
			Name: "appscript_" + operation, Service: "appscript", Risk: risk, Description: "Run native Apps Script project " + operation + " with explicit account/tool grants and approved project scopes. No script execution, bound-file creation or local pull is exposed.", Options: options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				if operation == "create" {
					title, err := requireMCPString(req, "title")
					if err != nil {
						return nil, err
					}
					return []string{"appscript", "create", "--title=" + title}, nil
				}
				id, err := requireMCPString(req, "script_id")
				if err != nil {
					return nil, err
				}
				return []string{"appscript", operation, "--", id}, nil
			},
		})
	}
	return result
}
