package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// handlePushEstimate writes an estimate onto the tracker item behind a topic.
func (s *server) handlePushEstimate(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req estimateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.svc.PushEstimate(r.Context(), sess, chi.URLParam(r, "topicId"), req.Value); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}
