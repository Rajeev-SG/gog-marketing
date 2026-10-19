package cmd

import (
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDocsTabFixedArguments(t *testing.T) {
	tools := hostedDocsTabTools()
	if len(tools) != 3 {
		t.Fatalf("tools=%d", len(tools))
	}
	byName := map[string]mcpToolSpec{}
	for _, tool := range tools {
		if tool.Service != "docs" || tool.Risk != mcpRiskWrite {
			t.Fatalf("unexpected tool: %+v", tool)
		}
		byName[tool.Name] = tool
	}
	for _, test := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{"docs_add_tab", map[string]any{"document_id": "selected-doc"}, []string{"docs", "tabs", "add", "--", "selected-doc"}},
		{"docs_add_tab", map[string]any{"document_id": "selected-doc", "title": "--account=else", "index": 0, "parent_tab": "Parent & 子", "icon_emoji": "🚀"}, []string{"docs", "tabs", "add", "--title=--account=else", "--index=0", "--parent-tab=Parent & 子", "--icon-emoji=🚀", "--", "selected-doc"}},
		{"docs_add_tab", map[string]any{"document_id": "selected-doc", "title": "Title 👩‍💻", "index": 0, "parent_tab": "Parent 👩‍💻", "icon_emoji": "👩‍💻"}, []string{"docs", "tabs", "add", "--title=Title 👩‍💻", "--index=0", "--parent-tab=Parent 👩‍💻", "--icon-emoji=👩‍💻", "--", "selected-doc"}},
		{"docs_rename_tab", map[string]any{"document_id": "selected-doc", "tab": "Tab 👩‍💻", "title": "New title"}, []string{"docs", "tabs", "rename", "--tab=Tab 👩‍💻", "--title=New title", "--", "selected-doc"}},
		{"docs_delete_tab", map[string]any{"document_id": "selected-doc", "tab": "tab-1"}, []string{"docs", "tabs", "delete", "--tab=tab-1", "--", "selected-doc"}},
	} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = test.args
		args, err := byName[test.name].BuildArgs(req)
		if err != nil || !slices.Equal(args, test.want) {
			t.Fatalf("%s argv=%q error=%v", test.name, args, err)
		}
	}
}

func TestHostedDocsTabsRejectMalformedInput(t *testing.T) {
	byName := map[string]mcpToolSpec{}
	for _, tool := range hostedDocsTabTools() {
		byName[tool.Name] = tool
	}
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"docs_add_tab", map[string]any{"document_id": "../other"}},
		{"docs_add_tab", map[string]any{"document_id": "selected-doc", "index": -1}},
		{"docs_add_tab", map[string]any{"document_id": "selected-doc", "index": 0.5}},
		{"docs_add_tab", map[string]any{"document_id": "selected-doc", "index": nil}},
		{"docs_add_tab", map[string]any{"document_id": "selected-doc", "title": nil}},
		{"docs_add_tab", map[string]any{"document_id": "selected-doc", "title": "bad\nlabel"}},
		{"docs_rename_tab", map[string]any{"document_id": "selected-doc", "tab": "old", "title": ""}},
		{"docs_delete_tab", map[string]any{"document_id": "selected-doc", "tab": ""}},
	} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = test.args
		if _, err := byName[test.name].BuildArgs(req); err == nil {
			t.Fatalf("%s accepted %v", test.name, test.args)
		}
	}
}
