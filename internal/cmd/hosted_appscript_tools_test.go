package cmd

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedAppScriptInventoryAdapterFlags(t *testing.T) {
	specs := hostedAppScriptProjectTools()
	if len(specs) != 5 {
		t.Fatalf("Apps Script adapter changed from the approved five-tool surface: %d", len(specs))
	}

	for _, operation := range []string{"deployments", "versions"} {
		name := "appscript_" + operation
		var selected *mcpToolSpec
		for _, spec := range specs {
			if spec.Name == name {
				candidate := spec
				selected = &candidate
				break
			}
		}
		if selected == nil || selected.Risk != mcpRiskRead || selected.Service != "appscript" {
			t.Fatalf("missing or unsafe %s adapter", name)
		}

		args, err := selected.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"script_id": "project-1", "max": 2, "page": "token-2", "all": true, "fail_empty": true,
			},
		}})
		if err != nil {
			t.Fatalf("%s arguments: %v", name, err)
		}

		want := []string{"appscript", operation, "--max=2", "--page=token-2", "--all", "--fail-empty", "--", "project-1"}
		if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("%s args = %#v, want %#v", name, args, want)
		}

		if _, err := selected.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{
			Arguments: map[string]any{"max": 2},
		}}); err == nil {
			t.Fatalf("%s accepted an empty script ID", name)
		}
	}
}
