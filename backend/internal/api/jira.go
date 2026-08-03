package api

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

func (s *server) handleJiraConnect(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	authURL, err := s.svc.JiraAuthorizeURL(r.Context(), sess)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorizeUrl": authURL})
}

func (s *server) handleJiraDisconnect(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	if err := s.svc.DisconnectJira(r.Context(), sess); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

// handleJiraCallback finishes the OAuth dance and bounces the host back into the
// room. It is protected by the single-use `state` value, not by a bearer token,
// because Atlassian performs a plain browser redirect.
func (s *server) handleJiraCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if oauthErr := q.Get("error"); oauthErr != "" {
		s.redirectToApp(w, r, "", "error", q.Get("error_description"))
		return
	}

	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		s.redirectToApp(w, r, "", "error", "the Jira callback was incomplete")
		return
	}

	roomCode, err := s.svc.JiraCompleteAuth(r.Context(), code, state)
	if err != nil {
		s.redirectToApp(w, r, roomCode, "error", publicMessage(err, "could not connect to Jira"))
		return
	}
	s.redirectToApp(w, r, roomCode, "connected", "")
}

// redirectToApp sends the browser back to the SPA. The destination always comes
// from configuration, so this cannot become an open redirect.
func (s *server) redirectToApp(w http.ResponseWriter, r *http.Request, roomCode, status, message string) {
	target := s.cfg.AppBaseURL + "/"
	if roomCode != "" {
		target = s.cfg.AppBaseURL + "/room/" + url.PathEscape(roomCode)
	}
	q := url.Values{}
	q.Set("jira", status)
	if message != "" {
		q.Set("message", message)
	}
	http.Redirect(w, r, target+"?"+q.Encode(), http.StatusFound)
}

func (s *server) handleJiraProjects(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	projects, err := s.svc.JiraProjects(r.Context(), sess, r.URL.Query().Get("query"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *server) handleJiraEpics(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	q := r.URL.Query()
	epics, err := s.svc.JiraEpics(r.Context(), sess, q.Get("project"), q.Get("query"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"epics": epics})
}

func (s *server) handleJiraEpicIssues(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	issues, err := s.svc.JiraEpicIssues(r.Context(), sess, chi.URLParam(r, "epicKey"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": issues})
}

type jiraImportRequest struct {
	Keys []string `json:"keys"`
}

func (s *server) handleJiraImport(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req jiraImportRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := s.svc.ImportJiraIssues(r.Context(), sess, req.Keys)
	if err != nil {
		writeError(w, r, err)
		return
	}
	state, err := s.svc.State(r.Context(), sess.Room.ID, sess.Participant.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result, "state": state})
}
