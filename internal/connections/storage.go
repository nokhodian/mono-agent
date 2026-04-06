package connections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Connection represents a stored platform connection.
type Connection struct {
	ID         string                 `json:"id"`
	Platform   string                 `json:"platform"`
	Method     AuthMethod             `json:"method"`
	Label      string                 `json:"label"`
	AccountID  string                 `json:"account_id"`
	Data       map[string]interface{} `json:"data"`
	Status     string                 `json:"status"`      // "active" | "expired" | "error"
	LastTested string                 `json:"last_tested,omitempty"`
	CreatedAt  string                 `json:"created_at"`
	UpdatedAt  string                 `json:"updated_at"`
}

const createConnectionsTable = `
CREATE TABLE IF NOT EXISTS connections (
    id          TEXT PRIMARY KEY,
    platform    TEXT NOT NULL,
    method      TEXT NOT NULL,
    label       TEXT NOT NULL,
    account_id  TEXT NOT NULL DEFAULT '',
    data        TEXT NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'active',
    last_tested TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_connections_platform ON connections(platform);
CREATE INDEX IF NOT EXISTS idx_connections_status   ON connections(status);
`

// Store provides CRUD operations for connections.
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// EnsureTable creates the connections table and indices if they do not exist.
func (s *Store) EnsureTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, createConnectionsTable)
	return err
}

// Save upserts a connection. If ID is empty a new UUID is generated. If
// CreatedAt is empty it is set to now. UpdatedAt is always refreshed.
// Status defaults to "active" when empty.
func (s *Store) Save(ctx context.Context, c *Connection) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if c.CreatedAt == "" {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.Status == "" {
		c.Status = "active"
	}

	dataBytes, err := json.Marshal(c.Data)
	if err != nil {
		return fmt.Errorf("connections.Save: marshal data: %w", err)
	}

	const q = `
INSERT INTO connections (id, platform, method, label, account_id, data, status, last_tested, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    label=excluded.label, account_id=excluded.account_id,
    data=excluded.data, status=excluded.status,
    last_tested=excluded.last_tested, updated_at=excluded.updated_at`

	_, err = s.db.ExecContext(ctx, q,
		c.ID, c.Platform, string(c.Method), c.Label, c.AccountID,
		string(dataBytes), c.Status, c.LastTested, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("connections.Save: %w", err)
	}
	return nil
}

