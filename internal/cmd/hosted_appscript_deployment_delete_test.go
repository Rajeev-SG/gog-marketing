package cmd

import (
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedAppScriptDeploymentDeleteAdapter(t *testing.T) {
	var selected *mcpToolSpec
	for _, spec := range hostedAppScriptManagementTools() {
		if spec.Name == "appscript_deployment_delete" {
			candidate := spec
			selected = &candidate
		}
	}
	if selected == nil || selected.Risk != mcpRiskWrite {
		t.Fatal("missing deployment delete write adapter")
	}
	args, err := selected.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"script_id": "project-1", "deployment_id": "deployment-1"}}})
	if err != nil || !reflect.DeepEqual(args, []string{"appscript", "deployment-delete", "--", "project-1", "deployment-1"}) {
		t.Fatalf("args = %#v, error = %v", args, err)
	}
	flagArgs, err := selected.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"script_id": "--force", "deployment_id": "deployment-1"}}})
	if err != nil || !reflect.DeepEqual(flagArgs, []string{"appscript", "deployment-delete", "--", "--force", "deployment-1"}) {
		t.Fatalf("flag-shaped ID was not preserved as a positional: %#v %v", flagArgs, err)
	}

	for _, key := range []string{"script_id", "deployment_id"} {
		for _, value := range []any{"", "../foreign", "x/y", "x?y", 12} {
			input := map[string]any{"script_id": "project-1", "deployment_id": "deployment-1"}
			input[key] = value
			if _, err := selected.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: input}}); err == nil {
				t.Fatalf("accepted %s=%v", key, value)
			}
		}
	}
}
