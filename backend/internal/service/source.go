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
	"github.com/jatin-bhatia1/estimeet/backend/internal/source"
	"github.com/jatin-bhatia1/estimeet/backend/internal/store"
)

// CredentialTTL is how long a room may keep tracker credentials. They exist to
// pull a backlog in at the start of a session, not to be stored indefinitely,
// so they are deleted a day after they were handed over whether or not anyone
// disconnects.
const CredentialTTL = 24 * time.Hour

// ErrJiraOAuthDisabled is returned when the server has no Atlassian OAuth app.
// It only affects "Connect with Atlassian"; token connections need no setup.
var ErrJiraOAuthDisabled = errors.New("the Jira OAuth app is not configured on this server")

// ErrNotConnected is returned when the room has no usable tracker connection,
// which includes the case where its credentials have just been forgotten.
var ErrNotConnected = errors.New("this room is not connected to a backlog, or the connection has expired")

var errUnreadableCredentials = errors.New("the stored credentials are unreadable, please reconnect")

// Sources lists the trackers this server can import from.
func (s *Service) Sources() []source.Descriptor { return s.sources.Descriptors() }

// JiraOAuthAvailable reports whether "Connect with Atlassian" should be offered.
func (s *Service) JiraOAuthAvailable() bool {
	return s.jira != nil && s.jira.OAuthEnabled()
}

// PurgeExpiredCredentials deletes credentials past their retention deadline and
// any belonging to a closed room. It is safe to call on a timer.
func (s *Service) PurgeExpiredCredentials(ctx context.Context) (int64, error) {
	return s.store.PurgeSourceConnections(ctx, s.now())
}

// ------------------------------------------------------------------ connect

// ConnectSourceInput is a personal-access-token connection request.
type ConnectSourceInput struct {
	Provider string
	BaseURL  string
	Account  string
	Token    string
}

// ConnectSource links a room to a tracker with a personal access token (host
// only). The credentials are proven against the tracker before they are
// encrypted and stored, so a typo fails here rather than at import time.
func (s *Service) ConnectSource(ctx context.Context, sess Session, in ConnectSourceInput) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	kind, ok := source.ParseKind(in.Provider)
	if !ok {
		return fmt.Errorf("%w: unknown backlog source %q", domain.ErrInvalid, in.Provider)
	}
	provider, err := s.sources.Get(kind)
	if err != nil {
		return fmt.Errorf("%w: %s", domain.ErrInvalid, err.Error())
	}

	token := strings.TrimSpace(in.Token)
	if token == "" {
		return fmt.Errorf("%w: a token is required", domain.ErrInvalid)
	}

	creds := source.Credentials{Kind: kind, Token: token}
	var displayName string

	switch kind {
	case source.KindJira:
		// The site comes from a user, so only ever talk to a real Jira Cloud host.
		site, err := jira.NormalizeSiteURL(in.BaseURL)
		if err != nil {
			return fmt.Errorf("%w: %s", domain.ErrInvalid, err.Error())
		}
		email := clean(in.Account, 254)
		if email == "" {
			return fmt.Errorf("%w: your Atlassian account email is required", domain.ErrInvalid)
		}
		creds.BaseURL, creds.Account = site, email
		displayName = strings.TrimPrefix(site, "https://")

	case source.KindAzure:
		org, err := source.NormalizeAzureOrg(in.Account)
		if err != nil {
			return fmt.Errorf("%w: %s", domain.ErrInvalid, err.Error())
		}
		creds.Account = org
		creds.BaseURL = "https://dev.azure.com/" + org
		displayName = "dev.azure.com/" + org

	case source.KindGitHub:
		creds.BaseURL = "https://github.com"
	}

	account, err := provider.Verify(ctx, creds)
	if err != nil {
		return publicSourceError(kind, err)
	}
	if kind == source.KindGitHub {
		// GitHub identifies the connection by whoever the token belongs to.
		creds.Account = account.Email
		displayName = "github.com/" + account.Email
	}

	sealed, err := s.box.Seal(token)
	if err != nil {
		return err
	}
	now := s.now()
	if err := s.store.SaveSourceConnection(ctx, store.SourceConnection{
		RoomID:   sess.Room.ID,
		Provider: string(kind),
		AuthType: store.AuthToken,
		BaseURL:  creds.BaseURL,
		Name:     displayName,
		Account:  creds.Account,
		// A personal access token has no scheduled expiry of its own.
		TokenExpiresAt: now.Add(CredentialTTL),
		ExpiresAt:      now.Add(CredentialTTL),
	}, sealed, nil); err != nil {
		return err
	}

	s.publish(sess.Room.ID, "source.connected", map[string]string{
		"provider": string(kind),
		"name":     displayName,
		"account":  account.Name,
	})
	return nil
}

