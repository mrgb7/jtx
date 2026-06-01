package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	email      string
	token      string
	httpClient *http.Client
}

func NewClient() (*Client, error) {
	token := os.Getenv("JIRA_API_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("JIRA_API_TOKEN environment variable is not set")
	}
	baseURL := os.Getenv("JIRA_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("JIRA_URL environment variable is not set")
	}
	email := os.Getenv("JIRA_EMAIL")
	if email == "" {
		return nil, fmt.Errorf("JIRA_EMAIL environment variable is not set")
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		email:      email,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// BrowseURL returns the web URL for a given issue key.
func (c *Client) BrowseURL(key string) string {
	return c.baseURL + "/browse/" + key
}

func (c *Client) do(method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("jira API error %d: %s", resp.StatusCode, respBody)
	}
	if out != nil && len(respBody) > 0 {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

func (c *Client) SearchIssues(jql string, maxResults int) ([]Issue, error) {
	q := url.Values{}
	q.Set("jql", jql)
	q.Set("maxResults", fmt.Sprintf("%d", maxResults))
	q.Set("fields", "summary,status,issuetype,priority,assignee,reporter,created,updated,labels")

	var result searchResult
	if err := c.do("GET", "/rest/api/3/search/jql?"+q.Encode(), nil, &result); err != nil {
		return nil, err
	}
	return result.Issues, nil
}

func (c *Client) GetIssue(key string) (*Issue, error) {
	var issue Issue
	path := "/rest/api/3/issue/" + key +
		"?fields=summary,status,issuetype,priority,assignee,reporter,description,created,updated,labels,comment"
	if err := c.do("GET", path, nil, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// AddComment posts a plain-text comment to an issue using ADF format.
func (c *Client) AddComment(key, text string) error {
	body := map[string]any{
		"body": textToADF(text),
	}
	return c.do("POST", "/rest/api/3/issue/"+key+"/comment", body, nil)
}

// UpdateSummary changes the summary (title) of an issue.
func (c *Client) UpdateSummary(key, summary string) error {
	body := map[string]any{
		"fields": map[string]any{
			"summary": summary,
		},
	}
	return c.do("PUT", "/rest/api/3/issue/"+key, body, nil)
}

// IssueType is a lightweight type used for creation metadata.
type IssueTypeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetIssueTypes returns the issue types available for a project.
func (c *Client) GetIssueTypes(projectKey string) ([]IssueTypeRef, error) {
	type issuetypesMeta struct {
		Projects []struct {
			IssueTypes []IssueTypeRef `json:"issuetypes"`
		} `json:"projects"`
	}
	var meta issuetypesMeta
	path := "/rest/api/3/issue/createmeta?projectKeys=" + projectKey + "&expand=projects.issuetypes"
	if err := c.do("GET", path, nil, &meta); err != nil {
		return nil, err
	}
	if len(meta.Projects) == 0 {
		return nil, fmt.Errorf("project %s not found", projectKey)
	}
	return meta.Projects[0].IssueTypes, nil
}

// CreateIssue creates a new issue and returns its key.
func (c *Client) CreateIssue(projectKey, issueTypeID, summary, description string) (string, error) {
	body := map[string]any{
		"fields": map[string]any{
			"project":     map[string]any{"key": projectKey},
			"issuetype":   map[string]any{"id": issueTypeID},
			"summary":     summary,
			"description": textToADF(description),
		},
	}
	var result struct {
		Key string `json:"key"`
	}
	if err := c.do("POST", "/rest/api/3/issue", body, &result); err != nil {
		return "", err
	}
	return result.Key, nil
}

type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetTransitions returns the available transitions for an issue.
func (c *Client) GetTransitions(key string) ([]Transition, error) {
	var result struct {
		Transitions []Transition `json:"transitions"`
	}
	if err := c.do("GET", "/rest/api/3/issue/"+key+"/transitions", nil, &result); err != nil {
		return nil, err
	}
	return result.Transitions, nil
}

// TransitionIssue moves an issue to the status identified by transitionID.
func (c *Client) TransitionIssue(key, transitionID string) error {
	body := map[string]any{
		"transition": map[string]any{"id": transitionID},
	}
	return c.do("POST", "/rest/api/3/issue/"+key+"/transitions", body, nil)
}

// GetCurrentUser returns the authenticated user's profile.
func (c *Client) GetCurrentUser() (*User, error) {
	var user User
	if err := c.do("GET", "/rest/api/3/myself", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// SearchAssignableUsers returns users that can be assigned to the given issue.
func (c *Client) SearchAssignableUsers(issueKey, query string) ([]User, error) {
	q := url.Values{}
	q.Set("issueKey", issueKey)
	if query != "" {
		q.Set("query", query)
	}
	q.Set("maxResults", "10")
	var users []User
	if err := c.do("GET", "/rest/api/3/user/assignable/search?"+q.Encode(), nil, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// AssignIssue assigns an issue to the user with the given accountId.
// Pass an empty string to unassign.
func (c *Client) AssignIssue(key, accountID string) error {
	var body map[string]any
	if accountID == "" {
		body = map[string]any{"accountId": nil}
	} else {
		body = map[string]any{"accountId": accountID}
	}
	return c.do("PUT", "/rest/api/3/issue/"+key+"/assignee", body, nil)
}

// UpdateDescription changes the description of an issue.
func (c *Client) UpdateDescription(key, text string) error {
	body := map[string]any{
		"fields": map[string]any{
			"description": textToADF(text),
		},
	}
	return c.do("PUT", "/rest/api/3/issue/"+key, body, nil)
}

// textToADF wraps plain text in a minimal Atlassian Document Format structure.
func textToADF(text string) map[string]any {
	var paragraphs []any
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			paragraphs = append(paragraphs, map[string]any{
				"type":    "paragraph",
				"content": []any{},
			})
		} else {
			paragraphs = append(paragraphs, map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": line},
				},
			})
		}
	}
	return map[string]any{
		"version": 1,
		"type":    "doc",
		"content": paragraphs,
	}
}

type searchResult struct {
	Issues []Issue `json:"issues"`
	Total  int     `json:"total"`
}

type Issue struct {
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
}

type IssueFields struct {
	Summary     string      `json:"summary"`
	Status      Status      `json:"status"`
	IssueType   IssueType   `json:"issuetype"`
	Priority    Priority    `json:"priority"`
	Assignee    *User       `json:"assignee"`
	Reporter    *User       `json:"reporter"`
	Description any         `json:"description"`
	Created     string      `json:"created"`
	Updated     string      `json:"updated"`
	Labels      []string    `json:"labels"`
	Comment     CommentList `json:"comment"`
}

type Status struct {
	Name           string         `json:"name"`
	StatusCategory StatusCategory `json:"statusCategory"`
}

type StatusCategory struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type IssueType struct {
	Name string `json:"name"`
}

type Priority struct {
	Name string `json:"name"`
}

type User struct {
	AccountID    string `json:"accountId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

type CommentList struct {
	Comments []Comment `json:"comments"`
	Total    int       `json:"total"`
}

type Comment struct {
	Author  User   `json:"author"`
	Body    any    `json:"body"`
	Created string `json:"created"`
}

func ExtractText(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		return extractADF(t)
	}
	return fmt.Sprintf("%v", v)
}

func extractADF(node map[string]any) string {
	var sb strings.Builder
	if content, ok := node["content"].([]any); ok {
		for _, item := range content {
			if m, ok := item.(map[string]any); ok {
				switch m["type"] {
				case "text":
					if txt, ok := m["text"].(string); ok {
						sb.WriteString(txt)
					}
				default:
					sb.WriteString(extractADF(m))
				}
			}
		}
	}
	if node["type"] == "paragraph" || node["type"] == "heading" {
		sb.WriteString("\n")
	}
	return sb.String()
}
