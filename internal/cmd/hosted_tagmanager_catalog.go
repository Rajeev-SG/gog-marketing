package cmd

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/hosted/policy"
)

func hostedTagManagerTools() []mcpToolSpec {
	result := []mcpToolSpec{}
	for _, op := range policy.TagManagerOperations() {
		options := []mcp.ToolOption{}
		if op.Selector != "" {
			options = append(options, mcp.WithString(op.ResourceArgument(), mcp.Required()))
		}
		if op.Workspace {
			options = append(options, mcp.WithString("workspace_id", mcp.Required()))
		}
		if op.Item {
			key := "resource_id"
			if op.Family == "versions" {
				key = "version_id"
			}
			options = append(options, mcp.WithString(key, mcp.Required()))
		}
		mutation := op.Action == "create" || op.Action == "update"
		if mutation {
			options = append(options, mcp.WithString("body_json", mcp.Required(), mcp.Description("Inline native resource JSON object; no files or foreign paths")))
		}
		if op.Action == "list" {
			options = append(options, mcp.WithString("page"), mcp.WithBoolean("all"), mcp.WithBoolean("fail_empty"))
		}
		if op.Action == "create-version" {
			options = append(options, mcp.WithString("version_name"), mcp.WithString("notes"))
		}
		risk := mcpRiskRead
		if op.Method != http.MethodGet {
			risk = mcpRiskWrite
		}
		result = append(result, mcpToolSpec{Name: op.Name(), Service: "tagmanager", Risk: risk, Description: "Run native GTM " + op.Family + " " + op.Action + " under explicit account/tool and selected account/container grants. Mutations require method-appropriate consent; deletion and workspace version creation require destructive permission.", Options: options, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			path, err := op.Path(req.GetArguments())
			if err != nil {
				return nil, ErrHostedToolInput
			}
			args := []string{"tagmanager", op.Family, op.Action}
			if mutation {
				raw, err := requireMCPString(req, "body_json")
				if err != nil {
					return nil, err
				}
				var body map[string]any
				if json.Unmarshal([]byte(raw), &body) != nil || body == nil {
					return nil, ErrHostedToolInput
				}
				if value, present := body["path"]; present && value != "" && (op.Action == "create" || value != strings.TrimPrefix(path, "/tagmanager/v2/")) {
					return nil, ErrHostedToolInput
				}
				args = append(args, "--json-file="+raw)
			}
			if op.Action == "list" {
				if page := req.GetString("page", ""); page != "" {
					args = append(args, "--page-token="+page)
				}
				if req.GetBool("all", false) {
					args = append(args, "--all")
				}
				if req.GetBool("fail_empty", false) {
					args = append(args, "--fail-empty")
				}
			}
			if op.Action == "create-version" {
				args = append(args, "--name="+req.GetString("version_name", ""), "--notes="+req.GetString("notes", ""))
			}
			ids := []string{}
			if op.Selector != "" {
				parts := strings.Split(req.GetString(op.ResourceArgument(), ""), "/")
				ids = append(ids, parts[1])
				if op.Selector == "container" {
					ids = append(ids, parts[3])
				}
			}
			if op.Workspace {
				ids = append(ids, req.GetString("workspace_id", ""))
			}
			if op.Item {
				key := "resource_id"
				if op.Family == "versions" {
					key = "version_id"
				}
				ids = append(ids, req.GetString(key, ""))
			}
			return append(append(args, "--"), ids...), nil
		}})
	}
	return append(result, hostedTagManagerPublishTool())
}
