package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type CloudAdminCmd struct {
	Inventory CloudAdminInventoryCmd `cmd:"" help:"Inventory projects, billing links, enabled APIs, service accounts and transfer configs"`
	Transfers CloudAdminTransfersCmd `cmd:"" help:"Disable or delete BigQuery scheduled transfer configs"`
	APIs      CloudAdminAPIsCmd      `cmd:"" name:"apis" help:"Enable or disable Google Cloud APIs in a project"`
}

type CloudAdminInventoryCmd struct {
	Projects        CloudAdminInventoryProjectsCmd        `cmd:"" help:"List visible Google Cloud projects"`
	Billing         CloudAdminInventoryBillingCmd         `cmd:"" help:"Show billing links (all projects, or --project)"`
	APIs            CloudAdminInventoryAPIsCmd            `cmd:"" name:"apis" help:"Show enabled APIs (all projects, or --project)"`
	ServiceAccounts CloudAdminInventoryServiceAccountsCmd `cmd:"" name:"service-accounts" help:"Show service accounts (all projects, or --project)"`
	Transfers       CloudAdminInventoryTransfersCmd       `cmd:"" help:"Show scheduled transfer configs (all projects, or --project)"`
}

type CloudAdminProjectScope struct {
	Project string `name:"project-id" aliases:"cloud-project" help:"Restrict to one project (defaults to all visible projects)"`
}

type CloudAdminInventoryProjectsCmd struct {
	FailEmpty bool `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no projects"`
}

func (c *CloudAdminInventoryProjectsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := cloudAdminClient(ctx, account)
	if err != nil {
		return err
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"projects": projects}); err != nil {
			return err
		}
		return failEmptyExitIf(c.FailEmpty, len(projects) == 0)
	}
	if len(projects) == 0 {
		u.Err().Println("No projects visible")
		return failEmptyExitIf(c.FailEmpty, true)
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PROJECT_ID\tDISPLAY_NAME\tSTATE\tCREATE_TIME")
	for _, project := range projects {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(project.ProjectID), sanitizeTab(project.Name), sanitizeTab(project.LifecycleState), sanitizeTab(project.CreateTime))
	}
	return nil
}

type CloudAdminInventoryBillingCmd struct {
	CloudAdminProjectScope `embed:""`
}

func (c *CloudAdminInventoryBillingCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCloudAdminPerProject(ctx, flags, c.Project, "billing", "billing links", func(client googleapi.CloudAdminClient, projectID string) (any, error) {
		return client.GetProjectBillingInfo(ctx, projectID)
	})
}

type CloudAdminInventoryAPIsCmd struct {
	CloudAdminProjectScope `embed:""`
}

func (c *CloudAdminInventoryAPIsCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCloudAdminPerProject(ctx, flags, c.Project, "apis", "enabled APIs", func(client googleapi.CloudAdminClient, projectID string) (any, error) {
		return client.ListEnabledServices(ctx, projectID)
	})
}

type CloudAdminInventoryServiceAccountsCmd struct {
	CloudAdminProjectScope `embed:""`
}

func (c *CloudAdminInventoryServiceAccountsCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCloudAdminPerProject(ctx, flags, c.Project, "service-accounts", "service accounts", func(client googleapi.CloudAdminClient, projectID string) (any, error) {
		return client.ListServiceAccounts(ctx, projectID)
	})
}

type CloudAdminInventoryTransfersCmd struct {
	CloudAdminProjectScope `embed:""`
}

func (c *CloudAdminInventoryTransfersCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCloudAdminPerProject(ctx, flags, c.Project, "transfers", "transfer configs", func(client googleapi.CloudAdminClient, projectID string) (any, error) {
		return client.ListTransferConfigs(ctx, projectID)
	})
}

func runCloudAdminPerProject(
	ctx context.Context,
	flags *RootFlags,
	explicitProject, kind, label string,
	call func(googleapi.CloudAdminClient, string) (any, error),
) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := cloudAdminClient(ctx, account)
	if err != nil {
		return err
	}

	projectIDs := strings.TrimSpace(explicitProject)
	if projectIDs == "" {
		projects, listErr := client.ListProjects(ctx)
		if listErr != nil {
			return listErr
		}
		for _, project := range projects {
			if strings.TrimSpace(project.ProjectID) != "" {
				projectIDs = projectIDs + "," + project.ProjectID
			}
		}
		projectIDs = strings.TrimPrefix(projectIDs, ",")
	}

	type projectResult struct {
		Project string `json:"project"`
		Result  any    `json:"result"`
		Error   string `json:"error,omitempty"`
	}

	results := make([]projectResult, 0, 8)
	var firstErr error
	for _, projectID := range strings.Split(projectIDs, ",") {
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			continue
		}
		result, callErr := call(client, projectID)
		if callErr != nil {
			if firstErr == nil {
				firstErr = callErr
			}
			results = append(results, projectResult{Project: projectID, Error: callErr.Error()})
			continue
		}
		results = append(results, projectResult{Project: projectID, Result: result})
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{kind: results}); err != nil {
			return err
		}
	} else {
		var partialErrs []string
		for _, result := range results {
			if result.Error != "" {
				partialErrs = append(partialErrs, result.Project+": "+result.Error)
			}
		}
		if len(results) == 0 && firstErr == nil {
			u.Err().Println("No " + label)
			return nil
		}
		w, flush := tableWriter(ctx)
		defer flush()
		fmt.Fprintln(w, "PROJECT\tRESULT")
		for _, result := range results {
			fmt.Fprintf(w, "%s\t%s\n", sanitizeTab(result.Project), sanitizeTab(fmt.Sprintf("%v", summarizeCloudAdminResult(result.Result))))
		}
		for _, partialErr := range partialErrs {
			u.Err().Println("ERROR " + partialErr)
		}
	}
	return firstErr
}

func summarizeCloudAdminResult(result any) any {
	switch value := result.(type) {
	case *googleapi.CloudAdminBilling:
		return fmt.Sprintf("billing_account=%s enabled=%v", value.BillingAccount, value.BillingEnabled)
	case []googleapi.CloudAdminService:
		return fmt.Sprintf("%d APIs", len(value))
	case []googleapi.CloudAdminServiceAccount:
		return fmt.Sprintf("%d service accounts", len(value))
	case []googleapi.CloudAdminTransferConfig:
		return fmt.Sprintf("%d transfer configs", len(value))
	default:
		return value
	}
}
