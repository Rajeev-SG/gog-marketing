package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type CloudAdminTransfersCmd struct {
	Disable CloudAdminTransferDisableCmd `cmd:"" help:"Disable a scheduled transfer config (reversible)"`
	Delete  CloudAdminTransferDeleteCmd  `cmd:"" help:"Delete a scheduled transfer config (destructive)"`
}

type CloudAdminTransferName struct {
	Name    string `arg:"" name:"config" help:"Transfer config resource name (projects/PROJECT/locations/LOCATION/transferConfigs/ID)"`
	Project string `name:"project-id" aliases:"cloud-project" help:"Project that owns the transfer config"`
}

func (f *CloudAdminTransferName) requireProject() (string, error) {
	project := strings.TrimSpace(f.Project)
	if project == "" {
		return "", usage("transfer config requires --project (the owning project)")
	}
	return project, nil
}

func (f *CloudAdminTransferName) requireConfigName(project string) (string, error) {
	name := strings.TrimSpace(f.Name)
	if name == "" {
		return "", usage("transfer config name is required")
	}
	if !strings.Contains(name, "/") {
		name = fmt.Sprintf("projects/%s/locations/us/transferConfigs/%s", project, name)
	}
	return name, nil
}

type CloudAdminTransferDisableCmd struct {
	CloudAdminTransferName `embed:""`
}

func (c *CloudAdminTransferDisableCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	project, err := c.requireProject()
	if err != nil {
		return err
	}
	configName, err := c.requireConfigName(project)
	if err != nil {
		return err
	}
	dryRunErr := marketingDryRunExit(ctx, flags, "cloudadmin.transfers.disable", map[string]any{
		"transfer_config": configName,
		"project":         project,
		"action":          "disable",
	})
	if dryRunErr != nil {
		return dryRunErr
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := cloudAdminClient(ctx, account)
	if err != nil {
		return err
	}
	updated, err := client.DisableTransferConfig(ctx, configName)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"transfer_config": updated})
	}
	return writeResult(ctx, u,
		kv("transfer_config", updated.Name),
		kv("disabled", updated.Disabled),
	)
}

type CloudAdminTransferDeleteCmd struct {
	CloudAdminTransferName `embed:""`
}

func (c *CloudAdminTransferDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	project, err := c.requireProject()
	if err != nil {
		return err
	}
	configName, err := c.requireConfigName(project)
	if err != nil {
		return err
	}
	dryRunErr := marketingDryRunAndConfirmDestructive(ctx, flags, "cloudadmin.transfers.delete", map[string]any{
		"transfer_config": configName,
		"project":         project,
		"action":          "delete",
	}, fmt.Sprintf("Delete scheduled transfer config %s (irreversible; schedule stops billing first with disable)", configName))
	if dryRunErr != nil {
		return dryRunErr
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := cloudAdminClient(ctx, account)
	if err != nil {
		return err
	}
	if err := client.DeleteTransferConfig(ctx, configName); err != nil {
		return err
	}
	return writeResult(ctx, u,
		kv("deleted", true),
		kv("transfer_config", configName),
	)
}
