<div align="center">

<img src="https://cdn.pegg.dev/logo/infinity.gif" alt="Pegg" width="420" />

# Pegg Kanban GitHub

### GitHub issues on your Kanban board.

A Kanban integration for GitHub. It imports issues, prepares worktrees, and comments or opens a pull request when a card succeeds.

[Plugins docs](https://docs.pegg.dev/pegg/configurations/plugins/) · [Releases](https://github.com/peggco/pegg-kanban-github/releases) · [Website](https://pegg.dev) · [Discussions](https://github.com/peggco/pegg/discussions)

[![Latest release](https://img.shields.io/github/v/release/peggco/pegg-kanban-github?style=flat-square&sort=semver)](https://github.com/peggco/pegg-kanban-github/releases)
[![Go 1.26.5+](https://img.shields.io/badge/go-1.26.5%2B-00ADD8?style=flat-square&logo=go&logoColor=white)](https://github.com/peggco/pegg-kanban-github)
[![Plugin protocol v2](https://img.shields.io/badge/plugin%20protocol-v2-black?style=flat-square)](https://docs.pegg.dev/pegg/configurations/plugins/)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-black?style=flat-square)](https://github.com/peggco/pegg-kanban-github/blob/HEAD/LICENSE)

**Install**

```bash
pegg plugins install peggco/pegg-kanban
pegg plugins install peggco/pegg-kanban-github
```

Hooks-only. Install `pegg-kanban` alongside this plugin; it contributes no agent tools.
</div>

## Hooks

| Hook | Kind | Purpose |
|---|---|---|
| `kanban.providers` | filter | Advertise GitHub as a sync provider (`id: github`). |
| `kanban.issues.fetch` | filter | Import open GitHub issues as cards. |
| `kanban.run.prepare` | filter | Create a git worktree and `pegg/<slug>` branch for a card run. |
| `kanban.card.succeeded` | action | Comment the run summary and open a pull request. |

Provider IDs stay `github`, so existing kanban boards and sync state keep working.

## Build and install

```bash
go mod tidy
go vet ./...
go test ./...
go build -trimpath -ldflags "-s -w" -o bin/pegg-kanban-github .
install -m 0755 bin/pegg-kanban-github "$HOME/.pegg/plugins/pegg-kanban-github"
```

Verify with `pegg plugins list --runtime`. The plugin shows up but contributes no
agent tools; kanban is the only agent-facing surface.

## Configuration

Options live under `plugins.options["kanban-github"]` in `~/.pegg/pegg.json`. The
legacy keys `github` and `pegg-github` are still read for migration.

```json
{
  "plugins": {
    "options": {
      "kanban-github": {
        "enabled": true,
        "token": "",
        "owner": "",
        "repo": "",
        "api_url": "https://api.github.com",
        "default_base": "",
        "worktree_root": ".pegg-worktrees",
        "request_timeout_seconds": 30,
        "sync_state": "open",
        "sync_labels": [],
        "sync_limit": 50
      }
    }
  }
}
```

| Setting | Meaning |
|---|---|
| `enabled` | Enable the integration. |
| `token` | GitHub token. Falls back to `PEGG_GITHUB_TOKEN`, `GITHUB_TOKEN`, `GH_TOKEN`, then `gh auth token`. |
| `owner` / `repo` | Default repository. `PEGG_GITHUB_REPO=owner/name` overrides both. |
| `api_url` | GitHub API base; set for GitHub Enterprise. |
| `default_base` | Base branch for worktrees and pull requests. |
| `worktree_root` | Where worktrees are created (default `.pegg-worktrees`). |
| `sync_state` / `sync_labels` / `sync_limit` | Issue import filters. |

Environment: `PEGG_GITHUB_TOKEN`, `PEGG_GITHUB_REPO`, `PEGG_GITHUB_OWNER`,
`PEGG_GITHUB_REPO_NAME`, `PEGG_GITHUB_API_URL`, `PEGG_GITHUB_WORKTREE_ROOT`,
`PEGG_GITHUB_DEFAULT_BASE`, `PEGG_GITHUB_TIMEOUT_SECONDS`.

## Behavior

- Issue fetch maps GitHub issues to kanban cards with `source=github`, the issue
  number, URL, labels, and state.
- Run prepare creates a worktree at `<worktree_root>/<slug>` on branch `pegg/<slug>`,
  based on the card's base branch or `default_base`.
- Card success posts the run summary as an issue comment when feedback is enabled and
  opens a pull request from the run branch when `create_change_request` is enabled.
- Tokens are never printed, logged, or returned. Errors are redacted.

### Change-request routing

Kanban resolves a `change_request_provider` for each card and passes it in the
`kanban.run.prepare` and `kanban.card.succeeded` payloads. This plugin handles both hooks
only when that value is `github` (or, for older Kanban versions that omit the field, when
the issue provider is `github`). The issue comment is only posted when the issue itself
came from GitHub, so a Jira issue routed to GitHub opens a pull request without a GitHub
comment.

To route another issue source to GitHub, set the route in the Kanban options:

```json
{
  "plugins": {
    "options": {
      "kanban": {
        "change_request_providers": {"jira": "github"}
      }
    }
  }
}
```

The repository is taken from the issue repo when the issue came from GitHub, and from
`owner`/`repo` (or the git remote) otherwise.

## Tests

```bash
go vet ./...
go test ./...
```

The tests cover hook decoding, provider discovery, issue mapping, worktree creation,
and pull-request request construction without live GitHub calls.

## License

Apache-2.0. See [LICENSE](https://github.com/peggco/pegg-kanban-github/blob/HEAD/LICENSE).
