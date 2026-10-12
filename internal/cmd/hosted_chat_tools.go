package cmd

import (
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedChatTools() []mcpToolSpec {
	return []mcpToolSpec{
		{Name: "chat_spaces_list", Service: "chat", Risk: mcpRiskRead, Description: "List native Chat spaces with explicit account/tool permission. No message history is read.", Options: []mcp.ToolOption{mcp.WithInteger("max", mcp.Min(1), mcp.Max(1000)), mcp.WithString("page"), mcp.WithBoolean("all"), mcp.WithBoolean("fail_empty")}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			args := []string{"chat", "spaces", "list", "--max=" + strconv.Itoa(req.GetInt("max", 100))}
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
		}},
		{Name: "chat_spaces_create", Service: "chat", Risk: mcpRiskWrite, Description: "Create a native Chat space using audit-free space scope. Member setup is a separate pending capability.", Options: []mcp.ToolOption{mcp.WithString("display_name", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			name, err := requireMCPString(req, "display_name")
			if err != nil {
				return nil, err
			}
			return []string{"chat", "spaces", "create", "--", name}, nil
		}},
		{Name: "chat_messages_send", Service: "chat", Risk: mcpRiskWrite, Description: "Send native Chat text to an explicit space with chat.messages.create and account/tool grants; no history/update/delete scope or attachment upload is used.", Options: []mcp.ToolOption{mcp.WithString("space_name", mcp.Required()), mcp.WithString("text", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			name, err := requireMCPString(req, "space_name")
			if err != nil {
				return nil, err
			}
			text, err := requireMCPString(req, "text")
			if err != nil {
				return nil, err
			}
			return []string{"chat", "messages", "send", "--text=" + text, "--", name}, nil
		}},
	}
}
