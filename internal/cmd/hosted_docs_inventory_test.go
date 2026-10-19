package cmd

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDocsInventoryTools(t *testing.T) {
	tools := hostedDocsInventoryTools()

	// Exactly the two inventory tools, correct names, service and read risk.
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	if !slices.Equal(names, []string{
		"docs_info", "docs_list_tabs", "docs_structure", "docs_headings_list",
		"docs_paragraphs_list", "docs_tables_list", "docs_images_list",
		"docs_named_range_list",
	}) {
		t.Fatalf("unexpected tool names: %v", names)
	}
	for _, tool := range tools {
		if tool.Service != "docs" {
			t.Errorf("%s: service = %q, want docs", tool.Name, tool.Service)
		}
		if tool.Risk != mcpRiskRead {
			t.Errorf("%s: risk = %q, want read", tool.Name, tool.Risk)
		}
	}

	// document_id is the only declared schema property.
	for _, tool := range tools {
		schema := mcp.NewTool(tool.Name, tool.Options...).InputSchema.Properties
		if len(schema) != 1 || schema["document_id"] == nil {
			t.Fatalf("%s: unexpected schema: %v", tool.Name, schema)
		}
	}

	cases := []struct {
		name string
		id   string
		argv []string
	}{
		{"docs_info", "1AbC_d-e-2", []string{"docs", "info", "--", "1AbC_d-e-2"}},
		{"docs_list_tabs", "1AbC_d-e-2", []string{"docs", "list-tabs", "--", "1AbC_d-e-2"}},
		{"docs_structure", "1AbC_d-e-2", []string{"docs", "structure", "--", "1AbC_d-e-2"}},
		{"docs_headings_list", "1AbC_d-e-2", []string{"docs", "headings", "list", "--", "1AbC_d-e-2"}},
		{"docs_paragraphs_list", "1AbC_d-e-2", []string{"docs", "paragraphs", "list", "--", "1AbC_d-e-2"}},
		{"docs_tables_list", "1AbC_d-e-2", []string{"docs", "tables", "list", "--", "1AbC_d-e-2"}},
		{"docs_images_list", "1AbC_d-e-2", []string{"docs", "images", "list", "--", "1AbC_d-e-2"}},
		{"docs_named_range_list", "1AbC_d-e-2", []string{"docs", "named-range", "list", "--", "1AbC_d-e-2"}},
	}
	byName := map[string]mcpToolSpec{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{"document_id": tc.id}
			argv, err := byName[tc.name].BuildArgs(req)
			if err != nil {
				t.Fatalf("BuildArgs() error = %v, want nil", err)
			}
			if !slices.Equal(argv, tc.argv) {
				t.Errorf("argv = %v, want %v", argv, tc.argv)
			}
		})
	}
}

func TestHostedDocsInventoryBadIDs(t *testing.T) {
	bad := []string{
		"",                       // empty
		"https://evil.example/a", // URL
		"../secrets",             // traversal
		"a/b",                    // separator
		"a b",                    // whitespace
		"1=2",                    // shell metachar
		strings.Repeat("a", 257), // too long
	}
	byName := map[string]mcpToolSpec{}
	for _, tool := range hostedDocsInventoryTools() {
		byName[tool.Name] = tool
	}
	for _, tool := range byName {
		for _, id := range bad {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{"document_id": id}
			argv, err := byName[tool.Name].BuildArgs(req)
			if err == nil {
				t.Errorf("%s: id %q: BuildArgs() error = nil, want error", tool.Name, id)
			}
			if argv != nil {
				t.Errorf("%s: id %q: argv = %v, want nil", tool.Name, id, argv)
			}
		}
		// Missing document_id must also fail.
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{}
		if _, err := byName[tool.Name].BuildArgs(req); err == nil {
			t.Errorf("%s: missing document_id: error = nil, want error", tool.Name)
		}
	}
	// Errors are the shared static sentinel.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"document_id": "../bad"}
	if _, err := byName["docs_info"].BuildArgs(req); !errors.Is(err, errHostedDocsInventoryID) {
		t.Errorf("errors.Is(err, errHostedDocsInventoryID) = false, want true (err = %v)", err)
	}
}
