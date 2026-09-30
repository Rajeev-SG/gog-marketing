package cmd

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/controlplane"
)

type ControlPlaneCmd struct {
	Listen                  string `name:"listen" help:"Control-plane listen address" default:"127.0.0.1:8080" env:"GOG_CONTROL_PLANE_LISTEN"`
	DatabaseURL             string `name:"database-url" help:"Postgres URL; use memory:// only for a disposable local run" env:"GOG_CONTROL_PLANE_DATABASE_URL"`
	ExternalBaseURL         string `name:"external-base-url" help:"Externally reachable base URL used for OAuth callbacks" env:"GOG_CONTROL_PLANE_EXTERNAL_BASE_URL"`
	OwnerEmail              string `name:"owner-email" help:"Initial owner/admin email allowed to sign in" env:"GOG_CONTROL_PLANE_OWNER_EMAIL"`
	OwnerName               string `name:"owner-name" help:"Initial owner display name" env:"GOG_CONTROL_PLANE_OWNER_NAME" default:"Control-plane owner"`
	OrganizationName        string `name:"organization-name" help:"Initial organisation/workspace name" default:"Default"`
	OrganizationSlug        string `name:"organization-slug" help:"Initial organisation/workspace slug" default:"default"`
	SecretStorePath         string `name:"secret-store-path" type:"path" help:"Encrypted local secret-store path for file backend" env:"GOG_CONTROL_PLANE_SECRET_STORE_PATH"`
	SecretBackend           string `name:"secret-backend" help:"Required secret backend: file or secret-manager" env:"GOG_CONTROL_PLANE_SECRET_BACKEND"`
	SecretManagerProject    string `name:"secret-manager-project" help:"GCP project for Google Secret Manager" env:"GOG_CONTROL_PLANE_SECRET_MANAGER_PROJECT"`
	MasterKey               string `name:"master-key" help:"Encryption master key for local secret storage" env:"GOG_CONTROL_PLANE_MASTER_KEY"`
	SessionKey              string `name:"session-key" help:"Stable signing key for web sessions" env:"GOG_CONTROL_PLANE_SESSION_KEY"`
	AdminToken              string `name:"admin-token" help:"Required bootstrap admin token used to sign in" env:"GOG_CONTROL_PLANE_ADMIN_TOKEN"`
	GoogleClientName        string `name:"google-client-name" help:"Named stored OAuth client supplying the central client ID and Keychain secret" env:"GOG_CONTROL_PLANE_GOOGLE_CLIENT_NAME"`
	GoogleClientID          string `name:"google-client-id" help:"Explicit central Google OAuth client ID for secret-injected deployments" env:"GOG_CONTROL_PLANE_GOOGLE_CLIENT_ID"`
	GoogleClientSecret      string `name:"google-client-secret" help:"Explicit central Google OAuth client secret for secret-injected deployments" env:"GOG_CONTROL_PLANE_GOOGLE_CLIENT_SECRET"`
	GoogleAdsDeveloperToken string `name:"google-ads-developer-token" help:"Google Ads developer token used for resource discovery" env:"GOG_GOOGLE_ADS_DEVELOPER_TOKEN"`
	GoogleAdsLoginCustomer  string `name:"google-ads-login-customer-id" help:"Optional Google Ads manager customer ID" env:"GOG_GOOGLE_ADS_LOGIN_CUSTOMER_ID"`
	BigQueryProjects        string `name:"bigquery-projects" help:"Comma-separated BigQuery projects to discover" env:"GOG_CONTROL_PLANE_BIGQUERY_PROJECTS"`
	SeedConnections         string `name:"seed-connections" help:"Comma-separated named connections to ensure at startup" default:"gmail,singulyr"`
	Services                string `name:"services" help:"Default marketing services for seeded connections" default:"analytics,tagmanager,googleads,searchconsole,bigquery"`
	SecureCookies           bool   `name:"secure-cookies" help:"Mark session cookies Secure (enable behind HTTPS)" env:"GOG_CONTROL_PLANE_SECURE_COOKIES"`
}

