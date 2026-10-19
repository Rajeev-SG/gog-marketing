package cmd

import "github.com/mark3labs/mcp-go/mcp"

func hostedTasksLifecycleTools() []mcpToolSpec {
	get := hostedTaskCommand("tasks_lists_get", "lists get", mcpRiskRead, []string{"tasklist_id"}, nil, nil, nil)
	get.Description = "Get full metadata for one explicit task-list ID or @default in the connected Google account, under exact account-level tool authority. No title lookup or Drive grants."
	rename := hostedTaskCommand("tasks_lists_rename", "lists rename", mcpRiskWrite, []string{"tasklist_id", "title"}, nil, nil, nil)
	rename.Description = "Rename one explicit task list; title-only patch preserves other fields. Requires exact account-level write-tool authority; chat requires one-use exact-argument approval. Single-attempt mutation, no title lookup."
	deletion := hostedTaskCommand("tasks_lists_delete", "lists delete", mcpRiskWrite, []string{"tasklist_id"}, nil, nil, nil)
	deletion.Description = "Irreversibly delete a non-default task list and ALL its tasks in the connected Google account. Requires destructive exact-tool authority and chat one-use approval. @default is rejected locally; Google enforces actual default-list protection. No automatic retries."
	move := mcpToolSpec{
		Name: "tasks_move", Service: "tasks", Risk: mcpRiskWrite,
		Description: "Move/reorder/reparent a task, optionally into another accessible list in the SAME connected Google account. Omit parent for top-level and previous for first position; parent/previous refer to the destination list when supplied. Requires exact account-level write-tool authority and one-use chat approval; no per-list Drive grant is implied. Google enforces assigned/repeating/hidden task limitations. No implicit list-title lookup or automatic retries.",
		Options:     []mcp.ToolOption{mcp.WithString("tasklist_id", mcp.Required()), mcp.WithString("task_id", mcp.Required()), mcp.WithString("parent"), mcp.WithString("previous"), mcp.WithString("destination_tasklist_id")}, BuildArgs: hostedTasksMoveArgs,
	}
	return []mcpToolSpec{get, rename, deletion, move}
}

func hostedTasksMoveArgs(req mcp.CallToolRequest) ([]string, error) {
	list, err := requireMCPString(req, "tasklist_id")
	if err != nil {
		return nil, err
	}
	task, err := requireMCPString(req, "task_id")
	if err != nil {
		return nil, err
	}
	command := TasksMoveCmd{TasklistID: list, TaskID: task}
	args := []string{"tasks", "move"}
	for _, field := range []struct {
		key, flag string
		target    *string
	}{
		{"parent", "parent", &command.Parent}, {"previous", "previous", &command.Previous}, {"destination_tasklist_id", "destination-tasklist-id", &command.DestinationTasklistID},
	} {
		if raw, present := req.GetArguments()[field.key]; present {
			value, ok := raw.(string)
			if !ok {
				return nil, usage("task move selectors must be strings")
			}
			*field.target = value
			args = append(args, "--"+field.flag+"="+value)
		}
	}
	if _, planErr := command.plan(); planErr != nil {
		return nil, planErr
	}
	return append(args, "--", list, task), nil
}
