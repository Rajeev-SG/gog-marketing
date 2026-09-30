package controlplane

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"

	"github.com/openclaw/gogcli/internal/googleauth"
)

type Actor struct {
	UserID         string
	OrganizationID string
	Role           string
}

type Service struct {
	tokenLocks            sync.Map
	Store                 Store
	Secrets               SecretStore
	OAuth                 OAuthProvider
	Discoverer            Discoverer
	RedirectURI           string
	Now                   func() time.Time
	AnalyticsAdminFactory func(context.Context, string) (*analyticsadmin.Service, error)
}

type OAuthStart struct {
	URL   string `json:"url"`
	State string `json:"-"`
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}

	return time.Now().UTC()
}

func (s *Service) CreateConnection(ctx context.Context, actor Actor, name string, services []string) (Connection, error) {
	if actor.Role != "owner" && actor.Role != "admin" {
		return Connection{}, ErrForbidden
	}

	normalized := make([]string, 0, len(services))
	for _, service := range services {
		service = strings.ToLower(strings.TrimSpace(service))
		if service == "" {
			continue
		}

		if _, err := googleauth.Scopes(googleauth.Service(service)); err != nil {
			return Connection{}, fmt.Errorf("%w: unsupported service %q", ErrInvalid, service)
		}
		normalized = append(normalized, service)
	}

	scopes, err := googleauth.ScopesForManageWithOptions(serviceTypes(normalized), googleauth.ScopeOptions{Readonly: true})
	if err != nil {
		return Connection{}, fmt.Errorf("control-plane operation: %w", err)
	}

	connection, err := s.Store.CreateConnection(ctx, Connection{
		OrganizationID:  actor.OrganizationID,
		Name:            name,
		Services:        normalized,
		RequestedScopes: scopes,
		Status:          ConnectionNeedsConnect,
	})
	if err != nil {
		return Connection{}, fmt.Errorf("control-plane operation: %w", err)
	}

	s.audit(ctx, actor, connection.ID, "connection.created", "ok", "")

	return connection, nil
}

func (s *Service) ListConnections(ctx context.Context, actor Actor) ([]Connection, error) {
	if actor.OrganizationID == "" {
		return nil, ErrForbidden
	}

	connections, err := s.Store.ListConnections(ctx, actor.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}

	return connections, nil
}

func (s *Service) GetConnection(ctx context.Context, actor Actor, id string) (Connection, error) {
	connection, err := s.Store.GetConnection(ctx, actor.OrganizationID, id)
	if err != nil {
		return Connection{}, fmt.Errorf("get connection: %w", err)
	}

	return connection, nil
}

func (s *Service) RenameConnection(ctx context.Context, actor Actor, id, name string) (Connection, error) {
	connection, err := s.Store.GetConnection(ctx, actor.OrganizationID, id)
	if err != nil {
		return Connection{}, fmt.Errorf("control-plane operation: %w", err)
	}
	connection.Name = name

	updated, err := s.Store.UpdateConnection(ctx, connection)
	if err == nil {
		s.audit(ctx, actor, id, "connection.renamed", "ok", "")
	}

	if err != nil {
		return updated, fmt.Errorf("update connection: %w", err)
	}

	return updated, nil
}

