package acceptance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/controlplane"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const (
	AcceptanceOwnerID        = "acceptance-owner"
	AcceptanceOrganizationID = "acceptance-org"
)

type ResourceReader interface {
	Read(context.Context, controlplane.Connection, controlplane.OAuthToken, controlplane.ResourceGrant) error
}

type EngineResourceReader struct{}

func (EngineResourceReader) Read(ctx context.Context, connection controlplane.Connection, token controlplane.OAuthToken, grant controlplane.ResourceGrant) error {
	ctx = authclient.WithAccessToken(ctx, token.AccessToken)

	switch grant.Service {
	case "analytics":
		service, err := googleapi.NewAnalyticsAdmin(ctx, connection.GoogleEmail)
		if err != nil {
			return wrapAcceptanceError(err)
		}

		if grant.ResourceType == "property" {
			if _, propertyErr := service.Properties.Get(grant.ResourceID).Context(ctx).Do(); propertyErr != nil {
				return wrapAcceptanceError(propertyErr)
			}

			return nil
		}

		response, err := service.AccountSummaries.List().PageSize(200).Context(ctx).Do()
		if err != nil {
			return wrapAcceptanceError(err)
		}

		for _, summary := range response.AccountSummaries {
			if summary != nil && summary.Account == grant.ResourceID {
				return nil
			}
		}

		return ErrMissingEnabledGrant
	case "tagmanager":
		service, err := googleapi.NewTagManager(ctx, connection.GoogleEmail)
		if err != nil {
			return wrapAcceptanceError(err)
		}

		if grant.ResourceType == "container" {
			if _, err := service.Accounts.Containers.Get(grant.ResourceID).Context(ctx).Do(); err != nil {
				return wrapAcceptanceError(err)
			}

			return nil
		}

		if _, err := service.Accounts.Get(grant.ResourceID).Context(ctx).Do(); err != nil {
			return wrapAcceptanceError(err)
		}

		return nil
	case "searchconsole":
		service, err := googleapi.NewSearchConsole(ctx, connection.GoogleEmail)
		if err != nil {
			return wrapAcceptanceError(err)
		}

		if _, err := service.Sites.Get(grant.ResourceID).Context(ctx).Do(); err != nil {
			return wrapAcceptanceError(err)
		}

		return nil
	case "bigquery":
		parts := strings.SplitN(grant.ResourceID, ":", 2)

		project := strings.TrimSpace(parts[0])
		if project == "" {
			return ErrBigQueryGrant
		}

		client, err := googleapi.NewBigQuery(ctx, connection.GoogleEmail, project)
		if err != nil {
			return wrapAcceptanceError(err)
		}

		defer func() { _ = client.Close() }()

		if grant.ResourceType == "dataset" {
			if len(parts) != 2 {
				return ErrBigQueryGrant
			}

			if _, err := client.GetDataset(ctx, parts[1]); err != nil {
				return wrapAcceptanceError(err)
			}

			return nil
		}

		if _, err := client.ListDatasets(ctx); err != nil {
			return wrapAcceptanceError(err)
		}

		return nil
	default:
		return fmt.Errorf("%w %q", ErrUnsupportedReader, grant.Service)
	}
}

type Runtime struct {
	Paths     Paths
	Profile   Profile
	MasterKey []byte
	Store     controlplane.Store
	Secrets   controlplane.SecretStore
	OAuth     controlplane.OAuthProvider
	Reader    ResourceReader
	Retry     RetryPolicy
	Timeout   time.Duration
}

