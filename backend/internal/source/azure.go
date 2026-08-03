package source

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const azureAPIVersion = "7.1"

// azureOrgPattern is deliberately strict: the organisation goes straight into a
// dev.azure.com URL, so nothing that could change the host or escape the path
// is accepted.
var azureOrgPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ErrInvalidAzureOrg is returned for an organisation name that cannot be real.
var ErrInvalidAzureOrg = errors.New("azure: organisation must be the name in dev.azure.com/<organisation>")

// NormalizeAzureOrg accepts either the bare organisation name or the URL a user
// copied out of the browser, and returns the bare name.
func NormalizeAzureOrg(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	v = strings.TrimSuffix(v, "/")
	if v == "" {
		return "", ErrInvalidAzureOrg
	}
	// https://dev.azure.com/contoso or the legacy contoso.visualstudio.com
	if i := strings.Index(v, "://"); i >= 0 {
		u, err := url.Parse(v)
		if err != nil {
			return "", ErrInvalidAzureOrg
		}
		// Plain HTTP would send the token in clear, and credentials embedded in
		// the URL are a sign the value was not copied from a browser bar.
		if u.Scheme != "https" || u.User != nil {
			return "", ErrInvalidAzureOrg
		}
		host := strings.ToLower(u.Hostname())
		switch {
		case host == "dev.azure.com":
			v = strings.Trim(u.Path, "/")
			if i := strings.Index(v, "/"); i >= 0 {
				v = v[:i]
			}
		case strings.HasSuffix(host, ".visualstudio.com"):
			v = strings.TrimSuffix(host, ".visualstudio.com")
		default:
			return "", ErrInvalidAzureOrg
		}
	}
	if !azureOrgPattern.MatchString(v) {
		return "", ErrInvalidAzureOrg
	}
	return v, nil
}

// azureProvider reads Azure Boards through the REST API with a personal access
// token. Work items are hierarchical, so epics and features are the groups and
// their children are the items.
type azureProvider struct{}

func (p *azureProvider) Describe() Descriptor {
	return Descriptor{
		Kind:      KindAzure,
		Name:      "Azure DevOps",
		Container: "Project",
		Group:     "Epic or feature",
		Items:     "work items",
		Scopes:    "A personal access token with Work Items (Read) is enough.",
		Fields: []Field{
			{
				Name:        "account",
				Label:       "Organisation",
				Placeholder: "contoso  ·  or https://dev.azure.com/contoso",
				Type:        "text",
			},
			{
				Name:    "token",
				Label:   "Personal access token",
				Type:    "password",
				Help:    "It needs the Work Items (Read) scope.",
				HelpURL: "https://dev.azure.com",
			},
		},
	}
}

func (p *azureProvider) header(c Credentials) string {
	// Azure DevOps takes the PAT as the password of an empty basic-auth user.
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+c.Token))
}

func (p *azureProvider) org(c Credentials) string { return url.PathEscape(c.Account) }

func (p *azureProvider) Verify(ctx context.Context, c Credentials) (Account, error) {
	// Listing one project is the cheapest call that proves the PAT works and
	// that it was issued for this organisation.
	endpoint := fmt.Sprintf("https://dev.azure.com/%s/_apis/projects?$top=1&api-version=%s", p.org(c), azureAPIVersion)
	var res struct {
		Count int `json:"count"`
	}
	if err := fetch(ctx, KindAzure, "GET", endpoint, p.header(c), nil, &res); err != nil {
		return Account{}, err
	}
	return Account{Name: c.Account}, nil
}

