# CLAUDE.md — grove

Grove is a focused CLI tool that weaves a knowledge tree (vault) and a code
tree into tmux sessions. HOLMES OS is its first configuration, not the tool
itself. The engine is generic; all opinionated choices live in user config.

**Status:** Go module initialized, design finalized, implementation starting.
**Vault home:** `~/life-vault/01-Projects/grove/`
**Design doc (source of truth):** `docs/design.md`

---

## The model in one paragraph

A **context** = a tmux session. It has **lanes** (windows) and **views**
(panes). Lane 0 is always `home` — the context's home directory. Every other
lane is a working tree identified by `(repo, branch)` — two worktrees of the
same repo get separate lanes. A context's **source set** is one-or-more
container directories scanned for working trees. Picker rows follow the
display grammar locked in the design doc.

---

## Project structure

```
grove/
├── cmd/
│   └── grove/
│       └── main.go          # Entry point — stays tiny, delegates everything
├── internal/
│   ├── cli/                 # Cobra command definitions
│   │   ├── root.go          # Root command, global flags, version
│   │   ├── session.go       # session open / session list
│   │   ├── window.go        # window (picker)
│   │   ├── worktree.go      # worktree new
│   │   └── status.go        # status-segment
│   ├── model/               # Domain model: Context, Lane, SourceSet (package model)
│   ├── tmux/                # Thin wrapper over tmux shell-outs
│   ├── git/                 # Thin wrapper over git shell-outs
│   ├── config/              # Config loading (file + env + defaults)
│   ├── scanner/             # Scans code root and home tree
│   ├── picker/              # Picker row protocol; fzf integration
│   └── log/                 # Structured logging setup (thin wrapper on slog)
├── test/
│   ├── fixtures/            # Deterministic synthetic world for tests
│   └── integration/         # Integration tests (build tag: integration)
├── docs/
│   └── design.md
├── Makefile
├── go.mod
├── go.sum
└── CLAUDE.md
```

<!-- NOTE for Dalton: the cmd/ vs internal/ split is a Go convention.
     cmd/ holds the "main packages" — the actual executables. Go requires a
     file with `package main` and a `main()` function to produce a binary.
     internal/ holds everything else. Go ENFORCES that nothing outside this
     module can import from internal/ — it's the language's way of making
     private packages truly private. Use internal/ for 95% of the code. -->

---

## Go naming conventions

<!-- Go has strong, community-enforced naming rules. The compiler won't stop
     you from violating them but every other Go programmer will notice. -->

| Thing | Convention | Example |
|---|---|---|
| Packages | short, lowercase, no underscores | `config`, `tmux`, `picker` |
| Exported (public) names | PascalCase | `Context`, `OpenSession()` |
| Unexported (private) names | camelCase | `context`, `openSession()` |
| Interfaces | often ends in `-er` | `Scanner`, `Picker`, `Runner` |
| Error variables | start with `Err` | `ErrNotFound`, `ErrNoTmux` |
| Single-letter receivers | short, not `self` or `this` | `(c *Context)`, `(s *Scanner)` |

<!-- "Exported" means visible outside the package. In Go, if the first letter
     is uppercase, it's exported. Lowercase = package-private. This is the
     ONLY access modifier Go has — no public/private keywords. -->

**Avoid stutter.** Don't write `context.ContextType` — the package name already
provides the namespace, so `context.Type` or just `context.Context` is right.

---

## Error handling

<!-- Go has no exceptions. Every function that can fail returns an error as its
     last return value. The caller always checks it. This is verbose but
     explicit — you never wonder where an exception might have been thrown. -->

**Always wrap errors with context:**

```go
// ✓ wrap with %w so the caller can unwrap it later
if err != nil {
    return fmt.Errorf("scanning code root %s: %w", root, err)
}

// ✓ check specific error types
if errors.Is(err, os.ErrNotExist) {
    // handle missing directory
}
```

