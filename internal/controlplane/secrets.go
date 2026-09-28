package controlplane

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
)

var ErrSecretNotFound = errors.New("secret not found")

type SecretStore interface {
	Put(ctx context.Context, organizationID string, value []byte) (string, error)
	Get(ctx context.Context, organizationID, reference string) ([]byte, error)
	Delete(ctx context.Context, organizationID, reference string) error
}

type secretEnvelope struct {
	OrganizationID string `json:"organization_id"`
	Ciphertext     string `json:"ciphertext"`
}

type FileSecretStore struct {
	mu    sync.RWMutex
	path  string
	aead  cipher.AEAD
	items map[string]secretEnvelope
}

func NewFileSecretStore(path string, masterKey []byte) (*FileSecretStore, error) {
	if len(masterKey) < 16 {
		return nil, ErrSecretMasterKey
	}
	key := sha256.Sum256(masterKey)

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}

	store := &FileSecretStore{path: path, aead: aead, items: map[string]secretEnvelope{}}
	if err := store.load(); err != nil {
		return nil, wrapControlPlaneError(err)
	}

	return store, nil
}

func (s *FileSecretStore) load() error {
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read secret store: %w", err)
	}

	if len(raw) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw, &s.items); err != nil {
		return fmt.Errorf("decode secret store: %w", err)
	}

	return nil
}

func (s *FileSecretStore) persist() error {
	raw, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return wrapControlPlaneError(err)
	}

	//nolint:gosec // The path is an operator-provided secret-store location, not request input.
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return wrapControlPlaneError(err)
	}

	//nolint:gosec // The path is an operator-provided secret-store location, not request input.
	if writeErr := os.WriteFile(s.path, append(raw, '\n'), 0o600); writeErr != nil {
		return fmt.Errorf("write secret store: %w", writeErr)
	}

	return nil
}

func (s *FileSecretStore) Put(_ context.Context, organizationID string, value []byte) (string, error) {
	organizationID = stringsTrim(organizationID)
	if organizationID == "" || len(value) == 0 {
		return "", ErrInvalid
	}

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", wrapControlPlaneError(err)
	}
	ciphertext := s.aead.Seal(nonce, nonce, value, []byte(organizationID))
	reference := "secret_" + uuid.NewString()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[reference] = secretEnvelope{
		OrganizationID: organizationID,
		Ciphertext:     base64.RawStdEncoding.EncodeToString(ciphertext),
	}
	if err := s.persist(); err != nil {
		delete(s.items, reference)
		return "", wrapControlPlaneError(err)
	}

	return reference, nil
}

func (s *FileSecretStore) Get(_ context.Context, organizationID, reference string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	envelope, ok := s.items[reference]
	if !ok || envelope.OrganizationID != stringsTrim(organizationID) {
		return nil, ErrSecretNotFound
	}

	raw, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}

	nonceSize := s.aead.NonceSize()
	if len(raw) < nonceSize {
		return nil, ErrSecretCiphertext
	}

	plaintext, openErr := s.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], []byte(envelope.OrganizationID))
	if openErr != nil {
		return nil, fmt.Errorf("decrypt secret: %w", openErr)
	}

	return plaintext, nil
}

func (s *FileSecretStore) Delete(_ context.Context, organizationID, reference string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	envelope, ok := s.items[reference]
	if !ok || envelope.OrganizationID != stringsTrim(organizationID) {
		return ErrSecretNotFound
	}

	delete(s.items, reference)

	return s.persist()
}

func stringsTrim(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\n' || value[0] == '\t') {
		value = value[1:]
	}

	for len(value) > 0 && (value[len(value)-1] == ' ' || value[len(value)-1] == '\n' || value[len(value)-1] == '\t') {
		value = value[:len(value)-1]
	}

	return value
}
