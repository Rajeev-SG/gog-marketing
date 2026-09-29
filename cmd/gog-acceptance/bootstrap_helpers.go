package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclaw/gogcli/internal/acceptance"
	"github.com/openclaw/gogcli/internal/controlplane"
)

func newSecretStore(paths acceptance.Paths, key []byte) (controlplane.SecretStore, error) {
	store, err := controlplane.NewFileSecretStore(paths.Secrets, key)
	if err != nil {
		return nil, wrapMainError(err)
	}
	return store, nil
}

func importBootstrapConnections(ctx context.Context, paths acceptance.Paths, profile acceptance.Profile, clientSecret, gmailTokenFile, singulyrTokenFile, exportGog, gmailEmail, singulyrEmail string, timeout time.Duration) error {
	keyRaw, err := os.ReadFile(paths.MasterKey)
	if err != nil {
		return wrapMainError(err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(keyRaw)))
	if err != nil {
		return wrapMainError(err)
	}
	store, err := controlplane.OpenPostgresStore(ctx, profile.DatabaseURL)
	if err != nil {
		return wrapMainError(err)
	}
	defer func() { _ = store.Close() }()
	secrets, err := controlplane.NewFileSecretStore(paths.Secrets, key)
	if err != nil {
		return wrapMainError(err)
	}
	if _, _, err := store.BootstrapOwner(ctx, controlplane.User{Email: profile.OwnerEmail}, controlplane.Organization{Name: "Acceptance", Slug: profile.OrganizationSlug}, "owner"); err != nil {
		return wrapMainError(err)
	}
	// Keep the acceptance runtime identity stable across Postgres and SecretStore.
	// The Postgres organization UUID is metadata; token ownership must use acceptanceOrg.
	actor := controlplane.Actor{UserID: acceptance.AcceptanceOwnerID, OrganizationID: acceptance.AcceptanceOrganizationID, Role: "owner"}
	provider := controlplane.NewGoogleOAuthProvider(profile.GoogleClientID, clientSecret, "http://127.0.0.1/oauth/google/callback")
	service := &controlplane.Service{Store: store, Secrets: secrets, OAuth: provider}
	for _, item := range []struct{ name, file, email string }{{"gmail", gmailTokenFile, gmailEmail}, {"singulyr", singulyrTokenFile, singulyrEmail}} {
		connection, err := ensureBootstrapConnection(ctx, service, actor, item.name)
		if err != nil {
			return wrapMainError(err)
		}
		token, err := readBootstrapToken(item.file)
		if err != nil {
			return wrapMainError(err)
		}
		if installErr := installBootstrapToken(ctx, timeout, store, secrets, service, actor, connection, token); installErr != nil {
			if controlplane.AuthFailureCategoryFor(installErr) != controlplane.AuthFailureInvalidGrant {
				return fmt.Errorf("validate %s refresh token: %w", item.name, installErr)
			}
			if reauthErr := reauthorizeBootstrapToken(ctx, exportGog, item.email, item.file, timeout); reauthErr != nil {
				return reauthErr
			}
			token, err = readBootstrapToken(item.file)
			if err != nil {
				return wrapMainError(err)
			}
			connection, err = ensureBootstrapConnection(ctx, service, actor, item.name)
			if err != nil {
				return wrapMainError(err)
			}
			if reinstallErr := installBootstrapToken(ctx, timeout, store, secrets, service, actor, connection, token); reinstallErr != nil {
				return fmt.Errorf("validate %s refresh token after reauthorization: %w", item.name, reinstallErr)
			}
		}
		resources, err := service.Discover(ctx, actor, connection.ID)
		if err != nil {
			return fmt.Errorf("discover %s resources: %w", item.name, err)
		}
		for _, resource := range resources {
			if resource.Enabled {
				continue
			}
			if _, err := service.SetResourceEnabled(ctx, actor, connection.ID, resource.ResourceID, true); err != nil {
				return wrapMainError(err)
			}
			break
		}
	}

	return nil
}

