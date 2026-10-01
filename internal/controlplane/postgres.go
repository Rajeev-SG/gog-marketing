package controlplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // register the Postgres database/sql driver
)

type PostgresStore struct {
	db *sql.DB
}

func OpenPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open control-plane postgres: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping control-plane postgres: %w", err)
	}

	store := &PostgresStore{db: db}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, wrapControlPlaneError(err)
	}

	return store, nil
}

func (s *PostgresStore) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migrations := ControlPlaneMigrations()
	for _, migration := range migrations {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, migration.Version).Scan(&exists); err != nil {
			return wrapControlPlaneError(err)
		}

		if exists {
			continue
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return wrapControlPlaneError(err)
		}

		if _, err := tx.ExecContext(ctx, migration.Up); err != nil {
			_ = tx.Rollback()

			return fmt.Errorf("apply migration %s: %w", migration.Version, err)
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES($1, $2)`, migration.Version, time.Now().UTC()); err != nil {
			_ = tx.Rollback()

			return wrapControlPlaneError(err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

func (s *PostgresStore) RollBackMigrations(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migration004Down); err != nil {
		return wrapControlPlaneError(err)
	}

	if _, err := s.db.ExecContext(ctx, migration003Down); err != nil {
		return wrapControlPlaneError(err)
	}

	if _, err := s.db.ExecContext(ctx, migration002Down); err != nil {
		return wrapControlPlaneError(err)
	}

	if _, err := s.db.ExecContext(ctx, migration001Down); err != nil {
		return wrapControlPlaneError(err)
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version IN ($1,$2,$3,$4)`, "001", "002", "003", "004")

	return wrapControlPlaneError(err)
}

