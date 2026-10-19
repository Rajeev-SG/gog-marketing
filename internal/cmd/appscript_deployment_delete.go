package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type AppScriptDeploymentDeleteCmd struct {
	ScriptID     string `arg:"" name:"scriptId" help:"Script ID"`
	DeploymentID string `arg:"" name:"deploymentId" help:"Deployment ID"`
}

func (c *AppScriptDeploymentDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	scriptID := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if scriptID == "" {
		return usage("script id is required")
	}
	deploymentID := strings.TrimSpace(normalizeGoogleID(c.DeploymentID))
	if deploymentID == "" {
		return usage("deployment id is required")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "appscript.deployment.delete", map[string]any{
		"script_id":     scriptID,
		"deployment_id": deploymentID,
	}, "delete Apps Script deployment"); confirmErr != nil {
		return confirmErr
	}

	ctx = googleapi.WithoutRetries(ctx)
	svc, err := requireAppScriptService(ctx, flags)
	if err != nil {
		return err
	}

	if _, err := svc.Projects.Deployments.Delete(scriptID, deploymentID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete deployment: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"deleted":       true,
			"script_id":     scriptID,
			"deployment_id": deploymentID,
		})
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("script_id: %s", scriptID)
	u.Out().Linef("deployment_id: %s", deploymentID)
	u.Out().Linef("deleted: true")
	return nil
}
