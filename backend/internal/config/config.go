// Package config loads runtime configuration from a settings file and the
// environment, with the environment taking precedence.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every knob the server needs to boot.
type Config struct {
	Addr   string
	DBPath string
	// DBURL points at a PostgreSQL server. When it is set the SQLite file is
	// ignored, which is how the same image runs on a laptop and on a managed
	// database.
	DBURL          string
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
// Settings may also come from a file (see loadFile); the environment wins.
func Load() (Config, error) {
	if err := loadFile(env("ESTIMEET_CONFIG_FILE", DefaultFile)); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Addr:           listenAddr(),
		DBPath:         env("ESTIMEET_DB_PATH", "data/estimeet.db"),
		DBURL:          databaseURL(),
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

// DataSource is what the store should connect to: the PostgreSQL URL when there
// is one, the SQLite file otherwise.
func (c Config) DataSource() string {
	if c.DBURL != "" {
		return c.DBURL
	}
	return c.DBPath
}

// databaseURL resolves the PostgreSQL DSN. ESTIMEET_DB_URL wins; otherwise one
// is built from the parts, because managed platforms hand the host, database
// and credentials over as separate variables, and stitching them into a URL by
// hand is where a password with a punctuation mark in it goes wrong. Nothing is
// assembled without a host, so the SQLite default survives.
func databaseURL() string {
	if u := env("ESTIMEET_DB_URL", ""); u != "" {
		return u
	}
	host := env("ESTIMEET_DB_HOST", "")
	if host == "" {
		return ""
	}
	u := url.URL{
		Scheme:   "postgres",
		Host:     net.JoinHostPort(host, env("ESTIMEET_DB_PORT", "5432")),
		Path:     "/" + env("ESTIMEET_DB_NAME", "estimeet"),
		RawQuery: "sslmode=" + env("ESTIMEET_DB_SSLMODE", "require"),
	}
	if user := env("ESTIMEET_DB_USER", ""); user != "" {
		// Read the password without trimming: a surrounding space is unlikely but
		// silently dropping one would be maddening to debug.
		if pass := os.Getenv("ESTIMEET_DB_PASSWORD"); pass != "" {
			u.User = url.UserPassword(user, pass)
		} else {
			u.User = url.User(user)
		}
	}
	return u.String()
}

// SafeDataSource is DataSource without the credentials, because the startup log
// says which database it opened and a URL carries a password.
func (c Config) SafeDataSource() string {
	if c.DBURL == "" {
		return c.DBPath
	}
	u, err := url.Parse(c.DBURL)
	if err != nil {
		return "postgres"
	}
	return u.Scheme + "://" + u.Host + u.Path
}

// listenAddr resolves the address to listen on. Free hosts hand the port to the
// process as PORT and route to whatever it binds, so PORT fills in when
// ESTIMEET_ADDR says nothing. A bare number is accepted as well as ":8080".
func listenAddr() string {
	if addr := env("ESTIMEET_ADDR", ""); addr != "" {
		return addr
	}
	if port := env("PORT", ""); port != "" {
		return ":" + strings.TrimPrefix(port, ":")
	}
	return ":8090"
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
