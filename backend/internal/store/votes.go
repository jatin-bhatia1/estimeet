package store

import (
	"context"
	"time"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

// CastVote inserts or replaces a participant's card for a topic.
func (s *Store) CastVote(ctx context.Context, v domain.Vote) error {
	_, err := s.exec(ctx,
		`INSERT INTO votes (topic_id, participant_id, value, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (topic_id, participant_id) DO UPDATE SET value = excluded.value, created_at = excluded.created_at`,
		v.TopicID, v.ParticipantID, v.Value, toMillis(v.CreatedAt))
	return err
}

// ClearVote removes a single participant's card.
func (s *Store) ClearVote(ctx context.Context, topicID, participantID string) error {
	_, err := s.exec(ctx,
		`DELETE FROM votes WHERE topic_id = ? AND participant_id = ?`, topicID, participantID)
	return err
}

// ClearTopicVotes wipes a round so the team can re-vote.
func (s *Store) ClearTopicVotes(ctx context.Context, topicID string) error {
	_, err := s.exec(ctx, `DELETE FROM votes WHERE topic_id = ?`, topicID)
	return err
}

// ListVotesForTopic returns every card played on a topic.
func (s *Store) ListVotesForTopic(ctx context.Context, topicID string) ([]domain.Vote, error) {
	rows, err := s.query(ctx,
		`SELECT topic_id, participant_id, value, created_at FROM votes WHERE topic_id = ? ORDER BY created_at ASC`, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVotes(rows)
}

// ListVotesForRoom returns every card in the room, keyed by topic, so the whole
// board can be rendered with one query instead of N.
func (s *Store) ListVotesForRoom(ctx context.Context, roomID string) (map[string][]domain.Vote, error) {
	rows, err := s.query(ctx,
		`SELECT v.topic_id, v.participant_id, v.value, v.created_at
		 FROM votes v JOIN topics t ON t.id = v.topic_id
		 WHERE t.room_id = ?
		 ORDER BY v.created_at ASC`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	votes, err := scanVotes(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]domain.Vote, len(votes))
	for _, v := range votes {
		out[v.TopicID] = append(out[v.TopicID], v)
	}
	return out, nil
}

type rowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanVotes(rows rowScanner) ([]domain.Vote, error) {
	out := make([]domain.Vote, 0, 8)
	for rows.Next() {
		var (
			v       domain.Vote
			created int64
		)
		if err := rows.Scan(&v.TopicID, &v.ParticipantID, &v.Value, &created); err != nil {
			return nil, err
		}
		v.CreatedAt = time.UnixMilli(created).UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}
