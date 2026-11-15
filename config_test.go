package main

import (
	"os"
	"path/filepath"
	"testing"
)

func clearGitHubEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"PEGG_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN", "PEGG_GITHUB_REPO",
		"PEGG_GITHUB_OWNER", "PEGG_GITHUB_REPO_NAME", "PEGG_GITHUB_API_URL",
		"PEGG_GITHUB_WORKTREE_ROOT", "PEGG_GITHUB_DEFAULT_BASE",
		"PEGG_GITHUB_TIMEOUT_SECONDS",
	} {
		t.Setenv(name, "")
	}
}

func TestDefaultSettings(t *testing.T) {
	clearGitHubEnv(t)
	settings := defaultSettings()
	if !settings.Enabled {
		t.Fatal("enabled should default to true")
	}
	if settings.APIURL != "https://api.github.com" {
		t.Fatalf("api_url = %q", settings.APIURL)
	}
	if settings.WorktreeRoot != ".pegg-worktrees" {
		t.Fatalf("worktree_root = %q", settings.WorktreeRoot)
	}
	if settings.SyncState != "open" || settings.SyncLimit != 50 {
		t.Fatalf("sync defaults = %q/%d", settings.SyncState, settings.SyncLimit)
	}
	if settings.RequestTimeoutSeconds != 30 {
		t.Fatalf("request_timeout_seconds = %d", settings.RequestTimeoutSeconds)
	}
	if settings.Workdir == "" {
		t.Fatal("workdir should default to the current directory")
	}
}

func TestApplyHostMergesPluginOptions(t *testing.T) {
	clearGitHubEnv(t)
	settings := defaultSettings()
	settings.applyHost([]byte(`{
		"plugins": {
			"options": {
				"github": {
					"owner": "legacy",
					"repo": "legacy",
					"default_base": "legacy-branch"
				},
				"pegg-github": {
					"request_timeout_seconds": 45
				},
				"kanban-github": {
					"owner": "acme",
					"repo": "widgets",
					"api_url": "https://ghe.example.com/api/v3/"
				},
				"pegg-kanban-github": {
					"default_base": "develop"
				}
			}
		}
	}`))
	settings.normalize()
	if settings.Owner != "acme" || settings.Repo != "widgets" {
		t.Fatalf("repo = %q/%q", settings.Owner, settings.Repo)
	}
	if settings.APIURL != "https://ghe.example.com/api/v3" {
		t.Fatalf("api_url = %q", settings.APIURL)
	}
	if settings.DefaultBase != "develop" {
		t.Fatalf("default_base = %q", settings.DefaultBase)
	}
	if settings.RequestTimeoutSeconds != 45 {
		t.Fatalf("timeout = %d", settings.RequestTimeoutSeconds)
	}
}

func TestApplyOptionsOverridesHost(t *testing.T) {
	clearGitHubEnv(t)
	settings := defaultSettings()
	settings.applyHost([]byte(`{"plugins":{"options":{"github":{"owner":"host","repo":"host"}}}}`))
	settings.applyOptions([]byte(`{"owner":"deps","repo":"deps"}`))
	if settings.Owner != "deps" || settings.Repo != "deps" {
		t.Fatalf("options did not win: %q/%q", settings.Owner, settings.Repo)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	clearGitHubEnv(t)
	t.Setenv("PEGG_GITHUB_REPO", "env-owner/env-repo")
	t.Setenv("PEGG_GITHUB_API_URL", "http://127.0.0.1:9999/api/")
	t.Setenv("PEGG_GITHUB_WORKTREE_ROOT", "~/.pegg/worktrees")
	t.Setenv("PEGG_GITHUB_DEFAULT_BASE", "trunk")
	t.Setenv("PEGG_GITHUB_TIMEOUT_SECONDS", "12")

	settings := defaultSettings()
	settings.Owner = "config-owner"
	settings.Repo = "config-repo"
	settings.applyEnv()

	if settings.Owner != "env-owner" || settings.Repo != "env-repo" {
		t.Fatalf("repo = %q/%q", settings.Owner, settings.Repo)
	}
	if settings.APIURL != "http://127.0.0.1:9999/api" {
		t.Fatalf("api_url = %q", settings.APIURL)
	}
	if settings.DefaultBase != "trunk" {
		t.Fatalf("default_base = %q", settings.DefaultBase)
	}
	if settings.RequestTimeoutSeconds != 12 {
		t.Fatalf("timeout = %d", settings.RequestTimeoutSeconds)
	}
	home, _ := os.UserHomeDir()
	if settings.WorktreeRoot != filepath.Join(home, ".pegg", "worktrees") {
		t.Fatalf("worktree_root = %q", settings.WorktreeRoot)
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := expandHome("~"); got != home {
		t.Fatalf("~ = %q", got)
	}
	if got := expandHome("~/projects"); got != filepath.Join(home, "projects") {
		t.Fatalf("~/projects = %q", got)
	}
	if got := expandHome("relative/path"); got != "relative/path" {
		t.Fatalf("relative = %q", got)
	}
	if got := expandHome(""); got != "" {
		t.Fatalf("empty = %q", got)
	}
}
