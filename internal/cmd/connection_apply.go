package cmd

import (
	"fmt"
	"os"
	"strings"
)

// applyConnectionDefaults resolves --connection (or the stored default
// connection) and fills in unset account/client/quota/billing defaults before
// commands run. Explicit flags and GOG_* env vars always win; the connection
// only supplies what the operator left unspecified.
func applyConnectionDefaults(flags *RootFlags) error {
	if flags == nil || flags.configStoreResolver == nil {
		return nil
	}

	name := strings.TrimSpace(flags.Connection)
	if name == "" {
		// Config-independent commands (version, config keys) must not require
		// the config dir to resolve. The default connection is best-effort;
		// commands that need config resolve it themselves and fail loudly.
		store, err := flags.configStoreResolver()
		if err != nil {
			return nil //nolint:nilerr // default connection is best-effort; commands needing config fail loudly later
		}
		name, err = store.DefaultConnectionName()
		if err != nil {
			return nil //nolint:nilerr // default connection is best-effort; commands needing config fail loudly later
		}
		if name == "" {
			return nil
		}
	}

	store, err := flags.configStoreResolver()
	if err != nil {
		return err
	}
	conn, ok, err := store.ResolveConnection(name)
	if err != nil {
		return err
	}
	if !ok {
		return usage(fmt.Sprintf("connection %q not found; add one with: gog connection add %s --account <email>", name, name))
	}

	if conn.Client != "" && strings.TrimSpace(flags.Client) == "" {
		flags.Client = conn.Client
	}
	if conn.Account != "" && strings.TrimSpace(flags.Account) == "" && strings.TrimSpace(os.Getenv("GOG_ACCOUNT")) == "" {
		flags.Account = conn.Account
	}
	if conn.QuotaProject != "" && strings.TrimSpace(flags.QuotaProject) == "" {
		flags.QuotaProject = conn.QuotaProject
	}

	connCopy := conn
	flags.connection = &connCopy
	return nil
}
