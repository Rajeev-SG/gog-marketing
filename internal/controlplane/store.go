package controlplane

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Store interface {
	BootstrapOwner(ctx context.Context, user User, organization Organization, role string) (User, Organization, error)
	CreateConnection(ctx context.Context, connection Connection) (Connection, error)
	GetConnection(ctx context.Context, organizationID, id string) (Connection, error)
	ListConnections(ctx context.Context, organizationID string) ([]Connection, error)
	UpdateConnection(ctx context.Context, connection Connection) (Connection, error)
	DeleteConnection(ctx context.Context, organizationID, id string) error
	UpsertResourceGrant(ctx context.Context, grant ResourceGrant) (ResourceGrant, error)
	ListResourceGrants(ctx context.Context, organizationID, connectionID string) ([]ResourceGrant, error)
	GetResourceGrant(ctx context.Context, organizationID, connectionID, resourceID string) (ResourceGrant, error)
	SetResourceEnabled(ctx context.Context, organizationID, connectionID, resourceID string, enabled bool) (ResourceGrant, error)
	AppendAudit(ctx context.Context, event AuditEvent) error
	ListAudit(ctx context.Context, organizationID string, limit int) ([]AuditEvent, error)
	PutOAuthState(ctx context.Context, state OAuthState) error
	TakeOAuthState(ctx context.Context, state string) (OAuthState, error)
	Close() error
}

type MemoryStore struct {
	mu          sync.RWMutex
	users       map[string]User
	orgs        map[string]Organization
	memberships map[string]Membership
	connections map[string]Connection
	grants      map[string]ResourceGrant
	audit       []AuditEvent
	oauthStates map[string]OAuthState
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:       map[string]User{},
		orgs:        map[string]Organization{},
		memberships: map[string]Membership{},
		connections: map[string]Connection{},
		grants:      map[string]ResourceGrant{},
		oauthStates: map[string]OAuthState{},
	}
}

func (s *MemoryStore) BootstrapOwner(_ context.Context, user User, organization Organization, role string) (User, Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.users {
		if normalizeEmail(existing.Email) == normalizeEmail(user.Email) {
			user.ID = existing.ID
			user.CreatedAt = existing.CreatedAt

			break
		}
	}

	if user.ID == "" {
		user.ID = uuid.NewString()
	}

	for _, existing := range s.orgs {
		if existing.Slug == organization.Slug {
			organization.ID = existing.ID
			organization.CreatedAt = existing.CreatedAt

			break
		}
	}

	if organization.ID == "" {
		organization.ID = uuid.NewString()
	}

	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}

	user.UpdatedAt = now
	if organization.CreatedAt.IsZero() {
		organization.CreatedAt = now
	}
	organization.UpdatedAt = now
	s.users[user.ID] = user
	s.orgs[organization.ID] = organization
	s.memberships[user.ID+":"+organization.ID] = Membership{
		UserID: user.ID, OrganizationID: organization.ID, Role: role, CreatedAt: now,
	}

	return user, organization, nil
}

