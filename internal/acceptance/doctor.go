package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
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

	add("database", "PASS", "", "Postgres reachable and migrations current")

	for _, name := range []string{"gmail", "singulyr"} {
		connection, err := (&Runtime{Store: store}).ConnectionByName(ctx, name)
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

		if _, err := (&Runtime{Store: store}).EnabledGrant(ctx, connection.ID); err != nil {
			add(name+".resource", "FAIL", "resource_grant_unavailable", "enable one resource")
		} else {
			add(name+".resource", "PASS", "", "enabled grant present")
		}
	}

	if strings.TrimSpace(profile.GoogleClientID) == "" || strings.TrimSpace(profile.GoogleClientSecretRef) == "" {
		add("oauth_client", "FAIL", "oauth_client_unavailable", "client ID or secret reference missing")
		report.Remediation = bootstrapAction
	} else {
		if _, secretErr := secrets.Get(ctx, acceptanceOrg, profile.GoogleClientSecretRef); secretErr != nil {
			add("oauth_client_secret", "FAIL", "oauth_client_unavailable", "secret reference cannot be read")
			report.Remediation = bootstrapAction
		} else {
			add("oauth_client", "PASS", "", "client ID and secret reference configured")
		}
	}

	if strings.TrimSpace(profile.GoogleAdsDeveloperToken) == "" {
		add("google_ads", "INFO", "", "skipped (no developer token)")
	} else {
		add("google_ads", "PASS", "", "configured")
	}

	add("oauth_publishing_status", "INFO", "", "operator check required in Google Cloud Console; no stable public API or CLI exposes Testing/In production")

	if report.Remediation == "" && report.Status == "FAIL" {
		report.Remediation = bootstrapAction
	}

	return report, nil
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
