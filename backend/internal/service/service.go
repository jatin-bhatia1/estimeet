// Package service holds the application rules: who may do what, how a vote
// affects a topic, and how synchronous and asynchronous rooms differ.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
	"github.com/jatin-bhatia1/estimeet/backend/internal/hub"
	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
	"github.com/jatin-bhatia1/estimeet/backend/internal/secretbox"
	"github.com/jatin-bhatia1/estimeet/backend/internal/store"
)

// Field limits guard the database and the UI against abusive payloads.
const (
	MaxRoomNameLen     = 80
	MaxDisplayNameLen  = 40
	MaxTopicTitleLen   = 200
	MaxTopicDescLen    = 4000
	MaxTopicsPerRoom   = 500
	MaxPlayersPerRoom  = 100
	MaxTopicsPerImport = 100
)

// Service is the application façade used by the HTTP layer.
type Service struct {
	store *store.Store
	hub   *hub.Hub
	box   *secretbox.Box
	jira  *jira.Client
	now   func() time.Time
}

// New wires the service. jiraClient may be nil when Jira is not configured.
func New(st *store.Store, h *hub.Hub, box *secretbox.Box, jiraClient *jira.Client) *Service {
	return &Service{store: st, hub: h, box: box, jira: jiraClient, now: func() time.Time { return time.Now().UTC() }}
}

// Hub exposes the event hub to the WebSocket handler.
func (s *Service) Hub() *hub.Hub { return s.hub }

// ------------------------------------------------------------------ auth

// Session pairs an authenticated participant with their room.
type Session struct {
	Room        domain.Room
	Participant domain.Participant
}

// Authenticate resolves a bearer token into a session.
func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	if strings.TrimSpace(token) == "" {
		return Session{}, domain.ErrForbidden
	}
	p, err := s.store.ParticipantByTokenHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Session{}, domain.ErrForbidden
		}
		return Session{}, err
	}
	room, err := s.store.RoomByID(ctx, p.RoomID)
	if err != nil {
		return Session{}, err
	}
	return Session{Room: room, Participant: p}, nil
}

