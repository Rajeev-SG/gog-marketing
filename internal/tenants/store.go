package tenants

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store keeps the tenant registry as one JSON file in the operator's config
// dir. Tenant data itself never lives here — each tenant gets its own gog
// home under <tenants-root>/<name>.
type Store struct {
	path string
}

func NewStore(configDir string) *Store {
	return &Store{path: filepath.Join(configDir, "tenants.json")}
}

func (s *Store) Path() string {
	return s.path
}

// Home returns the isolated gog home for a tenant. The name is validated
// here so the path is safe by construction, not by caller convention.
func (s *Store) Home(name string) (string, error) {
	normalized, err := NormalizeName(name)
	if err != nil {
		return "", err
	}

	return filepath.Join(filepath.Dir(s.path), "tenants", normalized), nil
}

func (s *Store) List() ([]Tenant, error) {
	tenants, err := s.read()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(tenants))
	for name := range tenants {
		names = append(names, name)
	}

	sort.Strings(names)

	out := make([]Tenant, 0, len(names))
	for _, name := range names {
		out = append(out, tenants[name])
	}

	return out, nil
}

func (s *Store) Get(name string) (Tenant, bool, error) {
	normalized, err := NormalizeName(name)
	if err != nil {
		return Tenant{}, false, err
	}

	tenants, err := s.read()
	if err != nil {
		return Tenant{}, false, err
	}
	tenant, ok := tenants[normalized]

	return tenant, ok, nil
}

func (s *Store) Set(tenant Tenant) error {
	normalized, err := NormalizeName(tenant.Name)
	if err != nil {
		return err
	}
	tenant.Name = normalized

	account, err := NormalizeAccount(tenant.Account)
	if err != nil {
		return err
	}
	tenant.Account = account
	tenant.Client = strings.TrimSpace(tenant.Client)
	tenant.Notes = strings.TrimSpace(tenant.Notes)

	tools := make([]string, 0, len(tenant.AllowTools))
	for _, tool := range tenant.AllowTools {
		tool = strings.TrimSpace(tool)
		if tool != "" {
			tools = append(tools, tool)
		}
	}
	tenant.AllowTools = tools

	return s.update(func(existing map[string]Tenant) error {
		existing[normalized] = tenant
		return nil
	})
}

func (s *Store) Delete(name string) (bool, error) {
	normalized, err := NormalizeName(name)
	if err != nil {
		return false, err
	}
	var deleted bool

	err = s.update(func(existing map[string]Tenant) error {
		if _, ok := existing[normalized]; !ok {
			return nil
		}

		delete(existing, normalized)
		deleted = true

		return nil
	})
	if err != nil {
		return false, err
	}

	return deleted, nil
}

func (s *Store) read() (map[string]Tenant, error) {
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string]Tenant{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read tenant registry: %w", err)
	}

	var tenants map[string]Tenant
	if err := json.Unmarshal(raw, &tenants); err != nil {
		return nil, fmt.Errorf("decode tenant registry: %w", err)
	}

	return tenants, nil
}

func (s *Store) update(fn func(map[string]Tenant) error) error {
	tenants, err := s.read()
	if err != nil {
		return err
	}

	if updateErr := fn(tenants); updateErr != nil {
		return updateErr
	}

	encoded, err := json.MarshalIndent(tenants, "", "  ")
	if err != nil {
		return fmt.Errorf("encode tenant registry: %w", err)
	}

	encoded = append(encoded, '\n')

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("ensure tenant registry dir: %w", err)
	}

	if err := os.WriteFile(s.path, encoded, 0o600); err != nil { //nolint:wsl // write must be wrapped with context
		return fmt.Errorf("write tenant registry: %w", err)
	}

	return nil
}
