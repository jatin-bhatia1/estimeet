package service

import (
	"context"
	"time"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

// RoomView is the room as the client sees it.
type RoomView struct {
	ID             string      `json:"id"`
	Code           string      `json:"code"`
	Name           string      `json:"name"`
	Mode           domain.Mode `json:"mode"`
	Deck           []string    `json:"deck"`
	CurrentTopicID *string     `json:"currentTopicId"`
	AutoReveal     bool        `json:"autoReveal"`
	Closed         bool        `json:"closed"`
	CreatedAt      time.Time   `json:"createdAt"`
	// JiraAvailable means the room can connect to Jira at all; JiraOAuthAvailable
	// additionally means this server has an Atlassian OAuth app registered.
	JiraAvailable      bool   `json:"jiraAvailable"`
	JiraOAuthAvailable bool   `json:"jiraOauthAvailable"`
	JiraConnected      bool   `json:"jiraConnected"`
	JiraAuthType       string `json:"jiraAuthType,omitempty"`
	JiraAccountEmail   string `json:"jiraAccountEmail,omitempty"`
	JiraSiteName       string `json:"jiraSiteName,omitempty"`
	JiraSiteURL        string `json:"jiraSiteUrl,omitempty"`
}

// ParticipantView adds presence and progress to a participant.
type ParticipantView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	IsHost     bool      `json:"isHost"`
	IsObserver bool      `json:"isObserver"`
	Online     bool      `json:"online"`
	JoinedAt   time.Time `json:"joinedAt"`
	// VotedTopics is how many topics this player has already estimated. It is
	// what drives the progress column of an asynchronous session.
	VotedTopics int `json:"votedTopics"`
}

// VoteView is a single revealed card.
type VoteView struct {
	ParticipantID   string `json:"participantId"`
	ParticipantName string `json:"participantName"`
	Value           string `json:"value"`
}

// TopicView is a topic plus everything the current participant is allowed to see.
type TopicView struct {
	domain.Topic
	Revealed bool `json:"revealed"`
	// VotedBy is always visible: knowing *who* has played is not the same as
	// knowing *what* they played, and it is what makes the waiting UI work.
	VotedBy       []string      `json:"votedBy"`
	PendingVoters []string      `json:"pendingVoters"`
	MyVote        *string       `json:"myVote"`
	Votes         []VoteView    `json:"votes"`
	Stats         *domain.Stats `json:"stats,omitempty"`
	IsCurrent     bool          `json:"isCurrent"`
	CanVote       bool          `json:"canVote"`
}

// BoardSummary is the headline progress of the session.
type BoardSummary struct {
	TotalTopics     int      `json:"totalTopics"`
	EstimatedTopics int      `json:"estimatedTopics"`
	RevealedTopics  int      `json:"revealedTopics"`
	MyRemaining     int      `json:"myRemaining"`
	TotalPoints     *float64 `json:"totalPoints,omitempty"`
}

// RoomState is the full snapshot pushed over the WebSocket and returned by
// GET /api/rooms/{code}/state. It is rendered per participant because vote
// visibility depends on who is asking.
type RoomState struct {
	Room         RoomView          `json:"room"`
	Me           ParticipantView   `json:"me"`
	Participants []ParticipantView `json:"participants"`
	Topics       []TopicView       `json:"topics"`
	Summary      BoardSummary      `json:"summary"`
	ServerTime   time.Time         `json:"serverTime"`
}

