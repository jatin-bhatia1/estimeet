package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const githubAPI = "https://api.github.com"

// githubNamePattern matches one path segment of an owner or repository name.
var githubNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// ErrInvalidRepo is returned when a repository is not in owner/name form.
var ErrInvalidRepo = errors.New("github: repository must be in owner/name form")

// splitRepo validates and splits an owner/name pair before it is put in a URL.
func splitRepo(raw string) (owner, name string, err error) {
	v := strings.TrimSpace(raw)
	v = strings.TrimPrefix(v, "https://github.com/")
	v = strings.Trim(v, "/")
	owner, name, ok := strings.Cut(v, "/")
	if !ok || !githubNamePattern.MatchString(owner) || !githubNamePattern.MatchString(name) {
		return "", "", ErrInvalidRepo
	}
	return owner, name, nil
}

// githubProvider reads issues from github.com with a personal access token.
// Milestones stand in for epics, which is how most teams group an increment.
type githubProvider struct{}

func (p *githubProvider) Describe() Descriptor {
	return Descriptor{
		Kind:      KindGitHub,
		Name:      "GitHub",
		Container: "Repository",
		Group:     "Milestone",
		Items:     "issues",
		Scopes:    "A fine-grained token with Issues (Read-only), or a classic token with the repo scope for private repositories.",
		Fields: []Field{
			{
				Name:    "token",
				Label:   "Personal access token",
				Type:    "password",
				Help:    "It needs the repo scope, or public_repo for public repositories only.",
				HelpURL: "https://github.com/settings/tokens",
			},
		},
	}
}

func (p *githubProvider) header(c Credentials) string { return "Bearer " + c.Token }

func (p *githubProvider) Verify(ctx context.Context, c Credentials) (Account, error) {
	var res struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := fetch(ctx, KindGitHub, "GET", githubAPI+"/user", p.header(c), nil, &res); err != nil {
		return Account{}, err
	}
	if res.Login == "" {
		return Account{}, &Error{Kind: KindGitHub, Status: 401, Detail: "the token does not belong to a user"}
	}
	name := res.Name
	if name == "" {
		name = res.Login
	}
	return Account{Name: name, Email: res.Login}, nil
}

func (p *githubProvider) Containers(ctx context.Context, c Credentials, query string) ([]Container, error) {
	// A full owner/name is looked up directly, so a repository that is not in
	// the hundred most recently touched can still be reached.
	if owner, name, err := splitRepo(query); err == nil {
		var repo struct {
			FullName string `json:"full_name"`
		}
		endpoint := fmt.Sprintf("%s/repos/%s/%s", githubAPI, url.PathEscape(owner), url.PathEscape(name))
		if err := fetch(ctx, KindGitHub, "GET", endpoint, p.header(c), nil, &repo); err == nil && repo.FullName != "" {
			return []Container{{Key: repo.FullName, Name: repo.FullName}}, nil
		}
	}

	endpoint := githubAPI + "/user/repos?per_page=100&sort=updated&affiliation=owner,collaborator,organization_member"
	var res []struct {
		FullName    string `json:"full_name"`
		Description string `json:"description"`
	}
	if err := fetch(ctx, KindGitHub, "GET", endpoint, p.header(c), nil, &res); err != nil {
		return nil, err
	}

	needle := strings.ToLower(strings.TrimSpace(query))
	out := make([]Container, 0, len(res))
	for _, repo := range res {
		if needle != "" && !strings.Contains(strings.ToLower(repo.FullName), needle) {
			continue
		}
		out = append(out, Container{Key: repo.FullName, Name: repo.FullName})
	}
	return out, nil
}

func (p *githubProvider) Groups(ctx context.Context, c Credentials, container, query string) ([]Item, error) {
	owner, name, err := splitRepo(container)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/milestones?state=all&per_page=100&sort=updated&direction=desc",
		githubAPI, url.PathEscape(owner), url.PathEscape(name))

	var res []struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		OpenIssues  int    `json:"open_issues"`
		HTMLURL     string `json:"html_url"`
	}
	if err := fetch(ctx, KindGitHub, "GET", endpoint, p.header(c), nil, &res); err != nil {
		return nil, err
	}

	needle := strings.ToLower(strings.TrimSpace(query))
	out := make([]Item, 0, len(res))
	for _, m := range res {
		if needle != "" && !strings.Contains(strings.ToLower(m.Title), needle) {
			continue
		}
		out = append(out, Item{
			Key:         strconv.Itoa(m.Number),
			Title:       truncate(m.Title, 400),
			Description: truncate(m.Description, 4000),
			Type:        "Milestone",
			Status:      fmt.Sprintf("%s · %d open", m.State, m.OpenIssues),
			URL:         m.HTMLURL,
		})
	}
	return out, nil
}

func (p *githubProvider) Items(ctx context.Context, c Credentials, container, group string) ([]Item, error) {
	owner, name, err := splitRepo(container)
	if err != nil {
		return nil, err
	}
	milestone, err := strconv.Atoi(strings.TrimSpace(group))
	if err != nil || milestone <= 0 {
		return nil, fmt.Errorf("github: %q is not a milestone", group)
	}

	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues?milestone=%d&state=all&per_page=100",
		githubAPI, url.PathEscape(owner), url.PathEscape(name), milestone)

	var res []struct {
		Number      int             `json:"number"`
		Title       string          `json:"title"`
		Body        string          `json:"body"`
		State       string          `json:"state"`
		HTMLURL     string          `json:"html_url"`
		PullRequest json.RawMessage `json:"pull_request"`
	}
	if err := fetch(ctx, KindGitHub, "GET", endpoint, p.header(c), nil, &res); err != nil {
		return nil, err
	}

	out := make([]Item, 0, len(res))
	for _, issue := range res {
		// The issues endpoint returns pull requests too, and nobody estimates those.
		if len(issue.PullRequest) > 0 {
			continue
		}
		out = append(out, Item{
			Key:         fmt.Sprintf("%s/%s#%d", owner, name, issue.Number),
			Title:       truncate(issue.Title, 400),
			Description: truncate(issue.Body, 4000),
			Type:        "Issue",
			Status:      issue.State,
			URL:         issue.HTMLURL,
		})
	}
	return out, nil
}
