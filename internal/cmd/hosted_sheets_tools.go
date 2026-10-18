package cmd

import (
	"errors"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedSheetsTextInput = errors.New("sheets text input is invalid or too long")

func requireHostedSheetsText(req mcp.CallToolRequest, key string) (string, error) {
	value, err := requireMCPString(req, key)
	if err != nil {
		return "", err
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", errHostedSheetsTextInput
		}
	}
	if len(value) > 512 {
		return "", errHostedSheetsTextInput
	}
	return value, nil
}

// hostedSheetsTools reuses the existing native Sheets handlers for a bounded
// metadata/value/tab/number-format surface. Values are always inline literal
// JSON, never stdin or @file references; no generic batch proxy or copied
// validation is exposed.
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
		{Name: "sheets_add_tab", Service: "sheets", Risk: mcpRiskWrite, Description: "Add one empty tab to an explicitly enabled Picker-selected spreadsheet. Does not create a spreadsheet.", Options: []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("tab_name", mcp.Required()), mcp.WithInteger("index", mcp.Min(0), mcp.Max(999))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			tabName, err := requireHostedSheetsText(req, "tab_name")
			if err != nil {
				return nil, err
			}
			args := []string{"sheets", "add-tab"}
			if _, present := req.GetArguments()["index"]; present {
				args = append(args, "--index", strconv.Itoa(req.GetInt("index", -1)))
			}
			return append(args, "--", spreadsheetID, tabName), nil
		}},
		{Name: "sheets_rename_tab", Service: "sheets", Risk: mcpRiskWrite, Description: "Rename an existing tab in an explicitly enabled Picker-selected spreadsheet by its current title.", Options: []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("old_name", mcp.Required()), mcp.WithString("new_name", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			oldName, err := requireHostedSheetsText(req, "old_name")
			if err != nil {
				return nil, err
			}
			newName, err := requireHostedSheetsText(req, "new_name")
			if err != nil {
				return nil, err
			}
			return []string{"sheets", "rename-tab", "--", spreadsheetID, oldName, newName}, nil
		}},
		{Name: "sheets_delete_tab", Service: "sheets", Risk: mcpRiskWrite, Description: "Delete one existing tab by title in an explicitly enabled Picker-selected spreadsheet. Requires destructive permission; tab contents cannot be recovered through this tool.", Options: []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("tab_name", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			tabName, err := requireHostedSheetsText(req, "tab_name")
			if err != nil {
				return nil, err
			}
			return []string{"sheets", "delete-tab", "--", spreadsheetID, tabName}, nil
		}},
		{Name: "sheets_number_format_range", Service: "sheets", Risk: mcpRiskWrite, Description: "Apply a fixed enum of Sheets number formats to a range in an explicitly enabled Picker-selected spreadsheet. Arbitrary cell-format JSON and other formatting are not supported.", Options: []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("range", mcp.Required()), mcp.WithString("type", mcp.Enum("NUMBER", "CURRENCY", "PERCENT", "DATE", "TIME", "DATE_TIME", "SCIENTIFIC", "TEXT"), mcp.DefaultString("NUMBER")), mcp.WithString("pattern")}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			rangeSpec, err := requireMCPString(req, "range")
			if err != nil {
				return nil, err
			}
			formatType := req.GetString("type", "NUMBER")
			if formatType == "" {
				formatType = "NUMBER"
			}
			args := []string{"sheets", "number-format", "--type", formatType}
			if pattern, present := req.GetArguments()["pattern"]; present {
				text, isText := pattern.(string)
				if !isText || strings.TrimSpace(text) == "" || len(text) > 256 {
					return nil, errHostedSheetsTextInput
				}
				args = append(args, "--pattern", text)
			}
			return append(args, "--", spreadsheetID, rangeSpec), nil
		}},
	}
}