func newToken() (raw, hashed string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ------------------------------------------------------------------ rooms

// CreateRoomInput describes a new estimation session.
type CreateRoomInput struct {
	Name       string
	Mode       domain.Mode
	HostName   string
	AutoReveal *bool
}

// CreatedRoom is returned once, and carries the new participant's bearer token.
type CreatedRoom struct {
	Room  domain.Room
	Host  domain.Participant
	Token string
}

// CreateRoom opens a room and seats its host. The creator is the only
// participant who ever holds that role: it is never transferred or shared.
func (s *Service) CreateRoom(ctx context.Context, in CreateRoomInput) (CreatedRoom, error) {
	name := clean(in.Name, MaxRoomNameLen)
	hostName := clean(in.HostName, MaxDisplayNameLen)
	if name == "" {
		name = "Estimation session"
	}
	if hostName == "" {
		return CreatedRoom{}, fmt.Errorf("%w: your display name is required", domain.ErrInvalid)
	}
	if !in.Mode.Valid() {
		return CreatedRoom{}, fmt.Errorf(`%w: mode must be "sync" or "async"`, domain.ErrInvalid)
	}

	autoReveal := true
	if in.AutoReveal != nil {
		autoReveal = *in.AutoReveal
	}

	now := s.now()
	room := domain.Room{
		ID:         uuid.NewString(),
		Name:       name,
		Mode:       in.Mode,
		Deck:       domain.DefaultDeck(),
		AutoReveal: autoReveal,
		CreatedAt:  now,
	}

	// Room codes are random; retry on the (very unlikely) collision.
	var created bool
	for attempt := 0; attempt < 5 && !created; attempt++ {
		code, err := domain.NewRoomCode()
		if err != nil {
			return CreatedRoom{}, err
		}
		room.Code = code
		if err := s.store.CreateRoom(ctx, room); err != nil {
			if isUniqueViolation(err) {
				continue
			}
			return CreatedRoom{}, err
		}
		created = true
	}
	if !created {
		return CreatedRoom{}, fmt.Errorf("could not allocate a unique room code")
	}

	host := domain.Participant{
		ID:         uuid.NewString(),
		RoomID:     room.ID,
		Name:       hostName,
		IsHost:     true,
		JoinedAt:   now,
		LastSeenAt: now,
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return CreatedRoom{}, err
	}
	if err := s.store.CreateParticipant(ctx, host, tokenHash); err != nil {
		return CreatedRoom{}, err
	}
	return CreatedRoom{Room: room, Host: host, Token: token}, nil
}

// RoomSummary is the unauthenticated preview shown on the join screen.
type RoomSummary struct {
	Code         string      `json:"code"`
	Name         string      `json:"name"`
	Mode         domain.Mode `json:"mode"`
	Participants int         `json:"participants"`
	Topics       int         `json:"topics"`
	Closed       bool        `json:"closed"`
}

// RoomSummaryByCode fetches the public preview of a room.
func (s *Service) RoomSummaryByCode(ctx context.Context, code string) (RoomSummary, error) {
	room, err := s.store.RoomByCode(ctx, code)
	if err != nil {
		return RoomSummary{}, err
	}
	participants, err := s.store.ListParticipants(ctx, room.ID)
	if err != nil {
		return RoomSummary{}, err
	}
	topics, err := s.store.ListTopics(ctx, room.ID)
	if err != nil {
		return RoomSummary{}, err
	}
	return RoomSummary{
		Code:         room.Code,
		Name:         room.Name,
		Mode:         room.Mode,
		Participants: len(participants),
		Topics:       len(topics),
		Closed:       room.ClosedAt != nil,
	}, nil
}

// JoinRoom seats a new participant and issues their bearer token.
func (s *Service) JoinRoom(ctx context.Context, code, name string, asObserver bool) (CreatedRoom, error) {
	room, err := s.store.RoomByCode(ctx, code)
	if err != nil {
		return CreatedRoom{}, err
	}
	if room.ClosedAt != nil {
		return CreatedRoom{}, fmt.Errorf("%w: this session is closed", domain.ErrConflict)
	}

	display := clean(name, MaxDisplayNameLen)
	if display == "" {
		return CreatedRoom{}, fmt.Errorf("%w: your display name is required", domain.ErrInvalid)
	}

	existing, err := s.store.ListParticipants(ctx, room.ID)
	if err != nil {
		return CreatedRoom{}, err
	}
	if len(existing) >= MaxPlayersPerRoom {
		return CreatedRoom{}, fmt.Errorf("%w: this room is full", domain.ErrConflict)
	}
	for _, p := range existing {
		if strings.EqualFold(p.Name, display) {
			return CreatedRoom{}, fmt.Errorf("%w: the name %q is already taken in this room", domain.ErrConflict, display)
		}
	}

	now := s.now()
	participant := domain.Participant{
		ID:         uuid.NewString(),
		RoomID:     room.ID,
		Name:       display,
		IsObserver: asObserver,
		JoinedAt:   now,
		LastSeenAt: now,
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return CreatedRoom{}, err
	}
	if err := s.store.CreateParticipant(ctx, participant, tokenHash); err != nil {
		return CreatedRoom{}, err
	}

	s.publish(room.ID, "participant.joined", map[string]string{"name": participant.Name})
	return CreatedRoom{Room: room, Host: participant, Token: token}, nil
}

// UpdateProfile lets a participant rename themselves or toggle observer mode.
func (s *Service) UpdateProfile(ctx context.Context, sess Session, name string, isObserver bool) error {
	display := clean(name, MaxDisplayNameLen)
	if display == "" {
		return fmt.Errorf("%w: your display name is required", domain.ErrInvalid)
	}
	others, err := s.store.ListParticipants(ctx, sess.Room.ID)
	if err != nil {
		return err
	}
	for _, p := range others {
		if p.ID != sess.Participant.ID && strings.EqualFold(p.Name, display) {
			return fmt.Errorf("%w: the name %q is already taken", domain.ErrConflict, display)
		}
	}
	if err := s.store.UpdateParticipant(ctx, sess.Participant.ID, display, isObserver); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "participant.updated", nil)
	return nil
}

