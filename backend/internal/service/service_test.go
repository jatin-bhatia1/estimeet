package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
	"github.com/jatin-bhatia1/estimeet/backend/internal/hub"
	"github.com/jatin-bhatia1/estimeet/backend/internal/secretbox"
	"github.com/jatin-bhatia1/estimeet/backend/internal/service"
	"github.com/jatin-bhatia1/estimeet/backend/internal/store"
)

func newService(t *testing.T) *service.Service {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	box, err := secretbox.New("test-secret-value-at-least-16")
	if err != nil {
		t.Fatalf("secretbox: %v", err)
	}
	return service.New(st, hub.New(), box, nil)
}

// newRoom opens a room and returns the host session.
func newRoom(t *testing.T, svc *service.Service, mode domain.Mode) service.Session {
	t.Helper()
	created, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{
		Name:     "Sprint 42",
		Mode:     mode,
		HostName: "Ada",
	})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	sess, err := svc.Authenticate(context.Background(), created.Token)
	if err != nil {
		t.Fatalf("authenticate host: %v", err)
	}
	return sess
}

func join(t *testing.T, svc *service.Service, code, name string, observer bool) service.Session {
	t.Helper()
	joined, err := svc.JoinRoom(context.Background(), code, name, observer)
	if err != nil {
		t.Fatalf("join as %s: %v", name, err)
	}
	sess, err := svc.Authenticate(context.Background(), joined.Token)
	if err != nil {
		t.Fatalf("authenticate %s: %v", name, err)
	}
	return sess
}

func addTopics(t *testing.T, svc *service.Service, host service.Session, titles ...string) []domain.Topic {
	t.Helper()
	inputs := make([]service.TopicInput, 0, len(titles))
	for _, title := range titles {
		inputs = append(inputs, service.TopicInput{Title: title})
	}
	topics, err := svc.AddTopics(context.Background(), host, inputs)
	if err != nil {
		t.Fatalf("add topics: %v", err)
	}
	return topics
}

// reload refreshes a session so it sees committed room changes.
func reload(t *testing.T, svc *service.Service, sess service.Session) service.Session {
	t.Helper()
	state, err := svc.State(context.Background(), sess.Room.ID, sess.Participant.ID)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	sess.Room.CurrentTopicID = state.Room.CurrentTopicID
	return sess
}

func TestCreateRoomRejectsUnknownMode(t *testing.T) {
	svc := newService(t)
	_, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{
		Mode:     domain.Mode("hybrid"),
		HostName: "Ada",
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestJoinRejectsDuplicateName(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)

	join(t, svc, host.Room.Code, "Grace", false)
	_, err := svc.JoinRoom(context.Background(), host.Room.Code, "grace", false)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

// In a synchronous room the host drives the session, so a player may only vote
// on the topic that is currently on screen.
func TestSyncRoomRejectsVotesOnOtherTopics(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeSync)
	topics := addTopics(t, svc, host, "Login", "Signup")
	host = reload(t, svc, host)

	player := join(t, svc, host.Room.Code, "Linus", false)
	player = reload(t, svc, player)

	if err := svc.CastVote(context.Background(), player, topics[0].ID, "5"); err != nil {
		t.Fatalf("vote on current topic: %v", err)
	}
	err := svc.CastVote(context.Background(), player, topics[1].ID, "8")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden for a non-current topic", err)
	}
}

// In an asynchronous room every topic is open at the same time.
func TestAsyncRoomAcceptsVotesOnAnyTopic(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)
	topics := addTopics(t, svc, host, "Login", "Signup", "Logout")
	player := join(t, svc, host.Room.Code, "Linus", false)

	for _, topic := range topics {
		if err := svc.CastVote(context.Background(), player, topic.ID, "3"); err != nil {
			t.Fatalf("vote on %s: %v", topic.Title, err)
		}
	}

	state, err := svc.State(context.Background(), player.Room.ID, player.Participant.ID)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.Summary.MyRemaining != 0 {
		t.Fatalf("myRemaining = %d, want 0", state.Summary.MyRemaining)
	}
}

