package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
	"github.com/jatin-bhatia1/estimeet/backend/internal/source"
	"github.com/jatin-bhatia1/estimeet/backend/internal/store"
)

// This file holds the one connection method that is not a personal access
// token: Atlassian's OAuth 2.0 (3LO) flow, which a server can only offer when
// it has an app registered with Atlassian.

// JiraAuthorizeURL starts the OAuth 2.0 flow for a room (host only).
func (s *Service) JiraAuthorizeURL(ctx context.Context, sess Session) (string, error) {
	if !s.JiraOAuthAvailable() {
		return "", ErrJiraOAuthDisabled
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
		return "", ErrJiraOAuthDisabled
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
	// The same retention window applies to OAuth tokens as to pasted ones.
	if err := s.saveJiraOAuth(ctx, pending.RoomID, site, token, s.now().Add(CredentialTTL)); err != nil {
		return "", err
	}

	room, err := s.store.RoomByID(ctx, pending.RoomID)
	if err != nil {
		return "", err
	}
	s.publish(room.ID, "source.connected", map[string]string{
		"provider": string(source.KindJira),
		"name":     site.Name,
	})
	return room.Code, nil
}

// saveJiraOAuth encrypts and stores an access/refresh token pair. retainUntil
// is passed through unchanged so a token refresh cannot extend how long the
// credentials are kept.
func (s *Service) saveJiraOAuth(ctx context.Context, roomID string, site jira.Resource, token jira.Token, retainUntil time.Time) error {
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
	return s.store.SaveSourceConnection(ctx, store.SourceConnection{
		RoomID:         roomID,
		Provider:       string(source.KindJira),
		AuthType:       store.AuthOAuth,
		BaseURL:        strings.TrimRight(site.URL, "/"),
		CloudID:        site.ID,
		Name:           site.Name,
		TokenExpiresAt: token.ExpiresAt,
		ExpiresAt:      retainUntil,
	}, accessEnc, refreshEnc)
}
