package cmd

import (
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedAppScriptManagementTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name: "appscript_bound_create", Service: "appscript", Risk: mcpRiskWrite,
			Description: "Create an Apps Script project bound to an explicitly selected parent document, spreadsheet, presentation or form. Requires an enabled appscript file grant for that parent and exact-tool write approval.",
			Options:     []mcp.ToolOption{mcp.WithString("title", mcp.Required()), mcp.WithString("parent_id", mcp.Required())},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				title, err := requireMCPString(req, "title")
				if err != nil {
					return nil, err
				}
				parent, err := requireMCPString(req, "parent_id")
				if err != nil {
					return nil, err
				}
				return []string{"appscript", "bound-create", "--title=" + title, "--parent-id=" + parent}, nil
			},
		},
		{
			Name: "appscript_push", Service: "appscript", Risk: mcpRiskWrite,
			Description: "Replace ALL saved HEAD files, including the manifest, in an Apps Script project. Can affect existing triggers. Requires explicit account-level exact-tool destructive permission and approval. Supply literal files JSON, never a server path.",
			Options:     []mcp.ToolOption{mcp.WithString("script_id", mcp.Required()), mcp.WithString("files_json", mcp.Required())},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "script_id")
				if err != nil {
					return nil, err
				}
				raw, err := requireMCPString(req, "files_json")
				if err != nil {
					return nil, err
				}
				if _, err = decodeAppScriptPushFiles(raw); err != nil {
					return nil, err
				}
				return []string{"appscript", "push", "--files-json=" + raw, "--", id}, nil
			},
		},
		{
			Name: "appscript_version_create", Service: "appscript", Risk: mcpRiskWrite,
			Description: "Create an immutable Apps Script version of the current saved project under an account-level exact-tool write grant and approval.",
			Options:     []mcp.ToolOption{mcp.WithString("script_id", mcp.Required()), mcp.WithString("description")},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "script_id")
				if err != nil {
					return nil, err
				}
				return []string{"appscript", "version-create", "--description=" + req.GetString("description", ""), "--", id}, nil
			},
		},
		{
			Name: "appscript_deployment_create", Service: "appscript", Risk: mcpRiskWrite,
			Description: "Create an Apps Script deployment pinned to an immutable version. May expose executable entry points declared by that version. Requires explicit account-level exact-tool destructive permission and approval; does not run the script.",
			Options:     []mcp.ToolOption{mcp.WithString("script_id", mcp.Required()), mcp.WithInteger("version_number", mcp.Required(), mcp.Min(1), mcp.Max(2147483647)), mcp.WithString("description"), mcp.WithString("manifest_file_name")},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "script_id")
				if err != nil {
					return nil, err
				}
				v := req.GetInt("version_number", 0)
				if v < 1 || v > 2147483647 {
					return nil, fmt.Errorf("version must be a positive int32")
				}
				return []string{"appscript", "deployment-create", "--version-number=" + strconv.Itoa(v), "--description=" + req.GetString("description", ""), "--manifest-file-name=" + req.GetString("manifest_file_name", ""), "--", id}, nil
			},
		},
		{
			Name: "appscript_deployment_delete", Service: "appscript", Risk: mcpRiskWrite,
			Description: "Permanently delete a single Apps Script deployment by exact project and deployment ID. Account-wide destructive grant: any deployment under any project the account can access may be removed, cannot be undone, and may break existing web app or API executable consumers. Requires explicit account-level exact-tool destructive permission and one-use chat approval; does not run the script.",
			Options:     []mcp.ToolOption{mcp.WithString("script_id", mcp.Required()), mcp.WithString("deployment_id", mcp.Required())},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				scriptID, err := requireMCPString(req, "script_id")
				if err != nil {
					return nil, err
				}
				if !hostedDocsResourceIDPattern.MatchString(scriptID) {
					return nil, fmt.Errorf("invalid script project ID")
				}
				deploymentID, err := requireMCPString(req, "deployment_id")
				if err != nil {
					return nil, err
				}
				if !hostedDocsResourceIDPattern.MatchString(deploymentID) {
					return nil, fmt.Errorf("invalid deployment ID")
				}
				return []string{"appscript", "deployment-delete", "--", scriptID, deploymentID}, nil
			},
		},
	}
}
