package googleapi

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/bigquerydatatransfer/v1"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/cloudresourcemanager/v3"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/serviceusage/v1"

	"github.com/openclaw/gogcli/internal/googleauth"
)

// cloudAdminScope covers the Resource Manager, Service Usage, Cloud Billing,
// IAM and BigQuery Data Transfer reads/writes exposed through the narrow
// cloud-admin capability. These admin APIs have no read-only scope, so the
// capability is opt-in via an explicit "cloudadmin" auth service.
const scopeCloudAdmin = "https://www.googleapis.com/auth/cloud-platform"

type CloudAdminProject struct {
	ProjectID      string            `json:"project_id"`
	Name           string            `json:"display_name,omitempty"`
	ResourceName   string            `json:"resource_name,omitempty"`
	LifecycleState string            `json:"lifecycle_state,omitempty"`
	CreateTime     string            `json:"create_time,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
}

type CloudAdminBilling struct {
	ProjectID      string `json:"project_id"`
	BillingAccount string `json:"billing_account,omitempty"`
	BillingEnabled bool   `json:"billing_enabled"`
	ResourceName   string `json:"resource_name,omitempty"`
}

type CloudAdminService struct {
	Name  string `json:"name,omitempty"`
	APIID string `json:"api_id"`
	State string `json:"state,omitempty"`
	Title string `json:"title,omitempty"`
}

type CloudAdminServiceAccount struct {
	Name        string `json:"name,omitempty"`
	Email       string `json:"email"`
	UniqueID    string `json:"unique_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Disabled    bool   `json:"disabled"`
}

type CloudAdminTransferConfig struct {
	Name                 string `json:"name"`
	DisplayName          string `json:"display_name,omitempty"`
	DataSourceID         string `json:"data_source_id,omitempty"`
	DestinationDatasetID string `json:"destination_dataset_id,omitempty"`
	Schedule             string `json:"schedule,omitempty"`
	Disabled             bool   `json:"disabled"`
}

type CloudAdminOperation struct {
	OperationName string `json:"operation_name,omitempty"`
}

type CloudAdminClient interface {
	ListProjects(context.Context) ([]CloudAdminProject, error)
	GetProjectBillingInfo(context.Context, string) (*CloudAdminBilling, error)
	ListEnabledServices(context.Context, string) ([]CloudAdminService, error)
	ListServiceAccounts(context.Context, string) ([]CloudAdminServiceAccount, error)
	ListTransferConfigs(context.Context, string) ([]CloudAdminTransferConfig, error)
	DisableTransferConfig(context.Context, string) (*CloudAdminTransferConfig, error)
	DeleteTransferConfig(context.Context, string) error
	EnableService(context.Context, string, string) (*CloudAdminOperation, error)
	DisableService(context.Context, string, string) (*CloudAdminOperation, error)
}

type CloudAdminClientFactory func(context.Context, string) (CloudAdminClient, error)

func NewCloudAdmin(ctx context.Context, account string) (CloudAdminClient, error) {
	client, err := NewHTTPClientForScopes(ctx, string(googleauth.ServiceCloudAdmin), account, []string{scopeCloudAdmin})
	if err != nil {
		return nil, fmt.Errorf("cloud-admin auth client: %w", err)
	}

	resourceManager, err := cloudresourcemanager.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create resource manager client: %w", err)
	}

	billing, err := cloudbilling.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create cloud billing client: %w", err)
	}

	serviceUsage, err := serviceusage.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create service usage client: %w", err)
	}

	iamService, err := iam.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create iam client: %w", err)
	}

	dataTransfer, err := bigquerydatatransfer.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create bigquery data transfer client: %w", err)
	}

	return &cloudAdminAdapter{
		resourceManager: resourceManager,
		billing:         billing,
		serviceUsage:    serviceUsage,
		iam:             iamService,
		dataTransfer:    dataTransfer,
	}, nil
}

type cloudAdminAdapter struct {
	resourceManager *cloudresourcemanager.Service
	billing         *cloudbilling.APIService
	serviceUsage    *serviceusage.Service
	iam             *iam.Service
	dataTransfer    *bigquerydatatransfer.Service
}

