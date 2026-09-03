// Package api exposes the HTTP and WebSocket surface of Estimeet.
package api

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/jatin-bhatia1/estimeet/backend/internal/config"
	"github.com/jatin-bhatia1/estimeet/backend/internal/service"
)

type server struct {
	cfg config.Config
	svc *service.Service
}

// NewRouter builds the full HTTP handler.
func NewRouter(cfg config.Config, svc *service.Service) http.Handler {
	s := &server{cfg: cfg, svc: svc}

	// Creating and joining rooms is unauthenticated, so it is rate limited.
	joinLimiter := newRateLimiter(20, 60)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger)
	r.Use(securityHeaders)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", TokenHeader},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/config", s.handleAppConfig)

		r.With(joinLimiter.middleware).Post("/rooms", s.handleCreateRoom)
		r.Get("/rooms/{code}", s.handleRoomSummary)
		r.With(joinLimiter.middleware).Post("/rooms/{code}/join", s.handleJoinRoom)

		// The OAuth callback is authenticated by the single-use `state` value.
		r.Get("/jira/callback", s.handleJiraCallback)

		// Long-lived socket: registered outside the request-timeout group.
		r.With(s.requireSession).Get("/rooms/{code}/ws", s.handleWebSocket)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(30 * time.Second))
			r.Use(s.requireSession)

			r.Get("/rooms/{code}/state", s.handleState)
			r.Patch("/rooms/{code}", s.handleUpdateRoom)
			r.Put("/rooms/{code}/deck", s.handleSetDeck)
			r.Put("/rooms/{code}/roster", s.handleSetRoster)
			r.Patch("/rooms/{code}/me", s.handleUpdateProfile)
			r.Delete("/rooms/{code}/participants/{participantId}", s.handleKickParticipant)

			r.Post("/rooms/{code}/topics", s.handleAddTopics)
			r.Post("/rooms/{code}/topics/reorder", s.handleReorderTopics)
			r.Patch("/rooms/{code}/topics/{topicId}", s.handleUpdateTopic)
			r.Delete("/rooms/{code}/topics/{topicId}", s.handleDeleteTopic)

			r.Post("/rooms/{code}/topics/{topicId}/vote", s.handleVote)
			r.Delete("/rooms/{code}/topics/{topicId}/vote", s.handleClearVote)
			r.Post("/rooms/{code}/topics/{topicId}/reveal", s.handleReveal)
			r.Post("/rooms/{code}/topics/{topicId}/reset", s.handleResetTopic)
			r.Post("/rooms/{code}/topics/{topicId}/estimate", s.handleEstimate)
			r.Post("/rooms/{code}/current", s.handleSetCurrent)

			// Jira's OAuth flow is the one connection that is not a pasted token.
			r.Post("/rooms/{code}/jira/connect", s.handleJiraConnect)

			r.Post("/rooms/{code}/source", s.handleSourceConnect)
			r.Delete("/rooms/{code}/source", s.handleSourceDisconnect)
			r.Get("/rooms/{code}/source/containers", s.handleSourceContainers)
			r.Get("/rooms/{code}/source/groups", s.handleSourceGroups)
			r.Get("/rooms/{code}/source/items", s.handleSourceItems)
			r.Post("/rooms/{code}/source/import", s.handleSourceImport)
		})
	})

	// The same check at the root, because load balancer target groups are
	// usually configured for /health and some managed platforms do not let you
	// change the path.
	r.Get("/health", s.handleHealth)

	r.NotFound(s.notFound)
	return r
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// notFound serves the SPA for unknown non-API paths when a static build is configured.
func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	if s.cfg.StaticDir == "" || strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not found"})
		return
	}
	// Serve the requested asset if it exists, otherwise fall back to index.html
	// so client-side routing works on a hard refresh.
	clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if clean != "." && !strings.HasPrefix(clean, "..") {
		candidate := filepath.Join(s.cfg.StaticDir, clean)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			http.ServeFile(w, r, candidate)
			return
		}
	}
	http.ServeFile(w, r, filepath.Join(s.cfg.StaticDir, "index.html"))
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration", time.Since(start).String(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}
