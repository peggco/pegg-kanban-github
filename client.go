package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultAccept = "application/vnd.github+json"

type Client struct {
	BaseURL   string
	Token     string
	UserAgent string
	HTTP      *http.Client
}

func NewClient(settings Settings, token string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(settings.APIURL, "/"),
		Token:     token,
		UserAgent: "pegg-github-plugin",
		HTTP:      &http.Client{Timeout: settings.timeout()},
	}
}

type User struct {
	Login string `json:"login"`
	Name  string `json:"name,omitempty"`
}

type Label struct {
	Name string `json:"name"`
}

type Repository struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
	Private       bool   `json:"private"`
	Owner         User   `json:"owner"`
}

type Issue struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	HTMLURL     string    `json:"html_url"`
	User        User      `json:"user"`
	Labels      []Label   `json:"labels"`
	Assignees   []User    `json:"assignees"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
	Comments    int       `json:"comments"`
	PullRequest *struct{} `json:"pull_request,omitempty"`
}

type issueView struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	State     string   `json:"state"`
	URL       string   `json:"url"`
	Author    string   `json:"author"`
	Labels    []string `json:"labels"`
	Assignees []string `json:"assignees"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	Comments  int      `json:"comments"`
}

func viewIssue(issue Issue) issueView {
	labels := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		labels = append(labels, label.Name)
	}
	assignees := make([]string, 0, len(issue.Assignees))
	for _, assignee := range issue.Assignees {
		assignees = append(assignees, assignee.Login)
	}
	return issueView{
		Number:    issue.Number,
		Title:     issue.Title,
		Body:      issue.Body,
		State:     issue.State,
		URL:       issue.HTMLURL,
		Author:    issue.User.Login,
		Labels:    labels,
		Assignees: assignees,
		CreatedAt: issue.CreatedAt,
		UpdatedAt: issue.UpdatedAt,
		Comments:  issue.Comments,
	}
}

type PullRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type PullRequest struct {
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	State     string  `json:"state"`
	HTMLURL   string  `json:"html_url"`
	User      User    `json:"user"`
	Labels    []Label `json:"labels"`
	Head      PullRef `json:"head"`
	Base      PullRef `json:"base"`
	Draft     bool    `json:"draft"`
	Merged    bool    `json:"merged"`
	Mergeable *bool   `json:"mergeable,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Comments  int     `json:"comments"`
	Additions int     `json:"additions"`
	Deletions int     `json:"deletions"`
	Changed   int     `json:"changed_files"`
}

type pullView struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	State     string   `json:"state"`
	URL       string   `json:"url"`
	Author    string   `json:"author"`
	Head      string   `json:"head"`
	Base      string   `json:"base"`
	Labels    []string `json:"labels"`
	Draft     bool     `json:"draft"`
	Merged    bool     `json:"merged"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	Additions int      `json:"additions,omitempty"`
	Deletions int      `json:"deletions,omitempty"`
	Changed   int      `json:"changed_files,omitempty"`
}

func viewPull(pr PullRequest) pullView {
	labels := make([]string, 0, len(pr.Labels))
	for _, label := range pr.Labels {
		labels = append(labels, label.Name)
	}
	return pullView{
		Number:    pr.Number,
		Title:     pr.Title,
		Body:      pr.Body,
		State:     pr.State,
		URL:       pr.HTMLURL,
		Author:    pr.User.Login,
		Head:      pr.Head.Ref,
		Base:      pr.Base.Ref,
		Labels:    labels,
		Draft:     pr.Draft,
		Merged:    pr.Merged,
		CreatedAt: pr.CreatedAt,
		UpdatedAt: pr.UpdatedAt,
		Additions: pr.Additions,
		Deletions: pr.Deletions,
		Changed:   pr.Changed,
	}
}

type Comment struct {
	ID      int    `json:"id"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	User    User   `json:"user"`
}

type Review struct {
	ID      int    `json:"id"`
	State   string `json:"state"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	User    User   `json:"user"`
}

