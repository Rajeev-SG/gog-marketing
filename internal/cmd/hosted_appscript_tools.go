package cmd

import (
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedAppScriptProjectTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 5)
	for _, operation := range []string{"get", "content", "create"} {
		risk := mcpRiskRead
		options := []mcp.ToolOption{mcp.WithString("script_id", mcp.Required())}
		if operation == "create" {
			risk = mcpRiskWrite
			options = []mcp.ToolOption{mcp.WithString("title", mcp.Required())}
		}
		result = append(result, mcpToolSpec{
			Name: "appscript_" + operation, Service: "appscript", Risk: risk, Description: "Run native Apps Script project " + operation + " with explicit account/tool grants and approved project scopes. Script execution is a separate capability; local downloads use the native pull command.", Options: options,
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

	for _, operation := range []string{"deployments", "versions"} {
		operation := operation
		result = append(result, mcpToolSpec{
			Name: "appscript_" + operation, Service: "appscript", Risk: mcpRiskRead,
			Description: "Read native Apps Script " + operation + " by explicit script ID in the authorised Google account, under an account-level exact tool grant. No project asset grant is implied. This tool does not execute or mutate a project.",
			Options: []mcp.ToolOption{
				mcp.WithString("script_id", mcp.Required()),
				mcp.WithInteger("max", mcp.Min(1), mcp.Max(100)),
				mcp.WithString("page"),
				mcp.WithBoolean("all"),
				mcp.WithBoolean("fail_empty"),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "script_id")
				if err != nil {
					return nil, err
				}

				args := []string{"appscript", operation, "--max=" + strconv.Itoa(req.GetInt("max", 100))}
				if page := req.GetString("page", ""); page != "" {
					args = append(args, "--page="+page)
				}
				for _, key := range []string{"all", "fail_empty"} {
					if req.GetBool(key, false) {
						flag := key
						if key == "fail_empty" {
							flag = "fail-empty"
						}
						args = append(args, "--"+flag)
					}
				}

				return append(args, "--", id), nil
			},
		})
	}

	result = append(result, hostedAppScriptManagementTools()...)
	return append(result, hostedAppScriptExecutionTool())
}
