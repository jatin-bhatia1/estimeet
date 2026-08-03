package jira_test

import (
	"testing"

	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
)

func TestNormalizeSiteURLAccepts(t *testing.T) {
	cases := map[string]string{
		"https://acme.atlassian.net":        "https://acme.atlassian.net",
		"https://acme.atlassian.net/":       "https://acme.atlassian.net",
		"acme.atlassian.net":                "https://acme.atlassian.net",
		"  https://ACME.atlassian.net/jira": "https://acme.atlassian.net",
	}
	for in, want := range cases {
		got, err := jira.NormalizeSiteURL(in)
		if err != nil {
			t.Fatalf("NormalizeSiteURL(%q) returned %v", in, err)
		}
		if got != want {
			t.Fatalf("NormalizeSiteURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// The site URL is supplied by a room host, so anything that is not a Jira Cloud
// origin must be refused before the server makes a request to it.
func TestNormalizeSiteURLRejectsSSRFAttempts(t *testing.T) {
	cases := []string{
		"",
		"http://acme.atlassian.net",
		"https://169.254.169.254",
		"https://localhost",
		"https://acme.atlassian.net:8443",
		"https://user:pass@acme.atlassian.net",
		"https://acme.atlassian.net.attacker.example",
		"https://atlassian.net",
		"https://evil.com/?x=acme.atlassian.net",
		"file:///etc/passwd",
	}
	for _, in := range cases {
		if got, err := jira.NormalizeSiteURL(in); err == nil {
			t.Fatalf("NormalizeSiteURL(%q) = %q, want an error", in, got)
		}
	}
}