type WorkflowRun struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	DisplayTitle string `json:"display_title"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HTMLURL      string `json:"html_url"`
	HeadBranch   string `json:"head_branch"`
	Event        string `json:"event"`
	CreatedAt    string `json:"created_at"`
}

type Release struct {
	ID          int    `json:"id"`
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

type Notification struct {
	ID        string `json:"id"`
	Reason    string `json:"reason"`
	Unread    bool   `json:"unread"`
	UpdatedAt string `json:"updated_at"`
	Subject   struct {
		Title string `json:"title"`
		Type  string `json:"type"`
		URL   string `json:"url"`
	} `json:"subject"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

type SearchResult struct {
	TotalCount int     `json:"total_count"`
	Items      []Issue `json:"items"`
}

type MergeResult struct {
	SHA     string `json:"sha"`
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any, accept string) (*http.Response, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode GitHub request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return nil, redactError(fmt.Errorf("create GitHub request: %w", err))
	}
	if accept == "" {
		accept = defaultAccept
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.UserAgent)
	if c.UserAgent == "" {
		req.Header.Set("User-Agent", "pegg-github-plugin")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, redactErrWith(c.Token, fmt.Errorf("GitHub request failed: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp, c.apiError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp, nil
	}
	if str, ok := out.(*string); ok {
		data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return resp, redactErrWith(c.Token, fmt.Errorf("read GitHub response: %w", err))
		}
		*str = string(data)
		return resp, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out); err != nil {
		if err == io.EOF {
			return resp, nil
		}
		return resp, redactErrWith(c.Token, fmt.Errorf("decode GitHub response: %w", err))
	}
	return resp, nil
}

func (c *Client) apiError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &payload)
	message := strings.TrimSpace(payload.Message)
	if message == "" {
		message = strings.TrimSpace(string(data))
	}
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		reset := "unknown"
		if raw := resp.Header.Get("X-RateLimit-Reset"); raw != "" {
			if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
				reset = time.Unix(unix, 0).UTC().Format(time.RFC3339)
			}
		}
		return redactErrWith(c.Token, fmt.Errorf("GitHub API %d: rate limit exceeded (resets at %s): %s", resp.StatusCode, reset, message))
	}
	return redactErrWith(c.Token, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, message))
}

func (c *Client) CurrentUser(ctx context.Context) (string, error) {
	var user User
	if _, err := c.do(ctx, http.MethodGet, "/user", nil, &user, ""); err != nil {
		return "", err
	}
	return user.Login, nil
}

