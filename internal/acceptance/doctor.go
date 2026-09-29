package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/openclaw/gogcli/internal/controlplane"
)

type DoctorReport struct {
	Schema      string        `json:"schema"`
	Status      string        `json:"status"`
	Checks      []CheckResult `json:"checks"`
	Remediation string        `json:"remediation,omitempty"`
}

func RunDoctor(ctx context.Context, paths Paths) (DoctorReport, error) {
	report := DoctorReport{Schema: "gog-marketing-acceptance-doctor/v1", Status: "PASS"}

	add := func(name, result, category, detail string) {
		report.Checks = append(report.Checks, CheckResult{Name: name, Result: result, Category: category, Detail: redact(detail)})
		if result == "FAIL" && report.Status == "PASS" {
			report.Status = "FAIL"
		}
	}
	if err := ValidateStablePaths(paths); err != nil {
		add("profile", "FAIL", "profile_invalid", err.Error())
		report.Remediation = bootstrapAction

		return report, wrapAcceptanceError(err)
	}

	profile, key, err := LoadProfile(paths)
	if err != nil {
		add("profile", "FAIL", "profile_invalid", err.Error())
		report.Remediation = bootstrapAction

		return report, wrapAcceptanceError(err)
	}

	add("profile", "PASS", "", "stable profile loaded")

	secrets, secretErr := controlplane.NewFileSecretStore(paths.Secrets, key)
	if secretErr != nil {
		add("secret_store", "FAIL", "secret_store", secretErr.Error())
		report.Remediation = bootstrapAction

		return report, wrapAcceptanceError(secretErr)
	}

	add("secret_store", "PASS", "", "encrypted secret store loaded")

	store, err := controlplane.OpenPostgresStore(ctx, profile.DatabaseURL)
	if err != nil {
		add("database", "FAIL", "database_unavailable", err.Error())
		report.Remediation = "run make acceptance-local or make acceptance-bootstrap"

		return report, wrapAcceptanceError(err)
	}

	defer func() { _ = store.Close() }()

	versions, versionErr := store.MigrationVersions(ctx)
	requiredVersions := controlplane.RequiredMigrationVersions()

	if versionErr != nil {
		add("migrations", "FAIL", "database_unavailable", "migration versions cannot be read")
		report.Remediation = bootstrapAction

		return report, wrapAcceptanceError(versionErr)
	}

	if missing := missingVersions(versions, requiredVersions...); len(missing) > 0 {
		add("migrations", "FAIL", "database_unavailable", "missing "+strings.Join(missing, ","))
		report.Remediation = bootstrapAction

		return report, fmt.Errorf("%w: missing migrations %s", ErrDoctorFailed, strings.Join(missing, ","))
	}

	add("database", "PASS", "", "Postgres reachable")
	add("migrations", "PASS", "", strings.Join(requiredVersions, ",")+" current")

	for _, name := range []string{"gmail", "singulyr"} {
		connection, err := (&Runtime{Profile: profile, Store: store}).ConnectionByName(ctx, name)
		if err != nil {
			add(name+".connection", "FAIL", "missing_connection", err.Error())
			report.Remediation = bootstrapAction

			continue
		}

		if connection.Status == controlplane.ConnectionNeedsReconnect || connection.Status == controlplane.ConnectionNeedsConnect {
			add(name+".connection", "FAIL", "needs_reconnect", bootstrapAction)
			report.Remediation = bootstrapAction

			continue
		}

		add(name+".connection", "PASS", "", string(connection.Status))

		if connection.SecretRef == "" {
			add(name+".refresh_token", "FAIL", "google_invalid_grant", "missing secret reference")
			report.Remediation = bootstrapAction
		} else {
			add(name+".refresh_token", "PASS", "", "stored")
		}

		if len(connection.GrantedScopes) == 0 {
			add(name+".scopes", "FAIL", "google_scope_mismatch", "no granted scopes")
		} else {
			add(name+".scopes", "PASS", "", fmt.Sprintf("%d scopes", len(connection.GrantedScopes)))
		}

		if _, err := (&Runtime{Profile: profile, Store: store}).EnabledGrant(ctx, connection.ID); err != nil {
			add(name+".resource", "FAIL", "resource_grant_unavailable", "enable one resource")
		} else {
			add(name+".resource", "PASS", "", "enabled grant present")
		}
	}

	oauthCheck := doctorOAuthClientCheck(ctx, profile, secrets)
	add(oauthCheck.Name, oauthCheck.Result, oauthCheck.Category, oauthCheck.Detail)

	if oauthCheck.Result == "FAIL" {
		report.Remediation = bootstrapAction
	}

	if strings.TrimSpace(profile.GoogleAdsDeveloperToken) == "" {
		add("google_ads", "INFO", "", "skipped (no developer token)")
	} else {
		add("google_ads", "PASS", "", "configured")
	}

	add("oauth_publishing_status", "INFO", "", "operator check required in Google Cloud Console; no stable public API or CLI exposes Testing/In production")

	return report, finalizeDoctorReport(report)
}

func finalizeDoctorReport(report DoctorReport) error {
	if report.Remediation == "" && report.Status == "FAIL" {
		report.Remediation = bootstrapAction
	}

	if report.Status == "FAIL" {
		return fmt.Errorf("%w: one or more checks failed", ErrDoctorFailed)
	}

	return nil
}

func (r DoctorReport) JSON() string {
	raw, _ := json.MarshalIndent(r, "", "  ")
	return string(raw) + "\n"
}

func (r DoctorReport) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "acceptance doctor: %s\n", r.Status)

	for _, check := range r.Checks {
		fmt.Fprintf(&b, "%-24s %-4s %s\n", check.Name, check.Result, check.Detail)
	}

	if r.Remediation != "" {
		fmt.Fprintf(&b, "remediation: %s\n", r.Remediation)
	}

	return b.String()
}

func missingVersions(actual []string, required ...string) []string {
	present := make(map[string]bool, len(actual))
	for _, version := range actual {
		present[version] = true
	}
	missing := make([]string, 0)

	for _, version := range required {
		if !present[version] {
			missing = append(missing, version)
		}
	}

	sort.Strings(missing)

	return missing
}

func doctorOAuthClientCheck(ctx context.Context, profile Profile, secrets controlplane.SecretStore) CheckResult {
	if strings.TrimSpace(profile.GoogleClientID) == "" || strings.TrimSpace(profile.GoogleClientSecretRef) == "" {
		return CheckResult{Name: "oauth_client", Result: "FAIL", Category: "oauth_client_unavailable", Detail: "client ID or secret reference missing"}
	}

	if _, err := secrets.Get(ctx, AcceptanceOrganizationID, profile.GoogleClientSecretRef); err != nil {
		return CheckResult{Name: "oauth_client", Result: "FAIL", Category: "oauth_client_unavailable", Detail: "secret reference cannot be read"}
	}

	return CheckResult{Name: "oauth_client", Result: "PASS", Detail: "client ID and secret reference configured"}
}
