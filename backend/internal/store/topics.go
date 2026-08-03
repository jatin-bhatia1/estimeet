package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

const topicColumns = `id, room_id, title, description, external_key, external_url,
	position, status, final_estimate, created_at, revealed_at`

// CreateTopic inserts one topic.
func (s *Store) CreateTopic(ctx context.Context, t domain.Topic) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO topics (`+topicColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.RoomID, t.Title, t.Description, nullString(t.ExternalKey), nullString(t.ExternalURL),
		t.Position, string(t.Status), nullString(t.FinalEstimate), toMillis(t.CreatedAt), nullTime(t.RevealedAt),
	)
	return err
}

// TopicByID fetches a topic scoped to its room.
func (s *Store) TopicByID(ctx context.Context, roomID, topicID string) (domain.Topic, error) {
	return s.scanTopic(s.db.QueryRowContext(ctx,
		`SELECT `+topicColumns+` FROM topics WHERE id = ? AND room_id = ?`, topicID, roomID))
}

func (s *Store) scanTopic(row *sql.Row) (domain.Topic, error) {
	var (
		t        domain.Topic
		extKey   sql.NullString
		extURL   sql.NullString
		status   string
		final    sql.NullString
		created  int64
		revealed sql.NullInt64
	)
	err := row.Scan(&t.ID, &t.RoomID, &t.Title, &t.Description, &extKey, &extURL,
		&t.Position, &status, &final, &created, &revealed)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Topic{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Topic{}, err
	}
	t.ExternalKey = stringPtr(extKey)
	t.ExternalURL = stringPtr(extURL)
	t.Status = domain.TopicStatus(status)
	t.FinalEstimate = stringPtr(final)
	t.CreatedAt = fromMillis(created)
	t.RevealedAt = timePtr(revealed)
	return t, nil
}

// ListTopics returns the room backlog in display order.
func (s *Store) ListTopics(ctx context.Context, roomID string) ([]domain.Topic, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+topicColumns+` FROM topics WHERE room_id = ? ORDER BY position ASC, created_at ASC`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Topic, 0, 16)
	for rows.Next() {
		var (
			t        domain.Topic
			extKey   sql.NullString
			extURL   sql.NullString
			status   string
			final    sql.NullString
			created  int64
			revealed sql.NullInt64
		)
		if err := rows.Scan(&t.ID, &t.RoomID, &t.Title, &t.Description, &extKey, &extURL,
			&t.Position, &status, &final, &created, &revealed); err != nil {
			return nil, err
		}
		t.ExternalKey = stringPtr(extKey)
		t.ExternalURL = stringPtr(extURL)
		t.Status = domain.TopicStatus(status)
		t.FinalEstimate = stringPtr(final)
		t.CreatedAt = fromMillis(created)
		t.RevealedAt = timePtr(revealed)
		out = append(out, t)
	}
	return out, rows.Err()
}

// NextTopicPosition returns the position to use for a newly appended topic.
func (s *Store) NextTopicPosition(ctx context.Context, roomID string) (int, error) {
	var pos sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(position) FROM topics WHERE room_id = ?`, roomID).Scan(&pos)
	if err != nil {
		return 0, err
	}
	if !pos.Valid {
		return 0, nil
	}
	return int(pos.Int64) + 1, nil
}

// UpdateTopicDetails edits the human-readable fields.
func (s *Store) UpdateTopicDetails(ctx context.Context, roomID, topicID, title, description string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE topics SET title = ?, description = ? WHERE id = ? AND room_id = ?`,
		title, description, topicID, roomID)
	return affected(res, err)
}

// UpdateTopicStatus moves a topic through its lifecycle.
func (s *Store) UpdateTopicStatus(ctx context.Context, topicID string, status domain.TopicStatus, revealedAt *time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE topics SET status = ?, revealed_at = ? WHERE id = ?`,
		string(status), nullTime(revealedAt), topicID)
	return affected(res, err)
}

// FinalizeTopic stores the agreed estimate.
func (s *Store) FinalizeTopic(ctx context.Context, topicID string, estimate *string) error {
	status := domain.StatusEstimated
	if estimate == nil {
		status = domain.StatusRevealed
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE topics SET final_estimate = ?, status = ? WHERE id = ?`,
		nullString(estimate), string(status), topicID)
	return affected(res, err)
}

// ResetTopic clears the votes, the reveal timestamp and the agreed estimate in
// one transaction so the team can start the round over.
func (s *Store) ResetTopic(ctx context.Context, topicID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM votes WHERE topic_id = ?`, topicID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE topics SET status = 'pending', revealed_at = NULL, final_estimate = NULL WHERE id = ?`, topicID)
	if err := affected(res, err); err != nil {
		return err
	}
	return tx.Commit()
}

// ReorderTopics rewrites positions in a single transaction.
func (s *Store) ReorderTopics(ctx context.Context, roomID string, orderedIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `UPDATE topics SET position = ? WHERE id = ? AND room_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, id := range orderedIDs {
		if _, err := stmt.ExecContext(ctx, i, id, roomID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteTopic removes a topic and its votes.
func (s *Store) DeleteTopic(ctx context.Context, roomID, topicID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM topics WHERE id = ? AND room_id = ?`, topicID, roomID)
	return affected(res, err)
}

// ExistingExternalKeys reports which Jira keys are already imported, so imports are idempotent.
func (s *Store) ExistingExternalKeys(ctx context.Context, roomID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT external_key FROM topics WHERE room_id = ? AND external_key IS NOT NULL`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