// UpdateRoomSettings changes the room name and auto-reveal behaviour (host only).
func (s *Service) UpdateRoomSettings(ctx context.Context, sess Session, name string, autoReveal bool) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	clean := clean(name, MaxRoomNameLen)
	if clean == "" {
		return fmt.Errorf("%w: the room name is required", domain.ErrInvalid)
	}
	if err := s.store.UpdateRoomSettings(ctx, sess.Room.ID, clean, autoReveal); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "room.updated", nil)
	return nil
}

// KickParticipant removes somebody from the room (host only).
func (s *Service) KickParticipant(ctx context.Context, sess Session, participantID string) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	if participantID == sess.Participant.ID {
		return fmt.Errorf("%w: the host cannot remove themselves", domain.ErrInvalid)
	}
	target, err := s.store.ParticipantByID(ctx, participantID)
	if err != nil {
		return err
	}
	if target.RoomID != sess.Room.ID {
		return domain.ErrNotFound
	}
	if err := s.store.DeleteParticipant(ctx, participantID); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "participant.removed", map[string]string{"participantId": participantID})
	return nil
}

// ------------------------------------------------------------------ topics

// TopicInput is a manually created or edited topic.
type TopicInput struct {
	Title       string
	Description string
}

// AddTopics appends topics to the backlog (host only).
func (s *Service) AddTopics(ctx context.Context, sess Session, inputs []TopicInput) ([]domain.Topic, error) {
	if err := requireHost(sess); err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("%w: no topics supplied", domain.ErrInvalid)
	}

	existing, err := s.store.ListTopics(ctx, sess.Room.ID)
	if err != nil {
		return nil, err
	}
	if len(existing)+len(inputs) > MaxTopicsPerRoom {
		return nil, fmt.Errorf("%w: a room holds at most %d topics", domain.ErrConflict, MaxTopicsPerRoom)
	}

	pos, err := s.store.NextTopicPosition(ctx, sess.Room.ID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	created := make([]domain.Topic, 0, len(inputs))
	for _, in := range inputs {
		title := clean(in.Title, MaxTopicTitleLen)
		if title == "" {
			return nil, fmt.Errorf("%w: every topic needs a title", domain.ErrInvalid)
		}
		t := domain.Topic{
			ID:          uuid.NewString(),
			RoomID:      sess.Room.ID,
			Title:       title,
			Description: clean(in.Description, MaxTopicDescLen),
			Position:    pos,
			Status:      domain.StatusPending,
			CreatedAt:   now,
		}
		if err := s.store.CreateTopic(ctx, t); err != nil {
			return nil, err
		}
		created = append(created, t)
		pos++
	}

	// A synchronous room with nothing selected starts on the first new topic.
	if sess.Room.Mode == domain.ModeSync && sess.Room.CurrentTopicID == nil && len(created) > 0 {
		if err := s.store.SetCurrentTopic(ctx, sess.Room.ID, &created[0].ID); err != nil {
			return nil, err
		}
	}

	s.publish(sess.Room.ID, "topics.added", map[string]int{"count": len(created)})
	return created, nil
}

// UpdateTopic edits a topic's title and description (host only).
func (s *Service) UpdateTopic(ctx context.Context, sess Session, topicID string, in TopicInput) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	title := clean(in.Title, MaxTopicTitleLen)
	if title == "" {
		return fmt.Errorf("%w: the topic needs a title", domain.ErrInvalid)
	}
	if err := s.store.UpdateTopicDetails(ctx, sess.Room.ID, topicID, title, clean(in.Description, MaxTopicDescLen)); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "topic.updated", map[string]string{"topicId": topicID})
	return nil
}