// An asynchronous room reveals once every member has played, regardless of who
// happens to be connected at that moment.
func TestAsyncAutoRevealWhenEveryoneVoted(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)
	topics := addTopics(t, svc, host, "Login")
	player := join(t, svc, host.Room.Code, "Linus", false)

	if err := svc.CastVote(context.Background(), host, topics[0].ID, "5"); err != nil {
		t.Fatalf("host vote: %v", err)
	}

	state, _ := svc.State(context.Background(), host.Room.ID, host.Participant.ID)
	if state.Topics[0].Revealed {
		t.Fatalf("topic revealed before everybody voted")
	}
	if len(state.Topics[0].Votes) != 0 {
		t.Fatalf("face-down cards must not be exposed, got %d", len(state.Topics[0].Votes))
	}

	if err := svc.CastVote(context.Background(), player, topics[0].ID, "8"); err != nil {
		t.Fatalf("player vote: %v", err)
	}

	state, _ = svc.State(context.Background(), host.Room.ID, host.Participant.ID)
	if !state.Topics[0].Revealed {
		t.Fatalf("topic should auto-reveal once everybody voted")
	}
	if len(state.Topics[0].Votes) != 2 {
		t.Fatalf("revealed votes = %d, want 2", len(state.Topics[0].Votes))
	}
	if state.Topics[0].Stats == nil || *state.Topics[0].Stats.Average != 6.5 {
		t.Fatalf("unexpected stats: %+v", state.Topics[0].Stats)
	}
}

func TestObserversDoNotVoteAndDoNotBlockReveal(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)
	topics := addTopics(t, svc, host, "Login")
	observer := join(t, svc, host.Room.Code, "Watcher", true)

	err := svc.CastVote(context.Background(), observer, topics[0].ID, "5")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden for an observer", err)
	}

	if err := svc.CastVote(context.Background(), host, topics[0].ID, "5"); err != nil {
		t.Fatalf("host vote: %v", err)
	}
	state, _ := svc.State(context.Background(), host.Room.ID, host.Participant.ID)
	if !state.Topics[0].Revealed {
		t.Fatalf("an observer must not hold the reveal back")
	}
}

func TestVotesAreHiddenUntilRevealed(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)
	topics := addTopics(t, svc, host, "Login")
	player := join(t, svc, host.Room.Code, "Linus", false)
	_ = join(t, svc, host.Room.Code, "Margaret", false) // keeps the round open

	if err := svc.CastVote(context.Background(), player, topics[0].ID, "13"); err != nil {
		t.Fatalf("vote: %v", err)
	}

	state, _ := svc.State(context.Background(), host.Room.ID, host.Participant.ID)
	topic := state.Topics[0]
	if len(topic.Votes) != 0 || topic.Stats != nil {
		t.Fatalf("card values leaked before the reveal")
	}
	if len(topic.VotedBy) != 1 || topic.VotedBy[0] != player.Participant.ID {
		t.Fatalf("votedBy = %v, want the voter to be listed", topic.VotedBy)
	}
	if topic.MyVote != nil {
		t.Fatalf("the host has not voted, myVote must be nil")
	}
}

func TestOnlyHostCanRevealResetAndEstimate(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)
	topics := addTopics(t, svc, host, "Login")
	player := join(t, svc, host.Room.Code, "Linus", false)

	if err := svc.RevealTopic(context.Background(), player, topics[0].ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("reveal err = %v, want ErrForbidden", err)
	}
	if err := svc.ResetTopic(context.Background(), player, topics[0].ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("reset err = %v, want ErrForbidden", err)
	}
	if err := svc.FinalizeTopic(context.Background(), player, topics[0].ID, "5"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("finalize err = %v, want ErrForbidden", err)
	}
	if _, err := svc.AddTopics(context.Background(), player, []service.TopicInput{{Title: "Sneaky"}}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("addTopics err = %v, want ErrForbidden", err)
	}
}

