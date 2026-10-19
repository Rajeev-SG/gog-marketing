package cmd

import (
	"errors"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedSheetsRangeNoteArgument = errors.New("invalid Sheets range/note argument")

func hostedSheetsRangeNoteTools() []mcpToolSpec {
	records := []struct {
		name, description string
		options           []mcp.ToolOption
	}{
		{"sheets_named_ranges_add", "Create a named range in one explicitly granted spreadsheet using a sheet-qualified A1 range and exact write-tool grant with human approval.", []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("name", mcp.Required()), mcp.WithString("range", mcp.Required())}},
		{"sheets_named_ranges_update", "Update a named range by exact ID or case-insensitive name in one explicitly granted spreadsheet. Provide a new name, sheet-qualified range, or both; omitted fields stay unchanged. Requires write-tool grant and human approval.", []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("name_or_id", mcp.Required()), mcp.WithString("name"), mcp.WithString("range")}},
		{"sheets_named_ranges_delete", "Delete a named-range definition by exact ID or case-insensitive name in one explicitly granted spreadsheet. Requires destructive permission, write-tool grant and human approval.", []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("name_or_id", mcp.Required())}},
		{"sheets_update_note", "Set literal inline notes on a sheet-qualified range in one explicitly granted spreadsheet. Empty notes clear and additionally require destructive permission. No note-file input or formula evaluation; returns the native mutation summary, not server readback. Requires write-tool grant and human approval.", []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required()), mcp.WithString("range", mcp.Required()), mcp.WithString("note", mcp.Required())}},
	}
	tools := make([]mcpToolSpec, 0, len(records))
	for _, record := range records {
		tools = append(tools, mcpToolSpec{Name: record.name, Service: "sheets", Risk: mcpRiskWrite, Description: record.description, Options: record.options, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) { return hostedSheetsRangeNoteArgs(record.name, req) }})
	}
	return tools
}

func hostedSheetsRangeNoteText(req mcp.CallToolRequest, key string, required bool) (string, error) {
	value, ok := req.GetArguments()[key].(string)
	if !ok || len(value) > 4096 || strings.IndexFunc(value, unicode.IsControl) >= 0 || required && strings.TrimSpace(value) == "" {
		return "", errHostedSheetsRangeNoteArgument
	}
	return value, nil
}

func hostedSheetsRangeNoteArgs(tool string, req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedSheetsLayoutSpreadsheetID(req)
	if err != nil {
		return nil, err
	}
	switch tool {
	case "sheets_named_ranges_update":
		return hostedSheetsNamedRangeUpdateArgs(id, req)
	case "sheets_named_ranges_delete":
		selector, readErr := hostedSheetsRangeNoteText(req, "name_or_id", true)
		if readErr != nil {
			return nil, readErr
		}
		return []string{"sheets", "named-ranges", "delete", "--", id, selector}, nil
	case "sheets_named_ranges_add", "sheets_update_note":
		target, readErr := hostedSheetsRangeNoteText(req, "range", true)
		if readErr != nil {
			return nil, readErr
		}
		if _, parseErr := parseSheetRange(cleanRange(strings.TrimSpace(target)), "range"); parseErr != nil {
			return nil, parseErr
		}
		if tool == "sheets_named_ranges_add" {
			name, nameErr := hostedSheetsRangeNoteText(req, "name", true)
			if nameErr != nil {
				return nil, nameErr
			}
			return []string{"sheets", "named-ranges", "add", "--", id, name, target}, nil
		}
		note, ok := req.GetArguments()["note"].(string)
		if !ok || len(note) > 262144 {
			return nil, errHostedSheetsRangeNoteArgument
		}
		return []string{"sheets", "update-note", "--note=" + note, "--", id, target}, nil
	default:
		return nil, errHostedSheetsRangeNoteArgument
	}
}

func hostedSheetsNamedRangeUpdateArgs(id string, req mcp.CallToolRequest) ([]string, error) {
	selector, err := hostedSheetsRangeNoteText(req, "name_or_id", true)
	if err != nil {
		return nil, err
	}
	args := []string{"sheets", "named-ranges", "update"}
	changed := false
	for _, key := range []string{"name", "range"} {
		if _, ok := req.GetArguments()[key]; !ok {
			continue
		}
		text, err := hostedSheetsRangeNoteText(req, key, false)
		if err != nil {
			return nil, err
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			changed = true
			if key == "range" {
				if _, err := parseSheetRange(cleanRange(trimmed), "range"); err != nil {
					return nil, err
				}
			}
		}
		args = append(args, "--"+key+"="+text)
	}
	if !changed {
		return nil, errHostedSheetsRangeNoteArgument
	}
	return append(args, "--", id, selector), nil
}
