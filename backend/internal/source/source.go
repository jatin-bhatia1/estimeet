// Package source turns the trackers a backlog can be imported from — Jira,
// Azure Boards and GitHub — into one small vocabulary: a container (project,
// repository) holds groups (epics, milestones) which hold the items that
// become estimation topics.
package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/jatin-bhatia1/estimeet/backend/internal/jira"
)

// Kind identifies a tracker.
type Kind string

const (
	KindJira   Kind = "jira"
	KindAzure  Kind = "azure"
	KindGitHub Kind = "github"
)

// ParseKind validates a provider name coming from a request.
func ParseKind(raw string) (Kind, bool) {
	switch Kind(strings.ToLower(strings.TrimSpace(raw))) {
	case KindJira:
		return KindJira, true
	case KindAzure:
		return KindAzure, true
	case KindGitHub:
		return KindGitHub, true
	default:
		return "", false
	}
}

// Credentials are one room's resolved access to a tracker. For Jira the OAuth
// flow puts a short-lived access token here and sets CloudID; every other
// connection is a personal access token.
type Credentials struct {
	Kind    Kind
	OAuth   bool
	BaseURL string // Jira site origin, or https://dev.azure.com/{org}
	CloudID string // Jira OAuth only
	Account string // Atlassian email, GitHub login, Azure organisation
	Token   string
}

// Account is who a set of credentials belongs to, used to label the connection.
type Account struct {
	Name  string
	Email string
}

// Container is the top level of a tracker: a Jira project, an Azure Boards
// project, a GitHub repository.
type Container struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Item is anything that can be listed or imported: an epic, a milestone, a
// story. Key is stable and unique inside a room, and is what makes re-imports
// idempotent.
type Item struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Status      string `json:"status,omitempty"`
	URL         string `json:"url,omitempty"`
}

// Field describes one input of a provider's connect form. The UI is generated
// from these so a new tracker does not need new frontend code.
type Field struct {
	Name        string `json:"name"` // baseUrl, account or token
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Type        string `json:"type"` // text, email or password
	Help        string `json:"help,omitempty"`
	HelpURL     string `json:"helpUrl,omitempty"`
}

// Descriptor is everything the UI needs to talk about a provider.
type Descriptor struct {
	Kind      Kind    `json:"kind"`
	Name      string  `json:"name"`
	Container string  `json:"container"` // "Project", "Repository"
	Group     string  `json:"group"`     // "Epic", "Milestone"
	Items     string  `json:"items"`     // "stories", "issues"
	Scopes    string  `json:"scopes"`    // permissions the token needs
	Fields    []Field `json:"fields"`
}

// Provider is the read-only slice of a tracker that Estimeet needs.
type Provider interface {
	Describe() Descriptor
	// Verify checks the credentials and names their owner.
	Verify(ctx context.Context, c Credentials) (Account, error)
	// Containers lists projects or repositories, narrowed by an optional query.
	Containers(ctx context.Context, c Credentials, query string) ([]Container, error)
	// Groups lists epics or milestones inside a container.
	Groups(ctx context.Context, c Credentials, container, query string) ([]Item, error)
	// Items lists the children of a group, ready to become topics.
	Items(ctx context.Context, c Credentials, container, group string) ([]Item, error)
}

// Registry is the set of providers this server can use.
type Registry struct {
	providers map[Kind]Provider
	order     []Kind
}

// NewRegistry builds the registry. jiraClient may be nil, which drops Jira.
func NewRegistry(jiraClient *jira.Client) *Registry {
	r := &Registry{providers: map[Kind]Provider{}}
	if jiraClient != nil {
		r.add(KindJira, &jiraProvider{client: jiraClient})
	}
	r.add(KindAzure, &azureProvider{})
	r.add(KindGitHub, &githubProvider{})
	return r
}

func (r *Registry) add(kind Kind, p Provider) {
	r.providers[kind] = p
	r.order = append(r.order, kind)
}

// Get resolves a provider by kind.
func (r *Registry) Get(kind Kind) (Provider, error) {
	p, ok := r.providers[kind]
	if !ok {
		return nil, fmt.Errorf("%s is not available on this server", kind)
	}
	return p, nil
}

// Descriptors lists every available provider, in display order.
func (r *Registry) Descriptors() []Descriptor {
	out := make([]Descriptor, 0, len(r.order))
	for _, kind := range r.order {
		out = append(out, r.providers[kind].Describe())
	}
	return out
}

// Error is a non-2xx answer from a tracker. The upstream body is never
// forwarded verbatim: it can echo back the credentials that were sent.
type Error struct {
	Kind   Kind
	Status int
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s returned HTTP %d", e.Kind, e.Status)
	}
	return fmt.Sprintf("%s returned HTTP %d: %s", e.Kind, e.Status, e.Detail)
}

// Unauthorized reports whether the credentials were rejected.
func (e *Error) Unauthorized() bool { return e.Status == 401 || e.Status == 403 }

// truncate keeps titles and descriptions inside sane bounds before they travel.
func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "…"
}
