package cmd

import (
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedGroupsListTool() mcpToolSpec {
	return mcpToolSpec{Name: "groups_list", Service: "groups", Risk: mcpRiskRead, Description: "List native Cloud Identity groups for the explicitly connected Workspace account. Requires cloud-identity.groups.readonly and provider account privileges; consumer accounts are unsupported.", Options: []mcp.ToolOption{mcp.WithInteger("max", mcp.Min(1), mcp.Max(1000)), mcp.WithString("page"), mcp.WithBoolean("all"), mcp.WithBoolean("fail_empty")}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		args := []string{"groups", "list", "--max=" + strconv.Itoa(req.GetInt("max", 100))}
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
		return args, nil
	}}
}
