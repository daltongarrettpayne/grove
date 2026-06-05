# CLAUDE.md — grove

Grove is a Go CLI that weaves a knowledge tree (vault) and a code tree into tmux sessions. It enforces one workspace model — context = session, lanes = windows, home lane pinned, session derived from disk — so the workspace is stateless, idempotent, and reconstructible. HOLMES OS is one configuration of it, not the tool itself. The engine is generic; all opinionated choices live in user config.

**Repo:** `~/code/grove/grove/` (main) or a worktree for feature branches.
**Design doc (source of truth):** `docs/design.md`

---

## Two-audience model

- **User:** installs via `go install` (Homebrew tap coming), sets `GROVE_CODE_ROOT` and `GROVE_HOME_ROOT`, runs grove against their existing vault + code root. Never touches source.
- **Contributor:** works in worktrees off main, opens PRs, runs CI locally. This file is the contributor guide.

---

## The model (the cement)

A **context** = a tmux session. It has **lanes** (windows) and **views** (panes). Lane 0 is always `home` — the context's home directory. Every other lane is a working tree identified by `(repo, branch)` — two worktrees of the same repo get separate lanes because git forbids one branch in two worktrees.

A context's **source set** is one-or-more container directories scanned for working trees:
- one container → a single-project context
- many containers → a cockpit context
- zero containers → a plain context (home lane only)

**The session is a pure function of disk state.** No daemon, no cached state — every `session open` derives the workspace from the filesystem. This is the property that makes the workspace portable, durable, and reconstructible.

The **home tree** (pointed at by `GROVE_HOME_ROOT`) is the spine: its `01-Projects/` and `02-Areas/` subdirectories are the contexts. Filing controls lifecycle — archive a context folder and its session stops being offered.

See `docs/design.md` for the full model, display grammar, and design decisions.

---

## Project structure

```
grove/
├── cmd/
│   └── grove/
│       └── main.go              # Entry point — tiny, delegates to cli.Execute()
├── internal/
│   ├── cli/                     # Cobra command definitions
│   │   ├── root.go              # Root command, global flags, PersistentPreRunE (config + logger)
│   │   ├── session.go           # session open / session list
│   │   ├── window.go            # window list / window pick
│   │   ├── worktree.go          # worktree new
│   │   ├── status.go            # status-segment
│   │   └── doctor.go            # doctor (8 check categories)
│   ├── model/                   # Domain types: Context, Lane, SourceSet, LaneKind
│   │   ├── model.go             # Core types and DisplayRow / ParseWindowName
│   │   ├── convention.go        # ValidateBranchName, IsKebabCase
│   │   └── model_test.go / convention_test.go
│   ├── tmux/
│   │   └── tmux.go              # Thin shell-out wrapper: HasSession, NewSession, AttachOrSwitch, etc.
│   ├── git/
│   │   └── git.go               # Thin shell-out wrapper: IsRepo, IsWorktree, CurrentBranch, etc.
│   ├── config/
│   │   └── config.go            # Config struct + Load() (env vars + defaults)
│   ├── scanner/
│   │   └── scanner.go           # ScanSourceSet: walks a container directory → []Lane
│   ├── picker/
│   │   └── picker.go            # Fzf integration; ErrCancelled sentinel
│   └── log/
│       └── log.go               # Setup() — thin wrapper on log/slog
├── test/
│   ├── fixtures/
│   │   ├── fixtures.go          # Fixture world definition
│   │   └── gen/main.go          # Generator: writes deterministic fake root to /tmp/grove-fixtures
│   └── integration/             # Integration tests (build tag: integration)
├── docs/
│   └── design.md                # Source of truth for the model, decisions, and open questions
├── Makefile
├── go.mod
├── go.sum
└── CLAUDE.md
```

<!-- NOTE: cmd/ vs internal/ split is a Go convention. cmd/ holds main packages (the actual executables). internal/ holds everything else; Go enforces that nothing outside this module can import from internal/. -->

---

## Go naming conventions

| Thing | Convention | Example |
|---|---|---|
| Packages | short, lowercase, no underscores | `config`, `tmux`, `picker` |
| Exported (public) names | PascalCase | `Context`, `OpenSession()` |
| Unexported (private) names | camelCase | `context`, `openSession()` |
| Interfaces | often ends in `-er` | `Scanner`, `Picker`, `Runner` |
| Error variables | start with `Err` | `ErrNotFound`, `ErrNoTmux` |
| Single-letter receivers | short, not `self` or `this` | `(c *Context)`, `(s *Scanner)` |

**Avoid stutter.** Don't write `context.ContextType` — the package name already provides the namespace, so `context.Type` is right.

---

## Error handling

Go has no exceptions. Every function that can fail returns an error as its last return value.

**Always wrap errors with context:**

```go
// wrap with %w so the caller can unwrap it later
if err != nil {
    return fmt.Errorf("scanning code root %s: %w", root, err)
}

// check specific error types
if errors.Is(err, os.ErrNotExist) {
    // handle missing directory
}
```

**Never:**

```go
// silent discard
_ = someOperation()

// log.Fatal in library code — panics the whole program, caller can't handle it
log.Fatal(err)

// bare error with no context
return errors.New("failed")
```

