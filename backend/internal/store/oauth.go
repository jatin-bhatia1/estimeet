package store

// This file holds the short-lived state of the Jira OAuth 2.0
// authorization-code flow.

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

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
	if _, err := s.exec(ctx,
		`DELETE FROM oauth_states WHERE expires_at < ?`, toMillis(time.Now())); err != nil {
		return err
	}
	_, err := s.exec(ctx,
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
		s.rebind(`SELECT state, room_id, participant_id, code_verifier, expires_at FROM oauth_states WHERE state = ?`), state).
		Scan(&st.State, &st.RoomID, &st.ParticipantID, &st.CodeVerifier, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthState{}, domain.ErrNotFound
	}
	if err != nil {
		return OAuthState{}, err
	}
	if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM oauth_states WHERE state = ?`), state); err != nil {
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
