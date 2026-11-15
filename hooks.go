package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	peggplugin "github.com/peggco/pegg/pkg/plugin"
)

const (
	hookProviders     = "kanban.providers"
	hookIssueFetch    = "kanban.issues.fetch"
	hookRunPrepare    = "kanban.run.prepare"
	hookCardSucceeded = "kanban.card.succeeded"

	hookPriority = 10
)

type ProviderInfo struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Enabled       bool   `json:"enabled"`
	Available     bool   `json:"available"`
	Authenticated bool   `json:"authenticated"`
	Detail        string `json:"detail,omitempty"`
	Target        string `json:"target,omitempty"`
	SyncMinutes   int    `json:"sync_minutes,omitempty"`
}

type HookIssue struct {
	ExternalID string   `json:"external_id,omitempty"`
	Number     int      `json:"number,omitempty"`
	Key        string   `json:"key,omitempty"`
	Title      string   `json:"title"`
	Body       string   `json:"body,omitempty"`
	URL        string   `json:"url,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	Assignees  []string `json:"assignees,omitempty"`
	State      string   `json:"state,omitempty"`
	Priority   string   `json:"priority,omitempty"`
	CreatedAt  string   `json:"created_at,omitempty"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
}

type IssueFetch struct {
	Provider string      `json:"provider"`
	Target   string      `json:"target,omitempty"`
	Labels   []string    `json:"labels,omitempty"`
	State    string      `json:"state,omitempty"`
	Limit    int         `json:"limit,omitempty"`
	Query    string      `json:"query,omitempty"`
	Issues   []HookIssue `json:"issues"`
}

type IssueRef struct {
	Provider string `json:"provider,omitempty"`
	Repo     string `json:"repo"`
	Number   int    `json:"number"`
	Key      string `json:"key,omitempty"`
	URL      string `json:"url"`
}

type Card struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body,omitempty"`
	Column    string    `json:"column"`
	Priority  string    `json:"priority,omitempty"`
	Labels    []string  `json:"labels,omitempty"`
	Issue     *IssueRef `json:"issue,omitempty"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

type RunContext struct {
	Kind   string            `json:"kind,omitempty"`
	Path   string            `json:"path,omitempty"`
	Branch string            `json:"branch,omitempty"`
	Base   string            `json:"base,omitempty"`
	Label  string            `json:"label,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
	Meta   map[string]string `json:"meta,omitempty"`
}

type RunPrepare struct {
	Card       Card       `json:"card"`
	Repo       string     `json:"repo"`
	BaseBranch string     `json:"base_branch,omitempty"`
	Context    RunContext `json:"context"`
}

type CardSucceeded struct {
	Card                Card        `json:"card"`
	Context             *RunContext `json:"context,omitempty"`
	Summary             string      `json:"summary,omitempty"`
	Feedback            bool        `json:"feedback"`
	CreateChangeRequest bool        `json:"create_change_request"`
	TransitionOnSuccess string      `json:"transition_on_success,omitempty"`
	BaseBranch          string      `json:"base_branch,omitempty"`
}

type cardSucceededResult struct {
	IssueNumber    int    `json:"issue_number,omitempty"`
	Commented      bool   `json:"commented"`
	PullRequest    int    `json:"pull_request,omitempty"`
	PullRequestURL string `json:"pull_request_url,omitempty"`
}

func hookSpecs() []peggplugin.HookSpec {
	return []peggplugin.HookSpec{
		{
			Name:        hookProviders,
			Kind:        peggplugin.HookFilter,
			Priority:    hookPriority,
			Description: "Report GitHub as a Kanban provider",
		},
		{
			Name:        hookIssueFetch,
			Kind:        peggplugin.HookFilter,
			Priority:    hookPriority,
			Description: "Fetch GitHub issues for Kanban import",
		},
		{
			Name:        hookRunPrepare,
			Kind:        peggplugin.HookFilter,
			Priority:    hookPriority,
			Description: "Prepare a Kanban run environment with a GitHub worktree",
		},
		{
			Name:        hookCardSucceeded,
			Kind:        peggplugin.HookAction,
			Priority:    hookPriority,
			Description: "Comment and open a pull request when a Kanban card succeeds",
		},
	}
}

func (g *GitHub) Hooks() ([]peggplugin.HookSpec, error) {
	return hookSpecs(), nil
}

func (g *GitHub) apiClient(settings Settings) *Client {
	token, _ := resolveToken(settings)
	return NewClient(settings, token)
}

