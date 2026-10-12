package cmd

import "github.com/mark3labs/mcp-go/mcp"

func hostedSearchConsoleReadTools() []mcpToolSpec {
	return []mcpToolSpec{
		{Name: "searchconsole_sites_get", Service: "searchconsole", Risk: mcpRiskRead, Description: "Get metadata for an explicitly enabled Search Console site.", Options: []mcp.ToolOption{mcp.WithString("site_url", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			site, err := requireMCPString(req, "site_url")
			if err != nil {
				return nil, err
			}
			return []string{"searchconsole", "sites", "get", "--", site}, nil
		}},
		{Name: "searchconsole_sitemaps_get", Service: "searchconsole", Risk: mcpRiskRead, Description: "Read one sitemap belonging to an explicitly enabled Search Console site.", Options: []mcp.ToolOption{mcp.WithString("site_url", mcp.Required()), mcp.WithString("feed_path", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			site, err := requireMCPString(req, "site_url")
			if err != nil {
				return nil, err
			}
			feed, err := requireMCPString(req, "feed_path")
			if err != nil {
				return nil, err
			}
			return []string{"searchconsole", "sitemaps", "get", "--", site, feed}, nil
		}},
	}
}
