package jira_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
)

// fakeSite serves the two endpoints story-point write-back touches and records
// what was sent to the issue.
func fakeSite(t *testing.T, fields string) (auth jira.Auth, put *map[string]any, path *string) {
	t.Helper()
	var body map[string]any
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/field":
			_, _ = io.WriteString(w, fields)
		case r.Method == http.MethodPut:
			gotPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return jira.TokenAuth(srv.URL, "ada@example.com", "token"), &body, &gotPath
}

func TestSetStoryPointsUsesTheSitesOwnField(t *testing.T) {
	// The id differs per site, and a text field with a similar name must not win.
	auth, put, path := fakeSite(t, `[
		{"id":"customfield_10001","name":"Story Points Notes","schema":{"type":"string"}},
		{"id":"customfield_10020","name":"Story Points","schema":{"type":"number"}},
		{"id":"customfield_10016","name":"Story point estimate","schema":{"type":"number"}}
	]`)

	if err := jira.New("", "", "").SetStoryPoints(context.Background(), auth, "PROJ-12", 5); err != nil {
		t.Fatalf("SetStoryPoints: %v", err)
	}
	if *path != "/rest/api/3/issue/PROJ-12" {
		t.Fatalf("PUT path = %q", *path)
	}
	fields, _ := (*put)["fields"].(map[string]any)
	if got, ok := fields["customfield_10016"]; !ok || got != float64(5) {
		t.Fatalf("payload = %v, want customfield_10016 = 5", *put)
	}
}

func TestSetStoryPointsReportsAMissingField(t *testing.T) {
	auth, _, _ := fakeSite(t, `[{"id":"summary","name":"Summary","schema":{"type":"string"}}]`)
	err := jira.New("", "", "").SetStoryPoints(context.Background(), auth, "PROJ-12", 5)
	if !errors.Is(err, jira.ErrNoStoryPointsField) {
		t.Fatalf("err = %v, want ErrNoStoryPointsField", err)
	}
}

// The key becomes part of a URL path, so anything but PROJ-123 is refused
// before a request is made.
func TestSetStoryPointsRejectsOddKeys(t *testing.T) {
	auth, _, _ := fakeSite(t, `[]`)
	for _, key := range []string{"", "PROJ", "PROJ-", "../../myself", "PROJ-1/../x", "PROJ-1?a=b", "1-2"} {
		if err := jira.New("", "", "").SetStoryPoints(context.Background(), auth, key, 1); !errors.Is(err, jira.ErrInvalidIssueKey) {
			t.Errorf("key %q: err = %v, want ErrInvalidIssueKey", key, err)
		}
	}
}
