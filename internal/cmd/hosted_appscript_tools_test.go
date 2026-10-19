package cmd

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedAppScriptInventoryAdapterFlags(t *testing.T) {
	specs := hostedAppScriptProjectTools()
	if len(specs) != 9 {
		t.Fatalf("Apps Script adapter unexpected tool count: %d", len(specs))
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

func TestHostedAppScriptManagementAdapterArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{"appscript_bound_create", map[string]any{"title": "Bound", "parent_id": "parent123"}, []string{"appscript", "bound-create", "--title=Bound", "--parent-id=parent123"}},
		{"appscript_version_create", map[string]any{"script_id": "script123", "description": "release"}, []string{"appscript", "version-create", "--description=release", "--", "script123"}},
		{"appscript_deployment_create", map[string]any{"script_id": "script123", "version_number": 7}, []string{"appscript", "deployment-create", "--version-number=7", "--description=", "--manifest-file-name=", "--", "script123"}},
	} {
		for _, spec := range hostedAppScriptManagementTools() {
			if spec.Name != tc.name {
				continue
			}
			got, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}})
			if err != nil || strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("%s: %v %v", tc.name, got, err)
			}
			if _, err := spec.BuildArgs(mcp.CallToolRequest{}); err == nil {
				t.Fatalf("%s accepted missing arguments", tc.name)
			}
		}
	}
}
