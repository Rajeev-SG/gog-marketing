package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	"google.golang.org/api/tagmanager/v2"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleads"
	"github.com/openclaw/gogcli/internal/googleapi"
)

type Discoverer interface {
	Discover(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error)
}

type DiscoveryReport struct {
	Resources []ResourceGrant
	Statuses  map[string]DiscoveryServiceStatus
}

type ReportingDiscoverer interface {
	DiscoverReport(ctx context.Context, connection Connection, token OAuthToken) (DiscoveryReport, error)
}

type EngineDiscoverer struct {
	GoogleAdsDeveloperToken string
	GoogleAdsLoginCustomer  string
	BigQueryProjects        []string
	GoogleAdsDiscover       func(context.Context, Connection, OAuthToken) ([]ResourceGrant, error)
}

func (d EngineDiscoverer) Discover(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	report, err := d.DiscoverReport(ctx, connection, token)
	if err != nil {
		return nil, err
	}

	for _, rawService := range connection.Services {
		serviceName := strings.ToLower(strings.TrimSpace(rawService))

		status, ok := report.Statuses[serviceName]
		if ok && status.State == DiscoveryServiceError {
			return nil, wrapControlPlaneError(status.err)
		}
	}

	return report.Resources, nil
}

func (d EngineDiscoverer) DiscoverReport(ctx context.Context, connection Connection, token OAuthToken) (DiscoveryReport, error) {
	ctx = authclient.WithAccessToken(ctx, token.AccessToken)
	report := DiscoveryReport{Resources: make([]ResourceGrant, 0), Statuses: make(map[string]DiscoveryServiceStatus)}
	seen := make(map[string]bool)

	for _, rawService := range connection.Services {
		serviceName := strings.ToLower(strings.TrimSpace(rawService))
		if serviceName == "" || seen[serviceName] {
			continue
		}
		seen[serviceName] = true

		if serviceName == "googleads" && strings.TrimSpace(d.GoogleAdsDeveloperToken) == "" {
			report.Statuses[serviceName] = DiscoveryServiceStatus{State: DiscoveryServiceUnavailable, Detail: "google_ads_unconfigured", CheckedAt: time.Now().UTC()}
			continue
		}

		items, err := d.discoverService(ctx, serviceName, connection, token)
		if errors.Is(err, ErrUnsupportedDiscoveryService) {
			// Existing connections may contain retired or mistyped service names.
			// Preserve valid-service discovery while making the stale entry explicit.
			report.Statuses[serviceName] = DiscoveryServiceStatus{State: DiscoveryServiceUnsupported, Detail: "unsupported_service", CheckedAt: time.Now().UTC()}
			continue
		}

		if err != nil {
			report.Statuses[serviceName] = DiscoveryServiceStatus{State: DiscoveryServiceError, Detail: string(AuthFailureCategoryFor(err)), CheckedAt: time.Now().UTC(), err: err}
			continue
		}
		report.Statuses[serviceName] = DiscoveryServiceStatus{State: DiscoveryServiceOK, ResourceCount: len(items), CheckedAt: time.Now().UTC()}
		report.Resources = append(report.Resources, items...)
	}

	return report, nil
}

func (d EngineDiscoverer) discoverService(ctx context.Context, serviceName string, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	switch serviceName {
	case "analytics":
		return d.analytics(ctx, connection, token)
	case "tagmanager":
		return d.tagManager(ctx, connection, token)
	case "googleads":
		return d.discoverGoogleAds(ctx, connection, token)
	case "searchconsole":
		return d.searchConsole(ctx, connection, token)
	case "bigquery":
		return d.bigQuery(ctx, connection, token)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedDiscoveryService, serviceName)
	}
}

func (d EngineDiscoverer) analytics(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	service, err := googleapi.NewAnalyticsAdmin(ctx, connection.GoogleEmail)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}

	return discoverAnalyticsPages(ctx, service, connection, token)
}

