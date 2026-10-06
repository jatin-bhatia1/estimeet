package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

const (
	MaxNoteLen         = 500
	MaxNotesPerPlayer  = 10
	notesPublishedType = "notes.changed"
)

// AddNote records a question, concern or suggestion on a topic. Observers may
// leave one too: they do not vote, but they are often the ones asking.
func (s *Service) AddNote(ctx context.Context, sess Session, topicID string, kind domain.NoteKind, body string) error {
	if sess.Room.ClosedAt != nil {
		return fmt.Errorf("%w: this session is closed", domain.ErrConflict)
	}
	if !kind.Valid() {
		return fmt.Errorf("%w: a note is a question, a concern or a suggestion", domain.ErrInvalid)
	}
	body = clean(body, MaxNoteLen)
	if body == "" {
		return fmt.Errorf("%w: write something first", domain.ErrInvalid)
	}

	topic, err := s.store.TopicByID(ctx, sess.Room.ID, topicID)
	if err != nil {
		return err
	}
	count, err := s.store.CountNotesBy(ctx, topic.ID, sess.Participant.ID)
	if err != nil {
		return err
	}
	if count >= MaxNotesPerPlayer {
		return fmt.Errorf("%w: at most %d notes per topic each", domain.ErrConflict, MaxNotesPerPlayer)
	}

	if err := s.store.CreateNote(ctx, domain.TopicNote{
		ID:            uuid.NewString(),
		TopicID:       topic.ID,
		ParticipantID: sess.Participant.ID,
		Kind:          kind,
		Body:          body,
		CreatedAt:     s.now(),
	}); err != nil {
		return err
	}
	s.publish(sess.Room.ID, notesPublishedType, map[string]string{"topicId": topic.ID})
	return nil
}

// DeleteNote removes a note. Its author may take it back, and the host may
// clear anything that does not belong on the board.
func (s *Service) DeleteNote(ctx context.Context, sess Session, topicID, noteID string) error {
	topic, err := s.store.TopicByID(ctx, sess.Room.ID, topicID)
	if err != nil {
		return err
	}
	note, err := s.store.NoteByID(ctx, topic.ID, noteID)
	if err != nil {
		return err
	}
	if note.ParticipantID != sess.Participant.ID && !sess.Participant.IsHost {
		return fmt.Errorf("%w: only the author or the host can remove a note", domain.ErrForbidden)
	}
	if err := s.store.DeleteNote(ctx, topic.ID, note.ID); err != nil {
		return err
	}
	s.publish(sess.Room.ID, notesPublishedType, map[string]string{"topicId": topic.ID})
	return nil
}
