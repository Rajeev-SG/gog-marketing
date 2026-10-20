package cmd

import (
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDocsTableArgs(t *testing.T) {
	for _, tc := range []struct {
		command string
		args    map[string]any
		want    []string
	}{
		{"table-row insert", map[string]any{"at": "end"}, []string{"docs", "table-row", "insert", "--at=end", "--", "selected-doc"}},
		{"table-column insert", map[string]any{"at": "-1"}, []string{"docs", "table-column", "insert", "--at=-1", "--", "selected-doc"}},
		{"table-row delete", map[string]any{"row": 1}, []string{"docs", "table-row", "delete", "--row=1", "--", "selected-doc"}},
		{"table-column delete", map[string]any{"col": 2}, []string{"docs", "table-column", "delete", "--col=2", "--", "selected-doc"}},
		{"table-merge", map[string]any{"range": "1,1:2,2"}, []string{"docs", "table-merge", "--range=1,1:2,2", "--", "selected-doc"}},
		{"table-unmerge", map[string]any{"cell": "1,1"}, []string{"docs", "table-unmerge", "--cell=1,1", "--", "selected-doc"}},
		{"table-row pin-header", map[string]any{"rows": 0}, []string{"docs", "table-row", "pin-header", "--rows=0", "--", "selected-doc"}},
	} {
		tc.args["document_id"] = "selected-doc"
		got, err := hostedDocsTableArgs(tc.command, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}})
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s got %v %v want %v", tc.command, got, err, tc.want)
		}
	}
	for _, tc := range []struct {
		command string
		args    map[string]any
	}{
		{"table-row delete", map[string]any{"row": 0}},
		{"table-row delete", map[string]any{"row": 1.5}},
		{"table-column delete", map[string]any{}},
		{"table-merge", map[string]any{"range": "0,1:1,2"}},
		{"table-unmerge", map[string]any{"cell": "../secret"}},
		{"table-row insert", map[string]any{"at": "0"}},
		{"table-row pin-header", map[string]any{"rows": -1}},
	} {
		tc.args["document_id"] = "selected-doc"
		if _, err := hostedDocsTableArgs(tc.command, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}}); err == nil {
			t.Errorf("invalid %s admitted %v", tc.command, tc.args)
		}
	}
}
