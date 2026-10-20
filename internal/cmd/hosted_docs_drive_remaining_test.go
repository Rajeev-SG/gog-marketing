package cmd

import (
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDocsDriveRemainingArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{"docs_cell_style", map[string]any{"row": 1, "col": 1, "background_color": "#123456"}, []string{"docs", "cell-style", "--row=1", "--col=1", "--background-color=#123456", "--", "selected-file"}},
		{"docs_cell_update", map[string]any{"row": 1, "col": 2, "content": "--file=/tmp/literal"}, []string{"docs", "cell-update", "--format=plain", "--row=1", "--col=2", "--content=--file=/tmp/literal", "--", "selected-file"}},
		{"docs_table_row_style", map[string]any{"prevent_overflow": false}, []string{"docs", "table-row", "style", "--no-prevent-overflow", "--", "selected-file"}},
		{"drive_mkdir", map[string]any{"name": "Folder"}, []string{"drive", "mkdir", "--parent=selected-file", "--", "Folder"}},
		{"drive_bulk_remove_public", map[string]any{}, []string{"drive", "bulk", "remove-public", "--file=selected-file", "--"}},
		{"docs_copy", map[string]any{"title": "--parent=literal"}, []string{"docs", "copy", "--", "selected-file", "--parent=literal"}},
	} {
		found := false
		for _, definition := range hostedDocsDriveRemainingDefinitions() {
			if definition.name != tc.name {
				continue
			}
			found = true
			tc.args[definition.idKey] = "selected-file"
			got, err := hostedDocsDriveRemainingArgs(definition, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}})
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s got %v %v want %v", tc.name, got, err, tc.want)
			}
		}
		if !found {
			t.Fatal("missing tool")
		}
	}
	for _, definition := range hostedDocsDriveRemainingDefinitions() {
		args := map[string]any{definition.idKey: "../foreign"}
		if _, err := hostedDocsDriveRemainingArgs(definition, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}); err == nil {
			t.Errorf("invalid resource admitted %s", definition.name)
		}
	}
}
