package cmd

import (
	"errors"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedSheetsCellReadArgument = errors.New("invalid Sheets cell metadata argument")

type hostedSheetsCellReadToolRecord struct {
	name              string
	command           []string
	description       string
	supportsEffective bool
}

func hostedSheetsCellReadTools() []mcpToolSpec {
	records := []hostedSheetsCellReadToolRecord{
		{
			name:        "sheets_notes",
			command:     []string{"sheets", "notes"},
			description: "Read cell notes from one granted spreadsheet range.",
		},
		{
			name:        "sheets_links_get",
			command:     []string{"sheets", "links", "get"},
			description: "Read cell hyperlinks from one granted spreadsheet range; links are returned, not fetched.",
		},
		{
			name:              "sheets_read_format",
			command:           []string{"sheets", "read-format"},
			description:       "Read cell display formats from one granted spreadsheet range; supports effective formats.",
			supportsEffective: true,
		},
		{
			name:        "sheets_validation_get",
			command:     []string{"sheets", "validation", "get"},
			description: "Read cell validation rules, including native table-derived rules, from one granted spreadsheet range.",
		},
	}

	// buildCellReadArgs is the ONE shared BuildArgs body used by every record
	// in this read-only cell metadata tool family.
	buildCellReadArgs := func(command []string, supportsEffective bool, req mcp.CallToolRequest) ([]string, error) {
		id, err := requireMCPString(req, "spreadsheet_id")
		if err != nil {
			return nil, err
		}
		if id != req.GetString("spreadsheet_id", "") {
			return nil, errHostedSheetsCellReadArgument
		}
		validationErr := validateHostedSheetsInventoryID(id)
		if validationErr != nil {
			return nil, validationErr
		}

		rawRange, ok := req.GetArguments()["range"]
		if !ok || rawRange == nil {
			return nil, errHostedSheetsCellReadArgument
		}
		rangeValue, ok := rawRange.(string)
		if !ok {
			return nil, errHostedSheetsCellReadArgument
		}
		if strings.TrimSpace(rangeValue) == "" {
			return nil, errHostedSheetsCellReadArgument
		}
		if len(rangeValue) > 4096 {
			return nil, errHostedSheetsCellReadArgument
		}
		if strings.IndexFunc(rangeValue, unicode.IsControl) >= 0 {
			return nil, errHostedSheetsCellReadArgument
		}

		args := make([]string, 0, len(command)+5)
		args = append(args, command...)
		if supportsEffective {
			rawEffective, present := req.GetArguments()["effective"]
			if present {
				if rawEffective == nil {
					return nil, errHostedSheetsCellReadArgument
				}
				effective, ok := rawEffective.(bool)
				if !ok {
					return nil, errHostedSheetsCellReadArgument
				}
				if effective {
					args = append(args, "--effective")
				}
			}
		}
		args = append(args, "--", id, rangeValue)
		return args, nil
	}

	tools := make([]mcpToolSpec, 0, len(records))
	for _, record := range records {
		record := record
		command := record.command
		options := []mcp.ToolOption{
			mcp.WithString("spreadsheet_id", mcp.Required()),
			mcp.WithString("range", mcp.Required()),
		}
		if record.supportsEffective {
			options = append(options, mcp.WithBoolean("effective"))
		}
		tools = append(tools, mcpToolSpec{
			Name:        record.name,
			Service:     "sheets",
			Risk:        mcpRiskRead,
			Description: record.description,
			Options:     options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				return buildCellReadArgs(command, record.supportsEffective, req)
			},
		})
	}
	return tools
}