// State builds the snapshot for one participant.
func (s *Service) State(ctx context.Context, roomID, participantID string) (RoomState, error) {
	room, err := s.store.RoomByID(ctx, roomID)
	if err != nil {
		return RoomState{}, err
	}
	me, err := s.store.ParticipantByID(ctx, participantID)
	if err != nil {
		return RoomState{}, err
	}
	if me.RoomID != room.ID {
		return RoomState{}, domain.ErrForbidden
	}

	participants, err := s.store.ListParticipants(ctx, room.ID)
	if err != nil {
		return RoomState{}, err
	}
	topics, err := s.store.ListTopics(ctx, room.ID)
	if err != nil {
		return RoomState{}, err
	}
	votesByTopic, err := s.store.ListVotesForRoom(ctx, room.ID)
	if err != nil {
		return RoomState{}, err
	}

	online := s.hub.OnlineParticipants(room.ID)
	nameByID := make(map[string]string, len(participants))
	votedCount := make(map[string]int, len(participants))
	for _, p := range participants {
		nameByID[p.ID] = p.Name
	}

	state := RoomState{
		Room: RoomView{
			ID:             room.ID,
			Code:           room.Code,
			Name:           room.Name,
			Mode:           room.Mode,
			Deck:           room.Deck,
			CurrentTopicID: room.CurrentTopicID,
			AutoReveal:     room.AutoReveal,
			Closed:         room.ClosedAt != nil,
			CreatedAt:          room.CreatedAt,
			JiraAvailable:      s.JiraAvailable(),
			JiraOAuthAvailable: s.JiraOAuthAvailable(),
		},
		Participants: make([]ParticipantView, 0, len(participants)),
		Topics:       make([]TopicView, 0, len(topics)),
		ServerTime:   s.now(),
	}

	// A missing Jira connection is the normal case, so the error is ignored here.
	if conn, _, _, err := s.store.RawJiraConnection(ctx, room.ID); err == nil {
		state.Room.JiraConnected = true
		state.Room.JiraAuthType = conn.AuthType
		state.Room.JiraAccountEmail = conn.AccountEmail
		state.Room.JiraSiteName = conn.SiteName
		state.Room.JiraSiteURL = conn.SiteURL
	}

	var totalPoints float64
	var hasPoints bool

	for _, t := range topics {
		votes := votesByTopic[t.ID]
		revealed := t.Status == domain.StatusRevealed || t.Status == domain.StatusEstimated

		view := TopicView{
			Topic:         t,
			Revealed:      revealed,
			VotedBy:       make([]string, 0, len(votes)),
			PendingVoters: []string{},
			Votes:         []VoteView{},
			IsCurrent:     room.CurrentTopicID != nil && *room.CurrentTopicID == t.ID,
		}

		votedSet := make(map[string]bool, len(votes))
		for _, v := range votes {
			votedSet[v.ParticipantID] = true
			view.VotedBy = append(view.VotedBy, v.ParticipantID)
			votedCount[v.ParticipantID]++
			if v.ParticipantID == me.ID {
				value := v.Value
				view.MyVote = &value
			}
			if revealed {
				view.Votes = append(view.Votes, VoteView{
					ParticipantID:   v.ParticipantID,
					ParticipantName: nameByID[v.ParticipantID],
					Value:           v.Value,
				})
			}
		}

		for _, p := range s.expectedVoters(room, participants) {
			if !votedSet[p.ID] {
				view.PendingVoters = append(view.PendingVoters, p.ID)
			}
		}

		if revealed {
			stats := domain.Summarize(votes, room.Deck)
			view.Stats = &stats
		}

		// A player may vote when the cards are still face down and, in a
		// synchronous room, only on the topic the host is showing.
		view.CanVote = !me.IsObserver && room.ClosedAt == nil && !revealed &&
			(room.Mode == domain.ModeAsync || view.IsCurrent)

		if t.Status == domain.StatusEstimated && t.FinalEstimate != nil {
			if n, ok := domain.NumericValue(*t.FinalEstimate); ok {
				totalPoints += n
				hasPoints = true
			}
			state.Summary.EstimatedTopics++
		}
		if revealed {
			state.Summary.RevealedTopics++
		}
		if !revealed && view.MyVote == nil && !me.IsObserver {
			state.Summary.MyRemaining++
		}

		state.Topics = append(state.Topics, view)
	}

	state.Summary.TotalTopics = len(topics)
	if hasPoints {
		state.Summary.TotalPoints = &totalPoints
	}

	for _, p := range participants {
		view := ParticipantView{
			ID:          p.ID,
			Name:        p.Name,
			IsHost:      p.IsHost,
			IsObserver:  p.IsObserver,
			Online:      online[p.ID],
			JoinedAt:    p.JoinedAt,
			VotedTopics: votedCount[p.ID],
		}
		state.Participants = append(state.Participants, view)
		if p.ID == me.ID {
			state.Me = view
		}
	}

	return state, nil
}
