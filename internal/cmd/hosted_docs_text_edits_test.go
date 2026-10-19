package cmd

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDocsTextArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{"docs_insert", map[string]any{"document_id": "selected-doc", "text": "@/tmp/literal\n😀", "index": 1}, []string{"docs", "insert", "--index=1", "--", "selected-doc", "@/tmp/literal\n😀"}},
		{"docs_insert", map[string]any{"document_id": "selected-doc", "text": "body", "at": "anchor", "occurrence": 2, "match_case": true, "tab": "Main"}, []string{"docs", "insert", "--at=anchor", "--occurrence=2", "--match-case", "--tab=Main", "--", "selected-doc", "body"}},
		{"docs_delete", map[string]any{"document_id": "selected-doc", "start": 0, "end": 2, "segment": "header-1"}, []string{"docs", "delete", "--start=0", "--end=2", "--segment=header-1", "--", "selected-doc"}},
		{"docs_find_replace", map[string]any{"document_id": "selected-doc", "find": "a", "replacement": "", "first": true}, []string{"docs", "find-replace", "--format=plain", "--first", "--", "selected-doc", "a", ""}},
		{"docs_clear", map[string]any{"document_id": "selected-doc"}, []string{"docs", "clear", "--", "selected-doc"}},
	} {
		for _, spec := range hostedDocsTextEditTools() {
			if spec.Name != tc.name {
				continue
			}
			got, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}})
			if err != nil || strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("%s got%v err%v want%v", tc.name, got, err, tc.want)
			}
		}
	}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"docs_insert", map[string]any{"document_id": "selected-doc", "text": ""}},
		{"docs_insert", map[string]any{"document_id": "selected-doc", "text": "x", "index": 0}},
		{"docs_insert", map[string]any{"document_id": "selected-doc", "text": "x", "index": 1, "at": "anchor"}},
		{"docs_insert", map[string]any{"document_id": "selected-doc", "text": "x", "at": ""}},
		{"docs_delete", map[string]any{"document_id": "selected-doc"}},
		{"docs_delete", map[string]any{"document_id": "selected-doc", "start": 2, "end": 1}},
		{"docs_delete", map[string]any{"document_id": "selected-doc", "at": "anchor", "occurrence": 0}},
		{"docs_find_replace", map[string]any{"document_id": "selected-doc", "find": "", "replacement": "x"}},
		{"docs_find_replace", map[string]any{"document_id": "selected-doc", "find": "x"}},
		{"docs_find_replace", map[string]any{"document_id": "selected-doc", "find": "x", "replacement": "y", "first": "true"}},
	} {
		for _, spec := range hostedDocsTextEditTools() {
			if spec.Name == tc.name {
				if _, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}}); err == nil {
					t.Fatalf("invalid%s args%vadmitted", tc.name, tc.args)
				}
			}
		}
	}
}