// DeleteTopic drops a topic and its votes (host only).
func (s *Service) DeleteTopic(ctx context.Context, sess Session, topicID string) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	if err := s.store.DeleteTopic(ctx, sess.Room.ID, topicID); err != nil {
		return err
	}
	if sess.Room.CurrentTopicID != nil && *sess.Room.CurrentTopicID == topicID {
		next, err := s.firstUnestimatedTopicID(ctx, sess.Room.ID)
		if err != nil {
			return err
		}
		if err := s.store.SetCurrentTopic(ctx, sess.Room.ID, next); err != nil {
			return err
		}
	}
	s.publish(sess.Room.ID, "topic.deleted", map[string]string{"topicId": topicID})
	return nil
}

// ReorderTopics rewrites the backlog order (host only).
func (s *Service) ReorderTopics(ctx context.Context, sess Session, orderedIDs []string) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	current, err := s.store.ListTopics(ctx, sess.Room.ID)
	if err != nil {
		return err
	}
	if len(orderedIDs) != len(current) {
		return fmt.Errorf("%w: the new order must list every topic exactly once", domain.ErrInvalid)
	}
	known := make(map[string]bool, len(current))
	for _, t := range current {
		known[t.ID] = true
	}
	seen := make(map[string]bool, len(orderedIDs))
	for _, id := range orderedIDs {
		if !known[id] || seen[id] {
			return fmt.Errorf("%w: the new order must list every topic exactly once", domain.ErrInvalid)
		}
		seen[id] = true
	}
	if err := s.store.ReorderTopics(ctx, sess.Room.ID, orderedIDs); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "topics.reordered", nil)
	return nil
}

// SetCurrentTopic focuses the room on one topic. Synchronous rooms only.
func (s *Service) SetCurrentTopic(ctx context.Context, sess Session, topicID string) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	if sess.Room.Mode != domain.ModeSync {
		return fmt.Errorf("%w: asynchronous rooms have no shared current topic", domain.ErrConflict)
	}
	topic, err := s.store.TopicByID(ctx, sess.Room.ID, topicID)
	if err != nil {
		return err
	}
	if err := s.store.SetCurrentTopic(ctx, sess.Room.ID, &topic.ID); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "topic.focused", map[string]string{"topicId": topic.ID})
	return nil
}

// AdvanceCurrentTopic steps to the next or previous topic. Synchronous rooms only.
func (s *Service) AdvanceCurrentTopic(ctx context.Context, sess Session, direction string) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	if sess.Room.Mode != domain.ModeSync {
		return fmt.Errorf("%w: asynchronous rooms have no shared current topic", domain.ErrConflict)
	}
	topics, err := s.store.ListTopics(ctx, sess.Room.ID)
	if err != nil {
		return err
	}
	if len(topics) == 0 {
		return fmt.Errorf("%w: the backlog is empty", domain.ErrConflict)
	}

	idx := 0
	if sess.Room.CurrentTopicID != nil {
		for i, t := range topics {
			if t.ID == *sess.Room.CurrentTopicID {
				idx = i
				break
			}
		}
	}
	switch direction {
	case "next":
		idx++
	case "prev":
		idx--
	default:
		return fmt.Errorf(`%w: direction must be "next" or "prev"`, domain.ErrInvalid)
	}
	if idx < 0 || idx >= len(topics) {
		return fmt.Errorf("%w: no further topic in that direction", domain.ErrConflict)
	}

	if err := s.store.SetCurrentTopic(ctx, sess.Room.ID, &topics[idx].ID); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "topic.focused", map[string]string{"topicId": topics[idx].ID})
	return nil
}

