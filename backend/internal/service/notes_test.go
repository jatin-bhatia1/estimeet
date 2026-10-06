package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
	"github.com/jatin-bhatia1/estimeet/backend/internal/service"
)

func notesOn(t *testing.T, svc *service.Service, sess service.Session, topicID string) []service.NoteView {
	t.Helper()
	state, err := svc.State(context.Background(), sess.Room.ID, sess.Participant.ID)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	for _, topic := range state.Topics {
		if topic.ID == topicID {
			return topic.Notes
		}
	}
	t.Fatalf("topic %s not in state", topicID)
	return nil
}

// A suggestion like "this is a 13" would steer the vote the way a visible card
// would, so notes follow the cards: private until the reveal.
func TestNotesStayPrivateUntilTheCardsAreUp(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeSync)
	topics := addTopics(t, svc, host, "Login")
	host = reload(t, svc, host)
	player := join(t, svc, host.Room.Code, "Linus", false)
	player = reload(t, svc, player)

	if err := svc.AddNote(ctx, player, topics[0].ID, domain.NoteConcern, "  What about SSO?  "); err != nil {
		t.Fatalf("add note: %v", err)
	}

	if got := notesOn(t, svc, host, topics[0].ID); len(got) != 0 {
		t.Fatalf("host sees %d notes before the reveal, want 0", len(got))
	}
	mine := notesOn(t, svc, player, topics[0].ID)
	if len(mine) != 1 || !mine[0].Mine || mine[0].Body != "What about SSO?" {
		t.Fatalf("author should see their own trimmed note, got %+v", mine)
	}

	if err := svc.RevealTopic(ctx, host, topics[0].ID); err != nil {
		t.Fatalf("reveal: %v", err)
	}
	got := notesOn(t, svc, host, topics[0].ID)
	if len(got) != 1 || got[0].Mine || got[0].ParticipantName != "Linus" || got[0].Kind != domain.NoteConcern {
		t.Fatalf("host should see Linus's concern after the reveal, got %+v", got)
	}
}

func TestNotesAreValidatedAndCapped(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeSync)
	topics := addTopics(t, svc, host, "Login")
	host = reload(t, svc, host)

	if err := svc.AddNote(ctx, host, topics[0].ID, domain.NoteKind("rant"), "hi"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown kind: err = %v, want ErrInvalid", err)
	}
	if err := svc.AddNote(ctx, host, topics[0].ID, domain.NoteQuestion, "   "); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("blank body: err = %v, want ErrInvalid", err)
	}

	if err := svc.AddNote(ctx, host, topics[0].ID, domain.NoteQuestion, strings.Repeat("x", service.MaxNoteLen+50)); err != nil {
		t.Fatalf("long note should be trimmed, not rejected: %v", err)
	}
	if got := notesOn(t, svc, host, topics[0].ID); len(got) != 1 || len([]rune(got[0].Body)) != service.MaxNoteLen {
		t.Fatalf("long note was not trimmed to %d characters", service.MaxNoteLen)
	}

	for i := 1; i < service.MaxNotesPerPlayer; i++ {
		if err := svc.AddNote(ctx, host, topics[0].ID, domain.NoteSuggestion, "more"); err != nil {
			t.Fatalf("note %d: %v", i+1, err)
		}
	}
	if err := svc.AddNote(ctx, host, topics[0].ID, domain.NoteSuggestion, "one too many"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("over the cap: err = %v, want ErrConflict", err)
	}
}

func TestOnlyTheAuthorOrTheHostRemovesANote(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeSync)
	topics := addTopics(t, svc, host, "Login")
	host = reload(t, svc, host)
	linus := join(t, svc, host.Room.Code, "Linus", false)
	grace := join(t, svc, host.Room.Code, "Grace", false)

	if err := svc.AddNote(ctx, linus, topics[0].ID, domain.NoteQuestion, "Which browsers?"); err != nil {
		t.Fatalf("add note: %v", err)
	}
	noteID := notesOn(t, svc, linus, topics[0].ID)[0].ID

	if err := svc.DeleteNote(ctx, grace, topics[0].ID, noteID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("another player: err = %v, want ErrForbidden", err)
	}
	if err := svc.DeleteNote(ctx, host, topics[0].ID, noteID); err != nil {
		t.Fatalf("host should be able to clear a note: %v", err)
	}
	if got := notesOn(t, svc, linus, topics[0].ID); len(got) != 0 {
		t.Fatalf("note survived deletion: %+v", got)
	}
}
