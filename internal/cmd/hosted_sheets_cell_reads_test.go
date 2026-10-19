package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func defaultReadArgs() map[string]any {
	return map[string]any{
		"spreadsheet_id": "selected-sheet",
		"range":          "'Revenue Tab'!B3:B4",
	}
}

func mustBuildArgs(t *testing.T, spec mcpToolSpec, args map[string]any) []string {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	got, err := spec.BuildArgs(req)
	if err != nil {
		t.Fatalf("BuildArgs(%v) returned unexpected error: %v", args, err)
	}
	return got
}

func expectBuildArgsError(t *testing.T, spec mcpToolSpec, args map[string]any) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	if _, err := spec.BuildArgs(req); err == nil {
		t.Fatalf("BuildArgs(%v) expected an error, got none", args)
	}
}

func TestHostedSheetsCellReadArguments(t *testing.T) {
	tools := hostedSheetsCellReadTools()
	if len(tools) != 4 {
		t.Fatalf("hostedSheetsCellReadTools() returned %d tools, want 4", len(tools))
	}

	for _, spec := range tools {
		if spec.Service != "sheets" {
			t.Errorf("tool %q Service = %q, want %q", spec.Name, spec.Service, "sheets")
		}
		if spec.Risk != mcpRiskRead {
			t.Errorf("tool %q Risk = %q, want mcpRiskRead (%q)", spec.Name, spec.Risk, mcpRiskRead)
		}
	}

	names := make([]string, 0, len(tools))
	for _, spec := range tools {
		names = append(names, spec.Name)
	}
	slices.Sort(names)
	wantNames := []string{"sheets_links_get", "sheets_notes", "sheets_read_format", "sheets_validation_get"}
	if !slices.Equal(names, wantNames) {
		t.Fatalf("tool names = %v, want %v", names, wantNames)
	}

	byName := make(map[string]mcpToolSpec, len(tools))
	for _, spec := range tools {
		if _, exists := byName[spec.Name]; exists {
			t.Fatalf("duplicate tool name %q", spec.Name)
		}
		byName[spec.Name] = spec
	}

	type record struct {
		name    string
		command []string
	}
	records := []record{
		{name: "sheets_notes", command: []string{"sheets", "notes"}},
		{name: "sheets_links_get", command: []string{"sheets", "links", "get"}},
		{name: "sheets_read_format", command: []string{"sheets", "read-format"}},
		{name: "sheets_validation_get", command: []string{"sheets", "validation", "get"}},
	}

	t.Run("default args", func(t *testing.T) {
		for _, rec := range records {
			got := mustBuildArgs(t, byName[rec.name], defaultReadArgs())
			want := append(slices.Clone(rec.command), "--", "selected-sheet", "'Revenue Tab'!B3:B4")
			if !slices.Equal(got, want) {
				t.Errorf("%s: args = %q, want %q", rec.name, got, want)
			}
		}
	})

	t.Run("range literal kept", func(t *testing.T) {
		variants := []string{"ReviewRange", "--account=else"}
		for _, rec := range records {
			for _, variant := range variants {
				args := defaultReadArgs()
				args["range"] = variant
				got := mustBuildArgs(t, byName[rec.name], args)
				want := append(slices.Clone(rec.command), "--", "selected-sheet", variant)
				if !slices.Equal(got, want) {
					t.Errorf("%s: range %q: args = %q, want %q", rec.name, variant, got, want)
				}
				if variant == "--account=else" {
					if got[len(rec.command)] != "--" || got[len(got)-1] != "--account=else" {
						t.Errorf("%s: flag-like range must appear after --: %q", rec.name, got)
					}
				}
			}
		}
	})

	t.Run("effective flag", func(t *testing.T) {
		format := byName["sheets_read_format"]

		args := defaultReadArgs()
		args["effective"] = true
		got := mustBuildArgs(t, format, args)
		want := []string{"sheets", "read-format", "--effective", "--", "selected-sheet", "'Revenue Tab'!B3:B4"}
		if !slices.Equal(got, want) {
			t.Errorf("effective=true: args = %q, want %q", got, want)
		}

		args = defaultReadArgs()
		args["effective"] = false
		got = mustBuildArgs(t, format, args)
		want = append(slices.Clone([]string{"sheets", "read-format"}), "--", "selected-sheet", "'Revenue Tab'!B3:B4")
		if !slices.Equal(got, want) {
			t.Errorf("effective=false: args = %q, want %q", got, want)
		}

		for _, badEffective := range []any{nil, "true", 1} {
			badArgs := defaultReadArgs()
			badArgs["effective"] = badEffective
			expectBuildArgsError(t, format, badArgs)
		}
	})

	t.Run("bad spreadsheet_id", func(t *testing.T) {
		badIDs := []any{
			nil,
			42,
			"",
			"https://docs.google.com/spreadsheets/d/abc123",
			"/path/to/sheet",
			" spaced ",
		}
		for _, badID := range badIDs {
			for _, rec := range records {
				expectBuildArgsError(t, byName[rec.name], map[string]any{
					"spreadsheet_id": badID,
					"range":          "'Revenue Tab'!B3:B4",
				})
			}
		}
	})

	t.Run("bad range", func(t *testing.T) {
		tooLong := strings.Repeat("x", 4097)
		badRanges := []struct {
			label string
			args  map[string]any
		}{
			{"missing", map[string]any{"spreadsheet_id": "selected-sheet"}},
			{"nil", map[string]any{"spreadsheet_id": "selected-sheet", "range": nil}},
			{"number", map[string]any{"spreadsheet_id": "selected-sheet", "range": 7}},
			{"empty", map[string]any{"spreadsheet_id": "selected-sheet", "range": ""}},
			{"whitespace", map[string]any{"spreadsheet_id": "selected-sheet", "range": "   "}},
			{"newline", map[string]any{"spreadsheet_id": "selected-sheet", "range": "\n"}},
			{"nul", map[string]any{"spreadsheet_id": "selected-sheet", "range": "\x00"}},
			{"too-long", map[string]any{"spreadsheet_id": "selected-sheet", "range": tooLong}},
		}
		for _, bad := range badRanges {
			for _, rec := range records {
				expectBuildArgsError(t, byName[rec.name], bad.args)
			}
		}
	})

	t.Run("emoji in quoted tab range accepted", func(t *testing.T) {
		emojiRange := "'Revenue 👨‍👩‍👧 Tab'!B3:B4"
		for _, rec := range records {
			args := defaultReadArgs()
			args["range"] = emojiRange
			got := mustBuildArgs(t, byName[rec.name], args)
			want := append(slices.Clone(rec.command), "--", "selected-sheet", emojiRange)
			if !slices.Equal(got, want) {
				t.Errorf("%s: args = %q, want %q", rec.name, got, want)
			}
		}
	})
}
