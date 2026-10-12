package cmd

import "github.com/mark3labs/mcp-go/mcp"

// Fixed schemas invoke the existing Slides handlers, not a second integration.
func hostedSlidesTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name: "slides_list_slides", Service: "slides", Risk: mcpRiskRead,
			Description: "List slide object IDs and skipped state in a selected presentation.",
			Options:     []mcp.ToolOption{mcp.WithString("presentation_id", mcp.Required())},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "presentation_id")
				if err != nil {
					return nil, err
				}
				return []string{"slides", "list-slides", "--", id}, nil
			},
		},
		{
			Name: "slides_update_notes", Service: "slides", Risk: mcpRiskWrite,
			Description: "Replace speaker notes on a slide in a selected presentation. Requires destructive permission; empty notes clear existing notes.",
			Options:     []mcp.ToolOption{mcp.WithString("presentation_id", mcp.Required()), mcp.WithString("slide_id", mcp.Required()), mcp.WithString("notes", mcp.Required())},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "presentation_id")
				if err != nil {
					return nil, err
				}
				slide, err := requireMCPString(req, "slide_id")
				if err != nil {
					return nil, err
				}
				return []string{"slides", "update-notes", "--notes=" + req.GetString("notes", ""), "--", id, slide}, nil
			},
		},
	}
}
