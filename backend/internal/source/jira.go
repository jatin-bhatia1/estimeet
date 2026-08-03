package source

import (
	"context"
	"errors"

	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
)

// jiraProvider adapts the Jira Cloud client to the generic interface. Both the
// API-token and the OAuth connection end up here; only the Authorization
// header and the API host differ between them.
type jiraProvider struct {
	client *jira.Client
}

func (p *jiraProvider) Describe() Descriptor {
	return Descriptor{
		Kind:      KindJira,
		Name:      "Jira",
		Container: "Project",
		Group:     "Epic",
		Items:     "stories",
		Scopes:    "The token inherits your own Jira permissions; read access to the projects you estimate is enough.",
		Fields: []Field{
			{
				Name:        "baseUrl",
				Label:       "Jira site",
				Placeholder: "https://your-team.atlassian.net",
				Type:        "text",
			},
			{
				Name:        "account",
				Label:       "Atlassian account email",
				Placeholder: "ada@example.com",
				Type:        "email",
			},
			{
				Name:    "token",
				Label:   "API token",
				Type:    "password",
				Help:    "The import sees exactly the issues this account can see.",
				HelpURL: "https://id.atlassian.com/manage-profile/security/api-tokens",
			},
		},
	}
}

func (p *jiraProvider) auth(c Credentials) jira.Auth {
	if c.OAuth {
		return jira.OAuthAuth(c.CloudID, c.Token)
	}
	return jira.TokenAuth(c.BaseURL, c.Account, c.Token)
}

func (p *jiraProvider) Verify(ctx context.Context, c Credentials) (Account, error) {
	me, err := p.client.Myself(ctx, p.auth(c))
	if err != nil {
		return Account{}, wrapJira(err)
	}
	return Account{Name: me.DisplayName, Email: me.Email}, nil
}

func (p *jiraProvider) Containers(ctx context.Context, c Credentials, query string) ([]Container, error) {
	projects, err := p.client.Projects(ctx, p.auth(c), query)
	if err != nil {
		return nil, wrapJira(err)
	}
	out := make([]Container, 0, len(projects))
	for _, pr := range projects {
		out = append(out, Container{Key: pr.Key, Name: pr.Name})
	}
	return out, nil
}

func (p *jiraProvider) Groups(ctx context.Context, c Credentials, container, query string) ([]Item, error) {
	issues, err := p.client.SearchEpics(ctx, p.auth(c), container, query)
	if err != nil {
		return nil, wrapJira(err)
	}
	return p.convert(c, issues), nil
}

func (p *jiraProvider) Items(ctx context.Context, c Credentials, _, group string) ([]Item, error) {
	issues, err := p.client.IssuesInEpic(ctx, p.auth(c), group)
	if err != nil {
		return nil, wrapJira(err)
	}
	return p.convert(c, issues), nil
}

func (p *jiraProvider) convert(c Credentials, issues []jira.Issue) []Item {
	out := make([]Item, 0, len(issues))
	for _, it := range issues {
		out = append(out, Item{
			Key:         it.Key,
			Title:       truncate(it.Summary, 400),
			Description: truncate(it.Description, 4000),
			Type:        it.Type,
			Status:      it.Status,
			URL:         c.BaseURL + "/browse/" + it.Key,
		})
	}
	return out
}

func wrapJira(err error) error {
	var apiErr *jira.APIError
	if errors.As(err, &apiErr) {
		return &Error{Kind: KindJira, Status: apiErr.StatusCode, Detail: apiErr.Detail}
	}
	return err
}