func (a *cloudAdminAdapter) ListProjects(ctx context.Context) ([]CloudAdminProject, error) {
	var out []CloudAdminProject

	if err := a.resourceManager.Projects.Search().Pages(ctx, func(page *cloudresourcemanager.SearchProjectsResponse) error {
		for _, project := range page.Projects {
			out = append(out, CloudAdminProject{
				ProjectID:      projectIDFromResourceName(project.Name),
				Name:           project.DisplayName,
				ResourceName:   project.Name,
				LifecycleState: project.State,
				CreateTime:     project.CreateTime,
				Labels:         project.Labels,
			})
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("search projects: %w", err)
	}

	return out, nil
}

func (a *cloudAdminAdapter) GetProjectBillingInfo(ctx context.Context, projectID string) (*CloudAdminBilling, error) {
	info, err := a.billing.Projects.GetBillingInfo("projects/" + projectID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get billing info for %s: %w", projectID, err)
	}

	return &CloudAdminBilling{
		ProjectID:      projectID,
		BillingAccount: info.BillingAccountName,
		BillingEnabled: info.BillingEnabled,
		ResourceName:   info.Name,
	}, nil
}

func (a *cloudAdminAdapter) ListEnabledServices(ctx context.Context, projectID string) ([]CloudAdminService, error) {
	var out []CloudAdminService

	if err := a.serviceUsage.Services.List("projects/"+projectID).Pages(ctx, func(page *serviceusage.ListServicesResponse) error {
		for _, service := range page.Services {
			out = append(out, CloudAdminService{
				Name:  service.Name,
				APIID: apiIDFromServiceName(service.Name),
				State: service.State,
				Title: service.Config.Title,
			})
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("list services for %s: %w", projectID, err)
	}

	return out, nil
}

func (a *cloudAdminAdapter) ListServiceAccounts(ctx context.Context, projectID string) ([]CloudAdminServiceAccount, error) {
	var out []CloudAdminServiceAccount

	if err := a.iam.Projects.ServiceAccounts.List("projects/"+projectID).Pages(ctx, func(page *iam.ListServiceAccountsResponse) error {
		for _, account := range page.Accounts {
			out = append(out, CloudAdminServiceAccount{
				Name:        account.Name,
				Email:       account.Email,
				UniqueID:    account.UniqueId,
				DisplayName: account.DisplayName,
				Disabled:    account.Disabled,
			})
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("list service accounts for %s: %w", projectID, err)
	}

	return out, nil
}

func (a *cloudAdminAdapter) ListTransferConfigs(ctx context.Context, projectID string) ([]CloudAdminTransferConfig, error) {
	var out []CloudAdminTransferConfig

	if err := a.dataTransfer.Projects.TransferConfigs.List("projects/"+projectID).Pages(ctx, func(page *bigquerydatatransfer.ListTransferConfigsResponse) error {
		for _, config := range page.TransferConfigs {
			out = append(out, *transferConfigFromAPI(config))
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("list transfer configs for %s: %w", projectID, err)
	}

	return out, nil
}

func (a *cloudAdminAdapter) DisableTransferConfig(ctx context.Context, configName string) (*CloudAdminTransferConfig, error) {
	updated, err := a.dataTransfer.Projects.TransferConfigs.Patch(configName, &bigquerydatatransfer.TransferConfig{Disabled: true}).UpdateMask("disabled").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("disable transfer config %s: %w", configName, err)
	}

	return transferConfigFromAPI(updated), nil
}

func (a *cloudAdminAdapter) DeleteTransferConfig(ctx context.Context, configName string) error {
	if _, err := a.dataTransfer.Projects.TransferConfigs.Delete(configName).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete transfer config %s: %w", configName, err)
	}

	return nil
}

func (a *cloudAdminAdapter) EnableService(ctx context.Context, projectID, apiID string) (*CloudAdminOperation, error) {
	op, err := a.serviceUsage.Services.Enable(serviceName(projectID, apiID), &serviceusage.EnableServiceRequest{}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("enable %s in %s: %w", apiID, projectID, err)
	}

	return &CloudAdminOperation{OperationName: op.Name}, nil
}

func (a *cloudAdminAdapter) DisableService(ctx context.Context, projectID, apiID string) (*CloudAdminOperation, error) {
	op, err := a.serviceUsage.Services.Disable(serviceName(projectID, apiID), &serviceusage.DisableServiceRequest{}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("disable %s in %s: %w", apiID, projectID, err)
	}

	return &CloudAdminOperation{OperationName: op.Name}, nil
}

func transferConfigFromAPI(config *bigquerydatatransfer.TransferConfig) *CloudAdminTransferConfig {
	return &CloudAdminTransferConfig{
		Name:                 config.Name,
		DisplayName:          config.DisplayName,
		DataSourceID:         config.DataSourceId,
		DestinationDatasetID: config.DestinationDatasetId,
		Schedule:             config.Schedule,
		Disabled:             config.Disabled,
	}
}

func serviceName(projectID, apiID string) string {
	return "projects/" + projectID + "/services/" + apiID
}

func projectIDFromResourceName(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return name
}

func apiIDFromServiceName(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return name
}
