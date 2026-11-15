package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	peggplugin "github.com/peggco/pegg/pkg/plugin"
)

func testPlugin(t *testing.T, apiURL string) *GitHub {
	t.Helper()
	settings := defaultSettings()
	settings.APIURL = apiURL
	settings.Token = "test-token-value"
	settings.Workdir = t.TempDir()
	g := &GitHub{}
	g.setSettings(settings)
	return g
}

func TestCapabilitiesAreHooksOnly(t *testing.T) {
	got := (&GitHub{}).Capabilities()
	if len(got) != 1 || got[0] != string(peggplugin.CapabilityHooks) {
		t.Fatalf("capabilities = %#v, want exactly [hooks]", got)
	}
}

func TestHookSpecsAndCapability(t *testing.T) {
	g := &GitHub{}
	specs, err := g.Hooks()
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name string
		kind peggplugin.HookKind
	}{
		{hookProviders, peggplugin.HookFilter},
		{hookIssueFetch, peggplugin.HookFilter},
		{hookRunPrepare, peggplugin.HookFilter},
		{hookCardSucceeded, peggplugin.HookAction},
	}
	if len(specs) != len(want) {
		t.Fatalf("specs = %#v", specs)
	}
	for i, expected := range want {
		if specs[i].Name != expected.name || specs[i].Kind != expected.kind || specs[i].Priority != hookPriority {
			t.Fatalf("spec %d = %#v", i, specs[i])
		}
	}
	found := false
	for _, capability := range g.Capabilities() {
		if capability == string(peggplugin.CapabilityHooks) {
			found = true
		}
	}
	if !found {
		t.Fatalf("capabilities = %#v", g.Capabilities())
	}
}

func TestExecuteHookDisabledNoop(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	g := testPlugin(t, server.URL)
	settings := g.currentSettings()
	settings.Enabled = false
	g.setSettings(settings)

	hookRequests := []peggplugin.HookRequest{
		{Name: hookProviders},
		{Name: hookIssueFetch, Value: json.RawMessage(`{"provider":"github"}`)},
		{Name: hookCardSucceeded, Value: json.RawMessage(`{"card":{"issue":{"provider":"github","repo":"acme/widgets","number":1}}}`)},
	}
	for _, req := range hookRequests {
		result, err := g.ExecuteHook(req)
		if err != nil {
			t.Fatalf("%s: %v", req.Name, err)
		}
		if result == nil || result.Handled || result.Value != nil {
			t.Fatalf("%s result = %#v", req.Name, result)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("disabled plugin made %d requests", requests.Load())
	}
}

func TestProvidersHook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"login":"octocat"}`)
	}))
	defer server.Close()

	g := testPlugin(t, server.URL)
	settings := g.currentSettings()
	settings.Owner = "acme"
	settings.Repo = "widgets"
	g.setSettings(settings)

	result, err := g.ExecuteHook(peggplugin.HookRequest{
		Name:  hookProviders,
		Kind:  peggplugin.HookFilter,
		Value: json.RawMessage(`[{"id":"jira","label":"Jira","enabled":false}]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Handled {
		t.Fatalf("result = %#v", result)
	}
	var providers []ProviderInfo
	if err := json.Unmarshal(result.Value, &providers); err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 || providers[0].ID != "jira" {
		t.Fatalf("providers = %#v", providers)
	}
	github := providers[1]
	if github.ID != "github" || github.Label != "GitHub" {
		t.Fatalf("provider = %#v", github)
	}
	if !github.Enabled || !github.Available || !github.Authenticated || github.Target != "acme/widgets" {
		t.Fatalf("provider = %#v", github)
	}
}

func TestProvidersHookWithoutRepo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"login":"octocat"}`)
	}))
	defer server.Close()

	g := testPlugin(t, server.URL)

	result, err := g.ExecuteHook(peggplugin.HookRequest{Name: hookProviders})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Handled {
		t.Fatalf("result = %#v", result)
	}
	var providers []ProviderInfo
	if err := json.Unmarshal(result.Value, &providers); err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 {
		t.Fatalf("providers = %#v", providers)
	}
	github := providers[0]
	if !github.Available || github.Target != "" || !strings.Contains(github.Detail, "kanban-github.repo") {
		t.Fatalf("provider = %#v", github)
	}
}

func TestIssueFetchOtherProviderNoop(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()

	g := testPlugin(t, server.URL)
	result, err := g.ExecuteHook(peggplugin.HookRequest{
		Name:  hookIssueFetch,
		Kind:  peggplugin.HookFilter,
		Value: json.RawMessage(`{"provider":"jira","target":"PROJ"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Handled || result.Value != nil {
		t.Fatalf("result = %#v", result)
	}
	if requests.Load() != 0 {
		t.Fatalf("other provider made %d requests", requests.Load())
	}
}

