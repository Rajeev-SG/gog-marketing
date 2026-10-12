package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedFormsTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 5)
	for _, operation := range []string{"get", "update", "publish", "responses_list", "responses_get"} {
		risk := mcpRiskRead
		options := []mcp.ToolOption{mcp.WithString("form_id", mcp.Required())}
		fields := []string{}
		if operation == "update" {
			risk = mcpRiskWrite
			fields = []string{"title", "description", "quiz"}
		}
		if operation == "publish" {
			risk = mcpRiskWrite
			options = append(options, mcp.WithBoolean("unpublish"), mcp.WithBoolean("accepting_responses"))
		}
		if operation == "responses_list" {
			fields = []string{"page", "filter"}
			options = append(options, mcp.WithInteger("max", mcp.Min(1), mcp.Max(5000)))
		}
		if operation == "responses_get" {
			options = append(options, mcp.WithString("response_id", mcp.Required()))
		}
		for _, key := range fields {
			options = append(options, mcp.WithString(key))
		}
		result = append(result, mcpToolSpec{
			Name: "forms_" + operation, Service: "forms", Risk: risk, Description: "Run native Forms " + operation + " on an explicitly enabled Picker-selected form. Unpublishing requires destructive permission; creation, questions and Pub/Sub watches are separate pending capabilities.", Options: options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "form_id")
				if err != nil {
					return nil, err
				}
				args := []string{"forms"}
				if strings.HasPrefix(operation, "responses_") {
					args = append(args, "responses", strings.TrimPrefix(operation, "responses_"))
				} else {
					args = append(args, operation)
				}
				for _, key := range fields {
					if _, present := req.GetArguments()[key]; present {
						args = append(args, "--"+key+"="+req.GetString(key, ""))
					}
				}
				if operation == "publish" {
					for _, key := range []string{"unpublish", "accepting_responses"} {
						if _, present := req.GetArguments()[key]; present {
							args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+strconv.FormatBool(req.GetBool(key, false)))
						}
					}
				}
				if operation == "responses_list" {
					args = append(args, "--max="+strconv.Itoa(req.GetInt("max", 20)))
				}
				args = append(args, "--", id)
				if operation == "responses_get" {
					response, err := requireMCPString(req, "response_id")
					if err != nil {
						return nil, err
					}
					args = append(args, response)
				}
				return args, nil
			},
		})
	}
	return result
}
