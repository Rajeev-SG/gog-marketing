package cmd

import (
	"errors"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedSpreadsheetTitle = errors.New("invalid spreadsheet title")

func hostedSheetsCreateTools() []mcpToolSpec {
	return []mcpToolSpec{{
		Name: "sheets_create_blank", Service: "sheets", Risk: mcpRiskWrite,
		Description: "Create a blank spreadsheet by literal title under an explicit account-level write-tool grant. No parent, template or source is accepted. The returned file is not automatically granted for later reads or writes.",
		Options:     []mcp.ToolOption{mcp.WithString("title", mcp.Required())},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			title, err := requireMCPString(req, "title")
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(title) == "" || len(title) > 512 || strings.ContainsFunc(title, unicode.IsControl) {
				return nil, errHostedSpreadsheetTitle
			}
			return []string{"sheets", "create", "--", title}, nil
		},
	}}
}