// DisconnectSource forgets a room's credentials immediately (host only).
func (s *Service) DisconnectSource(ctx context.Context, sess Session) error {
	if err := requireHost(sess); err != nil {
		return err
	}
	if err := s.store.DeleteSourceConnection(ctx, sess.Room.ID); err != nil {
		return err
	}
	s.publish(sess.Room.ID, "source.disconnected", nil)
	return nil
}

// ------------------------------------------------------------------ browsing

// SourceContainers lists the projects or repositories of the connection.
func (s *Service) SourceContainers(ctx context.Context, sess Session, query string) ([]source.Container, error) {
	conn, provider, creds, err := s.sourceAccess(ctx, sess.Room.ID)
	if err != nil {
		return nil, err
	}
	out, err := provider.Containers(ctx, creds, clean(query, 100))
	if err != nil {
		return nil, publicSourceError(source.Kind(conn.Provider), err)
	}
	return out, nil
}

// SourceGroups lists the epics or milestones inside a container.
func (s *Service) SourceGroups(ctx context.Context, sess Session, container, query string) ([]source.Item, error) {
	conn, provider, creds, err := s.sourceAccess(ctx, sess.Room.ID)
	if err != nil {
		return nil, err
	}
	out, err := provider.Groups(ctx, creds, clean(container, 200), clean(query, 100))
	if err != nil {
		return nil, publicSourceError(source.Kind(conn.Provider), err)
	}
	return out, nil
}

// SourceItems lists what would be imported from a group.
func (s *Service) SourceItems(ctx context.Context, sess Session, container, group string) ([]source.Item, error) {
	if strings.TrimSpace(group) == "" {
		return nil, fmt.Errorf("%w: pick an epic or milestone first", domain.ErrInvalid)
	}
	conn, provider, creds, err := s.sourceAccess(ctx, sess.Room.ID)
	if err != nil {
		return nil, err
	}
	out, err := provider.Items(ctx, creds, clean(container, 200), clean(group, 200))
	if err != nil {
		return nil, publicSourceError(source.Kind(conn.Provider), err)
	}
	return out, nil
}

// ------------------------------------------------------------------- import

// ImportResult reports what an import actually did.
type ImportResult struct {
	Imported int      `json:"imported"`
	Skipped  []string `json:"skipped"`
}