func discoverAnalyticsPages(ctx context.Context, service *analyticsadmin.Service, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	out := make([]ResourceGrant, 0)

	err := service.AccountSummaries.List().PageSize(200).Pages(ctx, func(response *analyticsadmin.GoogleAnalyticsAdminV1betaListAccountSummariesResponse) error {
		for _, summary := range response.AccountSummaries {
			if summary == nil {
				continue
			}

			out = append(out, grant(connection, "analytics", "account", summary.Account, summary.DisplayName, "", token))
			for _, property := range summary.PropertySummaries {
				if property == nil {
					continue
				}

				parent := strings.TrimSpace(property.Parent)
				if parent == "" {
					parent = summary.Account
				}
				out = append(out, grant(connection, "analytics", "property", property.Property, property.DisplayName, parent, token))
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover GA4 resources: %w", err)
	}

	return uniqueDiscoveryResources(out), nil
}

func (d EngineDiscoverer) tagManager(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	service, err := googleapi.NewTagManager(ctx, connection.GoogleEmail)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}

	return discoverTagManagerPages(ctx, service, connection, token)
}

func discoverTagManagerPages(ctx context.Context, service *tagmanager.Service, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	out := make([]ResourceGrant, 0)
	seenAccounts := make(map[string]bool)

	err := service.Accounts.List().Pages(ctx, func(response *tagmanager.ListAccountsResponse) error {
		for _, account := range response.Account {
			if account == nil || seenAccounts[account.Path] {
				continue
			}
			seenAccounts[account.Path] = true
			out = append(out, grant(connection, "tagmanager", "account", account.Path, account.Name, "", token))

			containersErr := service.Accounts.Containers.List(account.Path).Pages(ctx, func(containers *tagmanager.ListContainersResponse) error {
				for _, container := range containers.Container {
					if container == nil {
						continue
					}
					out = append(out, grant(connection, "tagmanager", "container", container.Path, container.Name, account.Path, token))
				}

				return nil
			})
			if containersErr != nil {
				return fmt.Errorf("discover GTM containers: %w", containersErr)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover GTM resources: %w", err)
	}

	return uniqueDiscoveryResources(out), nil
}

func uniqueDiscoveryResources(resources []ResourceGrant) []ResourceGrant {
	out := make([]ResourceGrant, 0, len(resources))

	seen := make(map[string]bool, len(resources))
	for _, resource := range resources {
		key := resource.Service + "\x00" + resource.ResourceID
		if seen[key] {
			continue
		}
		seen[key] = true

		out = append(out, resource)
	}

	return out
}

func (d EngineDiscoverer) discoverGoogleAds(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	if d.GoogleAdsDiscover != nil {
		return d.GoogleAdsDiscover(ctx, connection, token)
	}

	return d.googleAds(ctx, connection, token)
}

func (d EngineDiscoverer) googleAds(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	if strings.TrimSpace(d.GoogleAdsDeveloperToken) == "" {
		return nil, nil
	}

	httpClient, err := googleapi.NewGoogleAdsHTTPClient(ctx, connection.GoogleEmail)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}
	client := googleads.Client{
		HTTP: httpClient, Version: googleads.DefaultVersion,
		DeveloperToken: d.GoogleAdsDeveloperToken, LoginCustomerID: d.GoogleAdsLoginCustomer,
	}

	names, err := client.ListAccessibleCustomers(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover Google Ads customers: %w", err)
	}

	out := make([]ResourceGrant, 0, len(names))
	for _, name := range names {
		id := strings.TrimPrefix(name, "customers/")
		out = append(out, grant(connection, "googleads", "customer", name, id, "", token))
	}

	return out, nil
}

func (d EngineDiscoverer) searchConsole(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	service, err := googleapi.NewSearchConsole(ctx, connection.GoogleEmail)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}

	response, err := service.Sites.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("discover Search Console sites: %w", err)
	}
	out := make([]ResourceGrant, 0)

	for _, site := range response.SiteEntry {
		if site == nil {
			continue
		}
		out = append(out, grant(connection, "searchconsole", "site", site.SiteUrl, site.SiteUrl, "", token))
	}

	return out, nil
}

func (d EngineDiscoverer) bigQuery(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	out := make([]ResourceGrant, 0)

	for _, project := range d.BigQueryProjects {
		project = strings.TrimSpace(project)
		if project == "" {
			continue
		}
		out = append(out, grant(connection, "bigquery", "project", project, project, "", token))

		client, err := googleapi.NewBigQuery(ctx, connection.GoogleEmail, project)
		if err != nil {
			return nil, wrapControlPlaneError(err)
		}
		datasets, err := client.ListDatasets(ctx)
		_ = client.Close()

		if err != nil {
			return nil, fmt.Errorf("discover BigQuery datasets: %w", err)
		}

		for _, dataset := range datasets {
			name := dataset.Name
			if name == "" {
				name = dataset.DatasetID
			}
			out = append(out, grant(connection, "bigquery", "dataset", dataset.FullID, name, project, token))
		}
	}

	return out, nil
}

func grant(connection Connection, service, resourceType, resourceID, displayName, parent string, _ OAuthToken) ResourceGrant {
	return ResourceGrant{
		ConnectionID:   connection.ID,
		OrganizationID: connection.OrganizationID,
		Service:        service,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		DisplayName:    strings.TrimSpace(displayName),
		Parent:         strings.TrimSpace(parent),
		Enabled:        false,
	}
}