func (g *GitHub) ExecuteHook(req peggplugin.HookRequest) (*peggplugin.HookResult, error) {
	settings := g.currentSettings()
	if !settings.Enabled {
		return &peggplugin.HookResult{Handled: false}, nil
	}
	timeout := settings.timeout()
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	value, handled, err := g.executeHook(ctx, settings, req)
	if err != nil {
		return nil, redactError(err)
	}
	return &peggplugin.HookResult{Value: value, Handled: handled}, nil
}

func (g *GitHub) executeHook(ctx context.Context, settings Settings, req peggplugin.HookRequest) (json.RawMessage, bool, error) {
	switch req.Name {
	case hookProviders:
		return g.providersHook(ctx, settings, req.Value)
	case hookIssueFetch:
		var payload IssueFetch
		if err := decodeHookValue(req.Value, &payload); err != nil {
			return nil, false, err
		}
		if strings.TrimSpace(payload.Provider) != "github" {
			return nil, false, nil
		}
		return g.fetchIssuesHook(ctx, settings, payload)
	case hookRunPrepare:
		var payload RunPrepare
		if err := decodeHookValue(req.Value, &payload); err != nil {
			return nil, false, err
		}
		return g.runPrepareHook(settings, payload)
	case hookCardSucceeded:
		var payload CardSucceeded
		if err := decodeHookValue(req.Value, &payload); err != nil {
			return nil, false, err
		}
		if payload.Card.Issue == nil || strings.TrimSpace(payload.Card.Issue.Provider) != "github" {
			return nil, false, nil
		}
		return g.cardSucceededHook(ctx, settings, payload)
	default:
		return nil, false, nil
	}
}

func decodeHookValue(raw json.RawMessage, out any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode hook payload: %w", err)
	}
	return nil
}

func (g *GitHub) providersHook(ctx context.Context, settings Settings, raw json.RawMessage) (json.RawMessage, bool, error) {
	providers, err := decodeProviders(raw)
	if err != nil {
		return nil, false, err
	}
	status := authenticationStatus(ctx, settings)
	target := ""
	if _, _, full, resolveErr := resolveRepo(settings, ""); resolveErr == nil {
		target = full
	}
	info := ProviderInfo{
		ID:            "github",
		Label:         "GitHub",
		Enabled:       settings.Enabled,
		Available:     settings.Enabled && status.Authenticated,
		Authenticated: status.Authenticated,
		Detail:        providerDetail(settings, status, target),
		Target:        target,
	}
	out := make([]ProviderInfo, 0, len(providers)+1)
	for _, provider := range providers {
		if provider.ID != "github" {
			out = append(out, provider)
		}
	}
	out = append(out, info)
	value, err := json.Marshal(out)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func decodeProviders(raw json.RawMessage) ([]ProviderInfo, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var providers []ProviderInfo
		if err := json.Unmarshal(raw, &providers); err != nil {
			return nil, fmt.Errorf("decode hook payload: %w", err)
		}
		return providers, nil
	}
	var provider ProviderInfo
	if err := json.Unmarshal(raw, &provider); err != nil {
		return nil, fmt.Errorf("decode hook payload: %w", err)
	}
	return []ProviderInfo{provider}, nil
}

func providerDetail(settings Settings, status AuthStatus, target string) string {
	switch {
	case !settings.Enabled:
		return "Plugin disabled; enable GitHub to sync issues"
	case !status.Authenticated:
		return "Authenticate with `gh auth login` or set plugins.options.kanban-github.token"
	case target == "":
		return "Set plugins.options.kanban-github.repo to owner/name"
	default:
		return "Ready to sync from " + target
	}
}

