package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/99designs/keyring"

	"github.com/openclaw/gogcli/internal/config"
)

const (
	keyringPasswordEnv     = "GOG_KEYRING_PASSWORD"      //nolint:gosec // environment variable name, not a credential
	keyringPasswordFileEnv = "GOG_KEYRING_PASSWORD_FILE" //nolint:gosec // environment variable name, not a credential
	keyringBackendEnv      = "GOG_KEYRING_BACKEND"
	keyringServiceNameEnv  = "GOG_KEYRING_SERVICE_NAME"
	keyringOpenTimeoutEnv  = "GOG_KEYRING_OPEN_TIMEOUT"
)

var (
	errKeyringPasswordFile         = errors.New("manage keyring password file")
	errKeyringPasswordFileInFlight = errors.New("keyring password file is in flight")
	errInvalidKeyringBackend       = errors.New("invalid keyring backend")
	errKeyringTimeout              = errors.New("keyring connection timed out")
	errNilConfigStore              = errors.New("config store is nil")
)

// legacyKeychainNotice emits a one-time actionable notice when the legacy
// "keychain" backend silently maps to the encrypted file keyring. The
// function is injectable so tests can capture the notice.
var (
	legacyKeychainNoticeOnce sync.Once
	legacyKeychainNoticeFn   = emitLegacyKeychainNotice
)

func emitLegacyKeychainNotice() {
	fmt.Fprintln(os.Stderr, "gog: legacy keyring_backend \"keychain\" now uses the encrypted file keyring; the macOS Keychain is never opened. Existing Keychain tokens are stranded and must reconnect once: gog auth add <email> ...")
}

func notifyLegacyKeychainBackend(info KeyringBackendInfo) {
	if info.Legacy {
		legacyKeychainNoticeOnce.Do(legacyKeychainNoticeFn)
	}
}

type KeyringBackendInfo struct {
	Value string
	// Legacy is true when a user-selected "keychain" backend was mapped to
	// the file backend. Existing macOS Keychain items are stranded in that
	// case and accounts must reconnect once.
	Legacy bool
	Source string
}

type OpenOptions struct {
	Layout        config.Layout
	Config        *config.ConfigStore
	Backend       string
	Password      string
	PasswordSet   bool
	ServiceName   string
	GOOS          string
	DBusAddress   string
	IsTTY         bool
	OpenTimeout   time.Duration
	LockTimeout   time.Duration
	openKeyringFn func(keyring.Config) (keyring.Keyring, error)
}

const (
	keyringBackendSourceEnv     = "env"
	keyringBackendSourceConfig  = "config"
	keyringBackendSourceDefault = "default"
	keyringBackendAuto          = "auto"
	keyringBackendKeychain      = "keychain"
)

func OpenOptionsFromLookup(
	layout config.Layout,
	store *config.ConfigStore,
	lookup func(string) (string, bool),
	goos string,
	isTTY bool,
) (OpenOptions, error) {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}

	backend, _ := lookup(keyringBackendEnv)

	password, passwordSet := lookup(keyringPasswordEnv)
	if !passwordSet {
		if passwordFile, fileSet := lookup(keyringPasswordFileEnv); fileSet && strings.TrimSpace(passwordFile) != "" {
			raw, readErr := os.ReadFile(strings.TrimSpace(passwordFile))
			if readErr != nil {
				return OpenOptions{}, fmt.Errorf("read %s: %w", keyringPasswordFileEnv, readErr)
			}
			password = strings.TrimSpace(string(raw))
			passwordSet = true
		}
	}
	serviceName, _ := lookup(keyringServiceNameEnv)
	dbusAddress, _ := lookup("DBUS_SESSION_BUS_ADDRESS")
	openTimeoutRaw, _ := lookup(keyringOpenTimeoutEnv)
	lockTimeoutRaw, _ := lookup(keyringLockTimeoutEnv)

	return OpenOptions{
		Layout:      layout,
		Config:      store,
		Backend:     backend,
		Password:    password,
		PasswordSet: passwordSet,
		ServiceName: strings.TrimSpace(serviceName),
		GOOS:        goos,
		DBusAddress: dbusAddress,
		IsTTY:       isTTY,
		OpenTimeout: parseKeyringOpenTimeout(openTimeoutRaw, goos),
		LockTimeout: parseKeyringLockTimeout(lockTimeoutRaw),
	}, nil
}

