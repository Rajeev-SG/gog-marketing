package cmd

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// hostedSheetsTools reuses the existing native Sheets handlers for a bounded
// metadata/append/clear surface. Values are always inline literal JSON, never
// stdin or @file references, and no batch proxy,copied validation or structural
// mutation is exposed.
func hostedSheetsTools() []mcpToolSpec {
	return []mcpToolSpec{
		{Name: "sheets_metadata", Service: "sheets", Risk: mcpRiskRead, Description: "Get native Sheets spreadsheet metadata (title, locale, time zone and tabs) for an explicitly enabled Picker-selected spreadsheet. Cell values are not included.", Options: []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			id, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			return []string{"sheets", "metadata", "--", id}, nil
		}},
		{Name: "sheets_append_range", Service: "sheets", Risk: mcpRiskWrite, Description: "Append inline literal JSON values to a Google Sheets range of an explicitly enabled Picker-selected spreadsheet. USER_ENTERED formulas may not import from an unselected file; structural edits, copied validation and raw batch requests are not supported.", Options: []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("range", mcp.Required()), mcp.WithString("values_json", mcp.Required()), mcp.WithString("input", mcp.Enum("RAW", "USER_ENTERED"), mcp.DefaultString("USER_ENTERED")), mcp.WithString("insert", mcp.Enum("OVERWRITE", "INSERT_ROWS"))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			rangeSpec, err := requireMCPString(req, "range")
			if err != nil {
				return nil, err
			}
			valuesJSON, err := requireMCPLiteralValuesJSON(req, "values_json")
			if err != nil {
				return nil, err
			}
			input := strings.TrimSpace(req.GetString("input", "USER_ENTERED"))
			if input == "" {
				input = "USER_ENTERED"
			}
			args := []string{"sheets", "append", "--values-json", valuesJSON, "--input", input}
			if insert := strings.TrimSpace(req.GetString("insert", "")); insert != "" {
				args = append(args, "--insert", insert)
			}
			return append(args, "--", spreadsheetID, rangeSpec), nil
		}},
		{Name: "sheets_clear_range", Service: "sheets", Risk: mcpRiskWrite, Description: "Clear values in a Google Sheets range for an explicitly enabled Picker-selected spreadsheet. Requires destructive permission and does not delete rows, tabs or formats.", Options: []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("range", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			rangeSpec, err := requireMCPString(req, "range")
			if err != nil {
				return nil, err
			}
			return []string{"sheets", "clear", "--", spreadsheetID, rangeSpec}, nil
		}},
	}
}
