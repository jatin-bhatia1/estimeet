package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

// Jira authentication types stored in jira_connections.auth_type.
const (
	// JiraAuthOAuth is the 3LO flow, routed through api.atlassian.com.
	JiraAuthOAuth = "oauth"
	// JiraAuthToken is an Atlassian account email plus API token, sent as HTTP Basic.
	JiraAuthToken = "token"
)

// JiraConnection is a room's link to a Jira Cloud site. Tokens are already
// decrypted when this struct leaves the store.
type JiraConnection struct {
	RoomID       string
	AuthType     string
	CloudID      string
	SiteURL      string
	SiteName     string
	AccountEmail string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	UpdatedAt    time.Time
}

// Expired reports whether the access token is past (or close to) its lifetime.
// API tokens never expire on their own, so only OAuth connections can go stale.
func (c JiraConnection) Expired(now time.Time) bool {
	if c.AuthType == JiraAuthToken {
		return false
	}
	return !now.Before(c.ExpiresAt.Add(-60 * time.Second))
}

// SaveJiraConnection upserts the encrypted tokens for a room.
func (s *Store) SaveJiraConnection(ctx context.Context, c JiraConnection, accessEnc, refreshEnc []byte) error {
	now := toMillis(time.Now())
	var refresh any
	if len(refreshEnc) > 0 {
		refresh = refreshEnc
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO jira_connections
		   (room_id, auth_type, cloud_id, site_url, site_name, account_email, access_token, refresh_token, expires_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (room_id) DO UPDATE SET
		   auth_type = excluded.auth_type,
		   cloud_id = excluded.cloud_id,
		   site_url = excluded.site_url,
		   site_name = excluded.site_name,
		   account_email = excluded.account_email,
		   access_token = excluded.access_token,
		   -- API-token connections have no refresh token; OAuth keeps the last one
		   -- Atlassian handed out if this write did not rotate it.
		   refresh_token = CASE
		     WHEN excluded.auth_type = 'token' THEN NULL
		     ELSE COALESCE(excluded.refresh_token, jira_connections.refresh_token)
		   END,
		   expires_at = excluded.expires_at,
		   updated_at = excluded.updated_at`,
		c.RoomID, c.AuthType, c.CloudID, c.SiteURL, c.SiteName, c.AccountEmail,
		accessEnc, refresh, toMillis(c.ExpiresAt), now, now)
	return err
}

// RawJiraConnection returns the row with tokens still encrypted.
func (s *Store) RawJiraConnection(ctx context.Context, roomID string) (JiraConnection, []byte, []byte, error) {
	var (
		c          JiraConnection
		accessEnc  []byte
		refreshEnc []byte
		expires    int64
		updated    int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT room_id, auth_type, cloud_id, site_url, site_name, account_email,
		        access_token, refresh_token, expires_at, updated_at
		 FROM jira_connections WHERE room_id = ?`, roomID).
		Scan(&c.RoomID, &c.AuthType, &c.CloudID, &c.SiteURL, &c.SiteName, &c.AccountEmail,
			&accessEnc, &refreshEnc, &expires, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return JiraConnection{}, nil, nil, domain.ErrNotFound
	}
	if err != nil {
		return JiraConnection{}, nil, nil, err
	}
	c.ExpiresAt = fromMillis(expires)
	c.UpdatedAt = fromMillis(updated)
	return c, accessEnc, refreshEnc, nil
}

// DeleteJiraConnection disconnects a room from Jira.
func (s *Store) DeleteJiraConnection(ctx context.Context, roomID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jira_connections WHERE room_id = ?`, roomID)
	return err
}

// ----------------------------------------------------------------- oauth state

// OAuthState is the anti-CSRF record created before redirecting to Atlassian.
type OAuthState struct {
	State         string
	RoomID        string
	ParticipantID string
	CodeVerifier  string
	ExpiresAt     time.Time
}

// SaveOAuthState persists a pending authorization request and prunes stale ones.
func (s *Store) SaveOAuthState(ctx context.Context, st OAuthState) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM oauth_states WHERE expires_at < ?`, toMillis(time.Now())); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO oauth_states (state, room_id, participant_id, code_verifier, expires_at) VALUES (?, ?, ?, ?, ?)`,
		st.State, st.RoomID, st.ParticipantID, st.CodeVerifier, toMillis(st.ExpiresAt))
	return err
}

// ConsumeOAuthState atomically fetches and deletes a pending state (single use).
func (s *Store) ConsumeOAuthState(ctx context.Context, state string) (OAuthState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OAuthState{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		st      OAuthState
		expires int64
	)
	err = tx.QueryRowContext(ctx,
		`SELECT state, room_id, participant_id, code_verifier, expires_at FROM oauth_states WHERE state = ?`, state).
		Scan(&st.State, &st.RoomID, &st.ParticipantID, &st.CodeVerifier, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthState{}, domain.ErrNotFound
	}
	if err != nil {
		return OAuthState{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state); err != nil {
		return OAuthState{}, err
	}
	if err := tx.Commit(); err != nil {
		return OAuthState{}, err
	}

	st.ExpiresAt = fromMillis(expires)
	if time.Now().After(st.ExpiresAt) {
		return OAuthState{}, domain.ErrForbidden
	}
	return st, nil
}
