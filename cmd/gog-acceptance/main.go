package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/openclaw/gogcli/internal/acceptance"
	"github.com/openclaw/gogcli/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errAcceptanceUsage
	}
	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	var profileRoot, outputRoot, databaseURL, ownerEmail, gmailEmail, singulyrEmail, googleClientID, googleClientSecretFile, gmailRefreshTokenFile, singulyrRefreshTokenFile, exportGog string
	var runs int
	var timeout time.Duration
	fs.StringVar(&profileRoot, "profile-root", "", "stable acceptance profile directory")
	fs.StringVar(&outputRoot, "output-root", "", "acceptance manifest output directory")
	fs.StringVar(&databaseURL, "database-url", "", "Postgres URL")
	fs.StringVar(&ownerEmail, "owner-email", "", "control-plane owner email")
	fs.StringVar(&gmailEmail, "gmail-email", "", "gmail connection Google email")
	fs.StringVar(&singulyrEmail, "singulyr-email", "", "singulyr connection Google email")
	fs.StringVar(&googleClientID, "google-client-id", "", "central OAuth client ID")
	fs.StringVar(&googleClientSecretFile, "google-client-secret-file", "", "file containing the central OAuth client secret")
	fs.StringVar(&gmailRefreshTokenFile, "gmail-refresh-token-file", "", "file containing the gmail refresh token export")
	fs.StringVar(&singulyrRefreshTokenFile, "singulyr-refresh-token-file", "", "file containing the singulyr refresh token export")
	fs.StringVar(&exportGog, "export-gog", "", "stable signed gog binary used only by bootstrap token export")
	fs.IntVar(&runs, "runs", 1, "live acceptance repetitions")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "per-operation timeout")
	if err := fs.Parse(args[1:]); err != nil {
		return wrapMainError(err)
	}

	paths, err := acceptance.DefaultPaths()
	if err != nil {
		return wrapMainError(err)
	}
	if strings.TrimSpace(profileRoot) != "" {
		paths.Root = profileRoot
		paths.Config = filepath.Join(profileRoot, "config.json")
		paths.MasterKey = filepath.Join(profileRoot, "master.key")
		paths.Secrets = filepath.Join(profileRoot, "secrets.json")
	}
	if strings.TrimSpace(outputRoot) != "" {
		paths.OutputRoot = outputRoot
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch command {
	case "doctor":
		report, err := acceptance.RunDoctor(ctx, paths)
		if strings.TrimSpace(os.Getenv("ACCEPTANCE_JSON")) == "1" {
			fmt.Print(report.JSON())
		} else {
			fmt.Print(report.Text())
		}
		return wrapMainError(err)
	case "local":
		manifest, err := acceptance.RunLocal(ctx, paths, databaseURL)
		if err != nil {
			return wrapMainError(err)
		}
		manifest.Finish()
		if strings.TrimSpace(os.Getenv("ACCEPTANCE_JSON")) == "1" {
			raw, _ := json.MarshalIndent(manifest, "", "  ")
			fmt.Println(string(raw))
		}
		return nil
	case "live":
		if runs < 1 {
			return errAcceptanceRuns
		}
		var finalErr error
		for i := 0; i < runs; i++ {
			runID := fmt.Sprintf("live-%s-%02d", time.Now().UTC().Format("20060102T150405Z"), i+1)
			runtime, err := acceptance.OpenRuntime(ctx, paths, true)
			if err != nil {
				manifest := acceptance.NewManifest("live", "gog-acceptance", "")
				manifest.Add("bootstrap", "FAIL", "profile_invalid", err.Error(), 0)
				manifest.Finish()
				_, _ = manifest.Write(paths.OutputRoot, fmt.Sprintf("live-fail-%s", time.Now().UTC().Format("20060102T150405Z")))
				finalErr = err
				break
			}
			_, runErr := runtime.RunLive(ctx, runID)
			closeErr := runtime.Close()
			if runErr != nil {
				finalErr = runErr
				break
			}
			if closeErr != nil {
				finalErr = closeErr
				break
			}
		}
		return finalErr
	case "live-repeat":
		if runs < 1 {
			return errAcceptanceRuns
		}
		var finalErr error
		for i := 0; i < runs; i++ {
			runtime, err := acceptance.OpenRuntime(ctx, paths, true)
			if err != nil {
				manifest := acceptance.NewManifest("live", "gog-acceptance", "")
				manifest.Add("bootstrap", "FAIL", "profile_invalid", err.Error(), 0)
				manifest.Finish()
				_, _ = manifest.Write(paths.OutputRoot, fmt.Sprintf("live-fail-%s", time.Now().UTC().Format("20060102T150405Z")))
				finalErr = err
				break
			}
			_, runErr := runtime.RunLive(ctx, fmt.Sprintf("live-repeat-%02d", i+1))
			closeErr := runtime.Close()
			if runErr != nil {
				finalErr = runErr
				break
			}
			if closeErr != nil {
				finalErr = closeErr
				break
			}
		}
		return finalErr
	case "bootstrap":
		return runBootstrap(ctx, paths, databaseURL, ownerEmail, gmailEmail, singulyrEmail, googleClientID, googleClientSecretFile, gmailRefreshTokenFile, singulyrRefreshTokenFile, exportGog, timeout)
	default:
		return fmt.Errorf("%w: %q", errAcceptanceUnknown, command)
	}
}

