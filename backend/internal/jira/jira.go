// Package jira wraps the small slice of the Jira Cloud REST API and the
// OAuth 2.0 (3LO) authorization-code + PKCE flow that Estimeet needs.
package jira

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	authorizeEndpoint = "https://auth.atlassian.com/authorize"
	tokenEndpoint     = "https://auth.atlassian.com/oauth/token"
	resourcesEndpoint = "https://api.atlassian.com/oauth/token/accessible-resources"
	apiBase           = "https://api.atlassian.com/ex/jira"

	// read:jira-work covers issue search; offline_access yields a refresh token.
	scopes = "read:jira-work read:jira-user offline_access"

	maxResponseBytes = 8 << 20 // 8 MiB guard against hostile/huge responses
)

// Client talks to Atlassian on behalf of a room.
type Client struct {
	clientID     string
	clientSecret string
	redirectURI  string
	http         *http.Client
}

// New builds a client. Callers should only construct it when credentials exist.
func New(clientID, clientSecret, redirectURI string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		http:         &http.Client{Timeout: 20 * time.Second},
	}
}

// Token is the OAuth token response.
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// Resource is one Jira site the user granted access to.
type Resource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Project is a Jira project shown in the import picker.
type Project struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Issue is a normalised Jira issue ready to become a topic.
type Issue struct {
	Key         string   `json:"key"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Status      string   `json:"status"`
	URL         string   `json:"url"`
	StoryPoints *float64 `json:"storyPoints,omitempty"`
}

// NewPKCE returns a fresh code verifier and its S256 challenge.
func NewPKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// AuthorizeURL builds the Atlassian consent URL.
func (c *Client) AuthorizeURL(state, challenge string) string {
	q := url.Values{}
	q.Set("audience", "api.atlassian.com")
	q.Set("client_id", c.clientID)
	q.Set("scope", scopes)
	q.Set("redirect_uri", c.redirectURI)
	q.Set("state", state)
	q.Set("response_type", "code")
	q.Set("prompt", "consent")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return authorizeEndpoint + "?" + q.Encode()
}

// Exchange trades an authorization code for tokens.
func (c *Client) Exchange(ctx context.Context, code, verifier string) (Token, error) {
	return c.token(ctx, map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"code":          code,
		"redirect_uri":  c.redirectURI,
		"code_verifier": verifier,
	})
}

// Refresh renews an expired access token via the rotating refresh token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	return c.token(ctx, map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"refresh_token": refreshToken,
	})
}

func (c *Client) token(ctx context.Context, payload map[string]string) (Token, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Token{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, bytes.NewReader(body))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	raw, err := c.do(req)
	if err != nil {
		return Token{}, err
	}

	var res struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return Token{}, fmt.Errorf("decode token response: %w", err)
	}
	if res.AccessToken == "" {
		return Token{}, fmt.Errorf("jira: empty access token")
	}
	if res.ExpiresIn == 0 {
		res.ExpiresIn = 3600
	}
	return Token{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(res.ExpiresIn) * time.Second),
	}, nil
}

// AccessibleResources lists the Jira sites the token can reach.
func (c *Client) AccessibleResources(ctx context.Context, accessToken string) ([]Resource, error) {
	raw, err := c.get(ctx, resourcesEndpoint, accessToken)
	if err != nil {
		return nil, err
	}
	var out []Resource
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode accessible resources: %w", err)
	}
	return out, nil
}

// Projects lists projects on a site, optionally filtered by a search string.
func (c *Client) Projects(ctx context.Context, cloudID, accessToken, query string) ([]Project, error) {
	q := url.Values{}
	q.Set("maxResults", "50")
	q.Set("orderBy", "lastIssueUpdatedTime")
	if query != "" {
		q.Set("query", query)
	}
	endpoint := fmt.Sprintf("%s/%s/rest/api/3/project/search?%s", apiBase, url.PathEscape(cloudID), q.Encode())

	raw, err := c.get(ctx, endpoint, accessToken)
	if err != nil {
		return nil, err
	}
	var res struct {
		Values []Project `json:"values"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decode projects: %w", err)
	}
	return res.Values, nil
}

// SearchEpics returns the open epics of a project.
func (c *Client) SearchEpics(ctx context.Context, cloudID, accessToken, projectKey, text string) ([]Issue, error) {
	var jql strings.Builder
	jql.WriteString(`issuetype = Epic`)
	if projectKey != "" {
		jql.WriteString(` AND project = ` + quoteJQL(projectKey))
	}
	if text != "" {
		jql.WriteString(` AND summary ~ ` + quoteJQL(text+"*"))
	}
	jql.WriteString(` ORDER BY updated DESC`)
	return c.search(ctx, cloudID, accessToken, jql.String(), 50)
}

