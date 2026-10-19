package cmd

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// hostedDocsSegmentReadTools returns the read-only docs segment tools:
// header list, footer list, and suggestions list.
func hostedDocsSegmentReadTools() []mcpToolSpec {
	records := []struct {
		name        string
		command     string
		description string
	}{
		{
			name:        "docs_header_list",
			command:     "header",
			description: "List headers of the selected docs document. By default headers are returned across all tabs; an optional tab restricts the listing to that tab.",
		},
		{
			name:        "docs_footer_list",
			command:     "footer",
			description: "List footers of the selected docs document. By default footers are returned across all tabs; an optional tab restricts the listing to that tab.",
		},
		{
			name:        "docs_suggestions_list",
			command:     "suggestions",
			description: "List suggestions of the selected docs document. By default the first tab is used; an optional tab resolves the listing within the selected document.",
		},
	}

	specs := make([]mcpToolSpec, 0, len(records))
	for _, rec := range records {
		command := rec.command
		spec := mcpToolSpec{
			Name:        rec.name,
			Service:     "docs",
			Risk:        mcpRiskRead,
			Description: rec.description,
			Options: []mcp.ToolOption{
				mcp.WithString("document_id", mcp.Required()),
				mcp.WithString("tab"),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "document_id")
				if err != nil {
					return nil, err
				}
				if id != req.GetString("document_id", "") {
					return nil, errHostedDocsInventoryID
				}
				validationErr := validateHostedDocsInventoryID(id)
				if validationErr != nil {
					return nil, validationErr
				}

				argv := []string{"docs", command, "list"}

				raw, ok := req.GetArguments()["tab"]
				if ok {
					tab, tabErr := validateHostedTabLabelArg(raw, false)
					if tabErr != nil {
						return nil, tabErr
					}
					argv = append(argv, "--tab="+tab)
				}

				argv = append(argv, "--", id)
				return argv, nil
			},
		}
		specs = append(specs, spec)
	}
	return specs
}
