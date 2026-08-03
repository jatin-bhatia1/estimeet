// Package domain contains the core estimation entities and rules that do
// not depend on transport or storage details.
package domain

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors. The HTTP layer maps these onto status codes.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
	ErrInvalid   = errors.New("invalid request")
)

// Mode selects how a room walks through its topics.
type Mode string

const (
	// ModeSync: the host drives one shared topic at a time and everybody votes together.
	ModeSync Mode = "sync"
	// ModeAsync: every topic is open at once and each player estimates at their own pace.
	ModeAsync Mode = "async"
)

// Valid reports whether the mode is one of the supported values.
func (m Mode) Valid() bool { return m == ModeSync || m == ModeAsync }

// TopicStatus is the lifecycle of a single item being estimated.
type TopicStatus string

const (
	// StatusPending means no vote has been cast yet.
	StatusPending TopicStatus = "pending"
	// StatusVoting means at least one vote is in but the cards are still face down.
	StatusVoting TopicStatus = "voting"
	// StatusRevealed means every card is face up and the discussion is open.
	StatusRevealed TopicStatus = "revealed"
	// StatusEstimated means the team agreed on a final value.
	StatusEstimated TopicStatus = "estimated"
)

// Special, non-numeric cards.
const (
	CardUnknown = "?"
	CardCoffee  = "coffee"
)

// FibonacciDeck is the default deck: the Fibonacci sequence plus the two escape cards.
var FibonacciDeck = []string{"0", "1", "2", "3", "5", "8", "13", "21", "34", "55", "89", CardUnknown, CardCoffee}

// DefaultDeck returns a copy of the Fibonacci deck so callers cannot mutate the package global.
func DefaultDeck() []string {
	out := make([]string, len(FibonacciDeck))
	copy(out, FibonacciDeck)
	return out
}

// NumericValue parses a card into a number. Special cards return ok=false and
// are therefore excluded from every statistic.
func NumericValue(card string) (float64, bool) {
	if card == CardUnknown || card == CardCoffee {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(card), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// Room is a single estimation session.
type Room struct {
	ID             string     `json:"id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Mode           Mode       `json:"mode"`
	Deck           []string   `json:"deck"`
	CurrentTopicID *string    `json:"currentTopicId"`
	AutoReveal     bool       `json:"autoReveal"`
	CreatedAt      time.Time  `json:"createdAt"`
	ClosedAt       *time.Time `json:"closedAt,omitempty"`
}

// Participant is somebody who joined a room. Authentication is a bearer token
// handed out at join time; only its hash is persisted.
type Participant struct {
	ID     string `json:"id"`
	RoomID string `json:"-"`
	Name   string `json:"name"`
	// IsHost is granted once, to whoever created the session, and is never
	// transferred. Only the host may reveal cards or change the backlog.
	IsHost     bool      `json:"isHost"`
	IsObserver bool      `json:"isObserver"`
	JoinedAt   time.Time `json:"joinedAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

// Topic is one item to estimate, either created by hand or imported from Jira.
type Topic struct {
	ID            string      `json:"id"`
	RoomID        string      `json:"-"`
	Title         string      `json:"title"`
	Description   string      `json:"description"`
	ExternalKey   *string     `json:"externalKey,omitempty"`
	ExternalURL   *string     `json:"externalUrl,omitempty"`
	Position      int         `json:"position"`
	Status        TopicStatus `json:"status"`
	FinalEstimate *string     `json:"finalEstimate,omitempty"`
	CreatedAt     time.Time   `json:"createdAt"`
	RevealedAt    *time.Time  `json:"revealedAt,omitempty"`
}

// Vote is a single card played by a participant on a topic.
type Vote struct {
	TopicID       string    `json:"topicId"`
	ParticipantID string    `json:"participantId"`
	Value         string    `json:"value"`
	CreatedAt     time.Time `json:"createdAt"`
}

// codeAlphabet omits characters that are easy to confuse when read aloud (0/O, 1/I).
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewRoomCode returns a short, human-shareable, cryptographically random room code.
func NewRoomCode() (string, error) {
	var sb strings.Builder
	for range 6 {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}
		sb.WriteByte(codeAlphabet[n.Int64()])
	}
	return sb.String(), nil
}

// NormalizeCode makes room codes case- and whitespace-insensitive for users.
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// DeckContains reports whether a card belongs to the room's deck.
func DeckContains(deck []string, card string) bool {
	for _, c := range deck {
		if c == card {
			return true
		}
	}
	return false
}
