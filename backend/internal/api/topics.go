package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jatin-bhatia1/estimeet/backend/internal/service"
)

type topicPayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type addTopicsRequest struct {
	// Either a single topic or a batch (used by the "paste a list" flow).
	Topic  *topicPayload  `json:"topic,omitempty"`
	Topics []topicPayload `json:"topics,omitempty"`
}

func (s *server) handleAddTopics(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req addTopicsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	payloads := req.Topics
	if req.Topic != nil {
		payloads = append(payloads, *req.Topic)
	}
	inputs := make([]service.TopicInput, 0, len(payloads))
	for _, p := range payloads {
		inputs = append(inputs, service.TopicInput{Title: p.Title, Description: p.Description})
	}

	if _, err := s.svc.AddTopics(r.Context(), sess, inputs); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

func (s *server) handleUpdateTopic(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req topicPayload
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	err := s.svc.UpdateTopic(r.Context(), sess, chi.URLParam(r, "topicId"),
		service.TopicInput{Title: req.Title, Description: req.Description})
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

func (s *server) handleDeleteTopic(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	if err := s.svc.DeleteTopic(r.Context(), sess, chi.URLParam(r, "topicId")); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

type reorderRequest struct {
	TopicIDs []string `json:"topicIds"`
}

func (s *server) handleReorderTopics(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req reorderRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.svc.ReorderTopics(r.Context(), sess, req.TopicIDs); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

type voteRequest struct {
	Value string `json:"value"`
}

func (s *server) handleVote(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req voteRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.svc.CastVote(r.Context(), sess, chi.URLParam(r, "topicId"), req.Value); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

func (s *server) handleClearVote(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	if err := s.svc.ClearVote(r.Context(), sess, chi.URLParam(r, "topicId")); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

func (s *server) handleReveal(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	if err := s.svc.RevealTopic(r.Context(), sess, chi.URLParam(r, "topicId")); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

func (s *server) handleResetTopic(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	if err := s.svc.ResetTopic(r.Context(), sess, chi.URLParam(r, "topicId")); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

type estimateRequest struct {
	Value string `json:"value"`
}

func (s *server) handleEstimate(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req estimateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.svc.FinalizeTopic(r.Context(), sess, chi.URLParam(r, "topicId"), req.Value); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

type setCurrentRequest struct {
	TopicID   string `json:"topicId,omitempty"`
	Direction string `json:"direction,omitempty"`
}

func (s *server) handleSetCurrent(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req setCurrentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	var err error
	if req.Direction != "" {
		err = s.svc.AdvanceCurrentTopic(r.Context(), sess, req.Direction)
	} else {
		err = s.svc.SetCurrentTopic(r.Context(), sess, req.TopicID)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}