// Get returns the connection with the given ID, or nil if not found.
func (s *Store) Get(ctx context.Context, id string) (*Connection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, platform, method, label, account_id, data, status, last_tested, created_at, updated_at
         FROM connections WHERE id = ?`, id)
	return scanConnection(row)
}

// GetOrResolve looks up a connection by ID, falling back to platform name lookup
// if the ID isn't found. When only one connection exists for a platform, it's used
// automatically. OAuth tokens are refreshed if expired.
func (s *Store) GetOrResolve(ctx context.Context, idOrPlatform string) (*Connection, error) {
	// Try by ID first.
	conn, err := s.Get(ctx, idOrPlatform)
	if err == nil && conn != nil {
		return s.ensureFreshToken(ctx, conn)
	}

	// Fallback: look up by platform name — auto-pick if only one exists.
	conns, err := s.ListByPlatform(ctx, idOrPlatform)
	if err != nil || len(conns) == 0 {
		return nil, fmt.Errorf("no connection found for %q", idOrPlatform)
	}
	// Prefer active connections.
	for i := range conns {
		if conns[i].Status == "active" {
			return s.ensureFreshToken(ctx, &conns[i])
		}
	}
	return s.ensureFreshToken(ctx, &conns[0])
}

// ensureFreshToken checks if an OAuth token is expired and refreshes it using
// the stored refresh_token. Returns the (possibly refreshed) connection.
func (s *Store) ensureFreshToken(ctx context.Context, conn *Connection) (*Connection, error) {
	expiresStr, _ := conn.Data["expires_at"].(string)
	if expiresStr == "" {
		return conn, nil // Not an OAuth connection or no expiry — use as-is.
	}
	expiresAt, err := time.Parse(time.RFC3339, expiresStr)
	if err != nil {
		return conn, nil // Can't parse — use as-is.
	}
	// Token still valid for at least 60 more seconds.
	if time.Now().UTC().Before(expiresAt.Add(-60 * time.Second)) {
		return conn, nil
	}

	// Token expired — try to refresh.
	refreshToken, _ := conn.Data["refresh_token"].(string)
	if refreshToken == "" {
		return conn, nil // No refresh token — use expired token (will likely fail).
	}

	p, ok := Get(conn.Platform)
	if !ok || p.OAuth == nil {
		return conn, nil // Unknown platform or no OAuth config.
	}

	cfg := *p.OAuth
	// Load client credentials from platform_oauth_credentials table (Wails UI stores them there),
	// then fall back to env vars.
	var dbClientID, dbClientSecret string
	_ = s.db.QueryRowContext(ctx,
		`SELECT client_id, client_secret FROM platform_oauth_credentials WHERE platform = ?`,
		conn.Platform,
	).Scan(&dbClientID, &dbClientSecret)
	if dbClientID != "" {
		cfg.ClientID = dbClientID
		cfg.ClientSecret = dbClientSecret
	}
	// Allow env var overrides.
	envPrefix := "MONOES_" + strings.ToUpper(strings.ReplaceAll(p.ID, "-", "_")) + "_"
	if cfg.ClientID == "" {
		cfg.ClientID = os.Getenv(envPrefix + "CLIENT_ID")
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = os.Getenv(envPrefix + "CLIENT_SECRET")
	}
	if cfg.ClientID == "" {
		return conn, nil // Can't refresh without client credentials.
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return conn, nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return conn, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return conn, nil
	}

	var result OAuthResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return conn, nil
	}

	// Update connection data with new tokens.
	conn.Data["access_token"] = result.AccessToken
	conn.Data["token_type"] = result.TokenType
	if result.RefreshToken != "" {
		conn.Data["refresh_token"] = result.RefreshToken
	}
	if result.ExpiresIn > 0 {
		conn.Data["expires_at"] = time.Now().UTC().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339)
	}

	// Persist refreshed token.
	_ = s.Save(ctx, conn)

	return conn, nil
}

func envLookup(key string) string {
	return os.Getenv(key)
}

// ListAll returns all connections ordered by platform then created_at.
func (s *Store) ListAll(ctx context.Context) ([]Connection, error) {
	const q = `SELECT id, platform, method, label, account_id, data, status, last_tested, created_at, updated_at
	           FROM connections ORDER BY platform, created_at`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("connections.ListAll: %w", err)
	}
	defer rows.Close()
	out, err := scanConnections(rows)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []Connection{}
	}
	return out, nil
}

// ListByPlatform returns all connections for a given platform, ordered by created_at.
func (s *Store) ListByPlatform(ctx context.Context, platform string) ([]Connection, error) {
	const q = `SELECT id, platform, method, label, account_id, data, status, last_tested, created_at, updated_at
	           FROM connections WHERE platform = ? ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q, platform)
	if err != nil {
		return nil, fmt.Errorf("connections.ListByPlatform: %w", err)
	}
	defer rows.Close()
	return scanConnections(rows)
}

// Delete removes a connection by ID. Returns an error if the row does not exist.
func (s *Store) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM connections WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("connections.Delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("connections.Delete: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("connections.Delete: id %q not found", id)
	}
	return nil
}

// MarkTested updates the status, last_tested, and updated_at fields for the
// given connection ID.
func (s *Store) MarkTested(ctx context.Context, id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `UPDATE connections SET status = ?, last_tested = ?, updated_at = ? WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, status, now, now, id)
	if err != nil {
		return fmt.Errorf("connections.MarkTested: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("connections.MarkTested: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("connections.MarkTested: id %q not found", id)
	}
	return nil
}

// scanConnection reads a single Connection from a *sql.Row.
func scanConnection(row *sql.Row) (*Connection, error) {
	var c Connection
	var dataJSON, method string
	err := row.Scan(&c.ID, &c.Platform, &method, &c.Label, &c.AccountID,
		&dataJSON, &c.Status, &c.LastTested, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanConnection: %w", err)
	}
	c.Method = AuthMethod(method)
	if err := json.Unmarshal([]byte(dataJSON), &c.Data); err != nil {
		return nil, fmt.Errorf("scanConnection: unmarshal data: %w", err)
	}
	return &c, nil
}

// scanConnections reads all Connection rows from a *sql.Rows result set.
func scanConnections(rows *sql.Rows) ([]Connection, error) {
	var out []Connection
	for rows.Next() {
		var c Connection
		var method, dataJSON string
		if err := rows.Scan(
			&c.ID, &c.Platform, &method, &c.Label, &c.AccountID,
			&dataJSON, &c.Status, &c.LastTested, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanConnections: %w", err)
		}
		c.Method = AuthMethod(method)
		if err := json.Unmarshal([]byte(dataJSON), &c.Data); err != nil {
			return nil, fmt.Errorf("scanConnections: unmarshal data: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanConnections: rows: %w", err)
	}
	return out, nil
}
