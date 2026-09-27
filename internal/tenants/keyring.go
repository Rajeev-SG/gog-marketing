package tenants

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
)

// DeriveKeyringPassword turns one operator-held master secret into a
// per-tenant keyring password. Each tenant then encrypts its OAuth tokens at
// rest with its own password, so a leaked tenant password does not unlock the
// other tenants' tokens.
func DeriveKeyringPassword(masterSecret []byte, tenantName string) ([]byte, error) {
	if len(masterSecret) == 0 {
		return nil, fmt.Errorf("%w: empty master secret", ErrMissingMasterSecret)
	}

	normalized, err := NormalizeName(tenantName)
	if err != nil {
		return nil, err
	}

	key, err := hkdf.Key(sha256.New, masterSecret, []byte("gog-tenant-keyring-v1"), normalized, 32)
	if err != nil {
		return nil, fmt.Errorf("derive tenant keyring password: %w", err)
	}

	return key, nil
}