func (s *Service) firstUnestimatedTopicID(ctx context.Context, roomID string) (*string, error) {
	topics, err := s.store.ListTopics(ctx, roomID)
	if err != nil {
		return nil, err
	}
	for _, t := range topics {
		if t.Status != domain.StatusEstimated {
			id := t.ID
			return &id, nil
		}
	}
	if len(topics) > 0 {
		id := topics[len(topics)-1].ID
		return &id, nil
	}
	return nil, nil
}

// ------------------------------------------------------------------ voting

// CastVote records a card. This is where the two modes really differ:
// a synchronous room only accepts votes on the shared current topic, while an
// asynchronous room accepts a vote on any topic that is still face down.
func (s *Service) CastVote(ctx context.Context, sess Session, topicID, value string) error {
	if sess.Room.ClosedAt != nil {
		return fmt.Errorf("%w: this session is closed", domain.ErrConflict)
	}
	if sess.Participant.IsObserver {
		return fmt.Errorf("%w: observers do not vote", domain.ErrForbidden)
	}
	if !domain.DeckContains(sess.Room.Deck, value) {
		return fmt.Errorf("%w: %q is not a card in this deck", domain.ErrInvalid, value)
	}

	topic, err := s.store.TopicByID(ctx, sess.Room.ID, topicID)
	if err != nil {
		return err
	}
	if sess.Room.Mode == domain.ModeSync {
		if sess.Room.CurrentTopicID == nil || *sess.Room.CurrentTopicID != topic.ID {
			return fmt.Errorf("%w: in a synchronous room you can only vote on the current topic", domain.ErrForbidden)
		}
	}
	if topic.Status == domain.StatusRevealed || topic.Status == domain.StatusEstimated {
		return fmt.Errorf("%w: the cards for this topic are already face up", domain.ErrConflict)
	}

	if err := s.store.CastVote(ctx, domain.Vote{
		TopicID:       topic.ID,
		ParticipantID: sess.Participant.ID,
		Value:         value,
		CreatedAt:     s.now(),
	}); err != nil {
		return err
	}
	if topic.Status == domain.StatusPending {
		if err := s.store.UpdateTopicStatus(ctx, topic.ID, domain.StatusVoting, nil); err != nil {
			return err
		}
		topic.Status = domain.StatusVoting
	}

	revealed, err := s.maybeAutoReveal(ctx, sess.Room, topic)
	if err != nil {
		return err
	}
	if revealed {
		s.publish(sess.Room.ID, "topic.revealed", map[string]string{"topicId": topic.ID})
	} else {
		s.publish(sess.Room.ID, "vote.cast", map[string]string{"topicId": topic.ID})
	}
	return nil
}

// ClearVote takes a participant's own card back before the reveal.
func (s *Service) ClearVote(ctx context.Context, sess Session, topicID string) error {
	topic, err := s.store.TopicByID(ctx, sess.Room.ID, topicID)
	if err != nil {
		return err
	}
	if topic.Status == domain.StatusRevealed || topic.Status == domain.StatusEstimated {
		return fmt.Errorf("%w: the cards for this topic are already face up", domain.ErrConflict)
	}
	if err := s.store.ClearVote(ctx, topic.ID, sess.Participant.ID); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "vote.cleared", map[string]string{"topicId": topicID})
	return nil
}

