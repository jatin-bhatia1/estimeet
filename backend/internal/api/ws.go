package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

const (
	wsSubprotocol = "estimeet.v1"
	wsReadLimit   = 4 << 10
	wsPingEvery   = 25 * time.Second
	wsWriteWait   = 10 * time.Second
)

type wsMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

// handleWebSocket streams room state to one participant. The client never has
// to poll: every mutation anywhere in the room results in a fresh, per-viewer
// snapshot being pushed here.
func (s *server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionFrom(r.Context())
	if !ok {
		writeError(w, r, domain.ErrForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:    []string{wsSubprotocol},
		OriginPatterns:  originPatterns(s.cfg.AllowedOrigins),
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		slog.Debug("websocket handshake failed", "error", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(wsReadLimit)

	hb := s.svc.Hub()
	client := hb.Register(sess.Room.ID, sess.Participant.ID)
	defer func() {
		hb.Unregister(client)
		s.svc.NotifyPresence(sess.Room.ID)
	}()

	// Detach from the request context so the socket is not killed by request
	// scoped cancellation, but still stops when the process shuts down.
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()

	// Reader: the client only sends keep-alives, but reading is required to
	// observe disconnects and to honour the protocol.
	go func() {
		defer cancel()
		for {
			var msg wsMessage
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				return
			}
			if err := s.svc.Touch(ctx, sess.Participant.ID); err != nil {
				slog.Debug("touch participant", "error", err)
			}
		}
	}()

	// Announce arrival, then send this viewer their first snapshot.
	s.svc.NotifyPresence(sess.Room.ID)
	if !s.pushState(ctx, conn, sess.Room.ID, sess.Participant.ID) {
		return
	}

	ticker := time.NewTicker(wsPingEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-client.Closed():
			return
		case ev := <-client.Send():
			if !writeWS(ctx, conn, wsMessage{Type: ev.Type, Payload: ev.Payload}) {
				return
			}
			if ev.WithState && !s.pushState(ctx, conn, sess.Room.ID, sess.Participant.ID) {
				return
			}
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, wsWriteWait)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				return
			}
		}
	}
}

func (s *server) pushState(ctx context.Context, conn *websocket.Conn, roomID, participantID string) bool {
	state, err := s.svc.State(ctx, roomID, participantID)
	if err != nil {
		slog.Debug("build websocket state", "error", err)
		return false
	}
	return writeWS(ctx, conn, wsMessage{Type: "state", Payload: state})
}

func writeWS(ctx context.Context, conn *websocket.Conn, msg wsMessage) bool {
	writeCtx, cancel := context.WithTimeout(ctx, wsWriteWait)
	defer cancel()
	if err := wsjson.Write(writeCtx, conn, msg); err != nil {
		slog.Debug("websocket write failed", "error", err)
		return false
	}
	return true
}

// originPatterns converts configured origins ("https://app.example.com") into
// the host patterns the WebSocket library matches against.
func originPatterns(origins []string) []string {
	out := make([]string, 0, len(origins))
	for _, o := range origins {
		if o == "*" {
			return []string{"*"}
		}
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			out = append(out, u.Host)
			continue
		}
		out = append(out, strings.TrimSpace(o))
	}
	return out
}
