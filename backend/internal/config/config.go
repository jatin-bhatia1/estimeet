// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every knob the server needs to boot.
type Config struct {
	Addr           string
	DBPath         string
	AllowedOrigins []string
	AppBaseURL     string
	StaticDir      string
	Secret         string
	LogLevel       slog.Level
	Jira           JiraConfig
	// ContactEmail is shown in the footer. It stays empty unless an operator opts
	// in, so a public deployment never leaks an address by accident.
	ContactEmail string
	IssuesURL    string
	// RoomRetention is how long an untouched session survives before it is
	// deleted with everything in it.
	RoomRetention time.Duration
}

// MinRoomRetention is the floor for ESTIMEET_ROOM_RETENTION_DAYS. A team that
// estimates in one sprint and reviews in the next must still find its session,
// so a shorter window is treated as a misconfiguration and clamped.
const MinRoomRetention = 14 * 24 * time.Hour

// JiraConfig holds the Jira Cloud OAuth 2.0 (3LO) application credentials.
type JiraConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// Enabled reports whether enough Jira credentials are present to run the OAuth dance.
func (j JiraConfig) Enabled() bool {
	return j.ClientID != "" && j.ClientSecret != "" && j.RedirectURI != ""
}

// Load reads the environment and applies development-friendly defaults.
func Load() (Config, error) {
	cfg := Config{
		Addr:           env("ESTIMEET_ADDR", ":8090"),
		DBPath:         env("ESTIMEET_DB_PATH", "data/estimeet.db"),
		AllowedOrigins: splitAndTrim(env("ESTIMEET_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		AppBaseURL:     strings.TrimRight(env("ESTIMEET_APP_BASE_URL", "http://localhost:5173"), "/"),
		StaticDir:      env("ESTIMEET_STATIC_DIR", ""),
		Secret:         env("ESTIMEET_SECRET", ""),
		LogLevel:       parseLevel(env("ESTIMEET_LOG_LEVEL", "info")),
		ContactEmail:   env("ESTIMEET_CONTACT_EMAIL", ""),
		IssuesURL:      env("ESTIMEET_ISSUES_URL", "https://github.com/jatin-bhatia1/estimeet/issues"),
		Jira: JiraConfig{
			ClientID:     env("JIRA_CLIENT_ID", ""),
			ClientSecret: env("JIRA_CLIENT_SECRET", ""),
			RedirectURI:  env("JIRA_REDIRECT_URI", "http://localhost:8090/api/jira/callback"),
		},
	}

	if cfg.Secret == "" {
		if isProd() {
			return Config{}, fmt.Errorf("ESTIMEET_SECRET must be set outside development")
		}
		// Deterministic dev-only fallback so restarts do not invalidate local data.
		cfg.Secret = "estimeet-development-secret-do-not-use-in-production"
	}
	if len(cfg.Secret) < 16 {
		return Config{}, fmt.Errorf("ESTIMEET_SECRET must be at least 16 characters")
	}
	if len(cfg.AllowedOrigins) == 0 {
		return Config{}, fmt.Errorf("ESTIMEET_ALLOWED_ORIGINS must list at least one origin")
	}

	days, err := strconv.Atoi(env("ESTIMEET_ROOM_RETENTION_DAYS", "30"))
	if err != nil || days <= 0 {
		return Config{}, fmt.Errorf("ESTIMEET_ROOM_RETENTION_DAYS must be a positive number of days")
	}
	cfg.RoomRetention = time.Duration(days) * 24 * time.Hour
	if cfg.RoomRetention < MinRoomRetention {
		cfg.RoomRetention = MinRoomRetention
	}
	return cfg, nil
}

func isProd() bool {
	v := strings.ToLower(env("ESTIMEET_ENV", "development"))
	return v == "production" || v == "prod"
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// EnvBool is a small helper for optional boolean flags.
func EnvBool(key string, fallback bool) bool {
	v, err := strconv.ParseBool(env(key, strconv.FormatBool(fallback)))
	if err != nil {
		return fallback
	}
	return v
}
