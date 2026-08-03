package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
	"github.com/jatin-bhatia1/estimeet/backend/internal/store"
)

// ErrJiraDisabled is returned when the server has no Atlassian OAuth credentials.
// It only affects "Connect with Atlassian"; API-token connections need no server setup.
var ErrJiraDisabled = errors.New("the Jira OAuth app is not configured on this server")

// ErrJiraNotConnected is returned when the room has not linked a Jira site yet.
var ErrJiraNotConnected = errors.New("this room is not connected to Jira")

// JiraOAuthAvailable reports whether the "Connect with Atlassian" button should be offered.
func (s *Service) JiraOAuthAvailable() bool {
	return s.jira != nil && s.jira.OAuthEnabled()
}

// JiraAvailable reports whether any Jira connection method is usable.
func (s *Service) JiraAvailable() bool {
	return s.jira != nil
}

// JiraAuthorizeURL starts the OAuth 2.0 flow for a room (host only).
func (s *Service) JiraAuthorizeURL(ctx context.Context, sess Session) (string, error) {
	if !s.JiraOAuthAvailable() {
		return "", ErrJiraDisabled
	}
	if err := requireHost(sess); err != nil {
		return "", err
	}

	verifier, challenge, err := jira.NewPKCE()
	if err != nil {
		return "", err
	}
	state := uuid.NewString()
	if err := s.store.SaveOAuthState(ctx, store.OAuthState{
		State:         state,
		RoomID:        sess.Room.ID,
		ParticipantID: sess.Participant.ID,
		CodeVerifier:  verifier,
		ExpiresAt:     s.now().Add(10 * time.Minute),
	}); err != nil {
		return "", err
	}
	return s.jira.AuthorizeURL(state, challenge), nil
}

// JiraCompleteAuth finishes the OAuth callback and stores the encrypted tokens.
// It returns the room code so the caller can redirect the user back.
func (s *Service) JiraCompleteAuth(ctx context.Context, code, state string) (string, error) {
	if !s.JiraOAuthAvailable() {
		return "", ErrJiraDisabled
	}
	pending, err := s.store.ConsumeOAuthState(ctx, state)
	if err != nil {
		return "", fmt.Errorf("%w: the authorization request expired, please try again", domain.ErrForbidden)
	}

	token, err := s.jira.Exchange(ctx, code, pending.CodeVerifier)
	if err != nil {
		return "", err
	}
	resources, err := s.jira.AccessibleResources(ctx, token.AccessToken)
	if err != nil {
		return "", err
	}
	if len(resources) == 0 {
		return "", fmt.Errorf("%w: no Jira site was granted to this app", domain.ErrConflict)
	}

	site := resources[0]
	if err := s.persistJiraToken(ctx, pending.RoomID, site, token); err != nil {
		return "", err
	}

	room, err := s.store.RoomByID(ctx, pending.RoomID)
	if err != nil {
		return "", err
	}
	s.publish(room.ID, "jira.connected", map[string]string{"site": site.Name})
	return room.Code, nil
}

func (s *Service) persistJiraToken(ctx context.Context, roomID string, site jira.Resource, token jira.Token) error {
	accessEnc, err := s.box.Seal(token.AccessToken)
	if err != nil {
		return err
	}
	var refreshEnc []byte
	if token.RefreshToken != "" {
		if refreshEnc, err = s.box.Seal(token.RefreshToken); err != nil {
			return err
		}
	}
	return s.store.SaveJiraConnection(ctx, store.JiraConnection{
		RoomID:    roomID,
		AuthType:  store.JiraAuthOAuth,
		CloudID:   site.ID,
		SiteURL:   strings.TrimRight(site.URL, "/"),
		SiteName:  site.Name,
		ExpiresAt: token.ExpiresAt,
	}, accessEnc, refreshEnc)
}

