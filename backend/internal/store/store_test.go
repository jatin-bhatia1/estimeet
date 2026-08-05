package store

import "testing"

// The whole package writes its queries once, in SQLite's ? form, and relies on
// rebind to translate them. If that translation ever slipped, every Postgres
// query would break at once, so it is worth pinning down.
func TestRebind(t *testing.T) {
	query := `SELECT id FROM rooms WHERE code = ? AND created_at > ?`

	sqlite := &Store{dialect: dialectSQLite}
	if got := sqlite.rebind(query); got != query {
		t.Fatalf("sqlite query was rewritten: %q", got)
	}

	postgres := &Store{dialect: dialectPostgres}
	want := `SELECT id FROM rooms WHERE code = $1 AND created_at > $2`
	if got := postgres.rebind(query); got != want {
		t.Fatalf("rebind() = %q, want %q", got, want)
	}
}

// pgx sends one statement per round trip, so the schema has to be fed to it in
// pieces rather than as a single script.
func TestSplitStatements(t *testing.T) {
	script := `
-- a comment, and a stray ; inside it
CREATE TABLE rooms (
    id TEXT PRIMARY KEY
);

CREATE INDEX idx_rooms_id ON rooms (id);
`
	got := splitStatements(script)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %#v", len(got), got)
	}
	if got[1] != "CREATE INDEX idx_rooms_id ON rooms (id)" {
		t.Fatalf("second statement = %q", got[1])
	}
}
