package cmd

import (
	"context"
	"fmt"
	"strings"

	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type AppScriptDeploymentUpdateCmd struct {
	ScriptID         string  `arg:"" name:"scriptId" help:"Script ID"`
	DeploymentID     string  `arg:"" name:"deploymentId" help:"Deployment ID"`
	VersionNumber    int64   `help:"New version number (positive)" required:""`
	Description      *string `help:"New description (empty string clears)"`
	ManifestFileName *string `help:"New manifest file name"`
}

func (c *AppScriptDeploymentUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	scriptID := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if scriptID == "" {
		return usage("script id is required")
	}
	deploymentID := strings.TrimSpace(normalizeGoogleID(c.DeploymentID))
	if deploymentID == "" {
		return usage("deployment id is required")
	}
	if c.VersionNumber <= 0 || c.VersionNumber > 2147483647 {
		return usage("version number must be a positive 32-bit integer")
	}
	if c.ManifestFileName != nil && strings.TrimSpace(*c.ManifestFileName) == "" {
		return usage("manifest file name must not be blank")
	}

	payload := map[string]any{
		"script_id":      scriptID,
		"deployment_id":  deploymentID,
		"version_number": c.VersionNumber,
	}
	if c.Description != nil {
		payload["description"] = *c.Description
	}
	if c.ManifestFileName != nil {
		payload["manifest_file_name"] = *c.ManifestFileName
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "appscript.deployment.update", payload, "update Apps Script deployment"); confirmErr != nil {
		return confirmErr
	}

	ctx = googleapi.WithoutRetries(ctx)
	svc, err := requireAppScriptService(ctx, flags)
	if err != nil {
		return err
	}

	deployment, err := svc.Projects.Deployments.Get(scriptID, deploymentID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if deployment.DeploymentConfig == nil {
		return fmt.Errorf("deployment %s has no config", deploymentID)
	}

	config := *deployment.DeploymentConfig
	config.VersionNumber = c.VersionNumber
	if c.Description != nil {
		config.Description = *c.Description
		if *c.Description == "" {
			config.ForceSendFields = append(config.ForceSendFields, "Description")
		}
	}
	if c.ManifestFileName != nil {
		config.ManifestFileName = *c.ManifestFileName
	}

	reply, err := svc.Projects.Deployments.Update(scriptID, deploymentID, &scriptapi.UpdateDeploymentRequest{
		DeploymentConfig: &config,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update deployment: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"updated":    true,
			"deployment": reply,
		})
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("script_id: %s", scriptID)
	u.Out().Linef("deployment_id: %s", deploymentID)
	u.Out().Linef("version_number: %d", c.VersionNumber)
	u.Out().Linef("updated: true")
	return nil
}
