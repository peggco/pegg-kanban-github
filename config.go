package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Settings struct {
	Enabled               bool     `json:"enabled"`
	Token                 string   `json:"token"`
	Owner                 string   `json:"owner"`
	Repo                  string   `json:"repo"`
	APIURL                string   `json:"api_url"`
	DefaultBase           string   `json:"default_base"`
	WorktreeRoot          string   `json:"worktree_root"`
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"`
	SyncState             string   `json:"sync_state"`
	SyncLabels            []string `json:"sync_labels"`
	SyncLimit             int      `json:"sync_limit"`
	Workdir               string   `json:"-"`
}

func defaultSettings() Settings {
	wd, _ := os.Getwd()
	return Settings{
		Enabled:               true,
		APIURL:                "https://api.github.com",
		WorktreeRoot:          ".pegg-worktrees",
		RequestTimeoutSeconds: 30,
		SyncState:             "open",
		SyncLimit:             50,
		Workdir:               wd,
	}
}

func (s *Settings) applyHost(configJSON []byte) {
	if len(configJSON) == 0 {
		return
	}
	var host struct {
		Plugins struct {
			Options map[string]json.RawMessage `json:"options"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(configJSON, &host); err != nil {
		return
	}
	for _, key := range []string{"github", "pegg-github", "kanban-github", "pegg-kanban-github"} {
		raw := host.Plugins.Options[key]
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		_ = json.Unmarshal(raw, s)
	}
}

func (s *Settings) applyOptions(raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	_ = json.Unmarshal(raw, s)
}

func (s *Settings) applyEnv() {
	if v := os.Getenv("PEGG_GITHUB_REPO"); v != "" {
		if owner, repo, ok := splitRepoRef(v); ok {
			s.Owner = owner
			s.Repo = repo
		}
	} else {
		if v := os.Getenv("PEGG_GITHUB_OWNER"); v != "" {
			s.Owner = v
		}
		if v := os.Getenv("PEGG_GITHUB_REPO_NAME"); v != "" {
			s.Repo = v
		}
	}
	if v := os.Getenv("PEGG_GITHUB_API_URL"); v != "" {
		s.APIURL = v
	}
	if v := os.Getenv("PEGG_GITHUB_WORKTREE_ROOT"); v != "" {
		s.WorktreeRoot = v
	}
	if v := os.Getenv("PEGG_GITHUB_DEFAULT_BASE"); v != "" {
		s.DefaultBase = v
	}
	if v := os.Getenv("PEGG_GITHUB_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			s.RequestTimeoutSeconds = n
		}
	}
	s.normalize()
}

func (s *Settings) normalize() {
	s.APIURL = strings.TrimRight(strings.TrimSpace(s.APIURL), "/")
	if s.APIURL == "" {
		s.APIURL = "https://api.github.com"
	}
	s.WorktreeRoot = expandHome(s.WorktreeRoot)
	if strings.TrimSpace(s.WorktreeRoot) == "" {
		s.WorktreeRoot = ".pegg-worktrees"
	}
	s.Owner = strings.TrimSpace(s.Owner)
	s.Repo = strings.TrimSpace(s.Repo)
	s.Token = strings.TrimSpace(s.Token)
	s.DefaultBase = strings.TrimSpace(s.DefaultBase)
	if s.RequestTimeoutSeconds <= 0 {
		s.RequestTimeoutSeconds = 30
	}
}

func (s Settings) timeout() time.Duration {
	if s.RequestTimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(s.RequestTimeoutSeconds) * time.Second
}

func (s Settings) workdir() string {
	if strings.TrimSpace(s.Workdir) != "" {
		return s.Workdir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
