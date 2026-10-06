package jira_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
)

// fakeSite serves the endpoints story-point write-back touches and records what
// was sent to the issue. editable lists the field ids on the issue's edit screen.
func fakeSite(t *testing.T, fields string, editable ...string) (auth jira.Auth, put *map[string]any, path *string) {
	t.Helper()
	body := map[string]any{}
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/field":
			_, _ = io.WriteString(w, fields)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/editmeta"):
			meta := map[string]map[string]any{"fields": {}}
			for _, id := range editable {
				meta["fields"][id] = map[string]any{"name": id}
			}
			_ = json.NewEncoder(w).Encode(meta)
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

func sentField(t *testing.T, put map[string]any) (string, any) {
	t.Helper()
	fields, _ := put["fields"].(map[string]any)
	for id, v := range fields {
		return id, v
	}
	t.Fatalf("nothing was written: %v", put)
	return "", nil
}

// A real site carries both: the board's "Story point estimate" and an older
// "Story Points", and only the older one is on the issue's edit screen. Writing
// to the other is refused, so the editable one has to win.
func TestSetStoryPointsWritesTheFieldOnTheEditScreen(t *testing.T) {
	auth, put, path := fakeSite(t, `[
		{"id":"customfield_10016","name":"Story point estimate","schema":{"type":"number"}},
		{"id":"customfield_10026","name":"Story Points","schema":{"type":"number"}},
		{"id":"customfield_11763","name":"Remaining Story Points","schema":{"type":"number"}}
	]`, "customfield_10026", "summary")

	if err := jira.New("", "", "").SetStoryPoints(context.Background(), auth, "PROJ-12", 5); err != nil {
		t.Fatalf("SetStoryPoints: %v", err)
	}
	if *path != "/rest/api/3/issue/PROJ-12" {
		t.Fatalf("PUT path = %q", *path)
	}
	if id, v := sentField(t, *put); id != "customfield_10026" || v != float64(5) {
		t.Fatalf("wrote %s = %v, want customfield_10026 = 5", id, v)
	}
}

func TestSetStoryPointsPrefersTheBoardFieldWhenBothAreEditable(t *testing.T) {
	auth, put, _ := fakeSite(t, `[
		{"id":"customfield_10026","name":"Story Points","schema":{"type":"number"}},
		{"id":"customfield_10016","name":"Story point estimate","schema":{"type":"number"}}
	]`, "customfield_10026", "customfield_10016")

	if err := jira.New("", "", "").SetStoryPoints(context.Background(), auth, "PROJ-12", 3); err != nil {
		t.Fatalf("SetStoryPoints: %v", err)
	}
	if id, _ := sentField(t, *put); id != "customfield_10016" {
		t.Fatalf("wrote %s, want customfield_10016", id)
	}
}

func TestSetStoryPointsReportsAMissingField(t *testing.T) {
	auth, _, _ := fakeSite(t, `[{"id":"summary","name":"Summary","schema":{"type":"string"}}]`)
	err := jira.New("", "", "").SetStoryPoints(context.Background(), auth, "PROJ-12", 5)
	if !errors.Is(err, jira.ErrNoStoryPointsField) {
		t.Fatalf("err = %v, want ErrNoStoryPointsField", err)
	}
}

func TestSetStoryPointsReportsAFieldThatIsNotOnTheEditScreen(t *testing.T) {
	auth, put, _ := fakeSite(t, `[{"id":"customfield_10016","name":"Story point estimate","schema":{"type":"number"}}]`, "summary")
	err := jira.New("", "", "").SetStoryPoints(context.Background(), auth, "PROJ-12", 5)
	if !errors.Is(err, jira.ErrNoStoryPointsField) {
		t.Fatalf("err = %v, want ErrNoStoryPointsField", err)
	}
	if len(*put) != 0 {
		t.Fatalf("nothing should have been written, got %v", *put)
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