// ConnectJiraToken links a room to a Jira Cloud site with an Atlassian account
// email and API token (host only). The credentials are verified against the
// site before the token is encrypted and stored.
func (s *Service) ConnectJiraToken(ctx context.Context, sess Session, siteURL, email, apiToken string) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	if s.jira == nil {
		return ErrJiraDisabled
	}

	// The site comes from a user, so only ever talk to a real Jira Cloud host.
	site, err := jira.NormalizeSiteURL(siteURL)
	if err != nil {
		return fmt.Errorf("%w: %s", domain.ErrInvalid, err.Error())
	}
	email = clean(email, 254)
	apiToken = strings.TrimSpace(apiToken)
	if email == "" || apiToken == "" {
		return fmt.Errorf("%w: your Atlassian email and API token are both required", domain.ErrInvalid)
	}

	auth := jira.TokenAuth(site, email, apiToken)
	account, err := s.jira.Myself(ctx, auth)
	if err != nil {
		var apiErr *jira.APIError
		if errors.As(err, &apiErr) && apiErr.Unauthorized() {
			return fmt.Errorf("%w: Jira rejected those credentials", domain.ErrForbidden)
		}
		return err
	}

	accessEnc, err := s.box.Seal(apiToken)
	if err != nil {
		return err
	}
	siteName := strings.TrimPrefix(site, "https://")
	if err := s.store.SaveJiraConnection(ctx, store.JiraConnection{
		RoomID:       sess.Room.ID,
		AuthType:     store.JiraAuthToken,
		SiteURL:      site,
		SiteName:     siteName,
		AccountEmail: email,
		// API tokens have no scheduled expiry; only revocation ends them.
		ExpiresAt: s.now().AddDate(10, 0, 0),
	}, accessEnc, nil); err != nil {
		return err
	}

	s.publish(sess.Room.ID, "jira.connected", map[string]string{"site": siteName, "account": account.DisplayName})
	return nil
}

// DisconnectJira drops the stored tokens for a room (host only).
func (s *Service) DisconnectJira(ctx context.Context, sess Session) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	if err := s.store.DeleteJiraConnection(ctx, sess.Room.ID); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "jira.disconnected", nil)
	return nil
}

// jiraAccess returns a usable connection plus ready-to-send credentials,
// refreshing an OAuth access token when it has gone stale.
func (s *Service) jiraAccess(ctx context.Context, roomID string) (store.JiraConnection, jira.Auth, error) {
	if s.jira == nil {
		return store.JiraConnection{}, jira.Auth{}, ErrJiraDisabled
	}
	conn, accessEnc, refreshEnc, err := s.store.RawJiraConnection(ctx, roomID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return store.JiraConnection{}, jira.Auth{}, ErrJiraNotConnected
		}
		return store.JiraConnection{}, jira.Auth{}, err
	}

	accessToken, err := s.box.Open(accessEnc)
	if err != nil {
		return store.JiraConnection{}, jira.Auth{}, errUnreadableJiraToken
	}

	if conn.AuthType == store.JiraAuthToken {
		return conn, jira.TokenAuth(conn.SiteURL, conn.AccountEmail, accessToken), nil
	}
	if !conn.Expired(s.now()) {
		return conn, jira.OAuthAuth(conn.CloudID, accessToken), nil
	}

	if len(refreshEnc) == 0 {
		return store.JiraConnection{}, jira.Auth{}, errJiraSessionExpired
	}
	refreshToken, err := s.box.Open(refreshEnc)
	if err != nil {
		return store.JiraConnection{}, jira.Auth{}, errUnreadableJiraToken
	}
	token, err := s.jira.Refresh(ctx, refreshToken)
	if err != nil {
		return store.JiraConnection{}, jira.Auth{}, errJiraSessionExpired
	}
	if err := s.persistJiraToken(ctx, roomID, jira.Resource{ID: conn.CloudID, Name: conn.SiteName, URL: conn.SiteURL}, token); err != nil {
		return store.JiraConnection{}, jira.Auth{}, err
	}
	conn.ExpiresAt = token.ExpiresAt
	return conn, jira.OAuthAuth(conn.CloudID, token.AccessToken), nil
}

var (
	errUnreadableJiraToken = errors.New("the stored Jira credentials are unreadable, please reconnect")
	errJiraSessionExpired  = fmt.Errorf("%w: the Jira session expired, please reconnect", domain.ErrForbidden)
)

// JiraProjects lists the projects of the connected site.
func (s *Service) JiraProjects(ctx context.Context, sess Session, query string) ([]jira.Project, error) {
	_, auth, err := s.jiraAccess(ctx, sess.Room.ID)
	if err != nil {
		return nil, err
	}
	return s.jira.Projects(ctx, auth, clean(query, 100))
}

// JiraEpics lists epics, optionally narrowed to a project and/or a search term.
func (s *Service) JiraEpics(ctx context.Context, sess Session, projectKey, query string) ([]jira.Issue, error) {
	conn, auth, err := s.jiraAccess(ctx, sess.Room.ID)
	if err != nil {
		return nil, err
	}
	issues, err := s.jira.SearchEpics(ctx, auth, clean(projectKey, 64), clean(query, 100))
	if err != nil {
		return nil, err
	}
	return withBrowseURLs(issues, conn.SiteURL), nil
}

