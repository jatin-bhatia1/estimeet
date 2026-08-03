package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
	"github.com/jatin-bhatia1/estimeet/backend/internal/service"
)

const maxRequestBytes = 1 << 20 // 1 MiB

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("write json response", "error", err)
	}
}

// writeError maps domain errors onto HTTP status codes and never leaks
// internal details for unexpected failures.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: publicMessage(err, "not found")})
	case errors.Is(err, domain.ErrForbidden):
		writeJSON(w, http.StatusForbidden, errorBody{Error: publicMessage(err, "you are not allowed to do that")})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, errorBody{Error: publicMessage(err, "conflict")})
	case errors.Is(err, domain.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: publicMessage(err, "invalid request")})
	case errors.Is(err, service.ErrJiraDisabled):
		writeJSON(w, http.StatusNotImplemented, errorBody{Error: err.Error()})
	case errors.Is(err, service.ErrJiraNotConnected):
		writeJSON(w, http.StatusPreconditionFailed, errorBody{Error: err.Error()})
	default:
		var apiErr *jira.APIError
		if errors.As(err, &apiErr) {
			slog.Warn("jira upstream error", "status", apiErr.StatusCode, "detail", apiErr.Detail)
			writeJSON(w, http.StatusBadGateway, errorBody{Error: "Jira rejected the request: " + apiErr.Detail})
			return
		}
		slog.Error("unhandled request error", "path", r.URL.Path, "method", r.Method, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "something went wrong on our side"})
	}
}

// publicMessage strips the sentinel prefix so users see the useful half only.
func publicMessage(err error, fallback string) string {
	msg := err.Error()
	if idx := strings.Index(msg, ": "); idx >= 0 && idx+2 < len(msg) {
		return msg[idx+2:]
	}
	if msg == "" {
		return fallback
	}
	return msg
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.Join(domain.ErrInvalid, errors.New("the request body is not valid JSON"))
	}
	// Reject trailing content so a single request cannot smuggle a second body.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.Join(domain.ErrInvalid, errors.New("the request body must contain a single JSON object"))
	}
	return nil
}