func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	var out Repository
	if _, err := c.do(ctx, http.MethodGet, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo), nil, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListIssues(ctx context.Context, owner, repo, state string, labels []string, limit int) ([]Issue, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	query := url.Values{}
	if state == "" {
		state = "open"
	}
	query.Set("state", state)
	query.Set("per_page", strconv.Itoa(limit))
	query.Set("sort", "updated")
	query.Set("direction", "desc")
	if len(labels) > 0 {
		query.Set("labels", strings.Join(labels, ","))
	}
	var issues []Issue
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/issues?" + query.Encode()
	if _, err := c.do(ctx, http.MethodGet, path, nil, &issues, ""); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.PullRequest != nil {
			continue
		}
		out = append(out, issue)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (c *Client) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	var out Issue
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateIssue(ctx context.Context, owner, repo string, payload map[string]any) (*Issue, error) {
	var out Issue
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/issues"
	if _, err := c.do(ctx, http.MethodPost, path, payload, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateIssue(ctx context.Context, owner, repo string, number int, payload map[string]any) (*Issue, error) {
	var out Issue
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodPatch, path, payload, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateIssueComment(ctx context.Context, owner, repo string, number int, body string) (*Comment, error) {
	var out Comment
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodPost, path, map[string]any{"body": body}, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) ([]Label, error) {
	var out []Label
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/labels", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodPost, path, map[string]any{"labels": labels}, &out, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) AddAssignees(ctx context.Context, owner, repo string, number int, assignees []string) (*Issue, error) {
	var out Issue
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/assignees", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodPost, path, map[string]any{"assignees": assignees}, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/labels/%s", url.PathEscape(owner), url.PathEscape(repo), number, url.PathEscape(label))
	_, err := c.do(ctx, http.MethodDelete, path, nil, nil, "")
	return err
}

func (c *Client) RemoveAssignees(ctx context.Context, owner, repo string, number int, assignees []string) (*Issue, error) {
	var out Issue
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/assignees", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodDelete, path, map[string]any{"assignees": assignees}, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListPulls(ctx context.Context, owner, repo, state string, limit int) ([]PullRequest, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	if state == "" {
		state = "open"
	}
	query := url.Values{}
	query.Set("state", state)
	query.Set("per_page", strconv.Itoa(limit))
	query.Set("sort", "updated")
	query.Set("direction", "desc")
	var pulls []PullRequest
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/pulls?" + query.Encode()
	if _, err := c.do(ctx, http.MethodGet, path, nil, &pulls, ""); err != nil {
		return nil, err
	}
	out := make([]PullRequest, 0, len(pulls))
	for _, pr := range pulls {
		if pr.Number == 0 {
			continue
		}
		out = append(out, pr)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (c *Client) GetPull(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	var out PullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreatePull(ctx context.Context, owner, repo string, payload map[string]any) (*PullRequest, error) {
	var out PullRequest
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/pulls"
	if _, err := c.do(ctx, http.MethodPost, path, payload, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) MergePull(ctx context.Context, owner, repo string, number int, method, title, message string) (*MergeResult, error) {
	payload := map[string]any{}
	if method != "" {
		payload["merge_method"] = method
	}
	if title != "" {
		payload["commit_title"] = title
	}
	if message != "" {
		payload["commit_message"] = message
	}
	var out MergeResult
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodPut, path, payload, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListReviews(ctx context.Context, owner, repo string, number int) ([]Review, error) {
	var out []Review
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateReview(ctx context.Context, owner, repo string, number int, payload map[string]any) (*Review, error) {
	var out Review
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", url.PathEscape(owner), url.PathEscape(repo), number)
	if _, err := c.do(ctx, http.MethodPost, path, payload, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PullDiff(ctx context.Context, owner, repo string, number int) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repo), number)
	var out string
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out, "application/vnd.github.v3.diff"); err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) ListWorkflowRuns(ctx context.Context, owner, repo string, limit int) ([]WorkflowRun, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var payload struct {
		Runs []WorkflowRun `json:"workflow_runs"`
	}
	path := fmt.Sprintf("/repos/%s/%s/actions/runs?per_page=%d", url.PathEscape(owner), url.PathEscape(repo), limit)
	if _, err := c.do(ctx, http.MethodGet, path, nil, &payload, ""); err != nil {
		return nil, err
	}
	return payload.Runs, nil
}

func (c *Client) RerunWorkflow(ctx context.Context, owner, repo string, runID int) error {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/rerun", url.PathEscape(owner), url.PathEscape(repo), runID)
	_, err := c.do(ctx, http.MethodPost, path, nil, nil, "")
	return err
}

func (c *Client) ListReleases(ctx context.Context, owner, repo string, limit int) ([]Release, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var out []Release
	path := fmt.Sprintf("/repos/%s/%s/releases?per_page=%d", url.PathEscape(owner), url.PathEscape(repo), limit)
	if _, err := c.do(ctx, http.MethodGet, path, nil, &out, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateRelease(ctx context.Context, owner, repo string, payload map[string]any) (*Release, error) {
	var out Release
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/releases"
	if _, err := c.do(ctx, http.MethodPost, path, payload, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var out SearchResult
	values := url.Values{}
	values.Set("q", query)
	values.Set("per_page", strconv.Itoa(limit))
	if _, err := c.do(ctx, http.MethodGet, "/search/issues?"+values.Encode(), nil, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListNotifications(ctx context.Context, limit int) ([]Notification, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var out []Notification
	if _, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/notifications?per_page=%d", limit), nil, &out, ""); err != nil {
		return nil, err
	}
	return out, nil
}
