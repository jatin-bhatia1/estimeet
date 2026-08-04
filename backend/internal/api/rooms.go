package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
	"github.com/jatin-bhatia1/estimeet/backend/internal/service"
)

type createRoomRequest struct {
	Name          string   `json:"name"`
	Mode          string   `json:"mode"`
	HostName      string   `json:"hostName"`
	AutoReveal    *bool    `json:"autoReveal"`
	ExpectedSize  int      `json:"expectedSize"`
	ExpectedNames []string `json:"expectedNames"`
}

type sessionResponse struct {
	Token       string             `json:"token"`
	RoomCode    string             `json:"roomCode"`
	Participant domain.Participant `json:"participant"`
	State       service.RoomState  `json:"state"`
}

func (s *server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req createRoomRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	created, err := s.svc.CreateRoom(r.Context(), service.CreateRoomInput{
		Name:          req.Name,
		Mode:          domain.Mode(req.Mode),
		HostName:      req.HostName,
		AutoReveal:    req.AutoReveal,
		ExpectedSize:  req.ExpectedSize,
		ExpectedNames: req.ExpectedNames,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.respondWithSession(w, r, created, http.StatusCreated)
}

type joinRoomRequest struct {
	Name       string `json:"name"`
	AsObserver bool   `json:"asObserver"`
}

func (s *server) handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	var req joinRoomRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	joined, err := s.svc.JoinRoom(r.Context(), chi.URLParam(r, "code"), req.Name, req.AsObserver)
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.respondWithSession(w, r, joined, http.StatusCreated)
}

func (s *server) respondWithSession(w http.ResponseWriter, r *http.Request, created service.CreatedRoom, status int) {
	state, err := s.svc.State(r.Context(), created.Room.ID, created.Host.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, status, sessionResponse{
		Token:       created.Token,
		RoomCode:    created.Room.Code,
		Participant: created.Host,
		State:       state,
	})
}

func (s *server) handleRoomSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.svc.RoomSummaryByCode(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionFrom(r.Context())
	if !ok {
		writeError(w, r, domain.ErrForbidden)
		return
	}
	state, err := s.svc.State(r.Context(), sess.Room.ID, sess.Participant.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type updateRoomRequest struct {
	Name       string `json:"name"`
	AutoReveal bool   `json:"autoReveal"`
}

func (s *server) handleUpdateRoom(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req updateRoomRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.svc.UpdateRoomSettings(r.Context(), sess, req.Name, req.AutoReveal); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

type rosterRequest struct {
	Size  int      `json:"size"`
	Names []string `json:"names"`
}

func (s *server) handleSetRoster(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req rosterRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.svc.SetRoster(r.Context(), sess, req.Size, req.Names); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

type updateProfileRequest struct {
	Name       string `json:"name"`
	IsObserver bool   `json:"isObserver"`
}

func (s *server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req updateProfileRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.svc.UpdateProfile(r.Context(), sess, req.Name, req.IsObserver); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

func (s *server) handleKickParticipant(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	if err := s.svc.KickParticipant(r.Context(), sess, chi.URLParam(r, "participantId")); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

// respondState returns the caller's fresh snapshot after a mutation.
func (s *server) respondState(w http.ResponseWriter, r *http.Request, sess service.Session) {
	state, err := s.svc.State(r.Context(), sess.Room.ID, sess.Participant.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
