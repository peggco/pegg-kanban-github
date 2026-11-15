package main

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

func splitRepoRef(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	if strings.HasPrefix(value, "git@") || strings.HasPrefix(value, "ssh://") {
		if owner, repo, ok := parseGitURL(value); ok {
			return owner, repo, true
		}
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		if owner, repo, ok := parseGitURL(value); ok {
			return owner, repo, true
		}
	}
	if strings.Contains(value, ":") && strings.Contains(value, "@") {
		if owner, repo, ok := parseGitURL(value); ok {
			return owner, repo, true
		}
	}
	value = strings.TrimSuffix(value, ".git")
	value = strings.Trim(value, "/")
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner, repo := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if !validRepoSegment(owner) || !validRepoSegment(repo) {
		return "", "", false
	}
	return owner, repo, true
}

func parseGitURL(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".git")
	if strings.HasPrefix(value, "git@") {
		at := strings.Index(value, "@")
		colon := strings.Index(value[at+1:], ":")
		if at < 0 || colon < 0 {
			return "", "", false
		}
		value = value[at+1+colon+1:]
	} else {
		if !strings.Contains(value, "://") {
			value = "https://" + value
		}
		parsed, err := url.Parse(value)
		if err != nil {
			return "", "", false
		}
		value = parsed.Path
	}
	value = strings.Trim(value, "/")
	parts := strings.Split(value, "/")
	if len(parts) < 2 {
		return "", "", false
	}
	owner, repo := parts[0], parts[1]
	if !validRepoSegment(owner) || !validRepoSegment(repo) {
		return "", "", false
	}
	return owner, repo, true
}

func validRepoSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	return !strings.ContainsAny(value, " \t\r\n")
}

func resolveRepo(settings Settings, arg string) (string, string, string, error) {
	if owner, repo, ok := splitRepoRef(arg); ok {
		return owner, repo, owner + "/" + repo, nil
	}
	if settings.Owner != "" && settings.Repo != "" {
		return settings.Owner, settings.Repo, settings.Owner + "/" + settings.Repo, nil
	}
	if remote := gitRemoteRepo(settings.workdir()); remote != "" {
		if owner, repo, ok := splitRepoRef(remote); ok {
			return owner, repo, owner + "/" + repo, nil
		}
	}
	return "", "", "", fmt.Errorf("repo is required: set plugins.options.kanban-github.repo to owner/name")
}

func gitRemoteRepo(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	if out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	out, err := exec.Command("git", "-C", dir, "remote").Output()
	if err != nil {
		return ""
	}
	for _, name := range strings.Fields(string(out)) {
		if remote, err := exec.Command("git", "-C", dir, "remote", "get-url", name).Output(); err == nil {
			return strings.TrimSpace(string(remote))
		}
	}
	return ""
}
