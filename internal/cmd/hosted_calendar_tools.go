package cmd

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedCalendarEventTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 4)
	for _, operation := range []string{"get", "create", "update", "delete"} {
		risk := mcpRiskWrite
		if operation == "get" {
			risk = mcpRiskRead
		}
		fields := []string{}
		options := []mcp.ToolOption{mcp.WithString("calendar_id", mcp.Required())}
		if operation != "create" {
			options = append(options, mcp.WithString("event_id", mcp.Required()))
		}
		if operation == "create" || operation == "update" {
			fields = []string{"summary", "from", "to", "start_timezone", "end_timezone", "timezone", "description", "location", "attendees", "visibility", "transparency", "event_color", "send_updates"}
			for _, key := range fields {
				options = append(options, mcp.WithString(key))
			}
			options = append(options, mcp.WithBoolean("all_day"), mcp.WithBoolean("with_meet"))
		}
		if operation == "delete" {
			fields = []string{"send_updates"}
			options = append(options, mcp.WithString("send_updates"))
		}
		if operation == "update" || operation == "delete" {
			fields = append(fields, "scope", "original_start")
			options = append(options, mcp.WithString("scope", mcp.Enum("single", "future", "all")), mcp.WithString("original_start"))
		}
		result = append(result, mcpToolSpec{
			Name: "calendar_event_" + operation, Service: "calendar", Risk: risk, Description: "Run native Calendar event " + operation + " in an explicitly enabled calendar. Delete requires destructive permission. Drive attachments, external integrations and moving to other calendars are not exposed by this tool.", Options: options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "calendar_id")
				if err != nil {
					return nil, err
				}
				command := operation
				if operation == "get" {
					command = "event"
				}
				args := []string{"calendar", command}
				for _, key := range fields {
					if _, present := req.GetArguments()[key]; present {
						args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+req.GetString(key, ""))
					}
				}
				for _, key := range []string{"all_day", "with_meet"} {
					if _, present := req.GetArguments()[key]; present {
						if req.GetBool(key, false) {
							args = append(args, "--"+strings.ReplaceAll(key, "_", "-"))
						} else {
							args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"=false")
						}
					}
				}
				args = append(args, "--", id)
				if operation != "create" {
					event, err := requireMCPString(req, "event_id")
					if err != nil {
						return nil, err
					}
					args = append(args, event)
				}
				return args, nil
			},
		})
	}
	return result
}
