package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

// How a room's tracker credentials were obtained.
const (
	// AuthOAuth is the Jira 3LO flow, routed through api.atlassian.com.
	AuthOAuth = "oauth"
	// AuthToken is a personal access token sent on every request.
	AuthToken = "token"
)

// SourceConnection is a room's link to a backlog tracker. The credentials
// themselves never live on this struct: they stay encrypted and are passed
// alongside it, so a stray log line cannot leak them.
type SourceConnection struct {
	RoomID   string
	Provider string
	AuthType string
	BaseURL  string
	CloudID  string
	Name     string // what the UI shows: the site, organisation or account
	Account  string // the email, login or organisation the token belongs to
	// TokenExpiresAt is the lifetime of an OAuth access token.
	TokenExpiresAt time.Time
	// ExpiresAt is when Estimeet forgets the credentials, whatever the tracker
	// thinks. Nothing here is meant to outlive the estimation session.
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TokenStale reports whether an OAuth access token needs refreshing. Personal
// access tokens have no scheduled expiry; only revocation ends them.
func (c SourceConnection) TokenStale(now time.Time) bool {
	if c.AuthType == AuthToken {
		return false
	}
	return !now.Before(c.TokenExpiresAt.Add(-60 * time.Second))
}

// SaveSourceConnection upserts a room's encrypted credentials. ExpiresAt is
// always written as given, so refreshing an OAuth token cannot quietly extend
// the retention window past what the host agreed to.
func (s *Store) SaveSourceConnection(ctx context.Context, c SourceConnection, tokenEnc, refreshEnc []byte) error {
	now := toMillis(time.Now())
	var refresh []byte
	if len(refreshEnc) > 0 {
		refresh = refreshEnc
	}
	_, err := s.exec(ctx,
		`INSERT INTO source_connections
		   (room_id, provider, auth_type, base_url, cloud_id, display_name, account,
		    access_token, refresh_token, token_expires_at, expires_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (room_id) DO UPDATE SET
		   provider = excluded.provider,
		   auth_type = excluded.auth_type,
		   base_url = excluded.base_url,
		   cloud_id = excluded.cloud_id,
		   display_name = excluded.display_name,
		   account = excluded.account,
		   access_token = excluded.access_token,
		   -- Personal access tokens have no refresh token; OAuth keeps the last
		   -- one the tracker handed out if this write did not rotate it.
		   refresh_token = CASE
		     WHEN excluded.auth_type = 'token' THEN NULL
		     ELSE COALESCE(excluded.refresh_token, source_connections.refresh_token)
		   END,
		   token_expires_at = excluded.token_expires_at,
		   expires_at = excluded.expires_at,
		   updated_at = excluded.updated_at`,
		c.RoomID, c.Provider, c.AuthType, c.BaseURL, c.CloudID, c.Name, c.Account,
		tokenEnc, refresh, toMillis(c.TokenExpiresAt), toMillis(c.ExpiresAt), now, now)
	return err
}

// RawSourceConnection returns the row with its credentials still encrypted.
// A connection past its retention deadline is reported as missing even if the
// purge has not run yet, so an expiry can never be missed.
func (s *Store) RawSourceConnection(ctx context.Context, roomID string, now time.Time) (SourceConnection, []byte, []byte, error) {
	var (
		c            SourceConnection
		tokenEnc     []byte
		refreshEnc   []byte
		tokenExpires int64
		expires      int64
		created      int64
		updated      int64
	)
	err := s.queryRow(ctx,
		`SELECT room_id, provider, auth_type, base_url, cloud_id, display_name, account,
		        access_token, refresh_token, token_expires_at, expires_at, created_at, updated_at
		 FROM source_connections WHERE room_id = ? AND expires_at > ?`, roomID, toMillis(now)).
		Scan(&c.RoomID, &c.Provider, &c.AuthType, &c.BaseURL, &c.CloudID, &c.Name, &c.Account,
			&tokenEnc, &refreshEnc, &tokenExpires, &expires, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return SourceConnection{}, nil, nil, domain.ErrNotFound
	}
	if err != nil {
		return SourceConnection{}, nil, nil, err
	}
	c.TokenExpiresAt = fromMillis(tokenExpires)
	c.ExpiresAt = fromMillis(expires)
	c.CreatedAt = fromMillis(created)
	c.UpdatedAt = fromMillis(updated)
	return c, tokenEnc, refreshEnc, nil
}

// DeleteSourceConnection disconnects a room from its tracker.
func (s *Store) DeleteSourceConnection(ctx context.Context, roomID string) error {
	_, err := s.exec(ctx, `DELETE FROM source_connections WHERE room_id = ?`, roomID)
	return err
}

// PurgeSourceConnections deletes everything past its retention deadline, plus
// anything belonging to a closed room. It returns how many rows went away.
func (s *Store) PurgeSourceConnections(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.exec(ctx,
		`DELETE FROM source_connections
		 WHERE expires_at <= ?
		    OR room_id IN (SELECT id FROM rooms WHERE closed_at IS NOT NULL)`, toMillis(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
