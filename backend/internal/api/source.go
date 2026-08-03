package api

import (
	"net/http"
	"net/url"

	"github.com/jatin-bhatia1/estimeet/backend/internal/service"
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

type sourceConnectRequest struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl"`
	Account  string `json:"account"`
	Token    string `json:"token"`
}

// handleSourceConnect links a room to a tracker with a personal access token.
func (s *server) handleSourceConnect(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req sourceConnectRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.svc.ConnectSource(r.Context(), sess, service.ConnectSourceInput{
		Provider: req.Provider,
		BaseURL:  req.BaseURL,
		Account:  req.Account,
		Token:    req.Token,
	}); err != nil {
		writeError(w, r, err)
		return
	}
	s.respondState(w, r, sess)
}

func (s *server) handleSourceDisconnect(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	if err := s.svc.DisconnectSource(r.Context(), sess); err != nil {
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

func (s *server) handleSourceContainers(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	containers, err := s.svc.SourceContainers(r.Context(), sess, r.URL.Query().Get("query"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"containers": containers})
}

func (s *server) handleSourceGroups(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	q := r.URL.Query()
	groups, err := s.svc.SourceGroups(r.Context(), sess, q.Get("container"), q.Get("query"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (s *server) handleSourceItems(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	q := r.URL.Query()
	items, err := s.svc.SourceItems(r.Context(), sess, q.Get("container"), q.Get("group"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type sourceImportRequest struct {
	Container string   `json:"container"`
	Group     string   `json:"group"`
	Keys      []string `json:"keys"`
}

func (s *server) handleSourceImport(w http.ResponseWriter, r *http.Request) {
	sess, _ := sessionFrom(r.Context())
	var req sourceImportRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := s.svc.ImportSourceItems(r.Context(), sess, req.Container, req.Group, req.Keys)
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

// handleAppConfig serves the handful of settings the UI needs before it knows
// anything about a room: which trackers exist, and where to reach the author.
func (s *server) handleAppConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"sources":            s.svc.Sources(),
		"jiraOauthAvailable": s.svc.JiraOAuthAvailable(),
		"contactEmail":       s.cfg.ContactEmail,
		"issuesUrl":          s.cfg.IssuesURL,
		"credentialTtlHours": int(service.CredentialTTL.Hours()),
	})
}
