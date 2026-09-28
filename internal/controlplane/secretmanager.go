package controlplane

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/secretmanager/v1"
)

// GoogleSecretManagerStore is the production SecretStore backend. Secret
// Manager supplies managed encryption at rest; the database stores only the
// returned version reference.
type GoogleSecretManagerStore struct {
	service *secretmanager.Service
	project string
}

func NewGoogleSecretManagerStore(ctx context.Context, project string) (*GoogleSecretManagerStore, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, ErrSecretManagerProject
	}

	service, err := secretmanager.NewService(ctx, option.WithScopes(secretmanager.CloudPlatformScope))
	if err != nil {
		return nil, fmt.Errorf("create Secret Manager service: %w", err)
	}

	return &GoogleSecretManagerStore{service: service, project: project}, nil
}

func (s *GoogleSecretManagerStore) Put(ctx context.Context, organizationID string, value []byte) (string, error) {
	organizationID = stringsTrim(organizationID)
	if organizationID == "" || len(value) == 0 {
		return "", ErrInvalid
	}
	secretID := secretIDForOrganization(organizationID)
	parent := fmt.Sprintf("projects/%s", s.project)
	secretName := fmt.Sprintf("%s/secrets/%s", parent, secretID)

	_, err := s.service.Projects.Secrets.Get(secretName).Context(ctx).Do()
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return "", fmt.Errorf("get Secret Manager secret: %w", err)
		}

		if _, createErr := s.service.Projects.Secrets.Create(parent, &secretmanager.Secret{
			Labels: map[string]string{"gog_control_plane": "true"},
			Replication: &secretmanager.Replication{
				Automatic: &secretmanager.Automatic{},
			},
		}).SecretId(secretID).Context(ctx).Do(); createErr != nil {
			return "", fmt.Errorf("create Secret Manager secret: %w", createErr)
		}
	}

	version, err := s.service.Projects.Secrets.AddVersion(secretName, &secretmanager.AddSecretVersionRequest{
		Payload: &secretmanager.SecretPayload{Data: base64.StdEncoding.EncodeToString(value)},
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("add Secret Manager version: %w", err)
	}

	return version.Name, nil
}

func (s *GoogleSecretManagerStore) Get(ctx context.Context, organizationID, reference string) ([]byte, error) {
	organizationID = stringsTrim(organizationID)
	if err := s.validateReference(organizationID, reference); err != nil {
		return nil, wrapControlPlaneError(err)
	}

	version, err := s.service.Projects.Secrets.Versions.Access(reference).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("access Secret Manager version: %w", err)
	}

	if version == nil || version.Payload == nil {
		return nil, ErrSecretNotFound
	}

	decoded, decodeErr := base64.StdEncoding.DecodeString(version.Payload.Data)
	if decodeErr != nil {
		return nil, fmt.Errorf("decode Secret Manager payload: %w", decodeErr)
	}

	return decoded, nil
}

func (s *GoogleSecretManagerStore) Delete(ctx context.Context, organizationID, reference string) error {
	if err := s.validateReference(organizationID, reference); err != nil {
		return wrapControlPlaneError(err)
	}

	_, err := s.service.Projects.Secrets.Versions.Destroy(reference, &secretmanager.DestroySecretVersionRequest{}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("destroy Secret Manager version: %w", err)
	}

	return nil
}

func (s *GoogleSecretManagerStore) validateReference(organizationID, reference string) error {
	organizationID = stringsTrim(organizationID)

	expected := fmt.Sprintf("projects/%s/secrets/%s/versions/", s.project, secretIDForOrganization(organizationID))
	if organizationID == "" || !strings.HasPrefix(reference, expected) {
		return ErrSecretNotFound
	}

	return nil
}

func secretIDForOrganization(organizationID string) string {
	clean := strings.ToLower(strings.TrimSpace(organizationID))
	var b strings.Builder
	b.WriteString("gog-control-plane-")

	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}

	return b.String()
}
