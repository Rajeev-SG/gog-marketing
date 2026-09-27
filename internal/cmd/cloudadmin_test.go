package cmd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
)

type fakeCloudAdminClient struct {
	projects         []googleapi.CloudAdminProject
	billing          map[string]*googleapi.CloudAdminBilling
	apis             map[string][]googleapi.CloudAdminService
	accounts         map[string][]googleapi.CloudAdminServiceAccount
	transfers        map[string][]googleapi.CloudAdminTransferConfig
	disabledConfigs  []string
	deletedConfigs   []string
	enabledServices  []string
	disabledServices []string
}

func (f *fakeCloudAdminClient) ListProjects(context.Context) ([]googleapi.CloudAdminProject, error) {
	return f.projects, nil
}

func (f *fakeCloudAdminClient) GetProjectBillingInfo(_ context.Context, projectID string) (*googleapi.CloudAdminBilling, error) {
	if info, ok := f.billing[projectID]; ok {
		return info, nil
	}
	return &googleapi.CloudAdminBilling{ProjectID: projectID}, nil
}

func (f *fakeCloudAdminClient) ListEnabledServices(_ context.Context, projectID string) ([]googleapi.CloudAdminService, error) {
	return f.apis[projectID], nil
}

func (f *fakeCloudAdminClient) ListServiceAccounts(_ context.Context, projectID string) ([]googleapi.CloudAdminServiceAccount, error) {
	return f.accounts[projectID], nil
}

func (f *fakeCloudAdminClient) ListTransferConfigs(_ context.Context, projectID string) ([]googleapi.CloudAdminTransferConfig, error) {
	return f.transfers[projectID], nil
}

func (f *fakeCloudAdminClient) DisableTransferConfig(_ context.Context, configName string) (*googleapi.CloudAdminTransferConfig, error) {
	f.disabledConfigs = append(f.disabledConfigs, configName)
	return &googleapi.CloudAdminTransferConfig{Name: configName, Disabled: true}, nil
}

func (f *fakeCloudAdminClient) DeleteTransferConfig(_ context.Context, configName string) error {
	f.deletedConfigs = append(f.deletedConfigs, configName)
	return nil
}

func (f *fakeCloudAdminClient) EnableService(_ context.Context, projectID, apiID string) (*googleapi.CloudAdminOperation, error) {
	f.enabledServices = append(f.enabledServices, projectID+"/"+apiID)
	return &googleapi.CloudAdminOperation{OperationName: "op-enable"}, nil
}

func (f *fakeCloudAdminClient) DisableService(_ context.Context, projectID, apiID string) (*googleapi.CloudAdminOperation, error) {
	f.disabledServices = append(f.disabledServices, projectID+"/"+apiID)
	return &googleapi.CloudAdminOperation{OperationName: "op-disable"}, nil
}

func executeWithCloudAdmin(t *testing.T, args []string, client *fakeCloudAdminClient) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		CloudAdmin: func(context.Context, string) (googleapi.CloudAdminClient, error) { return client, nil },
	}})
}

func TestCloudAdminInventoryProjectsJSON(t *testing.T) {
	client := &fakeCloudAdminClient{projects: []googleapi.CloudAdminProject{
		{ProjectID: "demo-proj", Name: "Demo", LifecycleState: "ACTIVE"},
	}}
	result := executeWithCloudAdmin(t, []string{"--account", "a@b.com", "--json", "cloudadmin", "inventory", "projects"}, client)
	if result.err != nil {
		t.Fatalf("execute: %v", result.err)
	}
	var resp struct {
		Projects []googleapi.CloudAdminProject `json:"projects"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &resp); err != nil {
		t.Fatalf("json: %v out=%s", err, result.stdout)
	}
	if len(resp.Projects) != 1 || resp.Projects[0].ProjectID != "demo-proj" {
		t.Fatalf("unexpected projects: %#v", resp.Projects)
	}
}

func TestCloudAdminInventoryBillingCoversAllProjects(t *testing.T) {
	client := &fakeCloudAdminClient{
		projects: []googleapi.CloudAdminProject{{ProjectID: "demo-proj"}, {ProjectID: "other-proj"}},
		billing: map[string]*googleapi.CloudAdminBilling{
			"demo-proj": {ProjectID: "demo-proj", BillingAccount: "billingAccounts/ABC-DEF-123", BillingEnabled: true},
		},
	}
	result := executeWithCloudAdmin(t, []string{"--account", "a@b.com", "--json", "cloudadmin", "inventory", "billing"}, client)
	if result.err != nil {
		t.Fatalf("execute: %v", result.err)
	}
	var resp struct {
		Billing []struct {
			Project string                       `json:"project"`
			Result  *googleapi.CloudAdminBilling `json:"result"`
		} `json:"billing"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &resp); err != nil {
		t.Fatalf("json: %v out=%s", err, result.stdout)
	}
	if len(resp.Billing) != 2 {
		t.Fatalf("expected both projects, got %#v", resp.Billing)
	}
	if resp.Billing[0].Result.BillingAccount != "billingAccounts/ABC-DEF-123" {
		t.Fatalf("unexpected billing link: %#v", resp.Billing[0])
	}
}

func TestCloudAdminTransferDisableDryRunAndForce(t *testing.T) {
	client := &fakeCloudAdminClient{}
	configName := "projects/demo-proj/locations/us/transferConfigs/tc-1"

	dry := executeWithCloudAdmin(t, []string{
		"--account", "a@b.com", "--dry-run", "--json",
		"cloudadmin", "transfers", "disable", "--project-id", "demo-proj", configName,
	}, client)
	if dry.err != nil {
		t.Fatalf("dry-run: %v", dry.err)
	}
	if len(client.disabledConfigs) != 0 {
		t.Fatal("dry-run must not mutate")
	}

	run := executeWithCloudAdmin(t, []string{
		"--account", "a@b.com", "--force", "--json",
		"cloudadmin", "transfers", "disable", "--project-id", "demo-proj", configName,
	}, client)
	if run.err != nil {
		t.Fatalf("disable: %v", run.err)
	}
	if len(client.disabledConfigs) != 1 || client.disabledConfigs[0] != configName {
		t.Fatalf("unexpected disabled configs: %v", client.disabledConfigs)
	}
}

func TestCloudAdminTransferDeleteRequiresConfirmation(t *testing.T) {
	client := &fakeCloudAdminClient{}
	configName := "projects/demo-proj/locations/us/transferConfigs/tc-1"

	refused := executeWithCloudAdmin(t, []string{
		"--account", "a@b.com", "--json",
		"cloudadmin", "transfers", "delete", "--project-id", "demo-proj", configName,
	}, client)
	if refused.err == nil {
		t.Fatal("delete must require confirmation without --force")
	}
	if len(client.deletedConfigs) != 0 {
		t.Fatal("refused delete must not mutate")
	}

	forced := executeWithCloudAdmin(t, []string{
		"--account", "a@b.com", "--force", "--json",
		"cloudadmin", "transfers", "delete", "--project-id", "demo-proj", configName,
	}, client)
	if forced.err != nil {
		t.Fatalf("forced delete: %v", forced.err)
	}
	if len(client.deletedConfigs) != 1 || client.deletedConfigs[0] != configName {
		t.Fatalf("unexpected deleted configs: %v", client.deletedConfigs)
	}
}