**Never:**

```go
// ✗ silent discard
_ = someOperation()

// ✗ log.Fatal in library code — panics the whole program, caller can't handle it
log.Fatal(err)

// ✗ bare error with no context
return errors.New("failed")
```

**Domain errors live in the relevant package:**

```go
// internal/tmux/errors.go
var ErrTmuxNotRunning = errors.New("tmux server is not running")
var ErrSessionExists  = errors.New("session already exists")
```

---

## Interface design

<!-- Interfaces in Go describe behavior, not identity. A type satisfies an
     interface automatically if it has the right methods — no "implements"
     keyword needed. This lets you define interfaces at the point of USE,
     not at the point of definition. -->

**Keep interfaces small and define them where they're consumed:**

```go
// internal/picker/picker.go — the picker package defines what it needs
type Backend interface {
    // Select sends rows to the picker and returns the chosen row.
    Select(rows []string) (string, error)
}

// internal/tmux/tmux.go — tmux package defines its own minimal interface
type SessionManager interface {
    HasSession(name string) (bool, error)
    NewSession(name, dir string) error
    AttachSession(name string) error
}
```

**Accept interfaces, return concrete types:**

```go
// ✓ caller can pass any Backend (fzf, sk, test stub)
func NewPicker(backend Backend) *Picker { ... }

// ✗ locks the caller to fzf specifically
func NewPicker(fzf *FzfBackend) *Picker { ... }
```

---

## CLI structure (Cobra)

<!-- Cobra is the standard Go CLI framework — used by Kubernetes, Docker, Hugo.
     It handles subcommands, flags, help text, and shell completion.
     The pattern: a Command object per subcommand, wired into a tree. -->

**Root command in `internal/cli/root.go`:**

```go
var rootCmd = &cobra.Command{
    Use:   "grove",
    Short: "Weave your knowledge tree and code tree into tmux sessions",
    // SilenceUsage: true prevents Cobra from printing usage on every error
    SilenceUsage: true,
}

// Execute is called from cmd/grove/main.go
func Execute() error {
    return rootCmd.Execute()
}
```

**Subcommand pattern:**

```go
// internal/cli/session.go
var sessionOpenCmd = &cobra.Command{
    Use:   "open <context>",
    Short: "Open or attach to a context session",
    Args:  cobra.ExactArgs(1),
    // RunE returns an error; RunE > Run because Run can't signal failure
    RunE: func(cmd *cobra.Command, args []string) error {
        return runSessionOpen(args[0])
    },
}

// init() registers subcommands; Go calls init() automatically at package load
func init() {
    sessionCmd.AddCommand(sessionOpenCmd)
    rootCmd.AddCommand(sessionCmd)
}
```

**`main.go` stays minimal:**

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

Config priority (high → low): flags → environment variables → config file → defaults.

```go
// internal/config/config.go
type Config struct {
    CodeRoot  string // root scanned for code repos, e.g. ~/code
    HomeRoot  string // root of the knowledge tree, e.g. ~/life-vault
    TmuxSocket string // optional: path to a private tmux socket
    Picker   string // picker binary, default "fzf"
    LogLevel string // "debug" | "info" | "warn" | "error"
}
```

<!-- Environment variables let grove work without a config file on a fresh
     machine, and let the Docker sandbox override paths without touching disk. -->

Environment variables: `GROVE_CODE_ROOT`, `GROVE_HOME_ROOT`, `GROVE_PICKER`,
`GROVE_LOG_LEVEL`. Documented in `grove help config`.

No hardcoded personal paths ever — config/env only.

---

## Logging

Use `log/slog` (Go standard library since 1.21). Never `fmt.Println` for
observability output.

```go
// internal/log/log.go
import "log/slog"

// Quiet by default. --verbose or GROVE_LOG_LEVEL=debug enables debug output.
// Structured output means logs are machine-parseable (JSON mode).
```