func TestIssueFetchHookMapsIssues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/issues" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("state") != "open" {
			t.Errorf("state = %s", r.URL.Query().Get("state"))
		}
		if r.URL.Query().Get("per_page") != "10" {
			t.Errorf("per_page = %s", r.URL.Query().Get("per_page"))
		}
		if r.URL.Query().Get("labels") != "bug,urgent" {
			t.Errorf("labels = %s", r.URL.Query().Get("labels"))
		}
		fmt.Fprint(w, `[
			{"number":42,"title":"Fix login","body":"broken","state":"open","html_url":"https://github.com/acme/widgets/issues/42","labels":[{"name":"bug"}],"assignees":[{"login":"dev"}],"created_at":"2026-11-13T12:00:00Z","updated_at":"2026-11-13T13:00:00Z"},
			{"number":43,"title":"A pull request","state":"open","html_url":"https://github.com/acme/widgets/pull/43","pull_request":{}}
		]`)
	}))
	defer server.Close()

	g := testPlugin(t, server.URL)
	result, err := g.ExecuteHook(peggplugin.HookRequest{
		Name: hookIssueFetch,
		Kind: peggplugin.HookFilter,
		Value: json.RawMessage(`{
			"provider":"github",
			"target":"acme/widgets",
			"labels":["bug","urgent"],
			"limit":10,
			"issues":[{"number":1,"title":"Already here"}]
		}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Handled {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(string(result.Value), "test-token-value") {
		t.Fatalf("token leaked: %s", result.Value)
	}
	var payload IssueFetch
	if err := json.Unmarshal(result.Value, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Provider != "github" || payload.Target != "acme/widgets" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload.Issues) != 2 {
		t.Fatalf("issues = %#v", payload.Issues)
	}
	if payload.Issues[0].Title != "Already here" {
		t.Fatalf("existing issue = %#v", payload.Issues[0])
	}
	issue := payload.Issues[1]
	if issue.Number != 42 || issue.Title != "Fix login" || issue.Body != "broken" ||
		issue.URL != "https://github.com/acme/widgets/issues/42" || issue.State != "open" ||
		issue.CreatedAt != "2026-11-13T12:00:00Z" || issue.UpdatedAt != "2026-11-13T13:00:00Z" {
		t.Fatalf("issue = %#v", issue)
	}
	if len(issue.Labels) != 1 || issue.Labels[0] != "bug" || len(issue.Assignees) != 1 || issue.Assignees[0] != "dev" {
		t.Fatalf("issue = %#v", issue)
	}
}

func TestCardSucceededHookCommentsAndCreatesPull(t *testing.T) {
	var commented, pulled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/widgets/issues/42/comments":
			commented = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode comment: %v", err)
			}
			if payload["body"] != "Run finished successfully" {
				t.Errorf("comment = %#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":1,"body":"Run finished successfully"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/widgets/pulls":
			pulled = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode pull: %v", err)
			}
			if payload["title"] != "Fix login" || payload["head"] != "pegg/fix" ||
				payload["base"] != "main" || payload["draft"] != false {
				t.Errorf("pull = %#v", payload)
			}
			body, _ := payload["body"].(string)
			if body != "Closes #42\n\nRun finished successfully" {
				t.Errorf("pull body = %q", body)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"number":7,"html_url":"https://github.com/acme/widgets/pull/7","state":"open"}`)
		default:
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	g := testPlugin(t, server.URL)
	value := map[string]any{
		"card": map[string]any{
			"id":     "card-1",
			"title":  "Fix login",
			"body":   "Closes #42",
			"column": "done",
			"issue": map[string]any{
				"provider": "github",
				"repo":     "acme/widgets",
				"number":   42,
				"url":      "https://github.com/acme/widgets/issues/42",
			},
		},
		"context": map[string]any{
			"kind":   "worktree",
			"path":   "/tmp/fix-login",
			"branch": "pegg/fix",
			"base":   "main",
		},
		"summary":               "Run finished successfully",
		"feedback":              true,
		"create_change_request": true,
		"base_branch":           "main",
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	result, err := g.ExecuteHook(peggplugin.HookRequest{
		Name:  hookCardSucceeded,
		Kind:  peggplugin.HookAction,
		Value: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Handled {
		t.Fatalf("result = %#v", result)
	}
	if !commented || !pulled {
		t.Fatalf("commented = %v pulled = %v", commented, pulled)
	}
	var output cardSucceededResult
	if err := json.Unmarshal(result.Value, &output); err != nil {
		t.Fatal(err)
	}
	if output.IssueNumber != 42 || !output.Commented || output.PullRequest != 7 ||
		output.PullRequestURL != "https://github.com/acme/widgets/pull/7" {
		t.Fatalf("output = %#v", output)
	}
	if strings.Contains(string(result.Value), "test-token-value") {
		t.Fatalf("token leaked: %s", result.Value)
	}
}

func TestCardSucceededOtherProviderNoop(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	g := testPlugin(t, server.URL)
	result, err := g.ExecuteHook(peggplugin.HookRequest{
		Name:  hookCardSucceeded,
		Kind:  peggplugin.HookAction,
		Value: json.RawMessage(`{"card":{"issue":{"provider":"gitlab","repo":"acme/widgets","number":1}},"feedback":true,"summary":"done"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Handled || result.Value != nil {
		t.Fatalf("result = %#v", result)
	}
	if requests.Load() != 0 {
		t.Fatalf("other provider made %d requests", requests.Load())
	}
}

func TestWorktreePrepareHook(t *testing.T) {
	repo := initGitRepo(t)
	g := &GitHub{settings: Settings{
		Enabled:      true,
		WorktreeRoot: ".pegg-worktrees",
		DefaultBase:  "main",
	}}
	payload, _ := json.Marshal(RunPrepare{
		Card:       Card{ID: "card-abc", Title: "Fix login"},
		Repo:       repo,
		BaseBranch: "main",
		Context:    RunContext{Kind: "repo", Path: repo},
	})
	result, err := g.ExecuteHook(peggplugin.HookRequest{Name: hookRunPrepare, Value: payload})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Handled {
		t.Fatalf("result = %#v", result)
	}
	var prepared RunPrepare
	if err := json.Unmarshal(result.Value, &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.Context.Kind != "worktree" || prepared.Context.Path == "" || !strings.Contains(prepared.Context.Path, ".pegg-worktrees") {
		t.Fatalf("context = %#v", prepared.Context)
	}
	if prepared.Context.Branch == "" || !strings.HasPrefix(prepared.Context.Branch, "pegg/") {
		t.Fatalf("branch = %q", prepared.Context.Branch)
	}
}