func TestFinalizeRequiresRevealAndNumericCard(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)
	topics := addTopics(t, svc, host, "Login")

	if err := svc.FinalizeTopic(context.Background(), host, topics[0].ID, "5"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict before the reveal", err)
	}
	if err := svc.RevealTopic(context.Background(), host, topics[0].ID); err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if err := svc.FinalizeTopic(context.Background(), host, topics[0].ID, domain.CardUnknown); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid for a non-numeric card", err)
	}
	if err := svc.FinalizeTopic(context.Background(), host, topics[0].ID, "13"); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	state, _ := svc.State(context.Background(), host.Room.ID, host.Participant.ID)
	if state.Summary.TotalPoints == nil || *state.Summary.TotalPoints != 13 {
		t.Fatalf("totalPoints = %v, want 13", state.Summary.TotalPoints)
	}
	if state.Topics[0].Status != domain.StatusEstimated {
		t.Fatalf("status = %v, want estimated", state.Topics[0].Status)
	}
}

func TestVotingRejectsCardsOutsideTheDeck(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)
	topics := addTopics(t, svc, host, "Login")

	if err := svc.CastVote(context.Background(), host, topics[0].ID, "7"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid (7 is not Fibonacci)", err)
	}
}

func TestResetTopicClearsTheRound(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)
	topics := addTopics(t, svc, host, "Login")

	if err := svc.CastVote(context.Background(), host, topics[0].ID, "5"); err != nil {
		t.Fatalf("vote: %v", err)
	}
	if err := svc.ResetTopic(context.Background(), host, topics[0].ID); err != nil {
		t.Fatalf("reset: %v", err)
	}

	state, _ := svc.State(context.Background(), host.Room.ID, host.Participant.ID)
	topic := state.Topics[0]
	if topic.Status != domain.StatusPending || topic.MyVote != nil || len(topic.VotedBy) != 0 {
		t.Fatalf("topic not reset: %+v", topic)
	}
}

func TestSyncRoomAdvancesThroughTheBacklog(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeSync)
	topics := addTopics(t, svc, host, "One", "Two")
	host = reload(t, svc, host)

	if host.Room.CurrentTopicID == nil || *host.Room.CurrentTopicID != topics[0].ID {
		t.Fatalf("a sync room should start on the first topic")
	}
	if err := svc.AdvanceCurrentTopic(context.Background(), host, "next"); err != nil {
		t.Fatalf("advance: %v", err)
	}
	host = reload(t, svc, host)
	if *host.Room.CurrentTopicID != topics[1].ID {
		t.Fatalf("current topic did not advance")
	}
	if err := svc.AdvanceCurrentTopic(context.Background(), host, "next"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict at the end of the backlog", err)
	}
}

func TestAsyncRoomHasNoCurrentTopic(t *testing.T) {
	svc := newService(t)
	host := newRoom(t, svc, domain.ModeAsync)
	topics := addTopics(t, svc, host, "One")

	err := svc.SetCurrentTopic(context.Background(), host, topics[0].ID)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict in async mode", err)
	}
}

func TestAuthenticateRejectsUnknownToken(t *testing.T) {
	svc := newService(t)
	newRoom(t, svc, domain.ModeSync)

	if _, err := svc.Authenticate(context.Background(), "not-a-real-token"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if _, err := svc.Authenticate(context.Background(), ""); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestParticipantsCannotReachAnotherRoom(t *testing.T) {
	svc := newService(t)
	roomA := newRoom(t, svc, domain.ModeAsync)
	roomB := newRoom(t, svc, domain.ModeAsync)
	topicsB := addTopics(t, svc, roomB, "Secret")

	err := svc.CastVote(context.Background(), roomA, topicsB[0].ID, "5")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound across rooms", err)
	}
	if _, err := svc.State(context.Background(), roomB.Room.ID, roomA.Participant.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}
