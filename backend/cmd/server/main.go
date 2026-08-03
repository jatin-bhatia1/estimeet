// Command server runs the Estimeet API and WebSocket gateway.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jatin-bhatia1/estimeet/backend/internal/api"
	"github.com/jatin-bhatia1/estimeet/backend/internal/config"
	"github.com/jatin-bhatia1/estimeet/backend/internal/hub"
	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
	"github.com/jatin-bhatia1/estimeet/backend/internal/secretbox"
	"github.com/jatin-bhatia1/estimeet/backend/internal/service"
	"github.com/jatin-bhatia1/estimeet/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	box, err := secretbox.New(cfg.Secret)
	if err != nil {
		return err
	}

	// The client is always built: connecting a room with an Atlassian API token
	// needs no server-side credentials. Only the OAuth flow depends on them.
	jiraClient := jira.New(cfg.Jira.ClientID, cfg.Jira.ClientSecret, cfg.Jira.RedirectURI)
	if cfg.Jira.Enabled() {
		slog.Info("jira oauth enabled", "redirect_uri", cfg.Jira.RedirectURI)
	} else {
		slog.Info("jira oauth disabled, api-token connections still available (set JIRA_CLIENT_ID, JIRA_CLIENT_SECRET, JIRA_REDIRECT_URI to enable)")
	}

	svc := service.New(st, hub.New(), box, jiraClient)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewRouter(cfg, svc),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: it would cut long-lived WebSocket connections.
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go purgeCredentials(ctx, svc)
	go purgeStaleRooms(ctx, svc, cfg.RoomRetention)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("estimeet api listening", "addr", cfg.Addr, "db", cfg.DBPath, "room_retention", cfg.RoomRetention)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// purgeStaleRooms deletes sessions nobody has touched for the retention window,
// together with their participants, topics and votes. Estimates are worth
// keeping for a sprint or two, not forever.
func purgeStaleRooms(ctx context.Context, svc *service.Service, retention time.Duration) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		purgeCtx, cancel := context.WithTimeout(ctx, time.Minute)
		n, err := svc.PurgeStaleRooms(purgeCtx, retention)
		cancel()
		switch {
		case err != nil:
			slog.Warn("room purge failed", "error", err)
		case n > 0:
			slog.Info("deleted sessions past the retention window", "rooms", n, "retention", retention)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// purgeCredentials deletes tracker credentials once they are past their
// retention window or their room has closed. Rooms are long-lived and imports
// are not, so this runs on a timer rather than waiting for someone to come back
// and disconnect by hand.
func purgeCredentials(ctx context.Context, svc *service.Service) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		purgeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		n, err := svc.PurgeExpiredCredentials(purgeCtx)
		cancel()
		switch {
		case err != nil:
			slog.Warn("credential purge failed", "error", err)
		case n > 0:
			slog.Info("forgot expired tracker credentials", "connections", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