func (g *GitHub) runPrepareHook(settings Settings, payload RunPrepare) (json.RawMessage, bool, error) {
	if !settings.Enabled || strings.TrimSpace(payload.Repo) == "" {
		return nil, false, nil
	}
	base := strings.TrimSpace(payload.BaseBranch)
	if base == "" {
		base = strings.TrimSpace(settings.DefaultBase)
	}
	issue := 0
	if payload.Card.Issue != nil {
		issue = payload.Card.Issue.Number
	}
	slug := worktreeSlugForCard(payload.Card)
	manager, err := NewWorktreeManager(payload.Repo, settings.WorktreeRoot, settings.DefaultBase)
	if err != nil {
		return nil, false, err
	}
	entry, err := manager.Create(CreateWorktreeOptions{
		Slug:   slug,
		Branch: "pegg/" + slug,
		Base:   base,
		Issue:  issue,
		CardID: payload.Card.ID,
	})
	if err != nil {
		return nil, false, err
	}
	payload.Context = RunContext{
		Kind:   "worktree",
		Path:   entry.Path,
		Branch: entry.Branch,
		Base:   entry.Base,
		Label:  entry.Slug,
	}
	value, err := json.Marshal(payload)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func worktreeSlugForCard(card Card) string {
	base := slugify(card.Title)
	if base == "" {
		base = "card"
	}
	if len(base) > 50 {
		base = strings.Trim(base[:50], "-")
	}
	suffix := card.ID
	if idx := strings.LastIndex(card.ID, "-"); idx >= 0 && idx+1 < len(card.ID) {
		suffix = card.ID[idx+1:]
	}
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	slug := slugify(base + "-" + suffix)
	if slug == "" {
		slug = "card"
	}
	return slug
}

func (g *GitHub) fetchIssuesHook(ctx context.Context, settings Settings, payload IssueFetch) (json.RawMessage, bool, error) {
	owner, repo, full, err := resolveRepo(settings, payload.Target)
	if err != nil {
		return nil, false, err
	}
	state := strings.TrimSpace(payload.State)
	if state == "" {
		state = strings.TrimSpace(settings.SyncState)
	}
	if state == "" {
		state = "open"
	}
	labels := payload.Labels
	if len(labels) == 0 {
		labels = settings.SyncLabels
	}
	limit := payload.Limit
	if limit <= 0 {
		limit = settings.SyncLimit
	}
	issues, err := g.apiClient(settings).ListIssues(ctx, owner, repo, state, labels, limit)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(payload.Target) == "" {
		payload.Target = full
	}
	for _, issue := range issues {
		payload.Issues = append(payload.Issues, hookIssue(issue))
	}
	if payload.Issues == nil {
		payload.Issues = []HookIssue{}
	}
	value, err := json.Marshal(payload)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func hookIssue(issue Issue) HookIssue {
	labels := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		labels = append(labels, label.Name)
	}
	assignees := make([]string, 0, len(issue.Assignees))
	for _, assignee := range issue.Assignees {
		assignees = append(assignees, assignee.Login)
	}
	return HookIssue{
		Number:    issue.Number,
		Title:     issue.Title,
		Body:      issue.Body,
		URL:       issue.HTMLURL,
		Labels:    labels,
		Assignees: assignees,
		State:     issue.State,
		CreatedAt: issue.CreatedAt,
		UpdatedAt: issue.UpdatedAt,
	}
}

func (g *GitHub) cardSucceededHook(ctx context.Context, settings Settings, payload CardSucceeded) (json.RawMessage, bool, error) {
	issue := payload.Card.Issue
	owner, repo, _, err := resolveRepo(settings, issue.Repo)
	if err != nil {
		return nil, false, err
	}
	client := g.apiClient(settings)
	result := cardSucceededResult{IssueNumber: issue.Number}

	if payload.Feedback && strings.TrimSpace(payload.Summary) != "" {
		if issue.Number <= 0 {
			return nil, false, fmt.Errorf("github issue number is required for feedback")
		}
		if _, err := client.CreateIssueComment(ctx, owner, repo, issue.Number, payload.Summary); err != nil {
			return nil, false, err
		}
		result.Commented = true
	}

	if payload.CreateChangeRequest && payload.Context != nil {
		branch := strings.TrimSpace(payload.Context.Branch)
		if branch != "" {
			title := strings.TrimSpace(payload.Card.Title)
			if title == "" {
				return nil, false, fmt.Errorf("card title is required to create a pull request")
			}
			base := strings.TrimSpace(payload.Context.Base)
			if base == "" {
				base = strings.TrimSpace(payload.BaseBranch)
			}
			if base == "" {
				base = strings.TrimSpace(settings.DefaultBase)
			}
			if base == "" {
				repository, err := client.GetRepository(ctx, owner, repo)
				if err != nil {
					return nil, false, fmt.Errorf("base branch is required (set base_branch or default_base)")
				}
				base = repository.DefaultBranch
			}
			pull, err := client.CreatePull(ctx, owner, repo, map[string]any{
				"title": title,
				"body":  cardChangeRequestBody(payload.Card.Body, payload.Summary),
				"base":  base,
				"head":  branch,
				"draft": false,
			})
			if err != nil {
				return nil, false, err
			}
			result.PullRequest = pull.Number
			result.PullRequestURL = pull.HTMLURL
		}
	}

	value, err := json.Marshal(result)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func cardChangeRequestBody(body, summary string) string {
	body = strings.TrimSpace(body)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return body
	}
	if body == "" {
		return summary
	}
	return body + "\n\n" + summary
}
