package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	settings := defaultSettings()
	settings.APIURL = server.URL
	settings.Token = "test-token-value"
	return NewClient(settings, settings.Token), server
}

func TestClientHeadersAndDecode(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/repos/acme/widgets" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != defaultAccept {
			t.Errorf("accept = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token-value" {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("api version = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "pegg-github-plugin" {
			t.Errorf("user agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"widgets","full_name":"acme/widgets","default_branch":"main","html_url":"https://github.com/acme/widgets","owner":{"login":"acme"}}`)
	})
	repo, err := client.GetRepository(context.Background(), "acme", "widgets")
	if err != nil {
		t.Fatal(err)
	}
	if repo.FullName != "acme/widgets" || repo.DefaultBranch != "main" || repo.Owner.Login != "acme" {
		t.Fatalf("repo = %#v", repo)
	}
}

func TestClientAPIError(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found","documentation_url":"https://docs.github.com"}`)
	})
	_, err := client.GetRepository(context.Background(), "acme", "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "GitHub API 404: Not Found") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "test-token-value") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestClientRateLimitError(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1800000000")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	})
	_, err := client.GetRepository(context.Background(), "acme", "widgets")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") || !strings.Contains(err.Error(), "resets at") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "test-token-value") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestClientAPIErrorRedactsResponseToken(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"message":"bad token ghp_abcdefghijklmnopqrstuvwxyz0123456789"}`)
	})
	_, err := client.GetRepository(context.Background(), "acme", "widgets")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "ghp_") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestListIssuesFiltersPullRequests(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/issues" {
			t.Errorf("path = %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("state") != "open" {
			t.Errorf("state = %q", query.Get("state"))
		}
		if query.Get("labels") != "bug,help" {
			t.Errorf("labels = %q", query.Get("labels"))
		}
		if query.Get("per_page") != "50" {
			t.Errorf("per_page = %q", query.Get("per_page"))
		}
		fmt.Fprint(w, `[
			{"number":1,"title":"issue","state":"open","pull_request":null},
			{"number":2,"title":"pr","state":"open","pull_request":{"url":"https://api.github.com/repos/acme/widgets/pulls/2"}},
			{"number":3,"title":"another","state":"open"}
		]`)
	})
	issues, err := client.ListIssues(context.Background(), "acme", "widgets", "open", []string{"bug", "help"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %d: %#v", len(issues), issues)
	}
	if issues[0].Number != 1 || issues[1].Number != 3 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestListPullsDecodeAndFilter(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/pulls" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `[
			{"number":0,"title":"invalid"},
			{"number":7,"title":"fix","state":"open","head":{"ref":"pegg/fix"},"base":{"ref":"main"},"html_url":"https://github.com/acme/widgets/pull/7"}
		]`)
	})
	pulls, err := client.ListPulls(context.Background(), "acme", "widgets", "open", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 || pulls[0].Number != 7 || pulls[0].Head.Ref != "pegg/fix" {
		t.Fatalf("pulls = %#v", pulls)
	}
}

func TestPullDiffUsesDiffAccept(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.github.v3.diff" {
			t.Errorf("accept = %q", got)
		}
		if r.URL.Path != "/repos/acme/widgets/pulls/7" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, "diff --git a/x b/x\n+hello\n")
	})
	diff, err := client.PullDiff(context.Background(), "acme", "widgets", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+hello") {
		t.Fatalf("diff = %q", diff)
	}
}

func TestCurrentUser(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"login":"octocat"}`)
	})
	login, err := client.CurrentUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if login != "octocat" {
		t.Fatalf("login = %q", login)
	}
}

func TestClientDecodesBridgeShapes(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widgets/issues/42":
			if r.Method != http.MethodGet {
				t.Errorf("method = %s", r.Method)
			}
			fmt.Fprint(w, `{"number":42,"title":"Fix login","body":"body","state":"open","html_url":"https://github.com/acme/widgets/issues/42","user":{"login":"alice"},"labels":[{"name":"bug"}],"assignees":[{"login":"dev"}],"created_at":"2026-11-13T12:00:00Z","updated_at":"2026-11-13T12:00:00Z","comments":3}`)
		case "/repos/acme/widgets/issues/42/comments":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("body: %v", err)
			}
			if payload["body"] != "hello" {
				t.Errorf("body = %#v", payload["body"])
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":1,"html_url":"https://github.com/acme/widgets/issues/42#issuecomment-1"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	issue, err := client.GetIssue(context.Background(), "acme", "widgets", 42)
	if err != nil {
		t.Fatal(err)
	}
	view := viewIssue(*issue)
	if view.Author != "alice" || len(view.Labels) != 1 || view.Labels[0] != "bug" {
		t.Fatalf("view = %#v", view)
	}
	comment, err := client.CreateIssueComment(context.Background(), "acme", "widgets", 42, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(comment.HTMLURL, "issuecomment-1") {
		t.Fatalf("comment = %#v", comment)
	}
}