// ImportSourceItems turns the selected items into topics, skipping keys already
// in the backlog so the host can re-run an import safely (host only).
func (s *Service) ImportSourceItems(ctx context.Context, sess Session, container, group string, keys []string) (ImportResult, error) {
	if err := requireHost(sess); err != nil {
		return ImportResult{}, err
	}
	if len(keys) == 0 {
		return ImportResult{}, fmt.Errorf("%w: select at least one item", domain.ErrInvalid)
	}
	if len(keys) > MaxTopicsPerImport {
		return ImportResult{}, fmt.Errorf("%w: import at most %d items at a time", domain.ErrInvalid, MaxTopicsPerImport)
	}

	// Re-reading the group is what keeps the import honest: titles come from the
	// tracker, never from the request body.
	items, err := s.SourceItems(ctx, sess, container, group)
	if err != nil {
		return ImportResult{}, err
	}
	wanted := make(map[string]bool, len(keys))
	for _, k := range keys {
		wanted[clean(k, 200)] = true
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
	created := make([]domain.Topic, 0, len(wanted))

	for _, item := range items {
		if !wanted[item.Key] {
			continue
		}
		if existing[item.Key] || len(current)+len(created) >= MaxTopicsPerRoom {
			result.Skipped = append(result.Skipped, item.Key)
			continue
		}
		key, url := item.Key, item.URL
		t := domain.Topic{
			ID:          uuid.NewString(),
			RoomID:      sess.Room.ID,
			Title:       clean(item.Title, MaxTopicTitleLen),
			Description: clean(item.Description, MaxTopicDescLen),
			ExternalKey: &key,
			Position:    pos,
			Status:      domain.StatusPending,
			CreatedAt:   now,
		}
		if url != "" {
			t.ExternalURL = &url
		}
		if t.Title == "" {
			t.Title = item.Key
		}
		if err := s.store.CreateTopic(ctx, t); err != nil {
			return ImportResult{}, err
		}
		created = append(created, t)
		existing[item.Key] = true
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

// ------------------------------------------------------------------- access

// sourceAccess resolves a room's connection into ready-to-use credentials,
// refreshing an OAuth access token when it has gone stale.
func (s *Service) sourceAccess(ctx context.Context, roomID string) (store.SourceConnection, source.Provider, source.Credentials, error) {
	fail := func(err error) (store.SourceConnection, source.Provider, source.Credentials, error) {
		return store.SourceConnection{}, nil, source.Credentials{}, err
	}

	conn, tokenEnc, refreshEnc, err := s.store.RawSourceConnection(ctx, roomID, s.now())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fail(ErrNotConnected)
		}
		return fail(err)
	}
	kind, ok := source.ParseKind(conn.Provider)
	if !ok {
		return fail(ErrNotConnected)
	}
	provider, err := s.sources.Get(kind)
	if err != nil {
		return fail(err)
	}

	token, err := s.box.Open(tokenEnc)
	if err != nil {
		return fail(errUnreadableCredentials)
	}

	creds := source.Credentials{
		Kind:    kind,
		OAuth:   conn.AuthType == store.AuthOAuth,
		BaseURL: conn.BaseURL,
		CloudID: conn.CloudID,
		Account: conn.Account,
		Token:   token,
	}
	if !creds.OAuth || !conn.TokenStale(s.now()) {
		return conn, provider, creds, nil
	}

	// OAuth only: swap the expired access token for a fresh one.
	if len(refreshEnc) == 0 {
		return fail(errSessionExpired)
	}
	refreshToken, err := s.box.Open(refreshEnc)
	if err != nil {
		return fail(errUnreadableCredentials)
	}
	fresh, err := s.jira.Refresh(ctx, refreshToken)
	if err != nil {
		return fail(errSessionExpired)
	}
	if err := s.saveJiraOAuth(ctx, conn.RoomID, jira.Resource{ID: conn.CloudID, Name: conn.Name, URL: conn.BaseURL}, fresh, conn.ExpiresAt); err != nil {
		return fail(err)
	}
	conn.TokenExpiresAt = fresh.ExpiresAt
	creds.Token = fresh.AccessToken
	return conn, provider, creds, nil
}

var errSessionExpired = fmt.Errorf("%w: the tracker session expired, please reconnect", domain.ErrForbidden)

// publicSourceError turns a tracker's answer into something safe to show. The
// upstream detail is kept only for the cases a host can act on.
func publicSourceError(kind source.Kind, err error) error {
	var srcErr *source.Error
	if errors.As(err, &srcErr) && srcErr.Unauthorized() {
		return fmt.Errorf("%w: %s rejected those credentials", domain.ErrForbidden, kind)
	}
	return err
}
