package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

const roomColumns = `id, code, name, mode, deck, current_topic_id, auto_reveal, created_at, closed_at, expected_size, expected_names`

// CreateRoom inserts a new room.
func (s *Store) CreateRoom(ctx context.Context, r domain.Room) error {
	deck, err := json.Marshal(r.Deck)
	if err != nil {
		return fmt.Errorf("marshal deck: %w", err)
	}
	names, err := marshalNames(r.ExpectedNames)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO rooms (`+roomColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Code, r.Name, string(r.Mode), string(deck),
		nullString(r.CurrentTopicID), r.AutoReveal, toMillis(r.CreatedAt), nullTime(r.ClosedAt),
		r.ExpectedSize, names,
	)
	return err
}

// RoomByCode looks a room up by its shareable code.
func (s *Store) RoomByCode(ctx context.Context, code string) (domain.Room, error) {
	return s.scanRoom(s.db.QueryRowContext(ctx,
		`SELECT `+roomColumns+` FROM rooms WHERE code = ?`, domain.NormalizeCode(code)))
}

// RoomByID looks a room up by its internal identifier.
func (s *Store) RoomByID(ctx context.Context, id string) (domain.Room, error) {
	return s.scanRoom(s.db.QueryRowContext(ctx,
		`SELECT `+roomColumns+` FROM rooms WHERE id = ?`, id))
}

func (s *Store) scanRoom(row *sql.Row) (domain.Room, error) {
	var (
		r         domain.Room
		mode      string
		deckJSON  string
		current   sql.NullString
		created   int64
		closed    sql.NullInt64
		namesJSON string
	)
	err := row.Scan(&r.ID, &r.Code, &r.Name, &mode, &deckJSON, &current, &r.AutoReveal, &created, &closed,
		&r.ExpectedSize, &namesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Room{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Room{}, err
	}
	if err := json.Unmarshal([]byte(deckJSON), &r.Deck); err != nil {
		return domain.Room{}, fmt.Errorf("unmarshal deck: %w", err)
	}
	if err := json.Unmarshal([]byte(namesJSON), &r.ExpectedNames); err != nil {
		return domain.Room{}, fmt.Errorf("unmarshal expected names: %w", err)
	}
	r.Mode = domain.Mode(mode)
	r.CurrentTopicID = stringPtr(current)
	r.CreatedAt = fromMillis(created)
	r.ClosedAt = timePtr(closed)
	return r, nil
}

// SetRoster stores how many people are expected and the names the host listed.
func (s *Store) SetRoster(ctx context.Context, roomID string, size int, names []string) error {
	encoded, err := marshalNames(names)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE rooms SET expected_size = ?, expected_names = ? WHERE id = ?`, size, encoded, roomID)
	return err
}

// PurgeStaleRooms deletes rooms nobody has touched since the cutoff. Everything
// belonging to them goes with them through ON DELETE CASCADE, so a purged
// session leaves no participants, topics or votes behind.
func (s *Store) PurgeStaleRooms(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM rooms
		 WHERE MAX(
		           created_at,
		           COALESCE((SELECT MAX(last_seen_at) FROM participants WHERE room_id = rooms.id), 0),
		           COALESCE((SELECT MAX(created_at) FROM topics WHERE room_id = rooms.id), 0)
		       ) <= ?`, toMillis(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// marshalNames keeps the stored value a JSON array even when there are no names.
func marshalNames(names []string) (string, error) {
	if names == nil {
		names = []string{}
	}
	encoded, err := json.Marshal(names)
	if err != nil {
		return "", fmt.Errorf("marshal expected names: %w", err)
	}
	return string(encoded), nil
}

// SetCurrentTopic points a synchronous room at the topic being estimated.
func (s *Store) SetCurrentTopic(ctx context.Context, roomID string, topicID *string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE rooms SET current_topic_id = ? WHERE id = ?`, nullString(topicID), roomID)
	return err
}

// UpdateRoomSettings changes the mutable room options.
func (s *Store) UpdateRoomSettings(ctx context.Context, roomID, name string, autoReveal bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE rooms SET name = ?, auto_reveal = ? WHERE id = ?`, name, autoReveal, roomID)
	return err
}

// ---------------------------------------------------------------- participants

const participantColumns = `id, room_id, name, is_host, is_observer, joined_at, last_seen_at`

// CreateParticipant stores a participant together with the SHA-256 hash of their bearer token.
func (s *Store) CreateParticipant(ctx context.Context, p domain.Participant, tokenHash string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO participants (id, room_id, name, token_hash, is_host, is_observer, joined_at, last_seen_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.RoomID, p.Name, tokenHash, p.IsHost, p.IsObserver, toMillis(p.JoinedAt), toMillis(p.LastSeenAt),
	)
	return err
}

// ParticipantByTokenHash resolves a bearer token to its participant.
func (s *Store) ParticipantByTokenHash(ctx context.Context, tokenHash string) (domain.Participant, error) {
	return s.scanParticipant(s.db.QueryRowContext(ctx,
		`SELECT `+participantColumns+` FROM participants WHERE token_hash = ?`, tokenHash))
}

// ParticipantByID fetches one participant.
func (s *Store) ParticipantByID(ctx context.Context, id string) (domain.Participant, error) {
	return s.scanParticipant(s.db.QueryRowContext(ctx,
		`SELECT `+participantColumns+` FROM participants WHERE id = ?`, id))
}

// ParticipantByRoomAndName finds an existing participant so a reconnecting user
// can reclaim their seat instead of creating a duplicate.
func (s *Store) ParticipantByRoomAndName(ctx context.Context, roomID, name string) (domain.Participant, error) {
	return s.scanParticipant(s.db.QueryRowContext(ctx,
		`SELECT `+participantColumns+` FROM participants WHERE room_id = ? AND name = ? COLLATE NOCASE`, roomID, name))
}

func (s *Store) scanParticipant(row *sql.Row) (domain.Participant, error) {
	var (
		p        domain.Participant
		joined   int64
		lastSeen int64
	)
	err := row.Scan(&p.ID, &p.RoomID, &p.Name, &p.IsHost, &p.IsObserver, &joined, &lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Participant{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Participant{}, err
	}
	p.JoinedAt = fromMillis(joined)
	p.LastSeenAt = fromMillis(lastSeen)
	return p, nil
}

// ListParticipants returns every participant of a room in join order.
func (s *Store) ListParticipants(ctx context.Context, roomID string) ([]domain.Participant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+participantColumns+` FROM participants WHERE room_id = ? ORDER BY joined_at ASC`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Participant, 0, 8)
	for rows.Next() {
		var (
			p        domain.Participant
			joined   int64
			lastSeen int64
		)
		if err := rows.Scan(&p.ID, &p.RoomID, &p.Name, &p.IsHost, &p.IsObserver, &joined, &lastSeen); err != nil {
			return nil, err
		}
		p.JoinedAt = fromMillis(joined)
		p.LastSeenAt = fromMillis(lastSeen)
		out = append(out, p)
	}
	return out, rows.Err()
}

// TouchParticipant records liveness for presence tracking.
func (s *Store) TouchParticipant(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants SET last_seen_at = ? WHERE id = ?`, toMillis(at), id)
	return err
}

// UpdateParticipant changes the display name and observer flag.
func (s *Store) UpdateParticipant(ctx context.Context, id, name string, isObserver bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants SET name = ?, is_observer = ? WHERE id = ?`, name, isObserver, id)
	return err
}

// DeleteParticipant removes a participant and, by cascade, their votes.
func (s *Store) DeleteParticipant(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM participants WHERE id = ?`, id)
	return err
}