func (c *ControlPlaneCmd) Run(ctx context.Context, _ *RootFlags) error {
	if strings.TrimSpace(c.OwnerEmail) == "" {
		return usage("--owner-email is required")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return usage("--database-url is required (Postgres URL or explicit memory://)")
	}
	if err := validateControlPlaneSecurity(c.Listen, c.SecretBackend, c.SecretManagerProject, c.AdminToken); err != nil {
		return err
	}
	masterKey := []byte(strings.TrimSpace(c.MasterKey))
	if len(masterKey) < 16 {
		return usage("--master-key must be at least 16 bytes")
	}
	sessionKey := []byte(strings.TrimSpace(c.SessionKey))
	if len(sessionKey) == 0 {
		sum := sha256.Sum256(append([]byte("session:"), masterKey...))
		sessionKey = sum[:]
	}
	if len(sessionKey) < 16 {
		return usage("--session-key must be at least 16 bytes")
	}

	googleClient, clientErr := resolveControlPlaneGoogleOAuthClient(ctx, c.GoogleClientName, c.GoogleClientID, c.GoogleClientSecret, authclient.ReadCredentials)
	if clientErr != nil {
		return clientErr
	}

	var store controlplane.Store
	if strings.TrimSpace(c.DatabaseURL) == "memory://" {
		store = controlplane.NewMemoryStore()
	} else {
		postgres, openErr := controlplane.OpenPostgresStore(ctx, c.DatabaseURL)
		if openErr != nil {
			return fmt.Errorf("control plane: %w", openErr)
		}
		store = postgres
	}
	defer func() { _ = store.Close() }()

	secretPath := strings.TrimSpace(c.SecretStorePath)
	if secretPath == "" {
		configDir, configErr := os.UserConfigDir()
		if configErr != nil {
			return fmt.Errorf("control plane: %w", configErr)
		}
		secretPath = filepath.Join(configDir, "gogcli", "control-plane-secrets.json")
	}
	var secretStore controlplane.SecretStore
	var secretErr error
	switch strings.ToLower(strings.TrimSpace(c.SecretBackend)) {
	case "file":
		secretStore, secretErr = controlplane.NewFileSecretStore(secretPath, masterKey)
	case "secret-manager":
		secretStore, secretErr = controlplane.NewGoogleSecretManagerStore(ctx, c.SecretManagerProject)
	default:
		return usage("--secret-backend must be file or secret-manager")
	}
	if secretErr != nil {
		return fmt.Errorf("control plane: %w", secretErr)
	}

	owner := controlplane.User{
		Email: c.OwnerEmail, ExternalSubject: "control-plane-owner:" + strings.ToLower(strings.TrimSpace(c.OwnerEmail)),
		DisplayName: c.OwnerName,
	}
	org := controlplane.Organization{Name: c.OrganizationName, Slug: c.OrganizationSlug}
	owner, org, bootstrapErr := store.BootstrapOwner(ctx, owner, org, "owner")
	if bootstrapErr != nil {
		return fmt.Errorf("bootstrap control-plane owner: %w", bootstrapErr)
	}
	actor := controlplane.Actor{UserID: owner.ID, OrganizationID: org.ID, Role: "owner"}

	baseURL := strings.TrimRight(strings.TrimSpace(c.ExternalBaseURL), "/")
	if baseURL == "" {
		baseURL = "http://" + strings.TrimSpace(c.Listen)
	}
	redirectURI := baseURL + "/oauth/google/callback"
	provider := controlplane.NewGoogleOAuthProvider(googleClient.ClientID, googleClient.ClientSecret, redirectURI)
	service := &controlplane.Service{
		Store: store, Secrets: secretStore, OAuth: provider, RedirectURI: redirectURI,
		Discoverer: controlPlaneDiscoverer(c),
	}
	for _, name := range splitCSV(c.SeedConnections) {
		existing, getErr := service.ListConnections(ctx, actor)
		if getErr != nil {
			return getErr
		}
		found := false
		for _, connection := range existing {
			if connection.Name == strings.ToLower(name) {
				found = true
				break
			}
		}
		if !found {
			if _, createErr := service.CreateConnection(ctx, actor, name, splitCSV(c.Services)); createErr != nil {
				return fmt.Errorf("seed connection %q: %w", name, createErr)
			}
		}
	}

	sessions, err := controlplane.NewSessionManager(sessionKey, 12*time.Hour, c.SecureCookies)
	if err != nil {
		return fmt.Errorf("control plane: %w", err)
	}
	adminHandler, err := controlplane.NewWebHandler(controlplane.WebConfig{
		Service: service, Sessions: sessions,
		Authenticator: controlplane.OwnerAuthenticator{Email: c.OwnerEmail, Token: c.AdminToken, Actor: actor},
		OwnerEmail:    c.OwnerEmail, DisplayName: c.OwnerName, ExternalBaseURL: baseURL, BasePath: "/admin",
	})
	if err != nil {
		return fmt.Errorf("control plane: %w", err)
	}
	productHandler, err := controlplane.NewProductHandler(controlplane.ProductConfig{
		Service: service, Sessions: sessions,
		Auth:        controlplane.NewGoogleProductAuthenticator(googleClient.ClientID, googleClient.ClientSecret, redirectURI, c.OwnerEmail, actor),
		DisplayName: c.OwnerName, DefaultServices: splitCSV(c.Services),
	})
	if err != nil {
		return fmt.Errorf("product shell: %w", err)
	}
	rootMux := http.NewServeMux()
	rootMux.Handle("/admin/", adminHandler)
	rootMux.Handle("/admin", http.RedirectHandler("/admin/", http.StatusPermanentRedirect))
	rootMux.Handle("/", productHandler)

	rootHandler, bridgeErr := controlPlaneLoopbackBridge(rootMux, baseURL, os.Getenv("PORTLESS_URL"))
	if bridgeErr != nil {
		return fmt.Errorf("local OAuth callback bridge: %w", bridgeErr)
	}

	server := &http.Server{
		Addr: strings.TrimSpace(c.Listen), Handler: rootHandler,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second,
	}
	fmt.Fprintf(os.Stderr, "gog-marketing control plane listening on http://%s\n", c.Listen)
	fmt.Fprintf(os.Stderr, "OAuth callback: %s\n", redirectURI)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("control plane: %w", err)
}

