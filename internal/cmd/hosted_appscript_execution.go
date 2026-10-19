package cmd

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedAppScriptExecutionTool() mcpToolSpec {
	return mcpToolSpec{
		Name: "appscript_run", Service: "appscript", Risk: mcpRiskWrite,
		Description: "Execute a deployed Apps Script function with JSON parameters. Enabling this exact account-level tool with destructive permission authorizes arbitrary script effects across the selected Google account within ALL scopes of that connection; selected-file grants do NOT sandbox script code. Chat additionally requires one-use exact-argument write approval. Tokens carrying BigQuery scope are rejected because arbitrary scripts cannot enforce max-bytes-billed. Requires an API-executable deployment, same standard Cloud project as the OAuth client, enabled Apps Script API and all scopes required by the entire script. Use deployment ID, not project ID; development/HEAD mode is not exposed.",
		Options:     []mcp.ToolOption{mcp.WithString("deployment_id", mcp.Required()), mcp.WithString("function", mcp.Required()), mcp.WithString("params_json", mcp.DefaultString("[]"))},
		BuildArgs:   hostedAppScriptRunArgs,
	}
}

// hostedAppScriptRunArgs builds native CLI argv for a hosted Apps Script run.
// The script identifier is the deployment ID (the Google discovery SDK path
// identifier), not the project ID. No dev-mode flags, local file/stdin/server
// paths, or credential inputs are accepted.
func hostedAppScriptRunArgs(req mcp.CallToolRequest) ([]string, error) {
	deploymentID, err := requireMCPString(req, "deployment_id")
	if err != nil {
		return nil, err
	}
	fnName, err := requireMCPString(req, "function")
	if err != nil {
		return nil, err
	}
	paramsRaw := req.GetString("params_json", "[]")
	trimmed := strings.TrimSpace(paramsRaw)
	if !strings.HasPrefix(trimmed, "[") {
		return nil, fmt.Errorf("usage: params_json must be a JSON array literal (e.g. [\"arg\",42])")
	}
	if _, parseErr := parseJSONArray(trimmed); parseErr != nil {
		return nil, fmt.Errorf("usage: params_json is not a valid JSON array: %w", parseErr)
	}
	return []string{
		"appscript",
		"run",
		"--params=" + paramsRaw,
		"--",
		deploymentID,
		fnName,
	}, nil
}
