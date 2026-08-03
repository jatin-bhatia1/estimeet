// Package hub fans real-time events out to every WebSocket client of a room and
// tracks who is currently connected.
package hub

import (
	"sync"
)

// Event is a message pushed to clients. Payload is optional; most events simply
// tell the client that the room changed and that it should re-render the state
// snapshot the server sends alongside.
type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
	// WithState asks the connection writer to append a fresh, per-participant
	// state snapshot after this event.
	WithState bool `json:"-"`
}

// Client is one live WebSocket connection.
type Client struct {
	RoomID        string
	ParticipantID string

	send   chan Event
	closed chan struct{}
	once   sync.Once
}

// Send returns the outbound channel the connection writer reads from.
func (c *Client) Send() <-chan Event { return c.send }

// Closed is signalled when the hub drops the client.
func (c *Client) Closed() <-chan struct{} { return c.closed }

func (c *Client) close() {
	c.once.Do(func() { close(c.closed) })
}

// Hub keeps the per-room client registries.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*Client]struct{}
}

// New builds an empty hub.
func New() *Hub {
	return &Hub{rooms: make(map[string]map[*Client]struct{})}
}

// Register adds a connection to a room and returns the client handle.
func (h *Hub) Register(roomID, participantID string) *Client {
	c := &Client{
		RoomID:        roomID,
		ParticipantID: participantID,
		send:          make(chan Event, 32),
		closed:        make(chan struct{}),
	}
	h.mu.Lock()
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = make(map[*Client]struct{})
	}
	h.rooms[roomID][c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Unregister removes a connection.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	if clients, ok := h.rooms[c.RoomID]; ok {
		delete(clients, c)
		if len(clients) == 0 {
			delete(h.rooms, c.RoomID)
		}
	}
	h.mu.Unlock()
	c.close()
}

// Publish fans an event out to every client of a room. Slow consumers are
// dropped rather than allowed to block the publisher.
func (h *Hub) Publish(roomID string, ev Event) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.rooms[roomID]))
	for c := range h.rooms[roomID] {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	var stalled []*Client
	for _, c := range clients {
		select {
		case c.send <- ev:
		default:
			stalled = append(stalled, c)
		}
	}
	for _, c := range stalled {
		h.Unregister(c)
	}
}

// OnlineParticipants reports which participants of a room have a live connection.
func (h *Hub) OnlineParticipants(roomID string) map[string]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	out := make(map[string]bool, len(h.rooms[roomID]))
	for c := range h.rooms[roomID] {
		out[c.ParticipantID] = true
	}
	return out
}

// ConnectionCount returns the number of live sockets in a room.
func (h *Hub) ConnectionCount(roomID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[roomID])
}
