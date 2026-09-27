package config

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveConnection returns a stored connection by name (exact, normalized).
func (s *ConfigStore) ResolveConnection(name string) (Connection, bool, error) {
	normalized, err := NormalizeConnectionName(name)
	if err != nil {
		return Connection{}, false, err
	}

	cfg, err := s.Read()
	if err != nil {
		return Connection{}, false, err
	}

	conn, ok := cfg.Connections[normalized]
	if !ok {
		return Connection{}, false, nil
	}

	return conn, true, nil
}

// ListConnections returns the stored connections sorted by name.
func (s *ConfigStore) ListConnections() ([]Connection, error) {
	cfg, err := s.Read()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(cfg.Connections))
	for name := range cfg.Connections {
		names = append(names, name)
	}

	sort.Strings(names)

	out := make([]Connection, 0, len(names))
	for _, name := range names {
		conn := cfg.Connections[name]
		if conn.Name == "" {
			conn.Name = name
		}
		out = append(out, conn)
	}

	return out, nil
}

// DefaultConnectionName returns the configured default connection, if any.
func (s *ConfigStore) DefaultConnectionName() (string, error) {
	cfg, err := s.Read()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(cfg.DefaultConnection), nil
}

// SetConnection stores a connection. The map key is the normalized name.
func (s *ConfigStore) SetConnection(conn Connection) error {
	normalized, err := NormalizeConnectionName(conn.Name)
	if err != nil {
		return err
	}
	conn.Name = normalized
	conn.Account = strings.TrimSpace(conn.Account)

	if strings.TrimSpace(conn.Account) == "" {
		return fmt.Errorf("%w: connection %q has no account", errMissingConnection, normalized)
	}

	return s.Update(func(cfg *File) error {
		if cfg.Connections == nil {
			cfg.Connections = map[string]Connection{}
		}

		cfg.Connections[normalized] = conn

		return nil
	})
}

// DeleteConnection removes a stored connection and clears the default if it
// pointed at it.
func (s *ConfigStore) DeleteConnection(name string) (bool, error) {
	normalized, err := NormalizeConnectionName(name)
	if err != nil {
		return false, err
	}

	var deleted bool

	err = s.UpdateIfChanged(func(cfg *File) (bool, error) {
		if _, ok := cfg.Connections[normalized]; !ok {
			return false, nil
		}

		delete(cfg.Connections, normalized)
		deleted = true

		if strings.TrimSpace(cfg.DefaultConnection) == normalized {
			cfg.DefaultConnection = ""
		}

		return true, nil
	})
	if err != nil {
		return false, err
	}

	return deleted, nil
}

// SetDefaultConnection marks a stored connection as the default routing choice.
func (s *ConfigStore) SetDefaultConnection(name string) error {
	normalized, err := NormalizeConnectionName(name)
	if err != nil {
		return err
	}

	return s.Update(func(cfg *File) error {
		if _, ok := cfg.Connections[normalized]; !ok {
			return fmt.Errorf("%w: %q", errMissingConnection, normalized)
		}

		cfg.DefaultConnection = normalized

		return nil
	})
}