```go
// In application code
slog.Info("session opened", "name", sessionName, "lanes", len(lanes))
slog.Debug("scanning directory", "path", dir)
slog.Error("tmux shell-out failed", "cmd", cmd, "err", err)
```

<!-- Structured logging = key-value pairs alongside the message. This lets you
     grep for specific fields, pipe to jq, and see performance regressions over
     time without parsing free-form strings. -->

---

## Shelling out (tmux + git)

Grove drives tmux and git by shelling out to their CLIs. Never use a tmux
control-mode or libgit2 binding — shell-out is simpler and more durable.

```go
// internal/tmux/exec.go
// run executes a tmux command and returns stdout.
func run(args ...string) (string, error) {
    cmd := exec.Command("tmux", args...)
    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("tmux %v: %w", args, err)
    }
    return strings.TrimSpace(string(out)), nil
}
```

Always validate preconditions before mutating tmux state (is the session
already running? does the directory exist?). Never half-build a session.

---

## Testing

**Unit tests** live next to the file they test: `scanner.go` → `scanner_test.go`.

**Table-driven tests** are the Go idiom for multiple cases:

```go
func TestDisplayRow(t *testing.T) {
    tests := []struct {
        name  string   // human name shown on test failure
        input Lane
        want  string
    }{
        {"home lane", Lane{IsHome: true}, "home"},
        {"regular lane", Lane{Repo: "grove", Branch: "main"}, "grove  ·  main"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := tt.input.DisplayRow()
            if got != tt.want {
                t.Errorf("got %q, want %q", got, tt.want)
            }
        })
    }
}
```

**Integration tests** are tagged so they only run when explicitly requested:

```go
//go:build integration

// test/integration/session_test.go
// These hit real tmux — run with: go test -tags integration ./test/integration/
```

**Fixtures** — `test/fixtures/` contains a generator that creates a deterministic
fake code root and home tree. Tests use this, never `~/code` or `~/life-vault`.
The Docker clean-room uses the same fixture, making it the portability proof.

Fixture contexts (under `vault/01-Projects/`):
- `coding-project-big` — two repos (`repo-alpha`, `repo-beta`), each with multiple worktrees
- `coding-project-small` — one repo (`repo`) with a single worktree
- `non-coding-project` — no code directory (vault note only)

**Run tests:**
```sh
go test ./...                          # all unit tests
go test -race ./...                    # with race detector (always run this)
go test -tags integration ./test/...  # integration tests
```

<!-- The race detector finds concurrency bugs. Go makes concurrency easy, which
     makes races easy to introduce accidentally. Always test with -race. -->

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

<!-- ldflags lets you set Go variables from outside the source code at build
     time. This is the standard pattern for embedding version info without
     hardcoding it in source. -->

---

## Dependencies

Keep the dependency tree minimal. Prefer the standard library.

| Dep | Purpose | Why not stdlib |
|---|---|---|
| `github.com/spf13/cobra` | CLI subcommand framework | stdlib `flag` has no subcommand tree |
| (stdlib) `log/slog` | Structured logging | Built in since Go 1.21 |
| (stdlib) `os/exec` | Shell-out to tmux/git | Built in |
| (stdlib) `encoding/json` | JSON/TSV output | Built in |

Add a dependency only when the stdlib gap is real. Run `go mod tidy` before
every commit.

---

## Commit style

`✨ feat:`, `🐛 fix:`, `🔧 chore:`, `📚 docs:`, `🧪 test:`, `♻️ refactor:`

No personal paths, no hardcoded creds, no vault content in commits.

---

## Build order (from design.md)

0. Cleanup pass (pre-build; stabilize the live system)
1. Contracts + config-driven roots
2. `grove project list` — scan, emit JSON/TSV
3. `grove session open <ctx>` — idempotent build
4. `grove worktree new <branch>`
5. `grove window` — picker
6. `grove status-segment`
