package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (l Layout) ConfigPath() string {
	return filepath.Join(l.ConfigDir, "config.json")
}

func (l Layout) BatchDir() string {
	return filepath.Join(l.StateDir, "batches")
}

func (l Layout) PrimaryKeyringDir() string {
	return filepath.Join(l.DataDir, "keyring")
}

func (l Layout) LegacyKeyringDir() string {
	return filepath.Join(l.ConfigDir, "keyring")
}

func (l Layout) KeyringDir() string {
	primary := l.PrimaryKeyringDir()
	if l.ExplicitData {
		return primary
	}

	legacy := l.LegacyKeyringDir()
	if st, err := os.Stat(legacy); err == nil && st.IsDir() {
		return legacy
	}

	return primary
}

func (l Layout) EnsureKeyringDir() (string, error) {
	dir := l.KeyringDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure keyring dir: %w", err)
	}

	return dir, nil
}

// KeyringPasswordPath returns the 0600 machine-local passphrase file used by
// the file keyring backend. It lives next to config.json so secrets stay in
// user-owned dotfiles instead of the macOS Keychain.
//
// Reduced threat model (accepted trade-off): the key sits next to the
// ciphertext. Anyone or anything that can read the user's ConfigDir —
// including backups and file-sync services that copy that directory — gets
// both the encrypted keyring and the key that opens it. High-value
// deployments should instead supply GOG_KEYRING_PASSWORD (or
// GOG_KEYRING_PASSWORD_FILE) from a source outside ConfigDir, and should
// exclude keyring-password from any backup/sync of ConfigDir.
func (l Layout) KeyringPasswordPath() string {
	return filepath.Join(l.ConfigDir, "keyring-password")
}

func (l Layout) ClientCredentialsPathFor(client string) (string, error) {
	return clientCredentialsPathInDir(l.DataDir, client)
}

func (l Layout) LegacyClientCredentialsPathFor(client string) (string, error) {
	return clientCredentialsPathInDir(l.ConfigDir, client)
}

func (l Layout) DriveDownloadsDir() string {
	return filepath.Join(l.ConfigDir, "drive-downloads")
}

func (l Layout) GmailAttachmentsDir() string {
	return filepath.Join(l.ConfigDir, "gmail-attachments")
}

func (l Layout) PrimaryGmailWatchDir() string {
	return filepath.Join(l.StateDir, "gmail-watch")
}

func (l Layout) LegacyGmailWatchDir() string {
	return filepath.Join(l.ConfigDir, "state", "gmail-watch")
}

func (l Layout) GmailWatchDir() string {
	primary := l.PrimaryGmailWatchDir()
	if l.ExplicitState {
		return primary
	}

	legacy := l.LegacyGmailWatchDir()
	if !l.UsesXDG && !l.UsesXDGState {
		return legacy
	}

	if _, primaryErr := os.Stat(primary); os.IsNotExist(primaryErr) {
		if st, legacyErr := os.Stat(legacy); legacyErr == nil && st.IsDir() {
			return legacy
		}
	}

	return primary
}

func (l Layout) KeepServiceAccountPath(email string) string {
	return filepath.Join(l.DataDir, fmt.Sprintf("keep-sa-%s.json", safeEmailFilename(email)))
}

func (l Layout) KeepServiceAccountLegacySafePath(email string) string {
	return filepath.Join(l.ConfigDir, fmt.Sprintf("keep-sa-%s.json", safeEmailFilename(email)))
}

func (l Layout) KeepServiceAccountLegacyPath(email string) string {
	return filepath.Join(l.ConfigDir, fmt.Sprintf("keep-sa-%s.json", email))
}

func (l Layout) ServiceAccountPath(email string) string {
	return filepath.Join(l.DataDir, fmt.Sprintf("sa-%s.json", safeEmailFilename(email)))
}

func (l Layout) ServiceAccountLegacyPath(email string) string {
	return filepath.Join(l.ConfigDir, fmt.Sprintf("sa-%s.json", safeEmailFilename(email)))
}

func clientCredentialsPathInDir(dir string, client string) (string, error) {
	normalized, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return "", err
	}

	if normalized == DefaultClientName {
		return filepath.Join(dir, "credentials.json"), nil
	}

	return filepath.Join(dir, fmt.Sprintf("credentials-%s.json", normalized)), nil
}

func safeEmailFilename(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return base64.RawURLEncoding.EncodeToString([]byte(normalized))
}
