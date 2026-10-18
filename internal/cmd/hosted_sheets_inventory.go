package cmd

import (
	"errors"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedSheetsInventoryID = errors.New("invalid spreadsheet id")

// hostedSheetsInventoryIDPattern matches Sheets spreadsheet identifiers:
// opaque ASCII tokens of 1-256 characters. It rejects separators that could
// be interpreted as URLs or path traversal; the literal "--" argument
// separator prevents flag interpretation.
var hostedSheetsInventoryIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

// hostedSheetsInventoryBinding is one fixed read-only inventory tool: the
// sheets CLI list subcommand it wraps plus its description.
type hostedSheetsInventoryBinding struct {
	name        string
	subcommand  string
	description string
}

// hostedSheetsInventoryBindings is the static list of read-only sheets
// inventory tools. New inventory read tools are added here, never wired
// through a generic path.
var hostedSheetsInventoryBindings = []hostedSheetsInventoryBinding{
	{
		name:        "sheets_chart_list",
		subcommand:  "chart",
		description: "List native charts for one explicitly granted Google spreadsheet under an exact account/tool/file read grant.",
	},
	{
		name:        "sheets_named_ranges_list",
		subcommand:  "named-ranges",
		description: "List named ranges for one explicitly granted Google spreadsheet under an exact account/tool/file read grant.",
	},
	{
		name:        "sheets_table_list",
		subcommand:  "table",
		description: "List tables for one explicitly granted Google spreadsheet under an exact account/tool/file read grant.",
	},
	{
		name:        "sheets_banding_list",
		subcommand:  "banding",
		description: "List banded ranges for one explicitly granted Google spreadsheet under an exact account/tool/file read grant.",
	},
	{
		name:        "sheets_conditional_format_list",
		subcommand:  "conditional-format",
		description: "List conditional format rules for one explicitly granted Google spreadsheet under an exact account/tool/file read grant.",
	},
}

// hostedSheetsInventoryTools registers the five fixed read-only inventory
// tools for the sheets service. Each builds an existing sheets CLI list
// command against a single selected spreadsheet id. The caller never chooses
// the host, method, path or command shape, and there is no generic proxy path.
func hostedSheetsInventoryTools() []mcpToolSpec {
	build := func(subcommand string) func(mcp.CallToolRequest) ([]string, error) {
		return func(req mcp.CallToolRequest) ([]string, error) {
			id, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			if id != req.GetString("spreadsheet_id", "") {
				return nil, errHostedSheetsInventoryID
			}
			if err := validateHostedSheetsInventoryID(id); err != nil {
				return nil, err
			}

			return []string{"sheets", subcommand, "list", "--", id}, nil
		}
	}

	specs := make([]mcpToolSpec, 0, len(hostedSheetsInventoryBindings))
	for _, binding := range hostedSheetsInventoryBindings {
		specs = append(specs, mcpToolSpec{
			Name:        binding.name,
			Service:     "sheets",
			Risk:        mcpRiskRead,
			Description: binding.description,
			Options:     []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required())},
			BuildArgs:   build(binding.subcommand),
		})
	}

	return specs
}

// validateHostedSheetsInventoryID enforces the selected-spreadsheet contract
// shared by the sheets inventory tools: an opaque 1-256 character
// alphanumeric identifier with no URL or path-traversal syntax.
func validateHostedSheetsInventoryID(id string) error {
	if !hostedSheetsInventoryIDPattern.MatchString(id) {
		return errHostedSheetsInventoryID
	}

	return nil
}
