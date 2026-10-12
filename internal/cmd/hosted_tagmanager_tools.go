package cmd

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// Adapt only the selector shape; keep the local MCP argument builder and native
// publish handler. A canonical container path is the persisted resource identity.
func hostedTagManagerPublishTool() mcpToolSpec {
	spec := mcpTagManagerPublishTool()
	build := spec.BuildArgs
	spec.Options = []mcp.ToolOption{
		mcp.WithString("container_path", mcp.Required(), mcp.Description("Enabled container path: accounts/123/containers/456")),
		mcp.WithString("version", mcp.Required()),
		mcp.WithString("fingerprint"),
	}
	spec.Description = "Publish a version of an explicitly enabled GTM container. Requires publish scope, write and destructive permission."
	spec.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		path, err := requireMCPString(req, "container_path")
		if err != nil {
			return nil, err
		}

		parts := strings.Split(path, "/")
		if len(parts) != 4 || parts[0] != "accounts" || parts[2] != "containers" {
			return nil, ErrHostedToolInput
		}

		req.Params.Arguments = map[string]any{
			"account": parts[1], "container": parts[3],
			"version": req.GetString("version", ""), "fingerprint": req.GetString("fingerprint", ""),
		}
		return build(req)
	}
	return spec
}