func (s *Service) BeginOAuth(ctx context.Context, actor Actor, id string, forceConsent bool) (OAuthStart, error) {
	connection, err := s.Store.GetConnection(ctx, actor.OrganizationID, id)
	if err != nil {
		return OAuthStart{}, fmt.Errorf("control-plane operation: %w", err)
	}

	scopes, err := googleauth.ScopesForManageWithOptions(serviceTypes(connection.Services), googleauth.ScopeOptions{Readonly: true})
	if err != nil {
		return OAuthStart{}, fmt.Errorf("control-plane operation: %w", err)
	}

	state, err := randomToken(32)
	if err != nil {
		return OAuthStart{}, fmt.Errorf("control-plane operation: %w", err)
	}

	verifier, err := randomToken(48)
	if err != nil {
		return OAuthStart{}, fmt.Errorf("control-plane operation: %w", err)
	}

	nonce, err := randomToken(24)
	if err != nil {
		return OAuthStart{}, fmt.Errorf("control-plane operation: %w", err)
	}

	stored := OAuthState{
		State: state, OrganizationID: actor.OrganizationID, ConnectionID: id,
		CodeVerifier: verifier, RedirectURI: s.RedirectURI, Scope: scopes, Nonce: nonce,
		ExpiresAt: s.now().Add(10 * time.Minute),
	}
	if err := s.Store.PutOAuthState(ctx, stored); err != nil {
		return OAuthStart{}, fmt.Errorf("control-plane operation: %w", err)
	}

	connection.RequestedScopes = scopes
	if _, err := s.Store.UpdateConnection(ctx, connection); err != nil {
		return OAuthStart{}, fmt.Errorf("control-plane operation: %w", err)
	}

	s.audit(ctx, actor, id, "oauth.start", "ok", "")

	return OAuthStart{
		URL: s.OAuth.AuthorizationURL(OAuthStartInput{
			ForceConsent: forceConsent, State: state, CodeVerifier: verifier, Nonce: nonce,
			RedirectURI: s.RedirectURI, Scopes: scopes,
		}),
		State: state,
	}, nil
}

func (s *Service) CompleteOAuth(ctx context.Context, state, code string) (Connection, error) {
	stored, err := s.Store.TakeOAuthState(ctx, state)
	if err != nil {
		return Connection{}, fmt.Errorf("control-plane operation: %w", err)
	}

	connection, err := s.Store.GetConnection(ctx, stored.OrganizationID, stored.ConnectionID)
	if err != nil {
		return Connection{}, fmt.Errorf("control-plane operation: %w", err)
	}

	token, err := s.OAuth.Exchange(ctx, OAuthStartInput{
		State: stored.State, CodeVerifier: stored.CodeVerifier, Nonce: stored.Nonce,
		RedirectURI: stored.RedirectURI, Scopes: stored.Scope,
	}, code)
	if err != nil {
		connection.LastError = safeOAuthError(err)
		connection.Status = ConnectionNeedsReconnect
		_, _ = s.Store.UpdateConnection(ctx, connection)
		s.audit(ctx, Actor{OrganizationID: stored.OrganizationID}, connection.ID, "oauth.callback", "error", connection.LastError)

		return Connection{}, fmt.Errorf("control-plane operation: %w", err)
	}

	if saveErr := s.saveToken(ctx, Actor{OrganizationID: stored.OrganizationID}, &connection, token); saveErr != nil {
		return Connection{}, fmt.Errorf("control plane: %w", saveErr)
	}
	connection.GoogleEmail = token.Email
	connection.GoogleSubject = token.Subject
	connection.GrantedScopes = token.GrantedScopes
	connection.Status = ConnectionHealthy
	connection.LastError = ""
	connection.LastErrorCategory = ""
	validated := s.now()
	connection.LastValidatedAt = &validated

	updated, err := s.Store.UpdateConnection(ctx, connection)
	if err != nil {
		return Connection{}, fmt.Errorf("control-plane operation: %w", err)
	}

	s.audit(ctx, Actor{OrganizationID: stored.OrganizationID}, connection.ID, "oauth.callback", "ok", "")

	return updated, nil
}

func (s *Service) Refresh(ctx context.Context, actor Actor, id string) (Connection, error) {
	connection, token, err := s.ensureFreshToken(ctx, actor, id)
	if err != nil {
		return Connection{}, err
	}
	validated := s.now()
	connection.Status = ConnectionHealthy
	connection.LastError = ""
	connection.LastErrorCategory = ""
	connection.LastValidatedAt = &validated
	connection.GoogleEmail = token.Email
	connection.GrantedScopes = token.GrantedScopes

	updated, err := s.Store.UpdateConnection(ctx, connection)
	if err != nil {
		return updated, fmt.Errorf("update connection: %w", err)
	}

	s.audit(ctx, actor, id, "connection.validated", "ok", "")

	return updated, nil
}

func (s *Service) FreshToken(ctx context.Context, actor Actor, id string) (Connection, OAuthToken, error) {
	return s.ensureFreshToken(ctx, actor, id)
}

