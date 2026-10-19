package cmd

import (
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedAppScriptDeploymentUpdateAdapter(t *testing.T) {
	spec := hostedAppScriptDeploymentUpdateTool()
	if spec.Risk != mcpRiskWrite {
		t.Fatal("update must be a write tool")
	}
	call := func(input map[string]any) ([]string, error) {
		return spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: input}})
	}
	base := func() map[string]any {
		return map[string]any{"script_id": "project-1", "deployment_id": "deployment-1", "version_number": 2}
	}
	for _, tc := range []struct {
		name  string
		extra map[string]any
		flags []string
	}{
		{name: "omitted"},
		{name: "clear", extra: map[string]any{"description": ""}, flags: []string{"--description="}},
		{name: "manifest", extra: map[string]any{"manifest_file_name": "new"}, flags: []string{"--manifest-file-name=new"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := base()
			for k, v := range tc.extra {
				input[k] = v
			}
			got, err := call(input)
			want := append([]string{"appscript", "deployment-update", "--version-number=2"}, tc.flags...)
			want = append(want, "--", "project-1", "deployment-1")
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("args=%#v want=%#v err=%v", got, want, err)
			}
		})
	}
	for _, value := range []any{nil, 0, -1, 2.5, "2", true, 2147483648} {
		input := base()
		input["version_number"] = value
		if _, err := call(input); err == nil {
			t.Fatalf("accepted invalid version %v", value)
		}
	}
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"script_id", "../foreign"}, {"deployment_id", "x/y"}, {"description", true}, {"description", nil}, {"manifest_file_name", " "}, {"manifest_file_name", false},
	} {
		input := base()
		input[tc.key] = tc.value
		if _, err := call(input); err == nil {
			t.Fatalf("accepted %s=%v", tc.key, tc.value)
		}
	}
	input := base()
	input["script_id"] = "--force"
	got, err := call(input)
	if err != nil || !reflect.DeepEqual(got, []string{"appscript", "deployment-update", "--version-number=2", "--", "--force", "deployment-1"}) {
		t.Fatalf("flag-shaped ID not positional: %#v %v", got, err)
	}
}
