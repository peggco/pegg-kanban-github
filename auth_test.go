package main

import (
	"strings"
	"testing"
)

func TestResolveTokenSources(t *testing.T) {
	for _, name := range []string{"PEGG_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		t.Setenv(name, "")
	}
	settings := defaultSettings()
	settings.Token = "config-token"
	if token, source := resolveToken(settings); token != "config-token" || source != tokenSourceConfig {
		t.Fatalf("config token = %q %q", token, source)
	}
	settings.Token = ""
	t.Setenv("PEGG_GITHUB_TOKEN", "pegg-token")
	if token, source := resolveToken(settings); token != "pegg-token" || source != tokenSourceEnv {
		t.Fatalf("env token = %q %q", token, source)
	}
	t.Setenv("PEGG_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "github-token")
	if token, source := resolveToken(settings); token != "github-token" || source != tokenSourceEnv {
		t.Fatalf("github token = %q %q", token, source)
	}
}

func TestRedaction(t *testing.T) {
	secret := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	values := []string{
		"Authorization: Bearer " + secret,
		"authorization=" + secret,
		"https://x-access-token:" + secret + "@github.com/acme/widgets.git",
		"failed with token " + secret,
		"https://api.github.com/repos?access_token=" + secret,
	}
	for _, value := range values {
		got := redact(value)
		if strings.Contains(got, secret) {
			t.Errorf("redact(%q) leaked the secret: %q", value, got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("redact(%q) = %q, expected a redaction marker", value, got)
		}
	}
	if got := redactWith("short-secret", "request short-secret failed"); strings.Contains(got, "short-secret") {
		t.Fatalf("redactWith leaked: %q", got)
	}
	if got := redact("nothing to hide"); got != "nothing to hide" {
		t.Fatalf("redact changed clean text: %q", got)
	}
}