// maybeAutoReveal flips the cards when everybody who is expected to vote has voted.
//
//	sync  - "everybody" means the players currently connected to the room.
//	async - "everybody" means every non-observer member, since players drop in
//	        and out over hours or days.
func (s *Service) maybeAutoReveal(ctx context.Context, room domain.Room, topic domain.Topic) (bool, error) {
	if !room.AutoReveal {
		return false, nil
	}
	participants, err := s.store.ListParticipants(ctx, room.ID)
	if err != nil {
		return false, err
	}
	expected := s.expectedVoters(room, participants)
	if len(expected) == 0 {
		return false, nil
	}
	votes, err := s.store.ListVotesForTopic(ctx, topic.ID)
	if err != nil {
		return false, err
	}
	voted := make(map[string]bool, len(votes))
	for _, v := range votes {
		voted[v.ParticipantID] = true
	}
	for _, p := range expected {
		if !voted[p.ID] {
			return false, nil
		}
	}

	now := s.now()
	if err := s.store.UpdateTopicStatus(ctx, topic.ID, domain.StatusRevealed, &now); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) expectedVoters(room domain.Room, participants []domain.Participant) []domain.Participant {
	online := s.hub.OnlineParticipants(room.ID)
	out := make([]domain.Participant, 0, len(participants))
	for _, p := range participants {
		if p.IsObserver {
			continue
		}
		if room.Mode == domain.ModeSync && !online[p.ID] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// RevealTopic flips the cards early (host only).
func (s *Service) RevealTopic(ctx context.Context, sess Session, topicID string) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	topic, err := s.store.TopicByID(ctx, sess.Room.ID, topicID)
	if err != nil {
		return err
	}
	if topic.Status == domain.StatusRevealed || topic.Status == domain.StatusEstimated {
		return nil
	}
	now := s.now()
	if err := s.store.UpdateTopicStatus(ctx, topic.ID, domain.StatusRevealed, &now); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "topic.revealed", map[string]string{"topicId": topic.ID})
	return nil
}

// ResetTopic clears the round so the team can vote again (host only).
func (s *Service) ResetTopic(ctx context.Context, sess Session, topicID string) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	topic, err := s.store.TopicByID(ctx, sess.Room.ID, topicID)
	if err != nil {
		return err
	}
	if err := s.store.ResetTopic(ctx, topic.ID); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "topic.reset", map[string]string{"topicId": topic.ID})
	return nil
}

// FinalizeTopic stores the agreed estimate (host only).
func (s *Service) FinalizeTopic(ctx context.Context, sess Session, topicID, estimate string) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	topic, err := s.store.TopicByID(ctx, sess.Room.ID, topicID)
	if err != nil {
		return err
	}
	if topic.Status != domain.StatusRevealed && topic.Status != domain.StatusEstimated {
		return fmt.Errorf("%w: reveal the cards before agreeing on an estimate", domain.ErrConflict)
	}
	if !domain.DeckContains(sess.Room.Deck, estimate) {
		return fmt.Errorf("%w: %q is not a card in this deck", domain.ErrInvalid, estimate)
	}
	if _, ok := domain.NumericValue(estimate); !ok {
		return fmt.Errorf("%w: the final estimate must be a number", domain.ErrInvalid)
	}
	if err := s.store.FinalizeTopic(ctx, topic.ID, &estimate); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "topic.estimated", map[string]string{"topicId": topic.ID, "estimate": estimate})
	return nil
}

// ------------------------------------------------------------------ helpers

func requireHost(sess Session) error {
	if !sess.Participant.IsHost {
		return fmt.Errorf("%w: only the host can do that", domain.ErrForbidden)
	}
	return nil
}

func (s *Service) publish(roomID, evType string, payload any) {
	s.hub.Publish(roomID, hub.Event{Type: evType, Payload: payload, WithState: true})
}

// Touch records participant liveness (called on WebSocket activity).
func (s *Service) Touch(ctx context.Context, participantID string) error {
	return s.store.TouchParticipant(ctx, participantID, s.now())
}

// NotifyPresence tells the room that the connected-player set changed.
func (s *Service) NotifyPresence(roomID string) {
	s.publish(roomID, "presence.changed", nil)
}

func clean(v string, max int) string {
	v = strings.TrimSpace(v)
	// Strip control characters that would corrupt the UI or logs.
	v = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, v)
	if len([]rune(v)) > max {
		v = string([]rune(v)[:max])
	}
	return strings.TrimSpace(v)
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
