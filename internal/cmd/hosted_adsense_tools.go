package cmd

import (
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedAdSenseAccountTools() []mcpToolSpec {
	return []mcpToolSpec{
		{Name: "adsense_accounts_list", Service: "adsense", Risk: mcpRiskRead, Description: "List native AdSense accounts for the connected Google account. Requires adsense.readonly; no Google Ads permission or mutations.", Options: []mcp.ToolOption{mcp.WithInteger("max", mcp.Min(1), mcp.Max(100)), mcp.WithString("page"), mcp.WithBoolean("all"), mcp.WithBoolean("fail_empty")}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			args := []string{"adsense", "accounts", "list", "--max=" + strconv.Itoa(req.GetInt("max", 50))}
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
		{Name: "adsense_accounts_get", Service: "adsense", Risk: mcpRiskRead, Description: "Read a canonical AdSense account through the existing native client with explicit account/tool permission.", Options: []mcp.ToolOption{mcp.WithString("account_name", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			name, err := requireMCPString(req, "account_name")
			if err != nil {
				return nil, err
			}
			return []string{"adsense", "accounts", "get", "--", name}, nil
		}},
	}
}
