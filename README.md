# grove

Weaves your knowledge tree and code tree into tmux sessions — one model, enforced, so your workspace stops shifting under you.

## What it does

Grove maps your on-disk structure to tmux sessions deterministically. A **context** is a tmux session; its **lanes** are windows; lane 0 is always `home`. Every other lane is a git working tree identified by `(repo, branch)` — worktrees fall out for free. The session is a pure function of what is on disk: stateless, idempotent, reconstructible. Tear down a session and `grove session open <ctx>` rebuilds it exactly. There is no daemon, no cached state, nothing to drift.

## Prerequisites

- tmux >= 3.3
- git >= 2.38
- fzf (default picker; swap via `GROVE_PICKER`)

## Install

```sh
go install github.com/daltongarrettpayne/grove/cmd/grove@latest
```

Homebrew tap coming — not yet published.

## Setup

Two env vars are required. Add them to your shell profile (`~/.zshrc`, `~/.bashrc`, etc.):

```sh
export GROVE_CODE_ROOT=~/code       # where your git repos live
export GROVE_HOME_ROOT=~/           # root of your knowledge/vault tree
```

Grove expects this layout:

```
$GROVE_HOME_ROOT/01-Projects/   -- project contexts (one subdir per context)
$GROVE_HOME_ROOT/02-Areas/      -- area contexts (one subdir per context)
$GROVE_CODE_ROOT/<name>/        -- code for a context named <name>
```

Each subdirectory of `01-Projects/` or `02-Areas/` becomes a context. If a directory named `<name>` also exists under `GROVE_CODE_ROOT`, grove scans it for git working trees and creates a lane per working tree.

## Quick start

```sh
# Create a new project context with a matching code directory:
grove project init my-project --code

# Build and attach to the tmux session for that context:
grove session open my-project

# Inside the session, open the interactive window switcher:
grove window pick
```

## Command reference

| Command | Description | Key flags |
|---|---|---|
| `grove project init <name>` | Create a vault directory for a new context | `--code` to also create a code directory under `GROVE_CODE_ROOT` |
| `grove project list` | List all available contexts as JSON | |
| `grove project archive <name>` | Move a context to `04-Archive/` | |
| `grove session open <name>` | Build (or attach to) a tmux session for a context | |
| `grove session list` | List all available contexts and their lanes | `--json` |
| `grove session pick` | Open the fzf picker and open/switch to the chosen context | bind to a tmux key (see below) |
| `grove session delete <name>` | Kill a tmux session | |
| `grove window list [<ctx>]` | List windows for a context; marks the current window | |
| `grove window pick` | Open the fzf picker and switch to the chosen window | bind to a tmux key (see below) |
| `grove window delete [<name>]` | Delete a tmux window by name | |
| `grove worktree new <branch>` | Create a linked worktree and register it as a lane | `--repo <path>` to specify the main repo |
| `grove worktree list` | List all worktrees in the current session's context | `--json` |
| `grove worktree delete <branch>` | Remove a linked worktree and its tmux window | |
| `grove status` | Print the current context (session / repo / branch) | `--short`, `--json` |
| `grove status-bar` | Compact one-line context for tmux `status-left`/`status-right` | |
| `grove doctor` | Audit vault and code directories for convention violations | `--json`, `--check <category>` |

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `GROVE_CODE_ROOT` | `~/code` | Root directory scanned for code repos |
| `GROVE_HOME_ROOT` | `~` | Root of the knowledge/vault tree |
| `GROVE_TMUX_SOCKET` | (system default) | tmux socket path; grove targets it via `-S` on every call, so all grove sessions live on one isolated socket |
| `GROVE_PICKER` | `fzf` | Picker binary (must read rows on stdin, write selection to stdout) |
| `GROVE_LOG_LEVEL` | `warn` | Log level: `debug`, `info`, `warn`, `error` |

Config priority (high to low): CLI flags > environment variables > defaults.

Any picker binary that reads newline-separated rows on stdin and writes the chosen row to stdout works: `fzf`, `sk` (skim), `fzy`, etc.

## Shell integration

Bind the pickers to tmux keys in your `tmux.conf` (they open as a floating popup when run inside tmux):

```tmux
bind -n M-f run-shell 'grove session pick'   # Alt-f: jump to any context
bind -n M-w run-shell 'grove window pick'     # Alt-w: jump to any window in this context
set -g status-left '#(grove status-bar)'      # show session › branch in the status line
```

**Window naming.** grove names each window it creates (`home`, `<repo>  ·  <branch>`) and marks it with the `@pinned_name` tmux window option. If you run a shell prompt hook that auto-renames the tmux window to the cwd or repo (a common `precmd`/`chpwd` pattern), have it skip windows that have `@pinned_name` set so grove's names survive:

```sh
# in your precmd hook:
[ -n "$(tmux show-options -wqv @pinned_name)" ] && return   # leave grove's window name alone
```

**base-index.** grove builds correctly whether your `base-index` is 0 or 1 — it never assumes a fixed window index.

## Contributing

See [CLAUDE.md](CLAUDE.md) for the development guide.