type controlPlaneCredentialsReader func(context.Context, string) (config.ClientCredentials, error)

func resolveControlPlaneGoogleOAuthClient(ctx context.Context, clientName, clientID, clientSecret string, read controlPlaneCredentialsReader) (config.ClientCredentials, error) {
	clientName = strings.TrimSpace(clientName)
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientName != "" {
		if clientID != "" || clientSecret != "" {
			return config.ClientCredentials{}, usage("--google-client-name cannot be combined with --google-client-id or --google-client-secret")
		}
		if read == nil {
			return config.ClientCredentials{}, errors.New("stored OAuth credential reader is required")
		}
		credentials, err := read(ctx, clientName)
		if err != nil {
			return config.ClientCredentials{}, fmt.Errorf("read stored Google OAuth client %q: %w", clientName, err)
		}
		if strings.TrimSpace(credentials.ClientID) == "" || strings.TrimSpace(credentials.ClientSecret) == "" {
			return config.ClientCredentials{}, fmt.Errorf("stored Google OAuth client %q is incomplete", clientName)
		}
		return credentials, nil
	}
	if clientID == "" || clientSecret == "" {
		return config.ClientCredentials{}, usage("configure --google-client-name or both --google-client-id and --google-client-secret")
	}
	return config.ClientCredentials{ClientID: clientID, ClientSecret: clientSecret}, nil
}

func controlPlaneDiscoverer(cmd *ControlPlaneCmd) controlplane.EngineDiscoverer {
	return controlplane.EngineDiscoverer{
		GoogleAdsDeveloperToken: cmd.GoogleAdsDeveloperToken,
		GoogleAdsLoginCustomer:  cmd.GoogleAdsLoginCustomer,
		BigQueryProjects:        splitCSV(cmd.BigQueryProjects),
	}
}

func validateControlPlaneSecurity(listen, secretBackend, secretManagerProject, adminToken string) error {
	if len(strings.TrimSpace(adminToken)) < 32 {
		return usage("--admin-token must be at least 32 characters")
	}

	switch strings.ToLower(strings.TrimSpace(secretBackend)) {
	case "file":
		if !isLoopbackListen(listen) {
			return usage("--secret-backend=file is only allowed with a loopback --listen address")
		}
	case "secret-manager":
		if strings.TrimSpace(secretManagerProject) == "" {
			return usage("--secret-manager-project is required with --secret-backend=secret-manager")
		}
	default:
		return usage("--secret-backend must be explicitly set to file or secret-manager")
	}

	return nil
}

func isLoopbackListen(raw string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
