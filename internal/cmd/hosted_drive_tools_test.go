package cmd

import (
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDriveArgumentsKeepNamesAndIDsLiteral(t *testing.T) {
	for _, entry := range []struct {
		spec      mcpToolSpec
		operation string
		withName  bool
	}{
		{hostedDriveRenameTool(), "rename", true},
		{hostedDriveCopyTool(), "copy", true},
		{hostedDriveTrashTool(), "delete", false},
	} {
		name := "--parent=foreign-folder"
		args, err := entry.spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"file_id": "file-1", "new_name": name}}})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"drive", entry.operation, "--", "file-1"}
		if entry.withName {
			want = append(want, name)
		}
		if !slices.Equal(args, want) {
			t.Fatalf("%s allowed flag interpretation: %v", entry.spec.Name, args)
		}
	}
}
