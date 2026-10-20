package cmd

import (
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDocsStructuralArgs(t *testing.T) {
	cases := []struct {
		command string
		args    map[string]any
		want    []string
	}{
		{"insert-footnote", map[string]any{"text": "--file=/tmp/literal"}, []string{"docs", "insert-footnote", "--text=--file=/tmp/literal", "--", "selected-doc"}},
		{"insert-horizontal-rule", map[string]any{"index": 1}, []string{"docs", "insert-horizontal-rule", "--index=1", "--", "selected-doc"}},
		{"insert-page-break", map[string]any{"at_end": true}, []string{"docs", "insert-page-break", "--at-end", "--", "selected-doc"}},
		{"insert-section-break", map[string]any{"type": "continuous"}, []string{"docs", "insert-section-break", "--type=continuous", "--", "selected-doc"}},
		{"insert-table", map[string]any{"rows": 2, "cols": 3}, []string{"docs", "insert-table", "--rows=2", "--cols=3", "--", "selected-doc"}},
		{"insert-person", map[string]any{"email": "user@example.test"}, []string{"docs", "insert-person", "--email=user@example.test", "--", "selected-doc"}},
		{"insert-date-chip", map[string]any{"date": "2026-10-20", "format": "iso"}, []string{"docs", "insert-date-chip", "--date=2026-10-20", "--format=iso", "--", "selected-doc"}},
		{"header create", map[string]any{"text": "header"}, []string{"docs", "header", "create", "--text=header", "--", "selected-doc"}},
		{"footer create", map[string]any{"text": "footer"}, []string{"docs", "footer", "create", "--text=footer", "--", "selected-doc"}},
		{"header delete", map[string]any{"segment_id": "header-1", "tab": "Main"}, []string{"docs", "header", "delete", "--tab=Main", "--", "selected-doc", "header-1"}},
		{"footer delete", map[string]any{"segment_id": "footer-1"}, []string{"docs", "footer", "delete", "--", "selected-doc", "footer-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			tc.args["document_id"] = "selected-doc"
			got, err := hostedDocsStructuralArgs(tc.command, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}})
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		command string
		args    map[string]any
	}{
		{"insert-table", map[string]any{"rows": 0, "cols": 2}},
		{"insert-table", map[string]any{"rows": 2.5, "cols": 2}},
		{"insert-table", map[string]any{"rows": 101, "cols": 2}},
		{"insert-page-break", map[string]any{"index": 1, "at_end": true}},
		{"insert-page-break", map[string]any{"index": 0}},
		{"insert-page-break", map[string]any{"at_end": "true"}},
		{"insert-footnote", map[string]any{"text": true}},
		{"insert-footnote", map[string]any{"text": ""}},
		{"insert-section-break", map[string]any{"type": "unrecognized"}},
		{"insert-date-chip", map[string]any{"date": "2026-02-30"}},
		{"insert-date-chip", map[string]any{"date": "2026-10-20", "format": "raw"}},
		{"insert-person", map[string]any{"email": "user@example.test\n--file=x"}},
		{"header delete", map[string]any{"segment_id": "../../private"}},
	} {
		tc.args["document_id"] = "selected-doc"
		if _, err := hostedDocsStructuralArgs(tc.command, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}}); err == nil {
			t.Errorf("admitted invalid %s %v", tc.command, tc.args)
		}
	}
}
