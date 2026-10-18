package cmd

import (
	"errors"
	"math"
	"regexp"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

// errHostedSheetsLayoutInput rejects every layout argument that does not
// match the fixed contract: a canonical spreadsheet id, a clean sheet or tab
// label, and integral counts in 0..10000.
var errHostedSheetsLayoutInput = errors.New("invalid hosted sheets layout input")

// hostedSheetsLayoutLabelPattern admits ordinary sheet and tab titles while
// rejecting every Unicode control-category character.
var hostedSheetsLayoutLabelPattern = regexp.MustCompile(`^[^\p{C}]+$`)

// hostedSheetsLayoutSpreadsheetID validates the canonical spreadsheet id for
// the layout tools: no raw trimming, and the shared opaque identifier pattern
// used by the sheets inventory tools.
func hostedSheetsLayoutSpreadsheetID(req mcp.CallToolRequest) (string, error) {
	id, err := requireMCPString(req, "spreadsheet_id")
	if err != nil {
		return "", err
	}
	if id != req.GetString("spreadsheet_id", "") || !hostedSheetsInventoryIDPattern.MatchString(id) {
		return "", errHostedSheetsLayoutInput
	}

	return id, nil
}

// hostedSheetsLayoutLabel validates a sheet or tab label on top of the shared
// typed-text check: no raw leading or trailing whitespace, no Unicode control
// characters, and at most 512 UTF-8 bytes.
func hostedSheetsLayoutLabel(req mcp.CallToolRequest, field string) (string, error) {
	value, err := requireHostedSheetsText(req, field)
	if err != nil {
		return "", err
	}
	if value != req.GetString(field, "") || len(value) > 512 || !hostedSheetsLayoutLabelPattern.MatchString(value) {
		return "", errHostedSheetsLayoutInput
	}

	return value, nil
}

// hostedSheetsLayoutInteger reads one optional layout count. Only an actual
// integer or an integral float64 is accepted, bounded to 0..10000; fractional
// values, strings and other types are rejected outright, and there is no
// truncation. A missing field reports ok=false without an error so callers
// can distinguish "omitted stays unchanged" from invalid input.
func hostedSheetsLayoutInteger(req mcp.CallToolRequest, field string) (int, bool, error) {
	raw, ok := req.GetArguments()[field]
	if !ok {
		return 0, false, nil
	}

	if raw == nil {
		return 0, false, errHostedSheetsLayoutInput
	}

	switch value := raw.(type) {
	case int:
		if value < 0 || value > 10000 {
			return 0, false, errHostedSheetsLayoutInput
		}

		return value, true, nil
	case int64:
		if value < 0 || value > 10000 {
			return 0, false, errHostedSheetsLayoutInput
		}

		return int(value), true, nil
	case float64:
		if math.Trunc(value) != value || value < 0 || value > 10000 {
			return 0, false, errHostedSheetsLayoutInput
		}

		return int(value), true, nil
	default:
		return 0, false, errHostedSheetsLayoutInput
	}
}

// hostedSheetsLayoutTools registers the two fixed native sheets layout write
// tools. Each wraps one existing sheets CLI subcommand against one explicitly
// selected spreadsheet; the caller never chooses the host, method, path or
// command shape, and there is no generic proxy path and no implicit
// first-tab fallback.
func hostedSheetsLayoutTools() []mcpToolSpec {
	buildFreeze := func(req mcp.CallToolRequest) ([]string, error) {
		id, err := hostedSheetsLayoutSpreadsheetID(req)
		if err != nil {
			return nil, err
		}

		sheet, err := hostedSheetsLayoutLabel(req, "sheet")
		if err != nil {
			return nil, err
		}

		rows, rowsOK, err := hostedSheetsLayoutInteger(req, "rows")
		if err != nil {
			return nil, err
		}

		cols, colsOK, err := hostedSheetsLayoutInteger(req, "cols")
		if err != nil {
			return nil, err
		}

		if !rowsOK && !colsOK {
			return nil, errHostedSheetsLayoutInput
		}

		argv := []string{"sheets", "freeze", "--sheet=" + sheet}
		if rowsOK {
			argv = append(argv, "--rows="+strconv.Itoa(rows))
		}
		if colsOK {
			argv = append(argv, "--cols="+strconv.Itoa(cols))
		}

		return append(argv, "--", id), nil
	}

	buildReorder := func(req mcp.CallToolRequest) ([]string, error) {
		id, err := hostedSheetsLayoutSpreadsheetID(req)
		if err != nil {
			return nil, err
		}

		tab, err := hostedSheetsLayoutLabel(req, "tab")
		if err != nil {
			return nil, err
		}

		to, ok, err := hostedSheetsLayoutInteger(req, "to")
		if err != nil {
			return nil, err
		}

		if !ok {
			return nil, errHostedSheetsLayoutInput
		}

		return []string{"sheets", "reorder-tab", "--tab=" + tab, "--to=" + strconv.Itoa(to), "--", id}, nil
	}

	return []mcpToolSpec{
		{
			Name:        "sheets_freeze",
			Service:     "sheets",
			Risk:        mcpRiskWrite,
			Description: "Freeze or unfreeze rows and columns on one explicitly named tab of one explicitly granted Google spreadsheet under an exact account/tool/file write grant with human approval. Reversible layout only: no arbitrary batch requests, tab deletion, duplication, or data-source operations.",
			Options: []mcp.ToolOption{
				mcp.WithString("spreadsheet_id", mcp.Required()),
				mcp.WithString("sheet", mcp.Required()),
				mcp.WithInteger("rows", mcp.Min(0), mcp.Max(10000)),
				mcp.WithInteger("cols", mcp.Min(0), mcp.Max(10000)),
			},
			BuildArgs: buildFreeze,
		},
		{
			Name:        "sheets_reorder_tab",
			Service:     "sheets",
			Risk:        mcpRiskWrite,
			Description: "Move one explicitly named or numeric tab to a fixed 0-based position in one explicitly granted Google spreadsheet under an exact account/tool/file write grant with human approval. Reversible layout only: no arbitrary batch requests, tab deletion, duplication, or data-source operations.",
			Options: []mcp.ToolOption{
				mcp.WithString("spreadsheet_id", mcp.Required()),
				mcp.WithString("tab", mcp.Required()),
				mcp.WithInteger("to", mcp.Required(), mcp.Min(0), mcp.Max(10000)),
			},
			BuildArgs: buildReorder,
		},
	}
}