func (s *MemoryStore) CreateConnection(_ context.Context, connection Connection) (Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name, err := normalizeName(connection.Name)
	if err != nil {
		return Connection{}, wrapControlPlaneError(err)
	}

	connection.Name = name
	for _, existing := range s.connections {
		if existing.OrganizationID == connection.OrganizationID && existing.Name == name {
			return Connection{}, ErrConflict
		}
	}

	if connection.ID == "" {
		connection.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	connection.CreatedAt = now

	connection.UpdatedAt = now
	if connection.Status == "" {
		connection.Status = ConnectionNeedsConnect
	}
	s.connections[connection.ID] = cloneConnection(connection)

	return cloneConnection(connection), nil
}

func (s *MemoryStore) GetConnection(_ context.Context, organizationID, id string) (Connection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connection, ok := s.connections[id]
	if !ok || connection.OrganizationID != organizationID {
		return Connection{}, ErrNotFound
	}

	return cloneConnection(connection), nil
}

func (s *MemoryStore) ListConnections(_ context.Context, organizationID string) ([]Connection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Connection, 0)

	for _, connection := range s.connections {
		if connection.OrganizationID == organizationID {
			out = append(out, cloneConnection(connection))
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

func (s *MemoryStore) UpdateConnection(_ context.Context, connection Connection) (Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.connections[connection.ID]
	if !ok || existing.OrganizationID != connection.OrganizationID {
		return Connection{}, ErrNotFound
	}

	name, err := normalizeName(connection.Name)
	if err != nil {
		return Connection{}, wrapControlPlaneError(err)
	}

	for id, candidate := range s.connections {
		if id != connection.ID && candidate.OrganizationID == connection.OrganizationID && candidate.Name == name {
			return Connection{}, ErrConflict
		}
	}
	connection.Name = name
	connection.CreatedAt = existing.CreatedAt
	connection.UpdatedAt = time.Now().UTC()
	s.connections[connection.ID] = cloneConnection(connection)

	return cloneConnection(connection), nil
}

func (s *MemoryStore) DeleteConnection(_ context.Context, organizationID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	connection, ok := s.connections[id]
	if !ok || connection.OrganizationID != organizationID {
		return ErrNotFound
	}

	delete(s.connections, id)

	for grantID, grant := range s.grants {
		if grant.ConnectionID == id {
			delete(s.grants, grantID)
		}
	}

	return nil
}

func (s *MemoryStore) UpsertResourceGrant(_ context.Context, grant ResourceGrant) (ResourceGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	connection, ok := s.connections[grant.ConnectionID]
	if !ok || connection.OrganizationID != grant.OrganizationID {
		return ResourceGrant{}, ErrForbidden
	}
	now := time.Now().UTC()

	for id, existing := range s.grants {
		if existing.ConnectionID == grant.ConnectionID && existing.ResourceID == grant.ResourceID {
			grant.ID = id
			grant.Enabled = existing.Enabled
			grant.DiscoveredAt = existing.DiscoveredAt
			grant.UpdatedAt = now
			s.grants[id] = cloneGrant(grant)

			return cloneGrant(grant), nil
		}
	}

	if grant.ID == "" {
		grant.ID = uuid.NewString()
	}

	if grant.DiscoveredAt.IsZero() {
		grant.DiscoveredAt = now
	}
	grant.UpdatedAt = now
	s.grants[grant.ID] = cloneGrant(grant)

	return cloneGrant(grant), nil
}

func (s *MemoryStore) ListResourceGrants(_ context.Context, organizationID, connectionID string) ([]ResourceGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connection, ok := s.connections[connectionID]
	if !ok || connection.OrganizationID != organizationID {
		return nil, ErrForbidden
	}
	out := make([]ResourceGrant, 0)

	for _, grant := range s.grants {
		if grant.ConnectionID == connectionID {
			out = append(out, cloneGrant(grant))
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Service == out[j].Service {
			return out[i].ResourceID < out[j].ResourceID
		}

		return out[i].Service < out[j].Service
	})

	return out, nil
}

func (s *MemoryStore) GetResourceGrant(ctx context.Context, organizationID, connectionID, resourceID string) (ResourceGrant, error) {
	grants, err := s.ListResourceGrants(ctx, organizationID, connectionID)
	if err != nil {
		return ResourceGrant{}, wrapControlPlaneError(err)
	}

	for _, grant := range grants {
		if grant.ResourceID == resourceID {
			return grant, nil
		}
	}

	return ResourceGrant{}, ErrNotFound
}

func (s *MemoryStore) SetResourceEnabled(ctx context.Context, organizationID, connectionID, resourceID string, enabled bool) (ResourceGrant, error) {
	grant, err := s.GetResourceGrant(ctx, organizationID, connectionID, resourceID)
	if err != nil {
		return ResourceGrant{}, wrapControlPlaneError(err)
	}
	grant.Enabled = enabled
	grant.UpdatedAt = time.Now().UTC()
	s.grants[grant.ID] = cloneGrant(grant)

	return cloneGrant(grant), nil
}

func (s *MemoryStore) AppendAudit(_ context.Context, event AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if event.ID == "" {
		event.ID = uuid.NewString()
	}

	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	event.Detail = sanitizeAuditDetail(event.Detail)
	s.audit = append(s.audit, event)

	return nil
}

func (s *MemoryStore) ListAudit(_ context.Context, organizationID string, limit int) ([]AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	out := make([]AuditEvent, 0, limit)
	for i := len(s.audit) - 1; i >= 0 && len(out) < limit; i-- {
		if s.audit[i].OrganizationID == organizationID {
			out = append(out, s.audit[i])
		}
	}

	return out, nil
}

func (s *MemoryStore) PutOAuthState(_ context.Context, state OAuthState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.oauthStates[state.State] = state

	return nil
}

func (s *MemoryStore) TakeOAuthState(_ context.Context, state string) (OAuthState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.oauthStates[state]
	if !ok || time.Now().After(stored.ExpiresAt) {
		delete(s.oauthStates, state)
		return OAuthState{}, ErrNotFound
	}

	delete(s.oauthStates, state)

	return stored, nil
}

func (s *MemoryStore) Close() error { return nil }

func sanitizeAuditDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > 500 {
		return detail[:500]
	}

	return detail
}