func (s *Service) ensureFreshToken(ctx context.Context, actor Actor, id string) (Connection, OAuthToken, error) {
	lockValue, _ := s.tokenLocks.LoadOrStore(id, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	connection, token, err := s.loadToken(ctx, actor, id)
	if err != nil {
		return Connection{}, OAuthToken{}, fmt.Errorf("control-plane operation: %w", err)
	}

	if token.Expiry.IsZero() || token.Expiry.Before(s.now().Add(time.Minute)) {
		refreshedConnection, refreshedToken, refreshErr := s.refreshStoredToken(ctx, actor, connection, token)
		if refreshErr != nil {
			return Connection{}, OAuthToken{}, refreshErr
		}
		connection, token = refreshedConnection, refreshedToken
	}

	if missing := missingScopes(serviceScopeRequirements(connection.Services), token.GrantedScopes); len(missing) > 0 {
		connection.Status = ConnectionNeedsReconnect
		connection.LastError = "granted scopes no longer cover the configured services"
		connection.LastErrorCategory = AuthFailureScopeMismatch
		_, _ = s.Store.UpdateConnection(ctx, connection)
		s.audit(ctx, actor, id, "scope.validation", "error", strings.Join(missing, " "))

		return Connection{}, OAuthToken{}, &AuthFailure{Category: AuthFailureScopeMismatch, Operation: "validate scopes", Err: fmt.Errorf("%w: %s", ErrMissingScopes, strings.Join(missing, " "))}
	}

	return connection, token, nil
}

func (s *Service) refreshStoredToken(ctx context.Context, actor Actor, connection Connection, token OAuthToken) (Connection, OAuthToken, error) {
	refreshed, err := s.OAuth.Refresh(ctx, token)
	if err != nil {
		category := classifyAuthError(err)
		connection.Status = ConnectionNeedsReconnect
		connection.LastError = safeOAuthError(err)
		connection.LastErrorCategory = category
		_, _ = s.Store.UpdateConnection(ctx, connection)
		s.audit(ctx, actor, connection.ID, "token.refresh", "error", string(category))

		return Connection{}, OAuthToken{}, wrapAuthFailure("refresh token", err)
	}

	if saveErr := s.saveToken(ctx, actor, &connection, refreshed); saveErr != nil {
		return Connection{}, OAuthToken{}, fmt.Errorf("persist refreshed token: %w", saveErr)
	}

	return connection, refreshed, nil
}

func serviceScopeRequirements(services []string) []string {
	required, err := googleauth.ScopesForManageWithOptions(serviceTypes(services), googleauth.ScopeOptions{Readonly: true})
	if err != nil {
		return nil
	}

	filtered := make([]string, 0, len(required))
	for _, scope := range required {
		if scope == "openid" || scope == "email" || scope == "https://www.googleapis.com/auth/userinfo.email" {
			continue
		}
		filtered = append(filtered, scope)
	}

	return filtered
}

func missingScopes(requested, granted []string) []string {
	return googleauth.MissingScopes(requested, granted)
}

func (s *Service) Discover(ctx context.Context, actor Actor, id string) ([]ResourceGrant, error) {
	connection, token, err := s.ensureFreshToken(ctx, actor, id)
	if err != nil {
		return nil, err
	}

	if s.Discoverer == nil {
		return nil, ErrDiscovererNotConfigured
	}

	var resources []ResourceGrant
	statuses := make(map[string]DiscoveryServiceStatus)

	if reporter, ok := s.Discoverer.(ReportingDiscoverer); ok {
		report, reportErr := reporter.DiscoverReport(ctx, connection, token)
		if reportErr != nil {
			s.audit(ctx, actor, id, "resource.discovery", "error", safeOAuthError(reportErr))
			return nil, fmt.Errorf("control-plane operation: %w", reportErr)
		}
		resources = report.Resources
		statuses = report.Statuses
	} else {
		var discoverErr error

		resources, discoverErr = s.Discoverer.Discover(ctx, connection, token)
		if discoverErr != nil {
			s.audit(ctx, actor, id, "resource.discovery", "error", safeOAuthError(discoverErr))
			return nil, fmt.Errorf("control-plane operation: %w", discoverErr)
		}

		counts := make(map[string]int)
		for _, resource := range resources {
			counts[resource.Service]++
		}

		for _, service := range connection.Services {
			name := strings.ToLower(strings.TrimSpace(service))
			if name == "" {
				continue
			}
			statuses[name] = DiscoveryServiceStatus{State: DiscoveryServiceOK, ResourceCount: counts[name], CheckedAt: s.now()}
		}
	}

	out := make([]ResourceGrant, 0, len(resources))
	discoveredServices := make(map[string]bool)

	for service, status := range statuses {
		if status.State == DiscoveryServiceOK && status.ResourceCount > 0 {
			discoveredServices[service] = true
		}
	}

	seen := make(map[string]bool, len(resources))
	for _, resource := range resources {
		// Discovery never grants access; only the explicit selection mutation does.
		resource.Enabled = false
		resource.OrganizationID = actor.OrganizationID
		resource.ConnectionID = id
		resource.DiscoveredAt = s.now()
		seen[resource.Service+"\x00"+resource.ResourceID] = true

		saved, saveErr := s.Store.UpsertResourceGrant(ctx, resource)
		if saveErr != nil {
			return nil, fmt.Errorf("save resource grant: %w", saveErr)
		}

		out = append(out, saved)
	}

	// A successful non-empty inventory establishes that the service is reachable.
	// Remove access only for resources missing from that inventory. Services with
	// no returned inventory may be unavailable, so their existing selections stay.
	existing, listErr := s.Store.ListResourceGrants(ctx, actor.OrganizationID, id)
	if listErr != nil {
		return nil, fmt.Errorf("reconcile resource grants: %w", listErr)
	}
	staleDisabled := 0

	for _, grant := range existing {
		if !grant.Enabled || !discoveredServices[grant.Service] || seen[grant.Service+"\x00"+grant.ResourceID] {
			continue
		}

		if _, disableErr := s.Store.SetResourceEnabled(ctx, actor.OrganizationID, id, grant.ResourceID, false); disableErr != nil {
			return nil, fmt.Errorf("disable stale resource grant: %w", disableErr)
		}

		staleDisabled++
	}

	if len(statuses) > 0 {
		if updateErr := s.Store.UpdateDiscoveryStatus(ctx, actor.OrganizationID, id, statuses); updateErr != nil {
			return nil, fmt.Errorf("save discovery status: %w", updateErr)
		}
	}

	result := "ok"

	for _, status := range statuses {
		if status.State != DiscoveryServiceOK {
			result = "partial"
			break
		}
	}

	s.audit(ctx, actor, id, "resource.discovery", result, fmt.Sprintf("%d resources; stale_disabled=%d; %s", len(out), staleDisabled, discoveryStatusSummary(statuses)))

	return out, nil
}

func discoveryStatusSummary(statuses map[string]DiscoveryServiceStatus) string {
	keys := make([]string, 0, len(statuses))
	for key := range statuses {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		status := statuses[key]
		parts = append(parts, fmt.Sprintf("%s:%s=%d", key, status.State, status.ResourceCount))
	}

	return strings.Join(parts, " ")
}

func (s *Service) ListResources(ctx context.Context, actor Actor, id string) ([]ResourceGrant, error) {
	grants, err := s.Store.ListResourceGrants(ctx, actor.OrganizationID, id)
	if err != nil {
		return nil, fmt.Errorf("list resource grants: %w", err)
	}

	return grants, nil
}

func (s *Service) SetResourceEnabled(ctx context.Context, actor Actor, id, resourceID string, enabled bool) (ResourceGrant, error) {
	grant, err := s.Store.SetResourceEnabled(ctx, actor.OrganizationID, id, resourceID, enabled)
	if err != nil {
		return ResourceGrant{}, fmt.Errorf("control-plane operation: %w", err)
	}

	action := "asset.disabled"
	if enabled {
		action = "asset.enabled"
	}

	s.audit(ctx, actor, id, action, "ok", resourceID)

	return grant, nil
}

func (s *Service) Disconnect(ctx context.Context, actor Actor, id string) error {
	connection, err := s.Store.GetConnection(ctx, actor.OrganizationID, id)
	if err != nil {
		return wrapControlPlaneError(err)
	}

	if connection.SecretRef != "" {
		if tokenRaw, loadErr := s.Secrets.Get(ctx, actor.OrganizationID, connection.SecretRef); loadErr == nil {
			if token, decodeErr := unmarshalToken(tokenRaw); decodeErr == nil {
				if revoker, ok := s.OAuth.(interface {
					Revoke(context.Context, OAuthToken) error
				}); ok {
					_ = revoker.Revoke(ctx, token)
				}
			}
		}

		if err := s.Secrets.Delete(ctx, actor.OrganizationID, connection.SecretRef); err != nil && !errors.Is(err, ErrSecretNotFound) {
			return wrapControlPlaneError(err)
		}
	}
	connection.SecretRef = ""
	connection.Status = ConnectionDisconnected
	connection.LastError = ""
	connection.LastErrorCategory = ""
	connection.GoogleEmail = ""
	connection.GoogleSubject = ""

	connection.GrantedScopes = nil
	if _, err := s.Store.UpdateConnection(ctx, connection); err != nil {
		return wrapControlPlaneError(err)
	}

	s.audit(ctx, actor, id, "connection.disconnected", "ok", "")

	return nil
}

func (s *Service) loadToken(ctx context.Context, actor Actor, id string) (Connection, OAuthToken, error) {
	connection, err := s.Store.GetConnection(ctx, actor.OrganizationID, id)
	if err != nil {
		return Connection{}, OAuthToken{}, fmt.Errorf("control-plane operation: %w", err)
	}

	if connection.SecretRef == "" {
		return Connection{}, OAuthToken{}, fmt.Errorf("%w: connection is not authorized", ErrInvalid)
	}

	raw, err := s.Secrets.Get(ctx, actor.OrganizationID, connection.SecretRef)
	if err != nil {
		if errors.Is(err, ErrSecretNotFound) {
			connection.Status = ConnectionNeedsReconnect
			connection.LastError = "stored refresh token is missing"
			connection.LastErrorCategory = AuthFailureInvalidGrant
			_, _ = s.Store.UpdateConnection(ctx, connection)
			s.audit(ctx, actor, id, "token.validation", "error", string(AuthFailureInvalidGrant))

			return Connection{}, OAuthToken{}, &AuthFailure{Category: AuthFailureInvalidGrant, Operation: "load token", Err: err}
		}

		return Connection{}, OAuthToken{}, fmt.Errorf("control-plane operation: %w", err)
	}

	token, err := unmarshalToken(raw)
	if err != nil {
		return Connection{}, OAuthToken{}, fmt.Errorf("control-plane operation: %w", err)
	}

	return connection, token, nil
}

func (s *Service) saveToken(ctx context.Context, actor Actor, connection *Connection, token OAuthToken) error {
	raw, err := marshalToken(token)
	if err != nil {
		return wrapControlPlaneError(err)
	}

	reference, err := s.Secrets.Put(ctx, actor.OrganizationID, raw)
	if err != nil {
		return wrapControlPlaneError(err)
	}

	oldReference := connection.SecretRef
	connection.SecretRef = reference

	if _, updateErr := s.Store.UpdateConnection(ctx, *connection); updateErr != nil {
		connection.SecretRef = oldReference
		reclaimCtx := context.WithoutCancel(ctx)

		go func() {
			// Keep the rotated credential long enough for an operator to repair
			// the connection reference, then reap the orphaned secret.
			time.Sleep(24 * time.Hour)
			_ = s.Secrets.Delete(reclaimCtx, actor.OrganizationID, reference)
		}()

		return fmt.Errorf("store token secret reference; new secret %q remains stored for reclamation: %w", reference, updateErr)
	}

	if oldReference != "" && oldReference != reference {
		_ = s.Secrets.Delete(ctx, actor.OrganizationID, oldReference)
	}

	return nil
}

func (s *Service) audit(ctx context.Context, actor Actor, connectionID, action, result, detail string) {
	_ = s.Store.AppendAudit(ctx, AuditEvent{
		OrganizationID: actor.OrganizationID,
		ActingUserID:   actor.UserID,
		ConnectionID:   connectionID,
		Action:         action,
		Result:         result,
		Detail:         sanitizeAuditDetail(detail),
	})
}

func serviceTypes(services []string) []googleauth.Service {
	out := make([]googleauth.Service, 0, len(services))
	for _, service := range services {
		out = append(out, googleauth.Service(service))
	}

	return out
}

func randomToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("control-plane operation: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func safeOAuthError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()

	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 300 {
		value = value[:300]
	}

	return value
}
