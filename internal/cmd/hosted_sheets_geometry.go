package cmd

import (
	"errors"
	"strconv"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/sheetsdimension"
)

// errHostedSheetsGeometryArgument marks rejected hosted Sheets geometry inputs.
var errHostedSheetsGeometryArgument = errors.New("invalid Sheets geometry argument")

// hostedSheetsGeometryRecord is the compact per-tool record shared by the one
// BuildArgs closure reused across all four hosted Sheets geometry tools.
type hostedSheetsGeometryRecord struct {
	name      string
	command   string
	rangeArg  string
	sizeArg   string
	mergeFlag bool
}

func hostedSheetsGeometryTools() []mcpToolSpec {
	records := []hostedSheetsGeometryRecord{
		{name: "sheets_merge", command: "merge", rangeArg: "range", mergeFlag: true},
		{name: "sheets_unmerge", command: "unmerge", rangeArg: "range"},
		{name: "sheets_resize_columns", command: "resize-columns", rangeArg: "columns", sizeArg: "width"},
		{name: "sheets_resize_rows", command: "resize-rows", rangeArg: "rows", sizeArg: "height"},
	}
	tools := make([]mcpToolSpec, 0, len(records))
	for _, record := range records {
		tools = append(tools, mcpToolSpec{
			Name:        record.name,
			Service:     "sheets",
			Risk:        mcpRiskWrite,
			Description: hostedSheetsGeometryDescription(record),
			Options:     hostedSheetsGeometryOptions(record),
			BuildArgs:   hostedSheetsGeometryArgs(record),
		})
	}
	return tools
}

func hostedSheetsGeometryDescription(record hostedSheetsGeometryRecord) string {
	switch record.name {
	case "sheets_merge":
		return "Merge cells in a Sheet range. The range must name its sheet explicitly, for example Sheet1!A1:B2. " +
			"WARNING: merging can discard values other than each merged group's upper-left cell; " +
			"this destructive operation requires a destructive grant and human approval."
	case "sheets_unmerge":
		return "Unmerge previously merged cells in a Sheet range. The range must name its sheet explicitly, for example Sheet1!A1:B2."
	case "sheets_resize_columns":
		return "Resize columns to a fixed pixel width (width) or auto-fit them to their contents (auto=true); " +
			"exactly one of width or auto=true must be provided. An unqualified column range (for example A:D) " +
			"defaults to the first tab in the spreadsheet; that first-tab default is preserved."
	case "sheets_resize_rows":
		return "Resize rows to a fixed pixel height (height) or auto-fit them to their contents (auto=true); " +
			"exactly one of height or auto=true must be provided. An unqualified row range (for example 1:10) " +
			"defaults to the first tab in the spreadsheet; that first-tab default is preserved."
	}
	return ""
}

func hostedSheetsGeometryOptions(record hostedSheetsGeometryRecord) []mcp.ToolOption {
	options := []mcp.ToolOption{
		mcp.WithDescription(hostedSheetsGeometryDescription(record)),
		mcp.WithString("spreadsheet_id", mcp.Required()),
		mcp.WithString(record.rangeArg, mcp.Required()),
	}
	if record.mergeFlag {
		options = append(options,
			mcp.WithString("type",
				mcp.Enum("MERGE_ALL", "MERGE_COLUMNS", "MERGE_ROWS"),
				mcp.DefaultString("MERGE_ALL"),
			),
		)
	}
	if record.sizeArg != "" {
		options = append(options,
			mcp.WithInteger(record.sizeArg, mcp.Min(1), mcp.Max(2147483647)),
			mcp.WithBoolean("auto"),
		)
	}
	return options
}

// hostedSheetsGeometryArgs returns the single shared BuildArgs closure used by
// all four hosted Sheets geometry tools. It takes the compact record so one
// closure body serves merge, unmerge, and both resize commands.
func hostedSheetsGeometryArgs(record hostedSheetsGeometryRecord) func(mcp.CallToolRequest) ([]string, error) {
	return func(req mcp.CallToolRequest) ([]string, error) {
		args := req.GetArguments()

		spreadsheetID, err := hostedSheetsLayoutSpreadsheetID(req)
		if err != nil {
			return nil, err
		}

		// Range must be an actual string, non-blank, at most 4096 bytes, with
		// no control characters. The literal text is preserved for argv.
		rawRange, ok := args[record.rangeArg]
		if !ok {
			return nil, errHostedSheetsGeometryArgument
		}
		rangeText, ok := rawRange.(string)
		if !ok {
			return nil, errHostedSheetsGeometryArgument
		}
		if strings.TrimSpace(rangeText) == "" || len(rangeText) > 4096 {
			return nil, errHostedSheetsGeometryArgument
		}
		for _, r := range rangeText {
			if unicode.IsControl(r) {
				return nil, errHostedSheetsGeometryArgument
			}
		}

		argv := make([]string, 0, 7)
		argv = append(argv, "sheets", record.command)

		// Fixed merge type uses a literal --type= before the separator.
		if record.mergeFlag {
			mergeType := "MERGE_ALL"
			rawType, present := args["type"]
			if present {
				typeText, okType := rawType.(string)
				if !okType {
					return nil, errHostedSheetsGeometryArgument
				}
				mergeType = typeText
			}
			normalized, errType := normalizeMergeType(mergeType)
			if errType != nil {
				return nil, errType
			}
			argv = append(argv, "--type="+normalized)
		}

		if record.sizeArg != "" {
			// Resize path: fixed --width= / --height= literal or --auto.
			var pixelSize *int
			rawSize, present := args[record.sizeArg]
			if present {
				size, errSize := validateHostedTabIndexArg(rawSize)
				if errSize != nil {
					return nil, errSize
				}
				if size <= 0 {
					return nil, errHostedSheetsGeometryArgument
				}
				pixelSize = &size
			}
			var auto *bool
			rawAuto, present := args["auto"]
			if present {
				autoValue, okAuto := rawAuto.(bool)
				if !okAuto {
					return nil, errHostedSheetsGeometryArgument
				}
				auto = &autoValue
			}
			// Exactly one sizing mode: a positive size, or auto=true.
			// auto=false with a positive size is valid; true plus a size conflicts;
			// omitting both is an error.
			if auto != nil && *auto && pixelSize != nil {
				return nil, errHostedSheetsGeometryArgument
			}
			if pixelSize == nil && !(auto != nil && *auto) {
				return nil, errHostedSheetsGeometryArgument
			}
			if pixelSize != nil {
				argv = append(argv, "--"+record.sizeArg+"="+strconv.Itoa(*pixelSize))
			} else {
				argv = append(argv, "--auto")
			}
			// Native dimension parse (before the pure Google A1 parse below).
			// cleanRange is used only for pure validation; the original range
			// text is kept in argv. Unqualified ranges resolve to the first tab.
			var span sheetsdimension.Span
			var parseErr error
			if record.sizeArg == "width" {
				span, parseErr = sheetsdimension.ParseColumns(cleanRange(rangeText), "columns")
			} else {
				span, parseErr = sheetsdimension.ParseRows(cleanRange(rangeText), "rows")
			}
			if parseErr != nil {
				return nil, parseErr
			}
			if span.StartIndex < 0 || span.EndIndex > 2147483647 {
				return nil, errHostedSheetsGeometryArgument
			}
		} else {
			// Merge/unmerge ranges must name their sheet explicitly; the pure
			// parser rejects unqualified ranges.
			if _, errRange := parseSheetRange(cleanRange(rangeText), "range"); errRange != nil {
				return nil, errRange
			}
		}

		argv = append(argv, "--", spreadsheetID, rangeText)
		return argv, nil
	}
}