func OpenRuntime(ctx context.Context, paths Paths, requireOAuthClient bool) (*Runtime, error) {
	if err := ValidateStablePaths(paths); err != nil {
		return nil, wrapAcceptanceError(err)
	}

	profile, key, err := LoadProfile(paths)
	if err != nil {
		return nil, FailFast("profile_invalid", bootstrapAction, wrapAcceptanceError(err).Error())
	}

	if requireOAuthClient {
		if strings.TrimSpace(profile.GoogleClientID) == "" {
			return nil, FailFast("oauth_client_unavailable", bootstrapAction, "Google client ID is missing")
		}

		if strings.TrimSpace(profile.GoogleClientSecretRef) == "" {
			return nil, FailFast("oauth_client_unavailable", bootstrapAction, "Google client secret reference is missing")
		}
	}

	store, err := controlplane.OpenPostgresStore(ctx, profile.DatabaseURL)
	if err != nil {
		return nil, wrapAcceptanceError(err)
	}

	secrets, err := controlplane.NewFileSecretStore(paths.Secrets, key)
	if err != nil {
		_ = store.Close()
		return nil, wrapAcceptanceError(err)
	}
	var oauth controlplane.OAuthProvider

	if requireOAuthClient {
		secretRaw, err := secrets.Get(ctx, AcceptanceOrganizationID, profile.GoogleClientSecretRef)
		if err != nil {
			_ = store.Close()
			return nil, FailFast("oauth_client_unavailable", bootstrapAction, "Google client secret is missing from the acceptance SecretStore")
		}
		oauth = controlplane.NewGoogleOAuthProvider(profile.GoogleClientID, string(secretRaw), "http://127.0.0.1/oauth/google/callback")
	}

	return &Runtime{
		Paths: paths, Profile: profile, MasterKey: key, Store: store, Secrets: secrets,
		OAuth: oauth, Reader: EngineResourceReader{}, Retry: DefaultRetryPolicy(), Timeout: 30 * time.Second,
	}, nil
}

func (r *Runtime) Close() error {
	if err := r.Store.Close(); err != nil {
		return wrapAcceptanceError(err)
	}

	return nil
}

func (r *Runtime) Actor() controlplane.Actor {
	userID := strings.TrimSpace(r.Profile.UserID)
	if userID == "" {
		userID = AcceptanceOwnerID
	}

	organizationID := strings.TrimSpace(r.Profile.OrganizationID)
	if organizationID == "" {
		organizationID = AcceptanceOrganizationID
	}

	return controlplane.Actor{UserID: userID, OrganizationID: organizationID, Role: "owner"}
}

func (r *Runtime) Service() *controlplane.Service {
	return &controlplane.Service{Store: r.Store, Secrets: r.Secrets, OAuth: r.OAuth}
}

func (r *Runtime) ConnectionByName(ctx context.Context, name string) (controlplane.Connection, error) {
	connections, err := r.Store.ListConnections(ctx, r.Actor().OrganizationID)
	if err != nil {
		return controlplane.Connection{}, wrapAcceptanceError(err)
	}

	for _, connection := range connections {
		if connection.Name == name {
			return connection, nil
		}
	}

	return controlplane.Connection{}, controlplane.ErrNotFound
}

func (r *Runtime) EnabledGrant(ctx context.Context, connectionID string) (controlplane.ResourceGrant, error) {
	grants, err := r.Store.ListResourceGrants(ctx, r.Actor().OrganizationID, connectionID)
	if err != nil {
		return controlplane.ResourceGrant{}, wrapAcceptanceError(err)
	}

	for _, grant := range grants {
		if grant.Enabled {
			return grant, nil
		}
	}

	return controlplane.ResourceGrant{}, ErrMissingEnabledGrant
}

func (r *Runtime) RunLive(ctx context.Context, runID string) (Manifest, error) {
	manifest := NewManifest("live", executableIdentity(), os.Getenv("GOG_MARKETING_ACCEPTANCE_COMMIT"))
	defer func() {
		manifest.Finish()
		_, _ = manifest.Write(r.Paths.OutputRoot, runID)
	}()

	if r.OAuth == nil {
		manifest.Add("oauth_client", "FAIL", "oauth_client_unavailable", bootstrapAction, 0)
		return manifest, FailFast("oauth_client_unavailable", bootstrapAction, "OAuth provider unavailable")
	}

	for _, name := range []string{"gmail", "singulyr"} {
		if err := r.runConnection(ctx, &manifest, name); err != nil {
			return manifest, wrapAcceptanceError(err)
		}
	}

	return manifest, nil
}

