package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

type noteRequest struct {
	Kind string `json:"kind"`
	Body string `json:"body"`
}

func (s *server) handleAddNote(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req noteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.svc.AddNote(r.Context(), sess, chi.URLParam(r, "topicId"), domain.NoteKind(req.Kind), req.Body); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

func (s *server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	if err := s.svc.DeleteNote(r.Context(), sess, chi.URLParam(r, "topicId"), chi.URLParam(r, "noteId")); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}
