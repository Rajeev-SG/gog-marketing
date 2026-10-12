package cmd

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedMeetSpaceTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 4)
	for _, operation := range []string{"create", "get", "update", "end"} {
		risk := mcpRiskWrite
		options := []mcp.ToolOption{}
		if operation == "get" {
			risk = mcpRiskRead
		}
		if operation != "create" {
			options = append(options, mcp.WithString("space_name", mcp.Required(), mcp.Description("Canonical spaces/... resource name, not meeting-code alias")))
		}
		if operation == "create" || operation == "update" {
			options = append(options, mcp.WithString("access", mcp.Enum("open", "trusted", "restricted")), mcp.WithString("entry_point", mcp.Enum("all", "creator-only")))
		}
		result = append(result, mcpToolSpec{
			Name: "meet_space_" + operation, Service: "meet", Risk: risk, Description: "Run native Meet space " + operation + " for the connected account with approved Meet permissions. Ending requires destructive permission; browser opening is disabled.", Options: options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				args := []string{"meet", operation}
				for _, key := range []string{"access", "entry_point"} {
					if _, present := req.GetArguments()[key]; present {
						args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+req.GetString(key, ""))
					}
				}
				if operation != "create" {
					name, err := requireMCPString(req, "space_name")
					if err != nil {
						return nil, err
					}
					args = append(args, "--", name)
				}
				return args, nil
			},
		})
	}
	return result
}