func (r *Runtime) withRetryTimeout(ctx context.Context, fn func(context.Context) (int, error)) (int, error) {
	budget := r.Timeout + (r.Retry.MaxDelay * time.Duration(maxInt(1, r.Retry.MaxAttempts)))

	retryCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	return fn(retryCtx)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func (r *Runtime) withTimeout(ctx context.Context, fn func(context.Context) error) error {
	stepCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	return fn(stepCtx)
}

func (r *Runtime) runConnection(ctx context.Context, manifest *Manifest, name string) error {
	var connection controlplane.Connection

	err := r.withTimeout(ctx, func(stepCtx context.Context) error {
		var findErr error
		connection, findErr = r.ConnectionByName(stepCtx, name)

		return findErr
	})
	if err != nil {
		manifest.Add(name+".connection", "FAIL", "missing_connection", bootstrapAction, 0)
		return fmt.Errorf("%s: %w\nreason: missing_connection\naction: %s", name, ErrNeedsReconnect, bootstrapAction)
	}

	if connection.Status == controlplane.ConnectionNeedsReconnect || connection.Status == controlplane.ConnectionNeedsConnect {
		category := string(connection.LastErrorCategory)
		if category == "" {
			category = "google_invalid_grant"
		}

		manifest.Add(name+".refresh", "FAIL", category, bootstrapAction, 0)

		return FailFast(category, bootstrapAction, name+" needs reconnect")
	}

	service := r.Service()
	var refreshed controlplane.Connection
	var attempts int

	attempts, err = r.withRetryTimeout(ctx, func(stepCtx context.Context) (int, error) {
		return r.Retry.Run(stepCtx, name+".refresh", func(attemptCtx context.Context) error {
			var callErr error

			refreshed, callErr = service.Refresh(attemptCtx, r.Actor(), connection.ID)
			if callErr != nil {
				return wrapAcceptanceError(callErr)
			}

			return nil
		})
	})
	if err != nil {
		category := string(controlplane_auth_category(err))
		if category == "" {
			category = "unknown"
		}

		action := "inspect acceptance-doctor"
		if category == "google_invalid_grant" || category == "google_scope_mismatch" {
			action = bootstrapAction
		}

		manifest.Add(name+".refresh", "FAIL", category, action, attempts)

		return FailFast(category, action, wrapAcceptanceError(err).Error())
	}

	manifest.Add(name+".refresh", "PASS", "", string(refreshed.Status), attempts)

	var grant controlplane.ResourceGrant

	err = r.withTimeout(ctx, func(stepCtx context.Context) error {
		var grantErr error
		grant, grantErr = r.EnabledGrant(stepCtx, connection.ID)

		return grantErr
	})
	if err != nil {
		manifest.Add(name+".read", "FAIL", "resource_grant_unavailable", "enable one resource during bootstrap", 0)
		return FailFast("resource_grant_unavailable", "enable one resource during bootstrap", name+" has no enabled grant")
	}

	attempts, err = r.withRetryTimeout(ctx, func(stepCtx context.Context) (int, error) {
		return r.Retry.Run(stepCtx, name+".read", func(attemptCtx context.Context) error {
			_, token, tokenErr := service.FreshToken(attemptCtx, r.Actor(), connection.ID)
			if tokenErr != nil {
				return wrapAcceptanceError(tokenErr)
			}

			return r.Reader.Read(attemptCtx, connection, token, grant)
		})
	})
	if err != nil {
		category := string(controlplane_auth_category(err))
		if category == "" {
			category = "unknown"
		}

		manifest.Add(name+".read", "FAIL", category, "resource "+HashResourceID(grant.ResourceID), attempts)

		return FailFast(category, "run acceptance-doctor", wrapAcceptanceError(err).Error())
	}

	manifest.Add(name+".read", "PASS", "", "service="+grant.Service+" resource="+HashResourceID(grant.ResourceID), attempts)

	return nil
}

func controlplane_auth_category(err error) controlplane.AuthFailureCategory {
	return controlplane.AuthFailureCategoryFor(err)
}

func executableIdentity() string {
	path, err := os.Executable()
	if err != nil {
		return "unknown"
	}

	return filepath.Base(path)
}