// JiraEpicIssues lists the stories under an epic, ready to be imported.
func (s *Service) JiraEpicIssues(ctx context.Context, sess Session, epicKey string) ([]jira.Issue, error) {
	if strings.TrimSpace(epicKey) == "" {
		return nil, fmt.Errorf("%w: an epic key is required", domain.ErrInvalid)
	}
	conn, auth, err := s.jiraAccess(ctx, sess.Room.ID)
	if err != nil {
		return nil, err
	}
	issues, err := s.jira.IssuesInEpic(ctx, auth, epicKey)
	if err != nil {
		return nil, err
	}
	return withBrowseURLs(issues, conn.SiteURL), nil
}

// ImportResult reports what an import actually did.
type ImportResult struct {
	Imported int      `json:"imported"`
	Skipped  []string `json:"skipped"`
}

// ImportJiraIssues turns Jira issues into topics, skipping keys already present
// so the host can re-run an import safely (host only).
func (s *Service) ImportJiraIssues(ctx context.Context, sess Session, keys []string) (ImportResult, error) {
	if err := requireHost(sess); err != nil {
		return ImportResult{}, err
	}
	if len(keys) == 0 {
		return ImportResult{}, fmt.Errorf("%w: select at least one issue", domain.ErrInvalid)
	}
	if len(keys) > MaxTopicsPerImport {
		return ImportResult{}, fmt.Errorf("%w: import at most %d issues at a time", domain.ErrInvalid, MaxTopicsPerImport)
	}

	conn, auth, err := s.jiraAccess(ctx, sess.Room.ID)
	if err != nil {
		return ImportResult{}, err
	}

	quoted := make([]string, 0, len(keys))
	for _, k := range keys {
		k = clean(k, 64)
		if k == "" {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(k, `"`, ``)+`"`)
	}
	if len(quoted) == 0 {
		return ImportResult{}, fmt.Errorf("%w: select at least one issue", domain.ErrInvalid)
	}

	jql := fmt.Sprintf("issuekey IN (%s) ORDER BY created ASC", strings.Join(quoted, ", "))
	issues, err := s.jira.SearchJQL(ctx, auth, jql, len(quoted))
	if err != nil {
		return ImportResult{}, err
	}

	existing, err := s.store.ExistingExternalKeys(ctx, sess.Room.ID)
	if err != nil {
		return ImportResult{}, err
	}
	current, err := s.store.ListTopics(ctx, sess.Room.ID)
	if err != nil {
		return ImportResult{}, err
	}
	pos, err := s.store.NextTopicPosition(ctx, sess.Room.ID)
	if err != nil {
		return ImportResult{}, err
	}

	result := ImportResult{Skipped: []string{}}
	now := s.now()
	created := make([]domain.Topic, 0, len(issues))

	for _, issue := range issues {
		if existing[issue.Key] {
			result.Skipped = append(result.Skipped, issue.Key)
			continue
		}
		if len(current)+len(created) >= MaxTopicsPerRoom {
			result.Skipped = append(result.Skipped, issue.Key)
			continue
		}
		key := issue.Key
		url := conn.SiteURL + "/browse/" + issue.Key
		t := domain.Topic{
			ID:          uuid.NewString(),
			RoomID:      sess.Room.ID,
			Title:       clean(issue.Summary, MaxTopicTitleLen),
			Description: clean(issue.Description, MaxTopicDescLen),
			ExternalKey: &key,
			ExternalURL: &url,
			Position:    pos,
			Status:      domain.StatusPending,
			CreatedAt:   now,
		}
		if t.Title == "" {
			t.Title = issue.Key
		}
		if err := s.store.CreateTopic(ctx, t); err != nil {
			return ImportResult{}, err
		}
		created = append(created, t)
		existing[issue.Key] = true
		pos++
	}
	result.Imported = len(created)

	if sess.Room.Mode == domain.ModeSync && sess.Room.CurrentTopicID == nil && len(created) > 0 {
		if err := s.store.SetCurrentTopic(ctx, sess.Room.ID, &created[0].ID); err != nil {
			return ImportResult{}, err
		}
	}

	s.publish(sess.Room.ID, "topics.imported", map[string]int{"count": result.Imported})
	return result, nil
}

func withBrowseURLs(issues []jira.Issue, siteURL string) []jira.Issue {
	for i := range issues {
		issues[i].URL = siteURL + "/browse/" + issues[i].Key
	}
	return issues
}
