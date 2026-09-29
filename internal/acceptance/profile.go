package acceptance

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	profileFileName   = "config.json"
	masterKeyFileName = "master.key"
	secretStoreName   = "secrets.json"
)

type Profile struct {
	Version                 int      `json:"version"`
	UserID                  string   `json:"user_id,omitempty"`
	OrganizationID          string   `json:"organization_id,omitempty"`
	DatabaseURL             string   `json:"database_url"`
	OwnerEmail              string   `json:"owner_email"`
	OrganizationSlug        string   `json:"organization_slug"`
	GoogleClientID          string   `json:"google_client_id"`
	GoogleClientSecretRef   string   `json:"google_client_secret_ref,omitempty"`
	BigQueryProjects        []string `json:"bigquery_projects,omitempty"`
	RequireAdsToken         bool     `json:"require_ads_token,omitempty"`
	GoogleAdsDeveloperToken string   `json:"google_ads_developer_token,omitempty"`
}

type Paths struct {
	Root       string
	Config     string
	MasterKey  string
	Secrets    string
	OutputRoot string
}

func DefaultPaths() (Paths, error) {
	root := os.Getenv("GOG_MARKETING_ACCEPTANCE_HOME")
	if strings.TrimSpace(root) == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve user config dir: %w", wrapAcceptanceError(err))
		}
		root = filepath.Join(configDir, "gog-marketing", "acceptance")
	}

	stateDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve user state dir: %w", wrapAcceptanceError(err))
	}

	output := os.Getenv("GOG_MARKETING_ACCEPTANCE_OUTPUT")
	if strings.TrimSpace(output) == "" {
		output = filepath.Join(stateDir, "gog-marketing", "acceptance-output")
	}

	return Paths{
		Root:       root,
		Config:     filepath.Join(root, profileFileName),
		MasterKey:  filepath.Join(root, masterKeyFileName),
		Secrets:    filepath.Join(root, secretStoreName),
		OutputRoot: output,
	}, nil
}

func EnsureProfile(paths Paths, profile Profile) error {
	if strings.TrimSpace(profile.DatabaseURL) == "" {
		return ErrInvalidProfile
	}

	if strings.TrimSpace(profile.OwnerEmail) == "" {
		return ErrInvalidProfile
	}

	if strings.TrimSpace(profile.OrganizationSlug) == "" {
		profile.OrganizationSlug = "acceptance"
	}

	if profile.Version == 0 {
		profile.Version = 1
	}

	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		return fmt.Errorf("create acceptance profile: %w", wrapAcceptanceError(err))
	}

	if err := os.MkdirAll(paths.OutputRoot, 0o700); err != nil {
		return fmt.Errorf("create acceptance output: %w", wrapAcceptanceError(err))
	}

	if err := SaveProfile(paths, profile); err != nil {
		return fmt.Errorf("write acceptance profile: %w", wrapAcceptanceError(err))
	}

	if _, err := os.Stat(paths.MasterKey); errors.Is(err, os.ErrNotExist) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return wrapAcceptanceError(err)
		}

		if err := os.WriteFile(paths.MasterKey, []byte(hex.EncodeToString(key)), 0o600); err != nil {
			return fmt.Errorf("write acceptance master key: %w", wrapAcceptanceError(err))
		}
	}

	return nil
}

func SaveProfile(paths Paths, profile Profile) error {
	raw, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return wrapAcceptanceError(err)
	}

	if err := os.WriteFile(paths.Config, append(raw, '\n'), 0o600); err != nil {
		return wrapAcceptanceError(err)
	}

	return nil
}

func LoadProfile(paths Paths) (Profile, []byte, error) {
	raw, err := os.ReadFile(paths.Config)
	if err != nil {
		return Profile{}, nil, fmt.Errorf("read acceptance profile: %w", wrapAcceptanceError(err))
	}

	var profile Profile
	if decodeErr := json.Unmarshal(raw, &profile); decodeErr != nil {
		return Profile{}, nil, fmt.Errorf("decode acceptance profile: %w", wrapAcceptanceError(err))
	}

	keyText, err := os.ReadFile(paths.MasterKey)
	if err != nil {
		return Profile{}, nil, fmt.Errorf("read acceptance master key: %w", wrapAcceptanceError(err))
	}

	key, err := hex.DecodeString(strings.TrimSpace(string(keyText)))
	if err != nil || len(key) < 16 {
		return Profile{}, nil, ErrInvalidProfile
	}

	return profile, key, nil
}

func ValidateStablePaths(paths Paths) error {
	for _, path := range []string{paths.Root, paths.Config, paths.MasterKey, paths.Secrets} {
		if strings.Contains(path, os.TempDir()) {
			return fmt.Errorf("%w: %s", ErrTemporaryProfile, path)
		}
	}

	if strings.Contains(paths.Root, "/go-build") || strings.Contains(paths.Root, "/tmp/go-build") {
		return ErrTemporaryProfile
	}

	return nil
}

func SetControlPlaneIdentity(paths Paths, userID, organizationID string) error {
	profile, _, err := LoadProfile(paths)
	if err != nil {
		return wrapAcceptanceError(err)
	}

	profile.UserID = strings.TrimSpace(userID)
	profile.OrganizationID = strings.TrimSpace(organizationID)

	return SaveProfile(paths, profile)
}

func SetGoogleClientSecretRef(paths Paths, reference string) error {
	profile, _, err := LoadProfile(paths)
	if err != nil {
		return wrapAcceptanceError(err)
	}
	profile.GoogleClientSecretRef = reference

	return SaveProfile(paths, profile)
}
