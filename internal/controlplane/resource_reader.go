package controlplane

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleads"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var (
	ErrResourceReaderUnsupported = errors.New("resource reader does not support service")
	ErrResourceReaderMissing     = errors.New("resource reader could not find enabled resource")
	ErrBigQueryResourceInvalid   = errors.New("BigQuery resource must be project or project:dataset")
)

type ResourceReader interface {
	Read(context.Context, Connection, OAuthToken, ResourceGrant) error
}

type EngineResourceReader struct {
	GoogleAdsDeveloperToken string
	GoogleAdsLoginCustomer  string
}

func (r EngineResourceReader) Read(ctx context.Context, connection Connection, token OAuthToken, grant ResourceGrant) error {
	switch grant.Service {
	case "analytics":
		service, err := googleapi.NewAnalyticsAdmin(ctx, connection.GoogleEmail)
		if err != nil {
			return fmt.Errorf("create analytics reader: %w", err)
		}

		if grant.ResourceType == "property" {
			if _, propertyErr := service.Properties.Get(grant.ResourceID).Context(ctx).Do(); propertyErr != nil {
				return fmt.Errorf("read analytics property: %w", propertyErr)
			}

			return nil
		}

		response, err := service.AccountSummaries.List().PageSize(200).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("read analytics account: %w", err)
		}

		for _, summary := range response.AccountSummaries {
			if summary != nil && summary.Account == grant.ResourceID {
				return nil
			}
		}

		return ErrResourceReaderMissing
	case "tagmanager":
		service, err := googleapi.NewTagManager(ctx, connection.GoogleEmail)
		if err != nil {
			return fmt.Errorf("create tag manager reader: %w", err)
		}

		if grant.ResourceType == "container" {
			if _, err := service.Accounts.Containers.Get(grant.ResourceID).Context(ctx).Do(); err != nil {
				return fmt.Errorf("read tag manager container: %w", err)
			}

			return nil
		}

		if _, err := service.Accounts.Get(grant.ResourceID).Context(ctx).Do(); err != nil {
			return fmt.Errorf("read tag manager account: %w", err)
		}

		return nil
	case "searchconsole":
		service, err := googleapi.NewSearchConsole(ctx, connection.GoogleEmail)
		if err != nil {
			return fmt.Errorf("create search console reader: %w", err)
		}

		if _, err := service.Sites.Get(grant.ResourceID).Context(ctx).Do(); err != nil {
			return fmt.Errorf("read search console site: %w", err)
		}

		return nil
	case "bigquery":
		parts := strings.SplitN(grant.ResourceID, ":", 2)

		project := strings.TrimSpace(parts[0])
		if project == "" {
			return ErrBigQueryResourceInvalid
		}

		client, err := googleapi.NewBigQuery(ctx, connection.GoogleEmail, project)
		if err != nil {
			return fmt.Errorf("create BigQuery reader: %w", err)
		}

		defer func() { _ = client.Close() }()

		if grant.ResourceType == "dataset" {
			if len(parts) != 2 {
				return ErrBigQueryResourceInvalid
			}

			if _, err := client.GetDataset(ctx, parts[1]); err != nil {
				return fmt.Errorf("read BigQuery dataset: %w", err)
			}

			return nil
		}

		if _, err := client.ListDatasets(ctx); err != nil {
			return fmt.Errorf("read BigQuery project: %w", err)
		}

		return nil
	case "googleads":
		httpClient, err := googleapi.NewGoogleAdsHTTPClient(ctx, connection.GoogleEmail)
		if err != nil {
			return fmt.Errorf("create Google Ads reader: %w", err)
		}
		client := googleads.Client{
			HTTP: httpClient, Version: googleads.DefaultVersion,
			DeveloperToken: r.GoogleAdsDeveloperToken, LoginCustomerID: r.GoogleAdsLoginCustomer,
		}

		names, err := client.ListAccessibleCustomers(ctx)
		if err != nil {
			return fmt.Errorf("read Google Ads customer: %w", err)
		}

		for _, name := range names {
			if name == grant.ResourceID {
				return nil
			}
		}

		return ErrResourceReaderMissing
	default:
		return fmt.Errorf("%w: %s", ErrResourceReaderUnsupported, grant.Service)
	}
}

func validResourceRead(service, resourceType string) bool {
	switch service {
	case "analytics":
		return resourceType == "property" || resourceType == "account"
	case "tagmanager":
		return resourceType == "account" || resourceType == "container"
	case "searchconsole":
		return resourceType == "site"
	case "googleads":
		return resourceType == "customer"
	case "bigquery":
		return resourceType == "project" || resourceType == "dataset"
	default:
		return false
	}
}

func (s *Service) ReadResource(ctx context.Context, actor Actor, connectionID, resourceID string) (ResourceGrant, error) {
	const action = "agent.resource.read"

	deny := func(err error) (ResourceGrant, error) {
		s.audit(ctx, actor, connectionID, action, "deny", "resource access denied")
		return ResourceGrant{}, err
	}
	if actor.UserID == "" || actor.OrganizationID == "" || s.Reader == nil {
		return deny(ErrForbidden)
	}

	grant, err := s.Store.GetResourceGrant(ctx, actor.OrganizationID, connectionID, resourceID)
	if err != nil || !validResourceRead(grant.Service, grant.ResourceType) {
		return deny(ErrForbidden)
	}

	connection, err := s.Store.GetConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return deny(ErrForbidden)
	}

	if (connection.Status != ConnectionHealthy && connection.Status != ConnectionExpired) || !slices.Contains(connection.Services, grant.Service) {
		s.audit(ctx, actor, connectionID, action, "unavailable", "connection_not_ready")
		return ResourceGrant{}, ErrForbidden
	}

	if policyErr := (Policy{Store: s.Store}).Allow(ctx, actor, connectionID, grant.Service, resourceID); policyErr != nil {
		return deny(ErrForbidden)
	}

	connection, token, err := s.FreshToken(ctx, actor, connectionID)
	if err != nil {
		s.audit(ctx, actor, connectionID, action, "error", string(AuthFailureCategoryFor(err)))
		return ResourceGrant{}, err
	}

	if token.AccessToken == "" {
		return deny(ErrForbidden)
	}

	// Inject the stored token into context for the guarded Google API clients.
	ctx = googleapi.WithAuthDependencies(ctx, googleapi.AuthDependencies{Mode: googleapi.AuthModeStored})
	ctx = authclient.WithAccessToken(ctx, token.AccessToken)
	ctx = googleapi.WithReadOnly(googleapi.WithNoInput(ctx), true)

	if err := s.Reader.Read(ctx, connection, token, grant); err != nil {
		s.audit(ctx, actor, connectionID, action, "error", string(AuthFailureCategoryFor(err)))
		return ResourceGrant{}, fmt.Errorf("read resource: %w", err)
	}

	s.audit(ctx, actor, connectionID, action, "ok", grant.Service+";"+grant.ResourceID)

	return grant, nil
}
