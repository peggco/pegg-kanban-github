package main

import (
	"sync"

	peggplugin "github.com/peggco/pegg/pkg/plugin"
)

type GitHub struct {
	peggplugin.Base
	mu       sync.Mutex
	settings Settings
}

func (g *GitHub) Name() string { return "kanban-github" }

func (g *GitHub) Version() string { return "1.0.0" }

func (g *GitHub) Description() string {
	return "Kanban integration for GitHub issues, worktrees, and pull requests"
}

func (g *GitHub) Boot(deps peggplugin.Deps) error {
	settings := defaultSettings()
	if deps.Workdir != "" {
		settings.Workdir = deps.Workdir
	}
	settings.applyHost(deps.ConfigJSON)
	settings.applyOptions(deps.Options)
	settings.applyEnv()

	g.mu.Lock()
	g.settings = settings
	g.mu.Unlock()
	return nil
}

func (g *GitHub) Capabilities() []string {
	return []string{string(peggplugin.CapabilityHooks)}
}

func (g *GitHub) currentSettings() Settings {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.settings
}

func (g *GitHub) setSettings(settings Settings) {
	g.mu.Lock()
	g.settings = settings
	g.mu.Unlock()
}
