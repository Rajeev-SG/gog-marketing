package controlplane

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid input")
)

type User struct {
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	ExternalSubject string    `json:"external_subject,omitempty"`
	DisplayName     string    `json:"display_name"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Membership struct {
	UserID         string    `json:"user_id"`
	OrganizationID string    `json:"organization_id"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
}

type ConnectionStatus string

const (
	ConnectionNeedsConnect   ConnectionStatus = "needs_connect"
	ConnectionHealthy        ConnectionStatus = "healthy"
	ConnectionExpired        ConnectionStatus = "expired"
	ConnectionNeedsReconnect ConnectionStatus = "needs_reconnect"
	ConnectionDisconnected   ConnectionStatus = "disconnected"
)

type Connection struct {
	ID              string           `json:"id"`
	OrganizationID  string           `json:"organization_id"`
	Name            string           `json:"name"`
	GoogleEmail     string           `json:"google_email,omitempty"`
	GoogleSubject   string           `json:"-"`
	OAuthClientID   string           `json:"oauth_client_id,omitempty"`
	Services        []string         `json:"services"`
	RequestedScopes []string         `json:"requested_scopes"`
	GrantedScopes   []string         `json:"granted_scopes"`
	Status          ConnectionStatus `json:"status"`
	SecretRef       string           `json:"secret_ref,omitempty"`
	LastValidatedAt *time.Time       `json:"last_validated_at,omitempty"`
	LastError       string           `json:"last_error,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type ResourceGrant struct {
	ID             string            `json:"id"`
	ConnectionID   string            `json:"connection_id"`
	OrganizationID string            `json:"organization_id"`
	Service        string            `json:"service"`
	ResourceType   string            `json:"resource_type"`
	ResourceID     string            `json:"resource_id"`
	DisplayName    string            `json:"display_name"`
	Parent         string            `json:"parent,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Enabled        bool              `json:"enabled"`
	DiscoveredAt   time.Time         `json:"discovered_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type AuditEvent struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	ActingUserID   string    `json:"acting_user_id"`
	ConnectionID   string    `json:"connection_id,omitempty"`
	Action         string    `json:"action"`
	Result         string    `json:"result"`
	Detail         string    `json:"detail,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type OAuthState struct {
	State          string    `json:"state"`
	OrganizationID string    `json:"organization_id"`
	ConnectionID   string    `json:"connection_id"`
	CodeVerifier   string    `json:"-"`
	RedirectURI    string    `json:"redirect_uri"`
	Scope          []string  `json:"scope"`
	Nonce          string    `json:"-"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type OAuthToken struct {
	AccessToken   string    `json:"-"`
	RefreshToken  string    `json:"-"`
	TokenType     string    `json:"-"`
	Expiry        time.Time `json:"-"`
	Subject       string    `json:"-"`
	Email         string    `json:"-"`
	GrantedScopes []string  `json:"-"`
}

func normalizeName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return "", ErrInvalid
	}

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}

		return "", ErrInvalid
	}

	return name, nil
}

func normalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func cloneConnection(in Connection) Connection {
	out := in
	out.Services = append([]string(nil), in.Services...)
	out.RequestedScopes = append([]string(nil), in.RequestedScopes...)
	out.GrantedScopes = append([]string(nil), in.GrantedScopes...)

	return out
}

func cloneGrant(in ResourceGrant) ResourceGrant {
	out := in

	out.Metadata = map[string]string{}
	for k, v := range in.Metadata {
		out.Metadata[k] = v
	}

	return out
}