func installBootstrapToken(parent context.Context, timeout time.Duration, store *controlplane.PostgresStore, secrets controlplane.SecretStore, service *controlplane.Service, actor controlplane.Actor, connection controlplane.Connection, token controlplane.OAuthToken) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	token.Expiry = time.Now().Add(-time.Minute)
	raw, err := controlplane.MarshalOAuthToken(token)
	if err != nil {
		return wrapMainError(err)
	}
	reference, err := secrets.Put(ctx, actor.OrganizationID, raw)
	if err != nil {
		return wrapMainError(err)
	}
	connection.SecretRef = reference
	connection.GoogleEmail = token.Email
	connection.GoogleSubject = token.Subject
	connection.GrantedScopes = token.GrantedScopes
	connection.Status = controlplane.ConnectionNeedsReconnect
	if _, err := store.UpdateConnection(ctx, connection); err != nil {
		return wrapMainError(err)
	}
	if _, refreshErr := service.Refresh(ctx, actor, connection.ID); refreshErr != nil {
		return wrapMainError(refreshErr)
	}

	return nil
}

func ensureBootstrapConnection(ctx context.Context, service *controlplane.Service, actor controlplane.Actor, name string) (controlplane.Connection, error) {
	connections, err := service.ListConnections(ctx, actor)
	if err != nil {
		return controlplane.Connection{}, wrapMainError(err)
	}
	for _, connection := range connections {
		if connection.Name == name {
			return connection, nil
		}
	}
	connection, err := service.CreateConnection(ctx, actor, name, []string{"analytics", "tagmanager", "googleads", "searchconsole", "bigquery"})
	if err != nil {
		return controlplane.Connection{}, wrapMainError(err)
	}
	return connection, nil
}

func redact(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 500 {
		return value[:500]
	}
	return value
}

type bootstrapTokenExport struct {
	Email         string   `json:"email"`
	Subject       string   `json:"subject"`
	RefreshToken  string   `json:"refresh_token"`
	GrantedScopes []string `json:"scopes"`
}

func readBootstrapToken(path string) (controlplane.OAuthToken, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-provided bootstrap token path
	if err != nil {
		return controlplane.OAuthToken{}, wrapMainError(err)
	}
	var exported bootstrapTokenExport
	if err := json.Unmarshal(raw, &exported); err == nil && strings.TrimSpace(exported.RefreshToken) != "" {
		return controlplane.OAuthToken{Email: exported.Email, Subject: exported.Subject, RefreshToken: exported.RefreshToken, GrantedScopes: exported.GrantedScopes}, nil
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return controlplane.OAuthToken{}, errBootstrapTokenFile
	}
	return controlplane.OAuthToken{RefreshToken: token}, nil
}

func exportBootstrapTokens(ctx context.Context, exportGog, gmailEmail, singulyrEmail string) (string, string, error) {
	if strings.TrimSpace(exportGog) == "" {
		return "", "", errBootstrapExportBinary
	}
	dir, err := os.MkdirTemp("", "gog-acceptance-bootstrap-")
	if err != nil {
		return "", "", wrapMainError(err)
	}
	gmailPath := filepath.Join(dir, "gmail.json")
	singulyrPath := filepath.Join(dir, "singulyr.json")
	for _, item := range []struct {
		email, path string
	}{
		{gmailEmail, gmailPath},
		{singulyrEmail, singulyrPath},
	} {
		cmd := exec.CommandContext(ctx, exportGog, "auth", "tokens", "export", item.email, "--client", "personal-owned", "--out", item.path, "--overwrite", "--no-input") //nolint:gosec // bootstrap-only stable signed binary path
		cmd.Env = append(os.Environ(), "GOG_KEYRING_BACKEND=keychain")
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", "", fmt.Errorf("bootstrap token export failed for %s: %w: %s", item.email, wrapMainError(err), redact(string(output)))
		}
	}
	return gmailPath, singulyrPath, nil
}

func reauthorizeBootstrapToken(ctx context.Context, exportGog, email, tokenPath string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exportGog, "auth", "add", email, "--client", "personal-owned", "--services", "analytics,searchconsole,bigquery,ads", "--extra-scopes", "https://www.googleapis.com/auth/tagmanager.readonly", "--force-consent", "--login") //nolint:gosec // bootstrap-only stable signed binary path
	cmd.Env = append(os.Environ(), "GOG_KEYRING_BACKEND=keychain")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("human OAuth bootstrap failed for %s: %w: %s", email, err, redact(string(output)))
	}
	export := exec.CommandContext(ctx, exportGog, "auth", "tokens", "export", email, "--client", "personal-owned", "--out", tokenPath, "--overwrite", "--no-input") //nolint:gosec // bootstrap-only stable signed binary path
	export.Env = append(os.Environ(), "GOG_KEYRING_BACKEND=keychain")
	if output, err := export.CombinedOutput(); err != nil {
		return fmt.Errorf("post-OAuth token export failed for %s: %w: %s", email, err, redact(string(output)))
	}
	return nil
}
