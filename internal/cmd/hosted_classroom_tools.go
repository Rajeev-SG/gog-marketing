package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedClassroomCourseTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 5)
	for _, operation := range []string{"list", "get", "create", "update", "delete"} {
		risk := mcpRiskRead
		options := []mcp.ToolOption{}
		fields := []string{}
		if operation == "create" || operation == "update" || operation == "delete" {
			risk = mcpRiskWrite
		}
		if operation == "get" || operation == "update" || operation == "delete" {
			options = append(options, mcp.WithString("course_id", mcp.Required()))
		}
		if operation == "list" {
			fields = []string{"state", "teacher", "student", "page"}
			options = append(options, mcp.WithInteger("max", mcp.Min(1), mcp.Max(1000)), mcp.WithBoolean("all"), mcp.WithBoolean("fail_empty"))
		}
		if operation == "create" || operation == "update" {
			fields = []string{"name", "owner", "section", "description_heading", "description", "room", "state"}
		}
		for _, key := range fields {
			if operation == "create" && key == "name" {
				options = append(options, mcp.WithString(key, mcp.Required()))
			} else {
				options = append(options, mcp.WithString(key))
			}
		}
		result = append(result, mcpToolSpec{
			Name: "classroom_courses_" + operation, Service: "classroom", Risk: risk, Description: "Run native Classroom courses " + operation + " with account/tool grants and approved course scopes. Google account/teacher roles still apply; deletion requires destructive permission.", Options: options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				args := []string{"classroom", "courses", operation}
				for _, key := range fields {
					if _, present := req.GetArguments()[key]; present {
						args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+req.GetString(key, ""))
					}
				}
				if operation == "list" {
					args = append(args, "--max="+strconv.Itoa(req.GetInt("max", 100)))
					for _, key := range []string{"all", "fail_empty"} {
						if req.GetBool(key, false) {
							flag := key
							if key == "fail_empty" {
								flag = "fail-empty"
							}
							args = append(args, "--"+flag)
						}
					}
				}
				if operation == "get" || operation == "update" || operation == "delete" {
					id, err := requireMCPString(req, "course_id")
					if err != nil {
						return nil, err
					}
					args = append(args, "--", id)
				}
				return args, nil
			},
		})
	}
	return result
}
