package cmd

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedTasksMoveArguments(t *testing.T) {
	args, err := hostedTasksMoveArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"tasklist_id": "list-source-1234567890", "task_id": "task-1", "parent": "parent-1", "previous": "previous-1", "destination_tasklist_id": "list-dest-1234567890"}}})
	want := []string{"tasks", "move", "--parent=parent-1", "--previous=previous-1", "--destination-tasklist-id=list-dest-1234567890", "--", "list-source-1234567890", "task-1"}
	if err != nil || strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args%v err%v", args, err)
	}
	for _, overrides := range []map[string]any{{"parent": false}, {"previous": "task-1"}, {"destination_tasklist_id": "title"}, {"task_id": "../bad"}, {"parent": "parent-1", "previous": "parent-1"}} {
		fields := map[string]any{"tasklist_id": "list-source-1234567890", "task_id": "task-1"}
		for k, v := range overrides {
			fields[k] = v
		}
		if _, err := hostedTasksMoveArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: fields}}); err == nil {
			t.Fatalf("invalid move selectors accepted %v", fields)
		}
	}
}
