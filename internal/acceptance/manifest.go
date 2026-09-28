package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CheckResult struct {
	Name     string `json:"name"`
	Result   string `json:"result"`
	Category string `json:"category,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Retries  int    `json:"retries,omitempty"`
}

type Manifest struct {
	Schema          string         `json:"schema"`
	Mode            string         `json:"mode"`
	Status          string         `json:"status"`
	StartedAt       time.Time      `json:"started_at"`
	FinishedAt      time.Time      `json:"finished_at"`
	Binary          string         `json:"binary"`
	Commit          string         `json:"commit,omitempty"`
	Checks          []CheckResult  `json:"checks"`
	Interaction     map[string]int `json:"interaction"`
	BootstrapAction string         `json:"bootstrap_action,omitempty"`
}

func NewManifest(mode, binary, commit string) Manifest {
	return Manifest{
		Schema: "gog-marketing-acceptance/v1", Mode: mode, Status: "PASS",
		StartedAt: time.Now().UTC(), Binary: binary, Commit: commit,
		Interaction: map[string]int{"browser": 0, "keychain_dialog": 0, "oauth": 0, "stdin": 0, "go_run": 0},
	}
}

func (m *Manifest) Add(name, result, category, detail string, retries int) {
	m.Checks = append(m.Checks, CheckResult{Name: name, Result: result, Category: category, Detail: redact(detail), Retries: retries})
	if result == "FAIL" && m.Status == "PASS" {
		m.Status = "FAIL"
	}
}

func (m *Manifest) Finish() { m.FinishedAt = time.Now().UTC() }

func (m Manifest) Write(outputRoot, runID string) (string, error) {
	if m.FinishedAt.IsZero() {
		m.FinishedAt = time.Now().UTC()
	}

	if strings.TrimSpace(runID) == "" {
		runID = time.Now().UTC().Format("20060102T150405Z")
	}

	dir := filepath.Join(outputRoot, runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", wrapAcceptanceError(err)
	}

	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", wrapAcceptanceError(err)
	}

	path := filepath.Join(dir, "summary.json")
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil { //nolint:gosec // output root is an operator-provided evidence directory
		return "", wrapAcceptanceError(err)
	}

	return path, nil
}

func HashResourceID(value string) string {
	sum := sha256.Sum256([]byte(value))

	return "sha256:" + hex.EncodeToString(sum[:8])
}

func redact(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		value = strings.ReplaceAll(value, home, "~")
	}

	for _, marker := range []string{"ya29.", "GOCSPX-", "1//", "client_secret", "refresh_token", "access_token", "authorization_code"} {
		if index := strings.Index(strings.ToLower(value), strings.ToLower(marker)); index >= 0 {
			value = value[:index] + "[REDACTED]"
			break
		}
	}

	if len(value) > 500 {
		return value[:500]
	}

	return value
}

func FailFast(category, action, detail string) error {
	return fmt.Errorf("%w: %s: %s: %s", ErrMissingOAuthClient, category, action, redact(detail))
}
