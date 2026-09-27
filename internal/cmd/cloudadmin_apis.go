package cmd

import (
	"context"
	"strings"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type CloudAdminAPIsCmd struct {
	Enable  CloudAdminAPIEnableCmd  `cmd:"" help:"Enable an API in a project"`
	Disable CloudAdminAPIDisableCmd `cmd:"" help:"Disable an API in a project"`
}

type CloudAdminAPIChangeCmd struct {
	Service string `arg:"" name:"api" help:"API/service ID (e.g. cloudbilling.googleapis.com)"`
	Project string `name:"project-id" aliases:"cloud-project" help:"Project to change (required)"`
}

type CloudAdminAPIEnableCmd struct {
	CloudAdminAPIChangeCmd `embed:""`
}

type CloudAdminAPIDisableCmd struct {
	CloudAdminAPIChangeCmd `embed:""`
}

func (c *CloudAdminAPIChangeCmd) validate() (string, string, error) {
	apiID := strings.TrimSpace(c.Service)
	if apiID == "" {
		return "", "", usage("API ID is required")
	}
	project := strings.TrimSpace(c.Project)
	if project == "" {
		return "", "", usage("API changes require --project")
	}
	return project, apiID, nil
}

func (c *CloudAdminAPIEnableCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	project, apiID, err := c.validate()
	if err != nil {
		return err
	}
	op, err := changeCloudAdminAPI(ctx, flags, "cloudadmin.apis.enable", project, apiID, true)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"project": project, "api": apiID, "operation": op})
	}
	return writeResult(ctx, u,
		kv("project", project),
		kv("api", apiID),
		kv("action", "enable"),
		kv("operation", op.OperationName),
	)
}

func (c *CloudAdminAPIDisableCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	project, apiID, err := c.validate()
	if err != nil {
		return err
	}
	op, err := changeCloudAdminAPI(ctx, flags, "cloudadmin.apis.disable", project, apiID, false)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"project": project, "api": apiID, "operation": op})
	}
	return writeResult(ctx, u,
		kv("project", project),
		kv("api", apiID),
		kv("action", "disable"),
		kv("operation", op.OperationName),
	)
}

func changeCloudAdminAPI(ctx context.Context, flags *RootFlags, op string, project, apiID string, enable bool) (*googleapi.CloudAdminOperation, error) {
	plan := map[string]any{"project": project, "api": apiID, "action": "disable"}
	if enable {
		plan["action"] = "enable"
	}
	if err := marketingDryRunExit(ctx, flags, op, plan); err != nil {
		return nil, err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	client, err := cloudAdminClient(ctx, account)
	if err != nil {
		return nil, err
	}
	if enable {
		return client.EnableService(ctx, project, apiID)
	}
	return client.DisableService(ctx, project, apiID)
}