func ResolveKeyringBackendInfoWithOptions(options OpenOptions) (KeyringBackendInfo, error) {
	if v, legacy := effectiveKeyringBackend(options.Backend); v != "" {
		return KeyringBackendInfo{Value: v, Legacy: legacy, Source: keyringBackendSourceEnv}, nil
	}

	if options.Config == nil {
		return KeyringBackendInfo{}, errNilConfigStore
	}

	cfg, err := options.Config.Read()
	if err != nil {
		return KeyringBackendInfo{}, fmt.Errorf("resolve keyring backend: %w", err)
	}

	if cfg.KeyringBackend != "" {
		if v, legacy := effectiveKeyringBackend(cfg.KeyringBackend); v != "" {
			return KeyringBackendInfo{Value: v, Legacy: legacy, Source: keyringBackendSourceConfig}, nil
		}
	}

	return KeyringBackendInfo{Value: keyringBackendAuto, Source: keyringBackendSourceDefault}, nil
}

func allowedBackends(info KeyringBackendInfo) ([]keyring.BackendType, error) {
	switch info.Value {
	case "", keyringBackendAuto, keyringBackendKeychain, "file":
		// gog-marketing never uses the macOS Keychain. "auto", a legacy
		// "keychain" setting, and "file" all resolve to the encrypted file
		// backend so secrets stay in a 0600 dotfile (or GOG_KEYRING_PASSWORD).
		return []keyring.BackendType{keyring.FileBackend}, nil
	default:
		return nil, fmt.Errorf("%w: %q (expected %s, keychain, or file)", errInvalidKeyringBackend, info.Value, keyringBackendAuto)
	}
}

func wrapKeychainError(err error) error {
	return err
}

// effectiveKeyringBackend maps legacy "keychain" selections to the file
// backend. gog-marketing never touches the macOS Keychain. The second return
// value reports the legacy mapping so callers can warn the user that existing
// Keychain tokens are stranded.
func effectiveKeyringBackend(value string) (string, bool) {
	v := normalizeKeyringBackend(value)
	if v == keyringBackendKeychain {
		return "file", true
	}

	return v, false
}

func normalizeKeyringBackend(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func serviceNameFor(options OpenOptions) string {
	if serviceName := strings.TrimSpace(options.ServiceName); serviceName != "" {
		return serviceName
	}

	return config.AppName
}

// Keyring timeouts guard against unresponsive backends. macOS gets longer for
// interactive operations; other platforms retain the existing limit.
const (
	keyringOpenTimeout       = 10 * time.Second
	darwinKeyringOpenTimeout = 30 * time.Second
	goosDarwin               = "darwin"
	goosLinux                = "linux"
)

func defaultKeyringOpenTimeout(goos string) time.Duration {
	if goos == goosDarwin {
		return darwinKeyringOpenTimeout
	}

	return keyringOpenTimeout
}

// parseKeyringOpenTimeout resolves GOG_KEYRING_OPEN_TIMEOUT, falling back to
// the platform default when unset, unparseable, or non-positive.
func parseKeyringOpenTimeout(raw, goos string) time.Duration {
	fallback := defaultKeyringOpenTimeout(goos)
	if raw == "" {
		return fallback
	}

	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return fallback
	}

	return timeout
}

func shouldForceFileBackend(goos string, backendInfo KeyringBackendInfo, dbusAddr string) bool {
	return goos == goosLinux && backendInfo.Value == keyringBackendAuto && dbusAddr == ""
}

func shouldUseKeyringTimeout(goos string, backendInfo KeyringBackendInfo, dbusAddr string) bool {
	return goos == goosLinux && backendInfo.Value == keyringBackendAuto && dbusAddr != ""
}

func shouldUseKeyringOperationTimeout(goos string, backendInfo KeyringBackendInfo, dbusAddr string) bool {
	return goos == goosLinux && backendInfo.Value == keyringBackendAuto && dbusAddr != ""
}

func keyringTimeoutHint(goos string) string {
	switch goos {
	case goosDarwin:
		return "keyring backend may be unresponsive"
	case goosLinux:
		return "D-Bus SecretService may be unresponsive"
	default:
		return "keyring backend may be unresponsive"
	}
}

func isFileKeyring(ring keyring.Keyring) bool {
	if ring == nil {
		return false
	}

	return reflect.TypeOf(ring).String() == "*keyring.fileKeyring"
}

