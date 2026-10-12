package cmd

import "github.com/mark3labs/mcp-go/mcp"

func hostedPeopleMeTool() mcpToolSpec {
	return mcpToolSpec{Name: "people_me", Service: "people", Risk: mcpRiskRead, Description: "Read the connected account's native People self-profile with explicit account/tool permission and approved profile scope. No directory access or local-token fallback.", BuildArgs: func(mcp.CallToolRequest) ([]string, error) { return []string{"people", "me"}, nil }}
}
