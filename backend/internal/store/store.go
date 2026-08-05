// Package store is the persistence layer. It speaks to SQLite through the
// pure-Go modernc.org/sqlite driver, so the project builds without cgo, and to
// PostgreSQL through pgx. Both go through database/sql and share every query:
// the differences are collected in this file.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQLite string

//go:embed schema_postgres.sql
var schemaPostgres string

// dialect is the SQL flavour behind the handle. The queries are written for
// SQLite and adjusted on the way out, because that keeps the common case free
// of branching.
type dialect int

const (
	dialectSQLite dialect = iota
	dialectPostgres
)

// Store owns the database handle.
type Store struct {
	db      *sql.DB
	dialect dialect
}

// Open connects to the database named by dsn. A postgres:// or postgresql://
// URL selects PostgreSQL; anything else is the path to a SQLite file, which is
// created together with its directory when missing.
func Open(dsn string) (*Store, error) {
	if isPostgresURL(dsn) {
		return openPostgres(dsn)
	}
	return openSQLite(dsn)
}

func isPostgresURL(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

func openSQLite(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}

	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)",
		filepath.ToSlash(path),
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite allows a single writer. Serialising connections keeps the code free
	// of retry loops and is more than fast enough for estimation traffic.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if _, err := db.Exec(schemaSQLite); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	s := &Store{db: db, dialect: dialectSQLite}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// postgresWakeUp is how long Open keeps trying to connect. Aurora Serverless
// can be scaled to nothing between sessions and takes several seconds to come
// back, which is a slow start rather than an outage.
const postgresWakeUp = 45 * time.Second

func openPostgres(url string) (*Store, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	// Estimation traffic is small and bursty, and a serverless database charges
	// for idle connections, so the pool stays modest and lets them go.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), postgresWakeUp)
	defer cancel()
	if err := waitForPostgres(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	// pgx sends one statement per round trip, so the schema is applied a
	// statement at a time rather than as one script.
	for _, stmt := range splitStatements(schemaPostgres) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply schema: %w", err)
		}
	}
	return &Store{db: db, dialect: dialectPostgres}, nil
}

func waitForPostgres(ctx context.Context, db *sql.DB) error {
	var lastErr error
	for {
		if err := db.PingContext(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("ping postgres: %w", lastErr)
		case <-time.After(time.Second):
		}
	}
}

// splitStatements cuts a schema script into single statements. It is not a SQL
// parser: it strips line comments and splits on semicolons, which is enough for
// the schema files in this package and nothing else.
func splitStatements(script string) []string {
	var cleaned strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "--") {
			continue
		}
		cleaned.WriteString(line)
		cleaned.WriteString("\n")
	}

	out := make([]string, 0, 16)
	for _, stmt := range strings.Split(cleaned.String(), ";") {
		if stmt = strings.TrimSpace(stmt); stmt != "" {
			out = append(out, stmt)
		}
	}
	return out
}

// rebind turns the ? placeholders every query in this package is written with
// into the $1, $2 form Postgres wants. A ? inside a string literal would be
// rewritten too, so the queries do not contain one.
func (s *Store) rebind(query string) string {
	if s.dialect != dialectPostgres || !strings.Contains(query, "?") {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (s *Store) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.rebind(query), args...)
}

func (s *Store) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.rebind(query), args...)
}

func (s *Store) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.rebind(query), args...)
}

// migrate applies the changes that CREATE TABLE IF NOT EXISTS cannot make to a
// database created by an earlier version. It is idempotent.
func (s *Store) migrate() error {
	if s.dialect == dialectPostgres {
		// Postgres arrived after the schema settled, so there is no older shape
		// to repair: schema_postgres.sql is the whole story.
		return nil
	}
	db := s.db

	// jira_connections was replaced by source_connections. Its rows are stale
	// credentials, and they are dropped rather than migrated: nothing in there
	// was ever meant to outlive a session.
	if _, err := db.Exec(`DELETE FROM jira_connections`); err != nil && !missingTable(err) {
		return fmt.Errorf("clear jira_connections: %w", err)
	}

	// The roster columns were added after the first release, and CREATE TABLE IF
	// NOT EXISTS leaves an existing rooms table untouched.
	for _, stmt := range []string{
		`ALTER TABLE rooms ADD COLUMN expected_size INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE rooms ADD COLUMN expected_names TEXT NOT NULL DEFAULT '[]'`,
	} {
		if _, err := db.Exec(stmt); err != nil && !duplicateColumn(err) {
			return fmt.Errorf("migrate rooms: %w", err)
		}
	}
	return nil
}

func missingTable(err error) bool {
	return strings.Contains(err.Error(), "no such table")
}

func duplicateColumn(err error) bool {
	return strings.Contains(err.Error(), "duplicate column name")
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the handle for tests.
func (s *Store) DB() *sql.DB { return s.db }

func toMillis(t time.Time) int64 { return t.UTC().UnixMilli() }

func fromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

func nullTime(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: toMillis(*t), Valid: true}
}

func timePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromMillis(v.Int64)
	return &t
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func stringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