func (p *azureProvider) Containers(ctx context.Context, c Credentials, query string) ([]Container, error) {
	endpoint := fmt.Sprintf("https://dev.azure.com/%s/_apis/projects?$top=200&api-version=%s", p.org(c), azureAPIVersion)
	var res struct {
		Value []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"value"`
	}
	if err := fetch(ctx, KindAzure, "GET", endpoint, p.header(c), nil, &res); err != nil {
		return nil, err
	}

	needle := strings.ToLower(query)
	out := make([]Container, 0, len(res.Value))
	for _, pr := range res.Value {
		if needle != "" && !strings.Contains(strings.ToLower(pr.Name), needle) {
			continue
		}
		out = append(out, Container{Key: pr.Name, Name: pr.Name})
	}
	return out, nil
}

func (p *azureProvider) Groups(ctx context.Context, c Credentials, container, query string) ([]Item, error) {
	if strings.TrimSpace(container) == "" {
		return nil, fmt.Errorf("azure: pick a project first")
	}
	wiql := "SELECT [System.Id] FROM WorkItems" +
		" WHERE [System.TeamProject] = " + quoteWIQL(container) +
		" AND [System.WorkItemType] IN ('Epic', 'Feature')"
	if query != "" {
		if id, err := strconv.Atoi(strings.TrimPrefix(query, "#")); err == nil {
			wiql += " AND [System.Id] = " + strconv.Itoa(id)
		} else {
			wiql += " AND [System.Title] CONTAINS " + quoteWIQL(query)
		}
	}
	wiql += " ORDER BY [System.ChangedDate] DESC"

	ids, err := p.runWIQL(ctx, c, container, wiql, 50)
	if err != nil {
		return nil, err
	}
	return p.workItems(ctx, c, container, ids)
}

func (p *azureProvider) Items(ctx context.Context, c Credentials, container, group string) ([]Item, error) {
	parent, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(group), "#"))
	if err != nil || parent <= 0 {
		return nil, fmt.Errorf("azure: %q is not a work item id", group)
	}
	wiql := "SELECT [System.Id] FROM WorkItems" +
		" WHERE [System.TeamProject] = " + quoteWIQL(container) +
		" AND [System.Parent] = " + strconv.Itoa(parent) +
		" ORDER BY [System.Id] ASC"

	ids, err := p.runWIQL(ctx, c, container, wiql, 200)
	if err != nil {
		return nil, err
	}
	return p.workItems(ctx, c, container, ids)
}

func (p *azureProvider) runWIQL(ctx context.Context, c Credentials, container, wiql string, top int) ([]int, error) {
	endpoint := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/wiql?$top=%d&api-version=%s",
		p.org(c), url.PathEscape(container), top, azureAPIVersion)
	var res struct {
		WorkItems []struct {
			ID int `json:"id"`
		} `json:"workItems"`
	}
	body := map[string]string{"query": wiql}
	if err := fetch(ctx, KindAzure, "POST", endpoint, p.header(c), body, &res); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(res.WorkItems))
	for _, w := range res.WorkItems {
		ids = append(ids, w.ID)
	}
	return ids, nil
}

// workItems resolves ids to titles. WIQL only ever returns references, so this
// second call is unavoidable; 200 ids per batch is the documented ceiling.
func (p *azureProvider) workItems(ctx context.Context, c Credentials, container string, ids []int) ([]Item, error) {
	if len(ids) == 0 {
		return []Item{}, nil
	}
	if len(ids) > 200 {
		ids = ids[:200]
	}
	joined := make([]string, len(ids))
	for i, id := range ids {
		joined[i] = strconv.Itoa(id)
	}
	endpoint := fmt.Sprintf(
		"https://dev.azure.com/%s/_apis/wit/workitems?ids=%s&fields=System.Id,System.Title,System.State,System.WorkItemType,System.Description&api-version=%s",
		p.org(c), strings.Join(joined, ","), azureAPIVersion)

	var res struct {
		Value []struct {
			ID     int `json:"id"`
			Fields struct {
				Title       string `json:"System.Title"`
				State       string `json:"System.State"`
				Type        string `json:"System.WorkItemType"`
				Description string `json:"System.Description"`
			} `json:"fields"`
		} `json:"value"`
	}
	if err := fetch(ctx, KindAzure, "GET", endpoint, p.header(c), nil, &res); err != nil {
		return nil, err
	}

	// The batch endpoint does not preserve the order the ids were asked for.
	byID := make(map[int]Item, len(res.Value))
	for _, w := range res.Value {
		byID[w.ID] = Item{
			Key:         "AB#" + strconv.Itoa(w.ID),
			Title:       truncate(w.Fields.Title, 400),
			Description: truncate(stripHTML(w.Fields.Description), 4000),
			Type:        w.Fields.Type,
			Status:      w.Fields.State,
			URL: fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d",
				p.org(c), url.PathEscape(container), w.ID),
		}
	}
	out := make([]Item, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

// quoteWIQL renders a WIQL string literal. Doubling the quote is the escape
// WIQL defines, and it is what keeps a project name from ending the literal.
func quoteWIQL(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// stripHTML flattens the HTML that Azure Boards stores descriptions in.
var htmlTag = regexp.MustCompile(`(?s)<[^>]*>`)

func stripHTML(v string) string {
	if v == "" {
		return ""
	}
	v = strings.ReplaceAll(v, "<br>", "\n")
	v = strings.ReplaceAll(v, "<br/>", "\n")
	v = strings.ReplaceAll(v, "</div>", "\n")
	v = strings.ReplaceAll(v, "</p>", "\n")
	v = htmlTag.ReplaceAllString(v, "")
	v = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(v)
	return strings.TrimSpace(v)
}