func (s *PostgresStore) BootstrapOwner(ctx context.Context, user User, organization Organization, role string) (User, Organization, error) {
	if role != "owner" && role != "admin" {
		return User{}, Organization{}, ErrInvalid
	}

	if user.ID == "" {
		user.ID = uuid.NewString()
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, Organization{}, wrapControlPlaneError(err)
	}

	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO users(id,email,external_subject,display_name,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (email) DO UPDATE SET external_subject=EXCLUDED.external_subject, display_name=EXCLUDED.display_name, updated_at=EXCLUDED.updated_at`, user.ID, normalizeEmail(user.Email), user.ExternalSubject, user.DisplayName, user.CreatedAt, user.UpdatedAt); err != nil {
		return User{}, Organization{}, wrapControlPlaneError(err)
	}

	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email=$1`, normalizeEmail(user.Email)).Scan(&user.ID); err != nil {
		return User{}, Organization{}, wrapControlPlaneError(err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO organizations(id,name,slug,created_at,updated_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT (slug) DO UPDATE SET name=EXCLUDED.name, updated_at=EXCLUDED.updated_at`, organization.ID, organization.Name, organization.Slug, organization.CreatedAt, organization.UpdatedAt); err != nil {
		return User{}, Organization{}, wrapControlPlaneError(err)
	}

	if err := tx.QueryRowContext(ctx, `SELECT id FROM organizations WHERE slug=$1`, organization.Slug).Scan(&organization.ID); err != nil {
		return User{}, Organization{}, wrapControlPlaneError(err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO memberships(user_id,organization_id,role,created_at) VALUES($1,$2,$3,$4) ON CONFLICT (user_id,organization_id) DO UPDATE SET role=EXCLUDED.role`, user.ID, organization.ID, role, now); err != nil {
		return User{}, Organization{}, wrapControlPlaneError(err)
	}

	if err := tx.Commit(); err != nil {
		return User{}, Organization{}, fmt.Errorf("commit bootstrap transaction: %w", err)
	}

	return user, organization, nil
}

func (s *PostgresStore) CreateConnection(ctx context.Context, connection Connection) (Connection, error) {
	name, err := normalizeName(connection.Name)
	if err != nil {
		return Connection{}, wrapControlPlaneError(err)
	}

	connection.Name = name
	if connection.ID == "" {
		connection.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	connection.CreatedAt = now

	connection.UpdatedAt = now
	if connection.Status == "" {
		connection.Status = ConnectionNeedsConnect
	}
	services, _ := json.Marshal(connection.Services)
	requested, _ := json.Marshal(connection.RequestedScopes)
	granted, _ := json.Marshal(connection.GrantedScopes)
	discovery, _ := json.Marshal(connection.DiscoveryStatus)

	_, err = s.db.ExecContext(ctx, `INSERT INTO google_connections(id,organization_id,name,google_email,google_subject,oauth_client_id,services_json,requested_scopes_json,granted_scopes_json,status,token_secret_ref,product_managed,last_validated_at,last_error,discovery_status_json,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		connection.ID, connection.OrganizationID, connection.Name, connection.GoogleEmail, connection.GoogleSubject, connection.OAuthClientID, services, requested, granted, connection.Status, connection.SecretRef, connection.ProductManaged, connection.LastValidatedAt, connection.LastError, discovery, connection.CreatedAt, connection.UpdatedAt)
	if isUniqueViolation(err) {
		return Connection{}, ErrConflict
	}

	if err != nil {
		return Connection{}, wrapControlPlaneError(err)
	}

	return cloneConnection(connection), nil
}

func (s *PostgresStore) GetConnection(ctx context.Context, organizationID, id string) (Connection, error) {
	return scanConnection(s.db.QueryRowContext(ctx, connectionSelect+` WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

func (s *PostgresStore) ListConnections(ctx context.Context, organizationID string) ([]Connection, error) {
	rows, err := s.db.QueryContext(ctx, connectionSelect+` WHERE organization_id=$1 ORDER BY name`, organizationID)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}
	defer rows.Close()
	out := []Connection{}

	for rows.Next() {
		connection, err := scanConnectionRows(rows)
		if err != nil {
			return nil, wrapControlPlaneError(err)
		}

		out = append(out, connection)
	}

	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("read rows: %w", err)
	}

	return out, nil
}

// UpdateConnection deliberately does not write discovery_status_json. That
// status is owned exclusively by UpdateDiscoveryStatus to avoid stale clobbering.
func (s *PostgresStore) UpdateConnection(ctx context.Context, connection Connection) (Connection, error) {
	name, err := normalizeName(connection.Name)
	if err != nil {
		return Connection{}, wrapControlPlaneError(err)
	}
	connection.Name = name
	connection.UpdatedAt = time.Now().UTC()
	services, _ := json.Marshal(connection.Services)
	requested, _ := json.Marshal(connection.RequestedScopes)
	granted, _ := json.Marshal(connection.GrantedScopes)

	result, err := s.db.ExecContext(ctx, `UPDATE google_connections SET name=$1,google_email=$2,google_subject=$3,oauth_client_id=$4,services_json=$5,requested_scopes_json=$6,granted_scopes_json=$7,status=$8,token_secret_ref=$9,product_managed=$10,last_validated_at=$11,last_error=$12,last_error_category=$13,updated_at=$14 WHERE organization_id=$15 AND id=$16`,
		connection.Name, connection.GoogleEmail, connection.GoogleSubject, connection.OAuthClientID, services, requested, granted, connection.Status, connection.SecretRef, connection.ProductManaged, connection.LastValidatedAt, connection.LastError, connection.LastErrorCategory, connection.UpdatedAt, connection.OrganizationID, connection.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return Connection{}, ErrConflict
		}

		return Connection{}, wrapControlPlaneError(err)
	}

	if affected, _ := result.RowsAffected(); affected == 0 {
		return Connection{}, ErrNotFound
	}

	return cloneConnection(connection), nil
}

// UpdateDiscoveryStatus is the sole writer for per-service discovery status.
func (s *PostgresStore) UpdateDiscoveryStatus(ctx context.Context, organizationID, connectionID string, status map[string]DiscoveryServiceStatus) error {
	discovery, _ := json.Marshal(status)

	result, err := s.db.ExecContext(ctx, `UPDATE google_connections SET discovery_status_json=$1,updated_at=now() WHERE organization_id=$2 AND id=$3`, discovery, organizationID, connectionID)
	if err != nil {
		return wrapControlPlaneError(err)
	}

	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *PostgresStore) DeleteConnection(ctx context.Context, organizationID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM google_connections WHERE organization_id=$1 AND id=$2`, organizationID, id)
	if err != nil {
		return wrapControlPlaneError(err)
	}

	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	return nil
}

// Resource grants are authoritative per connection plus resource. The same
// Google resource can exist under two connections, but authorization always
// resolves the exact requested connection and never shares authority across it.
func (s *PostgresStore) UpsertResourceGrant(ctx context.Context, grant ResourceGrant) (ResourceGrant, error) {
	if grant.ID == "" {
		grant.ID = uuid.NewString()
	}

	now := time.Now().UTC()
	if grant.DiscoveredAt.IsZero() {
		grant.DiscoveredAt = now
	}
	grant.UpdatedAt = now
	metadata, _ := json.Marshal(grant.Metadata)

	_, err := s.db.ExecContext(ctx, `INSERT INTO resource_grants(id,connection_id,organization_id,service,resource_type,resource_id,display_name,parent,metadata_json,enabled,discovered_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (connection_id,resource_id) DO UPDATE SET service=EXCLUDED.service,resource_type=EXCLUDED.resource_type,display_name=EXCLUDED.display_name,parent=EXCLUDED.parent,metadata_json=EXCLUDED.metadata_json,updated_at=EXCLUDED.updated_at`,
		grant.ID, grant.ConnectionID, grant.OrganizationID, grant.Service, grant.ResourceType, grant.ResourceID, grant.DisplayName, grant.Parent, metadata, grant.Enabled, grant.DiscoveredAt, grant.UpdatedAt)
	if err != nil {
		return ResourceGrant{}, wrapControlPlaneError(err)
	}

	return s.GetResourceGrant(ctx, grant.OrganizationID, grant.ConnectionID, grant.ResourceID)
}

func (s *PostgresStore) ListResourceGrants(ctx context.Context, organizationID, connectionID string) ([]ResourceGrant, error) {
	rows, err := s.db.QueryContext(ctx, grantSelect+` WHERE organization_id=$1 AND connection_id=$2 ORDER BY service,resource_id`, organizationID, connectionID)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}
	defer rows.Close()
	out := []ResourceGrant{}

	for rows.Next() {
		grant, err := scanGrantRows(rows)
		if err != nil {
			return nil, wrapControlPlaneError(err)
		}

		out = append(out, grant)
	}

	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("read rows: %w", err)
	}

	return out, nil
}

func (s *PostgresStore) GetResourceGrant(ctx context.Context, organizationID, connectionID, resourceID string) (ResourceGrant, error) {
	return scanGrant(s.db.QueryRowContext(ctx, grantSelect+` WHERE organization_id=$1 AND connection_id=$2 AND resource_id=$3`, organizationID, connectionID, resourceID))
}

func (s *PostgresStore) SetResourceEnabled(ctx context.Context, organizationID, connectionID, resourceID string, enabled bool) (ResourceGrant, error) {
	// SetResourceEnabled is the explicit permission mutation; discovery upserts
	// deliberately leave this column unchanged. Use database time so restarts and
	// concurrent writers see one consistent audit/update ordering.
	result, err := s.db.ExecContext(ctx, `UPDATE resource_grants SET enabled=$1,updated_at=now() WHERE organization_id=$2 AND connection_id=$3 AND resource_id=$4`, enabled, organizationID, connectionID, resourceID)
	if err != nil {
		return ResourceGrant{}, wrapControlPlaneError(err)
	}

	if affected, _ := result.RowsAffected(); affected == 0 {
		return ResourceGrant{}, ErrNotFound
	}

	return s.GetResourceGrant(ctx, organizationID, connectionID, resourceID)
}

func (s *PostgresStore) AppendAudit(ctx context.Context, event AuditEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}

	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	event.Detail = sanitizeAuditDetail(event.Detail)
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_events(id,organization_id,acting_user_id,connection_id,action,result,detail,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		event.ID, event.OrganizationID, event.ActingUserID, event.ConnectionID, event.Action, event.Result, event.Detail, event.CreatedAt)

	return wrapControlPlaneError(err)
}

func (s *PostgresStore) ListAudit(ctx context.Context, organizationID string, limit int) ([]AuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id,organization_id,acting_user_id,connection_id,action,result,detail,created_at FROM audit_events WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}
	defer rows.Close()
	out := []AuditEvent{}

	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.ID, &event.OrganizationID, &event.ActingUserID, &event.ConnectionID, &event.Action, &event.Result, &event.Detail, &event.CreatedAt); err != nil {
			return nil, wrapControlPlaneError(err)
		}
		out = append(out, event)
	}

	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("read rows: %w", err)
	}

	return out, nil
}

func (s *PostgresStore) PutOAuthState(ctx context.Context, state OAuthState) error {
	scopes, _ := json.Marshal(state.Scope)
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_states(state,organization_id,connection_id,code_verifier,redirect_uri,scopes_json,nonce,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		state.State, state.OrganizationID, state.ConnectionID, state.CodeVerifier, state.RedirectURI, scopes, state.Nonce, state.ExpiresAt)

	return wrapControlPlaneError(err)
}

func (s *PostgresStore) TakeOAuthState(ctx context.Context, state string) (OAuthState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OAuthState{}, wrapControlPlaneError(err)
	}
	defer func() { _ = tx.Rollback() }()
	var stored OAuthState
	var scopes string

	err = tx.QueryRowContext(ctx, `SELECT state,organization_id,connection_id,code_verifier,redirect_uri,scopes_json,nonce,expires_at FROM oauth_states WHERE state=$1`, state).Scan(&stored.State, &stored.OrganizationID, &stored.ConnectionID, &stored.CodeVerifier, &stored.RedirectURI, &scopes, &stored.Nonce, &stored.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthState{}, ErrNotFound
	}

	if err != nil {
		return OAuthState{}, wrapControlPlaneError(err)
	}
	_ = json.Unmarshal([]byte(scopes), &stored.Scope)

	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state=$1`, state); err != nil {
		return OAuthState{}, wrapControlPlaneError(err)
	}

	if time.Now().After(stored.ExpiresAt) {
		return OAuthState{}, ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return stored, fmt.Errorf("commit OAuth state transaction: %w", err)
	}

	return stored, nil
}

func (s *PostgresStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close Postgres store: %w", err)
	}

	return nil
}

const (
	connectionSelect = `SELECT id,organization_id,name,google_email,google_subject,oauth_client_id,services_json,requested_scopes_json,granted_scopes_json,status,token_secret_ref,product_managed,last_validated_at,last_error,last_error_category,discovery_status_json,created_at,updated_at FROM google_connections`
	grantSelect      = `SELECT id,connection_id,organization_id,service,resource_type,resource_id,display_name,parent,metadata_json,enabled,discovered_at,updated_at FROM resource_grants`
)

type rowScanner interface{ Scan(dest ...any) error }

func scanConnection(row rowScanner) (Connection, error) {
	var connection Connection
	var services, requested, granted, discovery string

	err := row.Scan(&connection.ID, &connection.OrganizationID, &connection.Name, &connection.GoogleEmail, &connection.GoogleSubject, &connection.OAuthClientID, &services, &requested, &granted, &connection.Status, &connection.SecretRef, &connection.ProductManaged, &connection.LastValidatedAt, &connection.LastError, &connection.LastErrorCategory, &discovery, &connection.CreatedAt, &connection.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, ErrNotFound
	}

	if err != nil {
		return Connection{}, wrapControlPlaneError(err)
	}
	_ = json.Unmarshal([]byte(services), &connection.Services)
	_ = json.Unmarshal([]byte(requested), &connection.RequestedScopes)
	_ = json.Unmarshal([]byte(granted), &connection.GrantedScopes)
	_ = json.Unmarshal([]byte(discovery), &connection.DiscoveryStatus)

	return cloneConnection(connection), nil
}

func scanConnectionRows(rows *sql.Rows) (Connection, error) { return scanConnection(rows) }

func scanGrant(row rowScanner) (ResourceGrant, error) {
	var grant ResourceGrant
	var metadata string

	err := row.Scan(&grant.ID, &grant.ConnectionID, &grant.OrganizationID, &grant.Service, &grant.ResourceType, &grant.ResourceID, &grant.DisplayName, &grant.Parent, &metadata, &grant.Enabled, &grant.DiscoveredAt, &grant.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceGrant{}, ErrNotFound
	}

	if err != nil {
		return ResourceGrant{}, wrapControlPlaneError(err)
	}
	_ = json.Unmarshal([]byte(metadata), &grant.Metadata)

	return cloneGrant(grant), nil
}

func scanGrantRows(rows *sql.Rows) (ResourceGrant, error) { return scanGrant(rows) }

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}

func (s *PostgresStore) MigrationVersions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, wrapControlPlaneError(err)
	}
	defer rows.Close()
	versions := make([]string, 0, 2)

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, wrapControlPlaneError(err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapControlPlaneError(err)
	}

	return versions, nil
}