// IssuesInEpic returns the children of an epic, which become the estimation topics.
func (c *Client) IssuesInEpic(ctx context.Context, cloudID, accessToken, epicKey string) ([]Issue, error) {
	// `parent` works for both team-managed and company-managed projects on Jira Cloud.
	jql := fmt.Sprintf(`parent = %s AND issuetype != Sub-task ORDER BY created ASC`, quoteJQL(epicKey))
	return c.search(ctx, cloudID, accessToken, jql, 100)
}

// SearchJQL runs a caller-supplied JQL query.
func (c *Client) SearchJQL(ctx context.Context, cloudID, accessToken, jql string, max int) ([]Issue, error) {
	return c.search(ctx, cloudID, accessToken, jql, max)
}

func (c *Client) search(ctx context.Context, cloudID, accessToken, jql string, max int) ([]Issue, error) {
	if max <= 0 || max > 100 {
		max = 50
	}
	q := url.Values{}
	q.Set("jql", jql)
	q.Set("maxResults", strconv.Itoa(max))
	q.Set("fields", "summary,description,issuetype,status")
	// /search/jql replaced the deprecated /search endpoint on Jira Cloud.
	endpoint := fmt.Sprintf("%s/%s/rest/api/3/search/jql?%s", apiBase, url.PathEscape(cloudID), q.Encode())

	raw, err := c.get(ctx, endpoint, accessToken)
	if err != nil {
		return nil, err
	}

	var res struct {
		Issues []struct {
			Key    string `json:"key"`
			Self   string `json:"self"`
			Fields struct {
				Summary     string          `json:"summary"`
				Description json.RawMessage `json:"description"`
				IssueType   struct {
					Name string `json:"name"`
				} `json:"issuetype"`
				Status struct {
					Name string `json:"name"`
				} `json:"status"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	out := make([]Issue, 0, len(res.Issues))
	for _, it := range res.Issues {
		out = append(out, Issue{
			Key:         it.Key,
			Summary:     it.Fields.Summary,
			Description: FlattenADF(it.Fields.Description),
			Type:        it.Fields.IssueType.Name,
			Status:      it.Fields.Status.Name,
		})
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, endpoint, accessToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read jira response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Never echo the raw upstream body back to users; it can contain tokens.
		return nil, &APIError{StatusCode: res.StatusCode, Detail: summarize(body)}
	}
	return body, nil
}

// APIError reports a non-2xx response from Atlassian.
type APIError struct {
	StatusCode int
	Detail     string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jira api error: status %d: %s", e.StatusCode, e.Detail)
}

// Unauthorized reports whether the token should be refreshed or re-issued.
func (e *APIError) Unauthorized() bool {
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
}

func summarize(body []byte) string {
	var parsed struct {
		ErrorMessages []string `json:"errorMessages"`
		Error         string   `json:"error"`
		Message       string   `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if len(parsed.ErrorMessages) > 0 {
			return strings.Join(parsed.ErrorMessages, "; ")
		}
		if parsed.Message != "" {
			return parsed.Message
		}
		if parsed.Error != "" {
			return parsed.Error
		}
	}
	if len(body) > 200 {
		body = body[:200]
	}
	return string(body)
}

// quoteJQL escapes a value for safe interpolation into a JQL string literal.
func quoteJQL(v string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(v) + `"`
}

// FlattenADF converts an Atlassian Document Format body into plain text.
// Jira REST v3 returns rich documents; the estimation UI only needs the words.
func FlattenADF(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	// Older instances may still return a plain string.
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}

	var node adfNode
	if err := json.Unmarshal(raw, &node); err != nil {
		return ""
	}
	var sb strings.Builder
	walkADF(&node, &sb)
	return strings.TrimSpace(collapseBlankLines(sb.String()))
}

type adfNode struct {
	Type    string     `json:"type"`
	Text    string     `json:"text"`
	Content []adfNode  `json:"content"`
	Attrs   *adfAttrs  `json:"attrs"`
	Marks   []adfMarks `json:"marks"`
}

type adfAttrs struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type adfMarks struct {
	Type string `json:"type"`
}

func walkADF(n *adfNode, sb *strings.Builder) {
	switch n.Type {
	case "text":
		sb.WriteString(n.Text)
	case "hardBreak":
		sb.WriteString("\n")
	case "mention", "emoji":
		if n.Attrs != nil && n.Attrs.Text != "" {
			sb.WriteString(n.Attrs.Text)
		}
	case "listItem":
		sb.WriteString("- ")
	}

	for i := range n.Content {
		walkADF(&n.Content[i], sb)
	}

	switch n.Type {
	case "paragraph", "heading", "listItem", "blockquote", "codeBlock", "rule", "panel":
		sb.WriteString("\n")
	}
}

func collapseBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}
