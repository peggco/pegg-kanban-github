package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	tokenSourceConfig = "config"
	tokenSourceEnv    = "env"
	tokenSourceGH     = "gh"
	tokenSourceNone   = "none"
)

type AuthStatus struct {
	Authenticated bool   `json:"authenticated"`
	Login         string `json:"login,omitempty"`
	Source        string `json:"source"`
	APIURL        string `json:"api_url"`
}

var ghTokenCache struct {
	sync.Mutex
	resolved bool
	token    string
}

func resolveToken(settings Settings) (string, string) {
	if token := strings.TrimSpace(settings.Token); token != "" {
		return token, tokenSourceConfig
	}
	for _, name := range []string{"PEGG_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(name)); token != "" {
			return token, tokenSourceEnv
		}
	}
	if token := ghAuthToken(); token != "" {
		return token, tokenSourceGH
	}
	return "", tokenSourceNone
}

func ghAuthToken() string {
	ghTokenCache.Lock()
	defer ghTokenCache.Unlock()
	if ghTokenCache.resolved {
		return ghTokenCache.token
	}
	ghTokenCache.resolved = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	ghTokenCache.token = strings.TrimSpace(string(out))
	return ghTokenCache.token
}

func authenticationStatus(ctx context.Context, settings Settings) AuthStatus {
	token, source := resolveToken(settings)
	apiURL := strings.TrimRight(strings.TrimSpace(settings.APIURL), "/")
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	status := AuthStatus{Source: source, APIURL: apiURL}
	if token == "" {
		return status
	}
	settings.APIURL = apiURL
	login, err := NewClient(settings, token).CurrentUser(ctx)
	if err != nil {
		return status
	}
	status.Authenticated = true
	status.Login = login
	return status
}

var (
	reAuthHeaderBearer = regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)\S+`)
	reAuthHeader       = regexp.MustCompile(`(?i)(authorization\s*:\s*)\S+`)
	reURLToken         = regexp.MustCompile(`(https?://)([^/\s@]+):([^@\s/]+)@`)
	reXAccessToken     = regexp.MustCompile(`(?i)(x-access-token\s*:\s*)[^@\s]+`)
	reGitHubToken      = regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`)
	reTokenParam       = regexp.MustCompile(`(?i)([?&](?:access_token|token|api_key)=)[^&\s]+`)
)

func redact(value string) string {
	out := reAuthHeaderBearer.ReplaceAllString(value, "${1}[REDACTED]")
	out = reAuthHeader.ReplaceAllString(out, "${1}[REDACTED]")
	out = reURLToken.ReplaceAllString(out, "${1}[REDACTED]@")
	out = reXAccessToken.ReplaceAllString(out, "${1}[REDACTED]")
	out = reGitHubToken.ReplaceAllString(out, "[REDACTED]")
	out = reTokenParam.ReplaceAllString(out, "${1}[REDACTED]")
	return out
}

func redactWith(secret, value string) string {
	if secret == "" {
		return redact(value)
	}
	value = strings.ReplaceAll(value, secret, "[REDACTED]")
	return redact(value)
}

func redactError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", redact(err.Error()))
}

func redactErrWith(secret string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", redactWith(secret, err.Error()))
}