**Domain errors live in the relevant package:**

```go
// internal/tmux/tmux.go (or errors.go)
var ErrTmuxNotRunning = errors.New("tmux server is not running")
var ErrSessionExists  = errors.New("session already exists")
```

---

## Interface design

Keep interfaces small and define them where they're consumed:

```go
// internal/picker/picker.go
type Backend interface {
    Select(rows []string) (string, error)
}

// internal/tmux/tmux.go
type SessionManager interface {
    HasSession(name string) (bool, error)
    NewSession(name, dir string) error
    AttachSession(name string) error
}
```

Accept interfaces, return concrete types:

```go
// caller can pass any Backend (fzf, sk, test stub)
func NewPicker(backend Backend) *Picker { ... }
```

---

## CLI structure (Cobra)

Root command in `internal/cli/root.go`:

```go
var rootCmd = &cobra.Command{
    Use:          "grove",
    Short:        "Weave your knowledge tree and code tree into tmux sessions",
    SilenceUsage: true,   // don't print usage on every runtime error
    SilenceErrors: true,  // main.go controls the error message format
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        // loads config and inits logger before every subcommand
    },
}
```

Subcommand pattern (two-level: `grove session open`):

```go
// internal/cli/session.go
var sessionOpenCmd = &cobra.Command{
    Use:   "open <context>",
    Short: "Open or attach to a context session (idempotent)",
    Long:  `...`,
    Example: `...`,
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        return runSessionOpen(args[0])
    },
}

func init() {
    sessionCmd.AddCommand(sessionOpenCmd)
    rootCmd.AddCommand(sessionCmd)
}
```

`main.go` stays minimal:

```go
package main

import (
    "os"
    "github.com/daltongarrettpayne/grove/internal/cli"
)

func main() {
    if err := cli.Execute(); err != nil {
        os.Exit(1)
    }
}
```

---

## Configuration

Config priority (high to low): flags > environment variables > defaults.

```go
// internal/config/config.go
type Config struct {
    CodeRoot   string // root scanned for code repos, e.g. ~/code
    HomeRoot   string // root of the knowledge tree, e.g. ~/life-vault
    TmuxSocket string // optional: path to a private tmux socket
    Picker     string // picker binary, default "fzf"
    LogLevel   string // "debug" | "info" | "warn" | "error"
}
```

Environment variables: `GROVE_CODE_ROOT`, `GROVE_HOME_ROOT`, `GROVE_TMUX_SOCKET`, `GROVE_PICKER`, `GROVE_LOG_LEVEL`.

No hardcoded personal paths ever — config/env only.

---

## Output philosophy

Every command must be **human-readable by default** and **machine-parseable on request**.

- Default output is a clean, aligned plain-text table or a short confirmation line. No noise, no structured log lines, no JSON blobs.
- Any command that lists data must accept a `--json` flag that emits a stable JSON array suitable for piping to `jq`, scripts, or LLM tools.
- Error messages must be in plain English — translate OS errors (`invalid cross-device link`, `no such file`) into what actually went wrong from the user's perspective.
- `slog.Info` / `slog.Debug` calls are for internal observability only. They are suppressed by default (`warn` log level). Never use slog for user-facing feedback — use `fmt.Printf` for that.

Example pattern:

```go
// user-facing confirmation — always visible
fmt.Printf("archived %s\n      -> %s\n", src, dest)

// internal trace — only with -v
slog.Debug("archiving project", "src", src, "dest", dest)
```

---

## Logging

Use `log/slog` (Go standard library since 1.21) for internal observability only. Never use it for user-facing output.

```go
slog.Debug("scanning directory", "path", dir)   // -v only
slog.Warn("tmux not running, skipping check", "err", err)
slog.Error("tmux shell-out failed", "cmd", cmd, "err", err)
```

Default log level is `warn`. `--verbose` / `-v` flag or `GROVE_LOG_LEVEL=debug` enables debug output.

---

## Shelling out (tmux + git)

Grove drives tmux and git by shelling out to their CLIs. Never use tmux control-mode or libgit2 — shell-out is simpler and more durable.

```go
// internal/tmux/tmux.go
func run(args ...string) (string, error) {
    cmd := exec.Command("tmux", args...)
    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("tmux %v: %w", args, err)
    }
    return strings.TrimSpace(string(out)), nil
}
```

Always validate preconditions before mutating tmux state (does the session exist? does the directory exist?). Never half-build a session.

---

## Testing

**Tests are required for every new feature and every edge case.** CI will not pass without them. When adding a command or a non-trivial behaviour change, the PR must include:

- A unit test for the core logic (config parsing, model methods, scanner behaviour, etc.)
- A test for the error path (invalid input, not-found, already-exists, etc.)
- If the change affects CLI output format, a test that asserts the shape of that output

If a behaviour is untested, CI passing does not mean it works.

**Unit tests** live next to the file they test: `scanner.go` -> `scanner_test.go`.

**Table-driven tests** are the Go idiom:

