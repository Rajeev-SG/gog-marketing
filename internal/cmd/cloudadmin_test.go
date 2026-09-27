package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	failedOperation  string
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
	if projectID == "failing-proj" {
		return nil, fmt.Errorf("permission denied on failing-proj")
	}
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
	if f.failedOperation != "" {
		return &googleapi.CloudAdminOperation{OperationName: "op-failed", Done: true, Error: f.failedOperation}, nil
	}
	return &googleapi.CloudAdminOperation{OperationName: "op-enable", Done: true}, nil
}

func (f *fakeCloudAdminClient) DisableService(_ context.Context, projectID, apiID string) (*googleapi.CloudAdminOperation, error) {
	f.disabledServices = append(f.disabledServices, projectID+"/"+apiID)
	return &googleapi.CloudAdminOperation{OperationName: "op-disable", Done: true}, nil
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

func TestCloudAdminTransferBareIDRejected(t *testing.T) {
	client := &fakeCloudAdminClient{}
	result := executeWithCloudAdmin(t, []string{
		"--account", "a@b.com", "--force", "--json",
		"cloudadmin", "transfers", "disable", "--project-id", "demo-proj", "tc-1",
	}, client)
	if result.err == nil || !strings.Contains(result.err.Error(), "full transfer config resource name") {
		t.Fatalf("expected usage error for bare transfer ID, got %v", result.err)
	}
	if len(client.disabledConfigs) != 0 {
		t.Fatal("rejected name must not mutate")
	}
}

func TestCloudAdminWriteBlockedByReadOnly(t *testing.T) {
	client := &fakeCloudAdminClient{}
	configName := "projects/demo-proj/locations/us/transferConfigs/tc-1"

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"transfers-disable", []string{"cloudadmin", "transfers", "disable", "--project-id", "demo-proj", configName}},
		{"transfers-delete", []string{"cloudadmin", "transfers", "delete", "--project-id", "demo-proj", configName}},
		{"apis-enable", []string{"cloudadmin", "apis", "enable", "--project-id", "demo-proj", "cloudbilling.googleapis.com"}},
		{"apis-disable", []string{"cloudadmin", "apis", "disable", "--project-id", "demo-proj", "cloudbilling.googleapis.com"}},
	} {
		result := executeWithCloudAdmin(t, append([]string{"--account", "a@b.com", "--readonly", "--force", "--json"}, tc.args...), client)
		if result.err == nil {
			t.Fatalf("%s: --readonly must block the write", tc.name)
		}
	}
	if len(client.disabledConfigs) != 0 || len(client.deletedConfigs) != 0 || len(client.enabledServices) != 0 || len(client.disabledServices) != 0 {
		t.Fatal("readonly run must not mutate")
	}
}

func TestCloudAdminAPIDisableRequiresConfirmation(t *testing.T) {
	client := &fakeCloudAdminClient{}

	refused := executeWithCloudAdmin(t, []string{
		"--account", "a@b.com", "--json",
		"cloudadmin", "apis", "disable", "--project-id", "demo-proj", "cloudbilling.googleapis.com",
	}, client)
	if refused.err == nil {
		t.Fatal("apis disable must require confirmation without --force")
	}
	if len(client.disabledServices) != 0 {
		t.Fatal("refused disable must not mutate")
	}

	forced := executeWithCloudAdmin(t, []string{
		"--account", "a@b.com", "--force", "--json",
		"cloudadmin", "apis", "disable", "--project-id", "demo-proj", "cloudbilling.googleapis.com",
	}, client)
	if forced.err != nil {
		t.Fatalf("forced disable: %v", forced.err)
	}
	if len(client.disabledServices) != 1 {
		t.Fatalf("unexpected disabled services: %v", client.disabledServices)
	}
}

func TestCloudAdminInventoryIncludesPerProjectErrors(t *testing.T) {
	client := &fakeCloudAdminClient{projects: []googleapi.CloudAdminProject{{ProjectID: "demo-proj"}, {ProjectID: "failing-proj"}}}
	// Use inventory billing where the fake returns results for both projects;
	// exercise the error path through a nil-safe result caller instead.
	result := executeWithCloudAdmin(t, []string{
		"--account", "a@b.com", "--json", "cloudadmin", "inventory", "apis",
	}, client)
	if result.err == nil {
		t.Fatal("aggregate status must be nonzero when a project fails")
	}
	var resp struct {
		APIs []struct {
			Project string `json:"project"`
			Error   string `json:"error,omitempty"`
		} `json:"apis"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &resp); err != nil {
		t.Fatalf("json: %v out=%s", err, result.stdout)
	}
	if len(resp.APIs) != 2 || resp.APIs[1].Project != "failing-proj" || !strings.Contains(resp.APIs[1].Error, "permission denied") {
		t.Fatalf("unexpected entries: %#v", resp.APIs)
	}
}

func TestCloudAdminAPIChangeFailsOnFailedOperation(t *testing.T) {
	client := &fakeCloudAdminClient{}
	client.failedOperation = "permission denied"

	result := executeWithCloudAdmin(t, []string{
		"--account", "a@b.com", "--force", "--json",
		"cloudadmin", "apis", "enable", "--project-id", "demo-proj", "cloudbilling.googleapis.com",
	}, client)
	if result.err == nil || !strings.Contains(result.err.Error(), "permission denied") {
		t.Fatalf("expected nonzero exit for failed operation, got %v", result.err)
	}
}
