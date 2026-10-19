package cmd

import (
	"errors"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

var errChartIDPositive = errors.New("chart_id must be positive")

func hostedSheetsDetailReadTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name:        "sheets_chart_get",
			Service:     "sheets",
			Risk:        mcpRiskRead,
			Description: "Read a single chart definition from a hosted Google Sheets spreadsheet by chart ID.",
			Options: []mcp.ToolOption{
				mcp.WithString("spreadsheet_id", mcp.Required()),
				mcp.WithInteger("chart_id", mcp.Required(), mcp.Min(1), mcp.Max(2147483647)),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
				if err != nil {
					return nil, err
				}
				if spreadsheetID != req.GetString("spreadsheet_id", "") {
					return nil, errHostedSheetsInventoryID
				}
				if validationErr := validateHostedSheetsInventoryID(spreadsheetID); validationErr != nil {
					return nil, validationErr
				}
				chartID, err := validateHostedTabIndexArg(req.GetArguments()["chart_id"])
				if err != nil {
					return nil, err
				}
				if chartID == 0 {
					return nil, errChartIDPositive
				}
				return []string{"sheets", "chart", "get", "--", spreadsheetID, strconv.Itoa(chartID)}, nil
			},
		},
		{
			Name:        "sheets_named_ranges_get",
			Service:     "sheets",
			Risk:        mcpRiskRead,
			Description: "Read a single named range from a hosted Google Sheets spreadsheet by name or ID.",
			Options: []mcp.ToolOption{
				mcp.WithString("spreadsheet_id", mcp.Required()),
				mcp.WithString("name_or_id", mcp.Required()),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
				if err != nil {
					return nil, err
				}
				if spreadsheetID != req.GetString("spreadsheet_id", "") {
					return nil, errHostedSheetsInventoryID
				}
				if validationErr := validateHostedSheetsInventoryID(spreadsheetID); validationErr != nil {
					return nil, validationErr
				}
				nameOrID, err := validateHostedTabLabelArg(req.GetArguments()["name_or_id"], true)
				if err != nil {
					return nil, err
				}
				return []string{"sheets", "named-ranges", "get", "--", spreadsheetID, nameOrID}, nil
			},
		},
		{
			Name:        "sheets_table_get",
			Service:     "sheets",
			Risk:        mcpRiskRead,
			Description: "Read a single table definition from a hosted Google Sheets spreadsheet by table ID or name.",
			Options: []mcp.ToolOption{
				mcp.WithString("spreadsheet_id", mcp.Required()),
				mcp.WithString("table_id", mcp.Required()),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
				if err != nil {
					return nil, err
				}
				if spreadsheetID != req.GetString("spreadsheet_id", "") {
					return nil, errHostedSheetsInventoryID
				}
				if validationErr := validateHostedSheetsInventoryID(spreadsheetID); validationErr != nil {
					return nil, validationErr
				}
				tableID, err := validateHostedTabLabelArg(req.GetArguments()["table_id"], true)
				if err != nil {
					return nil, err
				}
				return []string{"sheets", "table", "get", "--", spreadsheetID, tableID}, nil
			},
		},
	}
}