func openKeyringWithOptions(options OpenOptions) (keyring.Keyring, error) {
	// On Linux/WSL/containers, OS keychains (secret-service/kwallet) may be unavailable.
	// In that case github.com/99designs/keyring falls back to the "file" backend,
	// which *requires* both a directory and a password prompt function.
	keyringDir, err := options.Layout.EnsureKeyringDir()
	if err != nil {
		return nil, fmt.Errorf("ensure keyring dir: %w", err)
	}

	backendInfo, err := ResolveKeyringBackendInfoWithOptions(options)
	if err != nil {
		return nil, err
	}

	notifyLegacyKeychainBackend(backendInfo)

	backends, err := allowedBackends(backendInfo)
	if err != nil {
		return nil, err
	}
	wrapFileKeys := fileKeyringBackendOnly(backends)

	// On Linux with "auto" backend and no D-Bus session, force file backend.
	// Without DBUS_SESSION_BUS_ADDRESS, SecretService will hang indefinitely
	// trying to connect (common on headless systems like Raspberry Pi).
	if shouldForceFileBackend(options.GOOS, backendInfo, options.DBusAddress) {
		backends = []keyring.BackendType{keyring.FileBackend}
		wrapFileKeys = true
	}

	cfg := keyring.Config{
		ServiceName:      serviceNameFor(options),
		AllowedBackends:  backends,
		FileDir:          keyringDir,
		FilePasswordFunc: fileKeyringPasswordFuncFrom(options),
	}

	openTimeout := options.OpenTimeout
	if openTimeout <= 0 {
		openTimeout = defaultKeyringOpenTimeout(options.GOOS)
	}

	open := options.openKeyringFn
	if open == nil {
		open = keyring.Open
	}

	// On Linux with D-Bus present, keyring.Open() can still hang if SecretService
	// is unresponsive (e.g., gnome-keyring installed but not running).
	// Use a timeout as a safety net.
	if shouldUseKeyringTimeout(options.GOOS, backendInfo, options.DBusAddress) {
		timeoutRing, timeoutErr := openKeyringWithTimeoutFunc(
			cfg,
			openTimeout,
			keyringTimeoutHint(options.GOOS),
			open,
		)
		if timeoutErr != nil {
			return nil, timeoutErr
		}

		return prepareKeyring(timeoutRing, backendInfo, wrapFileKeys, options), nil
	}

	ring, err := open(cfg)
	if err != nil {
		return nil, fmt.Errorf("open keyring: %w", err)
	}

	return prepareKeyring(ring, backendInfo, wrapFileKeys, options), nil
}

func prepareKeyring(
	ring keyring.Keyring,
	backendInfo KeyringBackendInfo,
	wrapFileKeys bool,
	options OpenOptions,
) keyring.Keyring {
	if wrapFileKeys || isFileKeyring(ring) {
		ring = newFileSafeKeyring(ring)
	}

	if shouldUseKeyringOperationTimeout(options.GOOS, backendInfo, options.DBusAddress) {
		timeout := options.OpenTimeout
		if timeout <= 0 {
			timeout = defaultKeyringOpenTimeout(options.GOOS)
		}
		ring = newTimeoutKeyring(ring, timeout, keyringTimeoutHint(options.GOOS))
	}

	return ring
}

// File keyring password provisioning is atomic: the password file is created
// with O_CREAT|O_EXCL so concurrent readers never generate two passwords, and
// only a genuinely missing file is provisioned. Permission and IO errors fail
// fast instead of silently regenerating (which would strand existing
// ciphertext).

var (
	readKeyringPasswordFileFn = os.ReadFile
	createKeyringPasswordFn   = func(path string) (*os.File, error) {
		return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // Operator-configured local keyring path; exclusive creation with private permissions.
	}
	keyringPasswordRetryDelay = 20 * time.Millisecond
	// Winners create the file first and write/fync/fsync after, so a losing
	// process can observe a created-but-unwritten (empty) file. That state is
	// transient: readers retry with backoff until the write lands or the
	// provision deadline passes (crash orphan), then fail with an actionable
	// error instead of silently regenerating.
	keyringPasswordRetryDeadline = 5 * time.Second
)

// fileKeyringPasswordFuncFrom returns a deterministic prompt function for the
// file keyring. Passwords come from GOG_KEYRING_PASSWORD (or
// GOG_KEYRING_PASSWORD_FILE via the environment lookup). When neither is set,
// gog provisions a machine-local 0600 password file so the file backend never
// prompts on a terminal.
func fileKeyringPasswordFuncFrom(options OpenOptions) keyring.PromptFunc {
	password, passwordSet, err := ensureFileKeyringPassword(options)
	if err != nil {
		return func(_ string) (string, error) {
			return "", fmt.Errorf("%w: %w", errKeyringPasswordFile, err)
		}
	}

	// Treat "set to empty string" as intentional; empty passphrase is valid.
	if !passwordSet {
		password = ""
	}

	return keyring.FixedStringPrompt(password)
}

