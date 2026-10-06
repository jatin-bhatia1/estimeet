package config

import "testing"

// The parts are assembled rather than pasted together by hand precisely so that
// a password full of punctuation survives, which is what this pins down.
func TestDatabaseURLFromParts(t *testing.T) {
	t.Setenv("ESTIMEET_DB_HOST", "estimeet.cluster-abc.eu-west-1.rds.amazonaws.com")
	t.Setenv("ESTIMEET_DB_NAME", "estimeet_db")
	t.Setenv("ESTIMEET_DB_USER", "sdcowner")
	t.Setenv("ESTIMEET_DB_PASSWORD", "p@ss/w:rd?")

	want := "postgres://sdcowner:p%40ss%2Fw%3Ard%3F@estimeet.cluster-abc.eu-west-1.rds.amazonaws.com:5432/estimeet_db?sslmode=require"
	if got := databaseURL(); got != want {
		t.Fatalf("databaseURL() = %q, want %q", got, want)
	}
}

func TestDatabaseURLPrefersExplicitURL(t *testing.T) {
	t.Setenv("ESTIMEET_DB_URL", "postgres://someone@elsewhere:5432/other")
	t.Setenv("ESTIMEET_DB_HOST", "ignored.example.com")

	if got := databaseURL(); got != "postgres://someone@elsewhere:5432/other" {
		t.Fatalf("databaseURL() = %q", got)
	}
}

// Without a host there is nothing to connect to, and the SQLite default has to
// survive: this is the path every local run takes.
func TestDatabaseURLEmptyWithoutHost(t *testing.T) {
	t.Setenv("ESTIMEET_DB_USER", "sdcowner")

	if got := databaseURL(); got != "" {
		t.Fatalf("databaseURL() = %q, want empty", got)
	}
}

// A canvas wants a value for every variable in every stage, so "sqlite" has to
// keep a stage on the file even when the other parts hold placeholders.
func TestDatabaseURLSqlitePlaceholderKeepsTheFile(t *testing.T) {
	for _, host := range []string{"sqlite", "SQLite", " sqlite "} {
		t.Setenv("ESTIMEET_DB_HOST", host)
		t.Setenv("ESTIMEET_DB_USER", "unused")
		t.Setenv("ESTIMEET_DB_PASSWORD", "unused")

		if got := databaseURL(); got != "" {
			t.Fatalf("host %q: databaseURL() = %q, want empty", host, got)
		}
	}
}
