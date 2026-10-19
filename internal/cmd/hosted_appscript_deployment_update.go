package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedAppScriptDeploymentUpdateTool() mcpToolSpec {
	return mcpToolSpec{
		Name: "appscript_deployment_update", Service: "appscript", Risk: mcpRiskWrite,
		Description: "Repoint an existing Apps Script deployment to the supplied immutable version, optionally changing its description and manifest file name. Account-wide destructive grant: any deployment under any project the account can access may be modified, and the change applies immediately to web app or API executable consumers. Requires explicit account-level exact-tool destructive permission and one-use chat approval. Omitted optional fields are preserved as-is; the deployment configuration is fetched (GET) and then replaced (PUT) with the merged result. There is no concurrency precondition, so simultaneous edits by others may be overwritten; failed calls are not retried to avoid uncertain partial updates; this never executes the script.",
		Options: []mcp.ToolOption{
			mcp.WithString("script_id", mcp.Required()),
			mcp.WithString("deployment_id", mcp.Required()),
			mcp.WithInteger("version_number", mcp.Required(), mcp.Min(1), mcp.Max(2147483647)),
			mcp.WithString("description"),
			mcp.WithString("manifest_file_name"),
		},
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

			raw := req.GetArguments()["version_number"]
			version, err := validateHostedTabIndexArg(raw)
			if err != nil || version < 1 {
				return nil, usage("version_number must be an integer between 1 and 2147483647")
			}

			argv := []string{"appscript", "deployment-update", "--version-number=" + strconv.Itoa(version)}

			if v, ok := req.GetArguments()["description"]; ok {
				desc, ok := v.(string)
				if !ok {
					return nil, fmt.Errorf("description must be a string")
				}

				argv = append(argv, "--description="+desc)
			}

			if v, ok := req.GetArguments()["manifest_file_name"]; ok {
				manifest, ok := v.(string)
				if !ok {
					return nil, fmt.Errorf("manifest_file_name must be a string")
				}
				if strings.TrimSpace(manifest) == "" {
					return nil, fmt.Errorf("manifest_file_name must not be blank")
				}

				argv = append(argv, "--manifest-file-name="+manifest)
			}

			return append(argv, "--", scriptID, deploymentID), nil
		},
	}
}
