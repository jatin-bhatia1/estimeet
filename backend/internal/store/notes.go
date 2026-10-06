package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

const noteColumns = `id, topic_id, participant_id, kind, body, created_at`

// CreateNote stores one discussion note.
func (s *Store) CreateNote(ctx context.Context, n domain.TopicNote) error {
	_, err := s.exec(ctx,
		`INSERT INTO topic_notes (`+noteColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		n.ID, n.TopicID, n.ParticipantID, string(n.Kind), n.Body, toMillis(n.CreatedAt))
	return err
}

// NoteByID fetches a note scoped to its topic.
func (s *Store) NoteByID(ctx context.Context, topicID, noteID string) (domain.TopicNote, error) {
	var (
		n       domain.TopicNote
		kind    string
		created int64
	)
	err := s.queryRow(ctx,
		`SELECT `+noteColumns+` FROM topic_notes WHERE id = ? AND topic_id = ?`, noteID, topicID).
		Scan(&n.ID, &n.TopicID, &n.ParticipantID, &kind, &n.Body, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TopicNote{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TopicNote{}, err
	}
	n.Kind = domain.NoteKind(kind)
	n.CreatedAt = fromMillis(created)
	return n, nil
}

// DeleteNote removes a note.
func (s *Store) DeleteNote(ctx context.Context, topicID, noteID string) error {
	res, err := s.exec(ctx, `DELETE FROM topic_notes WHERE id = ? AND topic_id = ?`, noteID, topicID)
	return affected(res, err)
}

// CountNotesBy reports how many notes a participant has left on a topic.
func (s *Store) CountNotesBy(ctx context.Context, topicID, participantID string) (int, error) {
	var n int
	err := s.queryRow(ctx,
		`SELECT COUNT(*) FROM topic_notes WHERE topic_id = ? AND participant_id = ?`, topicID, participantID).Scan(&n)
	return n, err
}

// ListNotesForRoom returns every note in the room keyed by topic, so the board
// is built with one query instead of one per topic.
func (s *Store) ListNotesForRoom(ctx context.Context, roomID string) (map[string][]domain.TopicNote, error) {
	rows, err := s.query(ctx,
		`SELECT n.id, n.topic_id, n.participant_id, n.kind, n.body, n.created_at
		 FROM topic_notes n JOIN topics t ON t.id = n.topic_id
		 WHERE t.room_id = ?
		 ORDER BY n.created_at ASC`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]domain.TopicNote)
	for rows.Next() {
		var (
			n       domain.TopicNote
			kind    string
			created int64
		)
		if err := rows.Scan(&n.ID, &n.TopicID, &n.ParticipantID, &kind, &n.Body, &created); err != nil {
			return nil, err
		}
		n.Kind = domain.NoteKind(kind)
		n.CreatedAt = fromMillis(created)
		out[n.TopicID] = append(out[n.TopicID], n)
	}
	return out, rows.Err()
}
