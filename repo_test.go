package main

import (
	"strings"
	"testing"
)

func TestSplitRepoRef(t *testing.T) {
	cases := []struct {
		input string
		owner string
		repo  string
		ok    bool
	}{
		{"acme/widgets", "acme", "widgets", true},
		{"acme/widgets.git", "acme", "widgets", true},
		{"https://github.com/acme/widgets", "acme", "widgets", true},
		{"https://github.com/acme/widgets.git", "acme", "widgets", true},
		{"git@github.com:acme/widgets.git", "acme", "widgets", true},
		{"ssh://git@github.com/acme/widgets.git", "acme", "widgets", true},
		{"https://ghe.example.com/acme/widgets", "acme", "widgets", true},
		{"not-a-repo", "", "", false},
		{"/home/user/repo", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		owner, repo, ok := splitRepoRef(tc.input)
		if ok != tc.ok || owner != tc.owner || repo != tc.repo {
			t.Errorf("splitRepoRef(%q) = %q, %q, %t; want %q, %q, %t", tc.input, owner, repo, ok, tc.owner, tc.repo, tc.ok)
		}
	}
}

func TestResolveRepoPrecedence(t *testing.T) {
	settings := defaultSettings()
	settings.Owner = "config-owner"
	settings.Repo = "config-repo"
	settings.Workdir = t.TempDir()

	owner, repo, full, err := resolveRepo(settings, "arg-owner/arg-repo")
	if err != nil || owner != "arg-owner" || repo != "arg-repo" || full != "arg-owner/arg-repo" {
		t.Fatalf("arg resolution = %q %q %q %v", owner, repo, full, err)
	}
	owner, repo, full, err = resolveRepo(settings, "")
	if err != nil || full != "config-owner/config-repo" {
		t.Fatalf("config resolution = %q %q %q %v", owner, repo, full, err)
	}

	gitRepo := initGitRepo(t)
	mustGit(t, gitRepo, "remote", "add", "origin", "git@github.com:remote-owner/remote-repo.git")
	settings.Owner = ""
	settings.Repo = ""
	settings.Workdir = gitRepo
	owner, repo, full, err = resolveRepo(settings, "")
	if err != nil || owner != "remote-owner" || repo != "remote-repo" {
		t.Fatalf("remote resolution = %q %q %q %v", owner, repo, full, err)
	}

	settings.Workdir = t.TempDir()
	if _, _, _, err := resolveRepo(settings, ""); err == nil || !strings.Contains(err.Error(), "repo is required") {
		t.Fatalf("missing repo error = %v", err)
	}
}
