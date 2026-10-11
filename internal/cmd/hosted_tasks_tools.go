package cmd

import "github.com/mark3labs/mcp-go/mcp"

func hostedTasksTools() []mcpToolSpec {
	return []mcpToolSpec{
		{Name: "tasks_lists", Service: "tasks", Risk: mcpRiskRead, Description: "List task lists in the explicitly authorized Google account.", BuildArgs: func(mcp.CallToolRequest) ([]string, error) {
			return []string{"tasks", "lists", "list"}, nil
		}},
		{Name: "tasks_lists_create", Service: "tasks", Risk: mcpRiskWrite, Description: "Create a task list in the explicitly authorized Google account.", Options: []mcp.ToolOption{mcp.WithString("title", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			title, err := requireMCPString(req, "title")
			if err != nil {
				return nil, err
			}
			return []string{"tasks", "lists", "create", "--", title}, nil
		}},
	}
}
