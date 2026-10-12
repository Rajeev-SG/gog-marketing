package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedDriveLabelsTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 2)
	for _, operation := range []string{"list", "get"} {
		fields := []string{"language", "view", "fields"}
		options := []mcp.ToolOption{}
		if operation == "get" {
			options = append(options, mcp.WithString("label_name", mcp.Required()))
		} else {
			fields = append(fields, "customer", "minimum_role", "page")
			options = append(options, mcp.WithInteger("max", mcp.Min(1), mcp.Max(200)), mcp.WithBoolean("published_only"))
		}
		for _, key := range fields {
			options = append(options, mcp.WithString(key))
		}
		result = append(result, mcpToolSpec{
			Name: "drivelabels_" + operation, Service: "drivelabels", Risk: mcpRiskRead, Description: "Read native Drive label schemas. Requires drive.labels.readonly and applicable Workspace permissions; no broad Drive access or schema writes.", Options: options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				args := []string{"drive", "labels", operation}
				for _, key := range fields {
					if _, present := req.GetArguments()[key]; present {
						args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+req.GetString(key, ""))
					}
				}
				if operation == "list" {
					args = append(args, "--max="+strconv.Itoa(req.GetInt("max", 50)))
					if _, present := req.GetArguments()["published_only"]; present {
						args = append(args, "--published-only="+strconv.FormatBool(req.GetBool("published_only", true)))
					}
				}
				if operation == "get" {
					name, err := requireMCPString(req, "label_name")
					if err != nil {
						return nil, err
					}
					args = append(args, "--", name)
				}
				return args, nil
			},
		})
	}
	return result
}
