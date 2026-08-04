package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "estimeet.conf")
	body := "" +
		"# a comment\n" +
		"\n" +
		"ESTIMEET_CONTACT_EMAIL = hello@example.com\n" +
		"export ESTIMEET_ISSUES_URL=https://example.com/issues # where to complain\n" +
		"ESTIMEET_APP_BASE_URL = \"https://estimeet.app\"\n" +
		"ESTIMEET_ADDR = :9999\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// A value already in the environment must survive: containers and CI pass
	// settings that way and must stay in charge.
	t.Setenv("ESTIMEET_ADDR", ":8090")

	// loadFile writes to the real environment, so put it back afterwards.
	for _, key := range []string{"ESTIMEET_CONTACT_EMAIL", "ESTIMEET_ISSUES_URL", "ESTIMEET_APP_BASE_URL"} {
		t.Cleanup(func() { _ = os.Unsetenv(key) })
	}

	if err := loadFile(path); err != nil {
		t.Fatalf("load: %v", err)
	}

	for _, tc := range []struct{ key, want string }{
		{"ESTIMEET_CONTACT_EMAIL", "hello@example.com"},
		{"ESTIMEET_ISSUES_URL", "https://example.com/issues"},
		{"ESTIMEET_APP_BASE_URL", "https://estimeet.app"},
		{"ESTIMEET_ADDR", ":8090"},
	} {
		if got := os.Getenv(tc.key); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestLoadFileIgnoresAMissingFile(t *testing.T) {
	if err := loadFile(filepath.Join(t.TempDir(), "nothing-here.conf")); err != nil {
		t.Fatalf("a missing file must be fine, got %v", err)
	}
}
