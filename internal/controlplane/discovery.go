package controlplane

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleads"
	"github.com/openclaw/gogcli/internal/googleapi"
)

type Discoverer interface {
	Discover(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error)
}

type EngineDiscoverer struct {
	GoogleAdsDeveloperToken string
	GoogleAdsLoginCustomer  string
	BigQueryProjects        []string
	GoogleAdsDiscover       func(context.Context, Connection, OAuthToken) ([]ResourceGrant, error)
}

func (d EngineDiscoverer) Discover(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	ctx = authclient.WithAccessToken(ctx, token.AccessToken)
	out := make([]ResourceGrant, 0)

	for _, service := range connection.Services {
		switch strings.ToLower(strings.TrimSpace(service)) {
		case "analytics":
			items, err := d.analytics(ctx, connection, token)
			if err != nil {
				return nil, wrapControlPlaneError(err)
			}

			out = append(out, items...)
		case "tagmanager":
			items, err := d.tagManager(ctx, connection, token)
			if err != nil {
				return nil, wrapControlPlaneError(err)
			}

			out = append(out, items...)
		case "googleads":
			items, err := d.discoverGoogleAds(ctx, connection, token)
			if err != nil {
				return nil, wrapControlPlaneError(err)
			}

			out = append(out, items...)
		case "searchconsole":
			items, err := d.searchConsole(ctx, connection, token)
			if err != nil {
				return nil, wrapControlPlaneError(err)
			}

			out = append(out, items...)
		case "bigquery":
			items, err := d.bigQuery(ctx, connection, token)
			if err != nil {
				return nil, wrapControlPlaneError(err)
			}

			out = append(out, items...)
		}
	}

	return out, nil
}

func (d EngineDiscoverer) analytics(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	service, err := googleapi.NewAnalyticsAdmin(ctx, connection.GoogleEmail)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}

	response, err := service.AccountSummaries.List().PageSize(200).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("discover GA4 resources: %w", err)
	}
	out := make([]ResourceGrant, 0)

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

	return out, nil
}

func (d EngineDiscoverer) tagManager(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	service, err := googleapi.NewTagManager(ctx, connection.GoogleEmail)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}

	response, err := service.Accounts.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("discover GTM resources: %w", err)
	}
	out := make([]ResourceGrant, 0)

	for _, account := range response.Account {
		if account == nil {
			continue
		}
		out = append(out, grant(connection, "tagmanager", "account", account.Path, account.Name, "", token))

		containers, err := service.Accounts.Containers.List(account.Path).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("discover GTM containers: %w", err)
		}

		for _, container := range containers.Container {
			if container == nil {
				continue
			}
			out = append(out, grant(connection, "tagmanager", "container", container.Path, container.Name, account.Path, token))
		}
	}

	return out, nil
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