func runBootstrap(ctx context.Context, paths acceptance.Paths, databaseURL, ownerEmail, gmailEmail, singulyrEmail, googleClientID, secretFile, gmailTokenFile, singulyrTokenFile, exportGog string, timeout time.Duration) error {
	if err := validateBootstrapInputs(databaseURL, ownerEmail, gmailEmail, singulyrEmail, secretFile, exportGog); err != nil {
		return err
	}
	if strings.TrimSpace(gmailTokenFile) == "" || strings.TrimSpace(singulyrTokenFile) == "" {
		var err error
		gmailTokenFile, singulyrTokenFile, err = exportBootstrapTokens(ctx, exportGog, gmailEmail, singulyrEmail)
		if err != nil {
			return wrapMainError(err)
		}
		defer func() {
			_ = os.Remove(gmailTokenFile)
			_ = os.Remove(singulyrTokenFile)
		}()
	}
	secret, err := os.ReadFile(secretFile) //nolint:gosec // operator-provided central client JSON path
	if err != nil {
		return wrapMainError(err)
	}
	credentials, err := resolveBootstrapClient(googleClientID, secret)
	if err != nil {
		return err
	}
	googleClientID = credentials.ClientID
	clientSecret := credentials.ClientSecret
	profile := acceptance.Profile{
		Version: 1, DatabaseURL: databaseURL, OwnerEmail: ownerEmail,
		OrganizationSlug: "acceptance", GoogleClientID: googleClientID,
	}
	if profileErr := acceptance.EnsureProfile(paths, profile); profileErr != nil {
		return wrapMainError(profileErr)
	}
	_, key, err := acceptance.LoadProfile(paths)
	if err != nil {
		return wrapMainError(err)
	}
	secrets, err := newSecretStore(paths, key)
	if err != nil {
		return wrapMainError(err)
	}
	reference, err := secrets.Put(ctx, "acceptance-org", []byte(clientSecret))
	if err != nil {
		return wrapMainError(err)
	}
	if err := acceptance.SetGoogleClientSecretRef(paths, reference); err != nil {
		return wrapMainError(err)
	}
	if err := importBootstrapConnections(ctx, paths, profile, clientSecret, gmailTokenFile, singulyrTokenFile, exportGog, gmailEmail, singulyrEmail, timeout); err != nil {
		return wrapMainError(err)
	}
	fmt.Println("acceptance bootstrap complete; run make acceptance-live")
	return nil
}

func validateBootstrapInputs(databaseURL, ownerEmail, gmailEmail, singulyrEmail, secretFile, exportGog string) error {
	if strings.TrimSpace(databaseURL) == "" || strings.TrimSpace(secretFile) == "" {
		return errBootstrapArguments
	}
	if strings.TrimSpace(ownerEmail) == "" || strings.TrimSpace(gmailEmail) == "" || strings.TrimSpace(singulyrEmail) == "" || strings.TrimSpace(exportGog) == "" {
		return errBootstrapArguments
	}

	return nil
}

func resolveBootstrapClient(explicitClientID string, raw []byte) (config.ClientCredentials, error) {
	clientID := strings.TrimSpace(explicitClientID)
	secretMaterial := strings.TrimSpace(string(raw))
	if credentials, err := config.ParseGoogleOAuthClientJSON(raw); err == nil {
		if clientID == "" {
			clientID = strings.TrimSpace(credentials.ClientID)
		}
		secretMaterial = strings.TrimSpace(credentials.ClientSecret)
	}
	if clientID == "" || secretMaterial == "" {
		return config.ClientCredentials{}, errBootstrapClient
	}

	resolved := config.ClientCredentials{ClientID: clientID}
	resolved.ClientSecret = secretMaterial

	return resolved, nil
}