func ensureFileKeyringPassword(options OpenOptions) (string, bool, error) {
	if options.PasswordSet {
		return options.Password, true, nil
	}

	path := options.Layout.KeyringPasswordPath()
	if path == "" {
		return "", false, fmt.Errorf("%w: password path unavailable", errKeyringPasswordFile)
	}

	password, err := readExistingKeyringPassword(path)
	if err == nil {
		return password, true, nil
	}

	if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, errKeyringPasswordFileInFlight) {
		return "", false, err
	}

	if errors.Is(err, errKeyringPasswordFileInFlight) {
		// The file exists but its winner has not finished writing yet. Wait
		// for the winner instead of creating a competing password.
		pendingPassword, waitErr := waitForKeyringPassword(path)
		if waitErr == nil {
			return pendingPassword, true, nil
		}

		return "", false, waitErr
	}

	raw := make([]byte, 32)
	if _, randErr := rand.Read(raw); randErr != nil {
		return "", false, fmt.Errorf("generate keyring password: %w", randErr)
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		return "", false, fmt.Errorf("create keyring password dir: %w", mkdirErr)
	}

	password = base64.RawURLEncoding.EncodeToString(raw)

	file, createErr := createKeyringPasswordFn(path)
	if createErr != nil && !errors.Is(createErr, fs.ErrExist) {
		return "", false, fmt.Errorf("create keyring password file: %w", createErr)
	}

	if createErr == nil {
		if _, writeErr := file.WriteString(password + "\n"); writeErr != nil {
			_ = file.Close()
			return "", false, fmt.Errorf("write keyring password file: %w", writeErr)
		}

		if syncErr := file.Sync(); syncErr != nil {
			_ = file.Close()
			return "", false, fmt.Errorf("sync keyring password file: %w", syncErr)
		}

		if closeErr := file.Close(); closeErr != nil {
			return "", false, fmt.Errorf("close keyring password file: %w", closeErr)
		}

		return password, true, nil
	}

	// Another process won the O_EXCL race. Read the winner's password instead
	// of generating a second one; the winner may still be mid-write.
	password, waitErr := waitForKeyringPassword(path)
	if waitErr == nil {
		return password, true, nil
	}

	return "", false, waitErr
}

func readExistingKeyringPassword(path string) (string, error) {
	raw, err := readKeyringPasswordFileFn(path)
	if err != nil {
		return "", err
	}

	password := strings.TrimSpace(string(raw))
	if password == "" {
		return "", fmt.Errorf("%w: %s exists but is empty (winner still writing or crashed mid-write)", errKeyringPasswordFileInFlight, path)
	}

	return password, nil
}

// waitForKeyringPassword retries a transient (missing or in-flight) password
// file with backoff until the winner's write is visible or the provision
// deadline elapses. Real permission/IO errors fail fast.
func waitForKeyringPassword(path string) (string, error) {
	deadline := time.Now().Add(keyringPasswordRetryDeadline)
	var lastErr error

	for {
		password, readErr := readExistingKeyringPassword(path)
		if readErr == nil {
			return password, nil
		}

		if !errors.Is(readErr, fs.ErrNotExist) && !errors.Is(readErr, errKeyringPasswordFileInFlight) {
			return "", readErr
		}
		lastErr = readErr

		if !time.Now().Before(deadline) {
			break
		}

		time.Sleep(keyringPasswordRetryDelay)
	}

	if errors.Is(lastErr, errKeyringPasswordFileInFlight) {
		return "", fmt.Errorf("%w: %s stayed empty; a previous gog run likely crashed mid-write — delete the file and rerun", errKeyringPasswordFile, path)
	}

	return "", fmt.Errorf("keyring password file disappeared during provisioning: %w", lastErr)
}

type keyringResult struct {
	ring keyring.Keyring
	err  error
}

// openKeyringWithTimeoutFunc prevents an unresponsive SecretService open from
// blocking the CLI indefinitely. The worker goroutine may remain blocked until
// process exit after a timeout.
func openKeyringWithTimeoutFunc(
	cfg keyring.Config,
	timeout time.Duration,
	hint string,
	open func(keyring.Config) (keyring.Keyring, error),
) (keyring.Keyring, error) {
	ch := make(chan keyringResult, 1)

	go func() {
		ring, err := open(cfg)
		ch <- keyringResult{ring, err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("open keyring: %w", res.err)
		}

		return res.ring, nil
	case <-time.After(timeout):
		return nil, keyringTimeoutError("opening keyring", timeout, hint)
	}
}

func Open(options OpenOptions) (Repository, error) {
	ring, err := openKeyringWithOptions(options)
	if err != nil {
		return nil, err
	}

	lock, _, err := keyringLockForRingInDir(ring, options.Layout.KeyringDir(), options.LockTimeout)
	if err != nil {
		return nil, err
	}

	return &KeyringStore{ring: ring, lock: lock}, nil
}