```go
func TestDisplayRow(t *testing.T) {
    tests := []struct {
        name  string
        input Lane
        want  string
    }{
        {"home lane", Lane{IsHome: true}, "home"},
        {"regular lane", Lane{Repo: "grove", Branch: "main"}, "grove  ·  main"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := tt.input.DisplayRow(len(tt.input.Repo))
            if got != tt.want {
                t.Errorf("got %q, want %q", got, tt.want)
            }
        })
    }
}
```

**Integration tests** require tmux + fzf and are gated by a build tag:

```go
//go:build integration
// test/integration/session_test.go
```

**Fixtures** — `test/fixtures/` contains a generator (`test/fixtures/gen/main.go`) that creates a deterministic fake world at `/tmp/grove-fixtures`:
- `vault/01-Projects/coding-project-big` — two repos (`repo-alpha`, `repo-beta`), each with multiple worktrees
- `vault/01-Projects/coding-project-small` — one repo (`repo`) with a single worktree
- `vault/01-Projects/non-coding-project` — vault note only, no code directory
- `vault/02-Areas/` — area contexts

Tests use `/tmp/grove-fixtures`, never `~/code` or `~/life-vault`.

**Run tests:**

```sh
make test                 # go test -race -cover ./...
make test-integration     # generates fixtures, then go test -race -tags integration ./test/...
make lint                 # golangci-lint run
```

---

## Worktree workflow (developing grove with grove)

Grove's own worktree commands work on grove itself. From inside the `grove` tmux session:

```sh
# Create a feature branch and worktree:
grove worktree new feat/my-feature

# This creates ~/code/grove/grove-my-feature/ and opens it in a new tmux window.
# Work there, then:
gh pr create

# After the PR merges, clean up:
grove worktree delete feat/my-feature   # (or: git worktree remove + close window)
```

You can also target the repo explicitly from outside a tmux session:

```sh
grove worktree new feat/my-feature --repo ~/code/grove/grove
```

---

## Build

```sh
make build   # go build -o bin/grove ./cmd/grove
make test    # go test -race -cover ./...
make lint    # golangci-lint run
make clean   # rm -rf bin/
```

Version is injected at build time:

```sh
go build -ldflags="-X main.Version=0.1.0" ./cmd/grove
```

Docker clean-room (also the portability proof):

```sh
make docker-build   # build the image
make docker-run     # interactive shell inside the clean-room with fixtures + tmux
```

---

## CI

CI runs on every PR via GitHub Actions (`.github/workflows/ci.yml`):
- `go test -race ./...` on ubuntu-latest and macos-latest
- `golangci-lint run`
- `go build ./cmd/grove`

Integration tests (require tmux + fzf) are not in CI by default; run locally with `make test-integration`.

**Run locally before pushing:**

```sh
make test && make lint
```

---

## Branch rules

This repo uses a GitFlow model:

- `main` — stable, tagged releases only. Every commit here is a shipped version.
- `develop` — integration target. All feature/fix PRs merge here first.
- `feat/*`, `fix/*`, `chore/*` — short-lived, branch from `develop`, PR back to `develop`.
- `hotfix/*` — branches from `main`, merges to both `main` AND `develop`.

No direct push to `main` or `develop`. No force push. CI must pass before any merge.

To release: open a PR from `develop` → `main`, merge, then tag (e.g. `v0.2.0`) — the release workflow builds binaries automatically.

---

## Commit style

`✨ feat:`, `🐛 fix:`, `🔧 chore:`, `📚 docs:`, `🧪 test:`, `♻️ refactor:`

No personal paths, no hardcoded credentials, no vault content in commits.

---

## Command map (current)

| Command | File | What it does |
|---|---|---|
| `grove session open <ctx>` | `internal/cli/session.go` | Scans vault tiers for context, validates home dir, creates session + windows, attaches |
| `grove session list [--json]` | `internal/cli/session.go` | Scans vault tiers, prints human-readable table (or JSON with `--json`) |
| `grove window list [<ctx>]` | `internal/cli/window.go` | Lists picker rows for a context; marks current window with `*` |
| `grove window pick` | `internal/cli/window.go` | Runs fzf over window rows for the current session, switches to selection |
| `grove worktree new <branch>` | `internal/cli/worktree.go` | Creates linked worktree at `<repo>-<slug>/`, optionally adds tmux window |
| `grove status-segment` | `internal/cli/status.go` | Prints `<session> › <repo> · <branch>` for tmux status-right |
| `grove doctor` | `internal/cli/doctor.go` | Runs 8 audit checks; exits 1 if any violations found |

---

## Build order / roadmap

Completed:
- Contracts + config-driven roots (env vars, Config struct)
- `grove session list` — scan, emit JSON
- `grove session open` — idempotent build
- `grove worktree new` — creates worktree + tmux window
- `grove window list` / `grove window pick` — picker
- `grove status-segment`
- `grove doctor` — 8 check categories

In parallel branches (will merge):
- `grove project init` / `grove project list` / `grove project archive`
- `grove session delete`
- `grove worktree list` / `grove worktree delete`
- `grove window delete`

Out of core (Q-H from design.md): focus unification, daily-recap, deeper status, full observability build-out, daemon/TUI.
