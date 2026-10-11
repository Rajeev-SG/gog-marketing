package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// Each descriptor fixes a native command and its permitted fields. No client
// command, argv, local-file source, credentials or endpoint can be supplied.
func hostedTasksTools() []mcpToolSpec {
	listStrings := []string{"page"}
	listBools := []string{"all", "fail_empty"}
	return []mcpToolSpec{
		hostedTaskCommand("tasks_lists", "lists list", mcpRiskRead, nil, listStrings, listBools, []string{"max"}),
		hostedTaskCommand("tasks_lists_create", "lists create", mcpRiskWrite, []string{"title"}, nil, nil, nil),
		hostedTaskCommand("tasks_list", "list", mcpRiskRead, []string{"tasklist_id"}, []string{"page", "due_min", "due_max", "completed_min", "completed_max", "updated_min"}, []string{"all", "fail_empty", "show_completed", "show_deleted", "show_hidden", "show_assigned"}, []string{"max"}),
		hostedTaskCommand("tasks_get", "get", mcpRiskRead, []string{"tasklist_id", "task_id"}, nil, nil, nil),
		hostedTaskCommand("tasks_raw", "raw", mcpRiskRead, []string{"tasklist_id", "task_id"}, nil, []string{"pretty"}, nil),
		hostedTaskCommand("tasks_add", "add", mcpRiskWrite, []string{"tasklist_id"}, []string{"title", "notes", "due", "parent", "previous", "repeat", "recur", "recur_rrule", "repeat_until"}, nil, []string{"repeat_count"}),
		hostedTaskCommand("tasks_update", "update", mcpRiskWrite, []string{"tasklist_id", "task_id"}, []string{"title", "notes", "due", "status"}, nil, nil),
		hostedTaskCommand("tasks_done", "done", mcpRiskWrite, []string{"tasklist_id", "task_id"}, nil, nil, nil),
		hostedTaskCommand("tasks_undo", "undo", mcpRiskWrite, []string{"tasklist_id", "task_id"}, nil, nil, nil),
		hostedTaskCommand("tasks_delete", "delete", mcpRiskWrite, []string{"tasklist_id", "task_id"}, nil, nil, nil),
		hostedTaskCommand("tasks_clear", "clear", mcpRiskWrite, []string{"tasklist_id"}, nil, nil, nil),
	}
}

func hostedTaskCommand(name, command string, risk mcpToolRisk, positional, stringFields, boolFields, intFields []string) mcpToolSpec {
	options := []mcp.ToolOption{}
	for _, key := range positional {
		options = append(options, mcp.WithString(key, mcp.Required()))
	}
	for _, key := range stringFields {
		if name == "tasks_add" && key == "title" {
			options = append(options, mcp.WithString(key, mcp.Required()))
		} else {
			options = append(options, mcp.WithString(key))
		}
	}
	for _, key := range boolFields {
		options = append(options, mcp.WithBoolean(key))
	}
	for _, key := range intFields {
		minimum, maximum := float64(0), float64(1000)
		if key == "max" {
			minimum = 1
			if name == "tasks_list" {
				maximum = 100
			}
		}
		options = append(options, mcp.WithInteger(key, mcp.Min(minimum), mcp.Max(maximum)))
	}
	return mcpToolSpec{
		Name: name, Service: "tasks", Risk: risk, Description: "Run native Google Tasks " + command + " for an explicitly authorized account. Use an opaque task-list ID or @default; delete/clear require destructive permission. Repeated creation requires repeat_count (1–1000), including when repeat_until is supplied.", Options: options,
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			args := append([]string{"tasks"}, strings.Fields(command)...)
			if name == "tasks_add" && (req.GetString("repeat", "") != "" || req.GetString("recur", "") != "" || req.GetString("recur_rrule", "") != "") && (req.GetInt("repeat_count", 0) < 1 || req.GetInt("repeat_count", 0) > 1000) {
				return nil, ErrHostedToolInput
			}
			fields := req.GetArguments()
			if name == "tasks_update" {
				for _, key := range []string{"title", "notes", "status"} {
					if value, present := fields[key]; present && value == "" {
						// The native patch omits empty text/status fields. Do not
						// report a successful clear until native serialization is fixed.
						return nil, ErrHostedToolInput
					}
				}
			}
			for _, key := range stringFields {
				if _, ok := fields[key]; ok {
					args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+req.GetString(key, ""))
				}
			}
			for _, key := range boolFields {
				if _, ok := fields[key]; ok {
					args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+strconv.FormatBool(req.GetBool(key, false)))
				}
			}
			for _, key := range intFields {
				if _, ok := fields[key]; ok {
					args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+strconv.Itoa(req.GetInt(key, 0)))
				}
			}
			args = append(args, "--")
			for _, key := range positional {
				value, err := requireMCPString(req, key)
				if err != nil {
					return nil, err
				}
				args = append(args, value)
			}
			return args, nil
		},
	}
}
