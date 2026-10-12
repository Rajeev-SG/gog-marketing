package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedPhotosLibraryTools() []mcpToolSpec {
	result := make([]mcpToolSpec, 0, 3)
	for _, operation := range []string{"list", "get", "search"} {
		options := []mcp.ToolOption{}
		fields := []string{}
		if operation == "get" {
			options = append(options, mcp.WithString("media_id", mcp.Required()))
		} else {
			options = append(options, mcp.WithInteger("max", mcp.Min(1), mcp.Max(100)))
			fields = append(fields, "page")
		}
		if operation == "search" {
			fields = append(fields, "album", "media_type", "from", "to", "order")
			options = append(options, mcp.WithBoolean("include_archived"))
		}
		for _, key := range fields {
			options = append(options, mcp.WithString(key))
		}
		result = append(result, mcpToolSpec{
			Name: "photos_" + operation, Service: "photos", Risk: mcpRiskRead, Description: "Read native app-created Photos media only with explicit account/tool grants and photoslibrary.readonly.appcreateddata. No general-library browsing or media writes.", Options: options,
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				args := []string{"photos", operation}
				for _, key := range fields {
					if _, present := req.GetArguments()[key]; present {
						args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+req.GetString(key, ""))
					}
				}
				if operation != "get" {
					args = append(args, "--max="+strconv.Itoa(req.GetInt("max", 25)))
				}
				if operation == "search" && req.GetBool("include_archived", false) {
					args = append(args, "--include-archived")
				}
				if operation == "get" {
					id, err := requireMCPString(req, "media_id")
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
