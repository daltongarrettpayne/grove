# grove

_The workspace tool. It weaves your knowledge tree and your code tree into tmux
sessions, windows, and panes, and enforces one model so the ground stops
shifting under you. Built open-source from the start; HOLMES OS is its first
configuration, not the tool itself._

**Status:** high-level design, in progress (started 2026-05-31).
**Repo:** `~/code/grove/grove/`  ·  **Vault home:** `01-Projects/grove/`

---

## Why this exists

Stop the infinite tinkering. Today the session/window scripts each improvise
their own model of "what is a project / what is a window," so the ground keeps
shifting and things break. Pour one model in cement: a single tool, driven by
on-disk structure, that every surface derives from. The system stops changing
and starts leveraging.

**Shape 2** (chosen): a focused tool; no daemon, no TUI yet. Minimal
formalization that kills the cobbling. Built **open-source**: a universal
engine, with every opinionated/personal choice living in user configuration.
The author's own setup (vault, PARA, cockpits, nvim+claude panes) is a
*configuration* of this tool, not the tool itself.

---

## The model (the cement)

- **Context = tmux session.** Made of **lanes** (windows), each holding
  **views** (panes). (Implements P-13.)
- **Lane 0 = `home`** — the context's home folder. Branchless, always present,
  pinned. (Implements S-09 window 0.)
- **Source set** — a context is defined by a list of **one-or-more** container
  directories. Each is scanned; every working tree under it becomes a lane.
  - **one** container → a **code project** (`kalashnikov`)
  - **many** containers → a **cockpit** context (`meta`)
  - **zero** containers → a **plain context** (home only; pointers in the note)
- **Lane source kinds:** owned clone · worktree · pointer.
- **Window identity key = `(repo, branch)`.** A worktree resolves to its main
  repo via `git rev-parse --git-common-dir`; the worktree **directory name is
  display-irrelevant**. Branch is the unique key (git forbids one branch in two
  worktrees).

### The home tree is the spine (PARA, in the author's config)

Every context's home is a folder in the home tree, so **the picker is a
projection of that tree.** In the author's config the home tree is the PARA
vault, and which tiers are workspaces falls out of PARA's own meaning:

| Tier | meaning | Workspace? |
|---|---|---|
| `01-Projects/` | things you build | **yes** — one context each |
| `02-Areas/` | things you inhabit | **yes** — one context each |
| `06-Meta/` | the system itself | **yes** — the `meta` cockpit |
| vault root | the whole knowledge base | **yes** — the `vault` cockpit |
| Resources / Archive / Inbox / Attachments | reference / transient | no |

- **Filing controls lifecycle.** Move a project to Archive and its session stops
  being offered. The home tree's filing *is* the session lifecycle.
- **Two cockpits** sit above the per-project contexts: `vault` (work *on the
  knowledge base*) and `meta` (work *on the system*).

The tier→workspace mapping is **config**, not baked in: a generic user can just
point at `~/projects` with no PARA at all.

### Display grammar (locked)

```
  <session> >
  home
  <repo>            ·  <branch>
  <repo>            ·  <branch>   ← same repo, different branch = a worktree
```

- `home` on top, no branch.
- Every other row: `repo · branch`, repo left-padded to the longest repo for
  alignment, ` · ` separator. Full repo names — no prefix stripping.
- **Sort:** alphabetical by repo; worktrees of the same repo clustered and
  sorted alphabetically by branch. `home` pinned first. Key: `(is_not_home,
  repo, branch)`.

---

## Decided

1. **Architecture: Shape 2.** A focused tool. No daemon, no TUI yet.
2. **Language: REOPENED by the OSS requirement** — see Q-I. (The Python call
   rested on cohesion with `holmes`; that premise is gone now it's standalone.)
3. **Core abstraction:** context → lanes → views; lane 0 = `home`. (P-13/S-09.)
4. **Source set:** one-or-more containers, scanned identically.
5. **Window identity = `(repo, branch)`.** Worktree dir ignored. (Already in
   `tmux-windowizer` `auto_rename_windows()`.)
6. **Display grammar + sort** — see above.
7. **The slider:** a component is reachable in its cockpit or solo. Same
   container, two zoom levels. (P-04.)
8. **The home tree is the spine; the picker is its projection.** Workspace tiers
   = Projects + Areas + the two cockpits. Reference tiers get no session. Filing
   controls lifecycle.
9. **Infra lives in the Meta tier**, not `01-Projects/`. `01-Projects/chezmoi/`
   is folded into Meta and deleted (artifact of the old `system` session).
10. **All Areas are eligible** (Q-A) — but **lazily**: offered, materialized
    only on open.
11. **vault-sync graduates to a real repo**; `meta` points to it. Deferred.
12. **Cruft removed** (`kalashnikov-ui-original`, etc.) — cleanup pass.
13. **Shell- and terminal-agnostic by construction.** tmux is the portability
    boundary. (See Portability & durability.)
14. **The tool enforces the model and fails honestly.** (See Enforcement.)
15. **Open-source: engine vs. configuration.** Source is a universal engine;
    every opinionated/personal choice is config the user owns, changeable
    without touching source. (See Open source.)
16. **HOLMES OS is a configuration of this tool, not the tool.** The author's
    vault/PARA/cockpit/pane setup ships (if at all) as an example preset.
17. **Language-agnostic contracts.** The tool is defined by contracts (config
    schema, picker row protocol, the tmux/git command surface, JSON/TSV I/O,
    the test fixtures), so the engine language is swappable. A Rust rewrite
    later is re-implementing a spec, not redesigning. (Q-I, P-08.)
18. **Observable.** Structured logging + performance timing from the start.
    (See Observability, P-15.)

---

## Cleanup pass — stabilize before building

Don't pour cement on mud. Get the current system to a stable, consistent state
to operate from in the meantime. Manual / lightly scripted — does **not** need
the new tool; it just makes reality match the model.

- Delete `01-Projects/chezmoi/`; fold chezmoi into Meta.
- Remove cruft repos (`kalashnikov-ui-original`) and dead worktrees.
- Reconcile live sessions: retire ad-hoc `system`; shape `meta`/`vault` to the
  cockpit model; project sessions follow S-09.
- Reconcile naming drift: `lotlens/car_lot_project`,
  `situation-reporter/situation_reporter`, `vantage/llm-vantage`,
  `mike/mike-agent` → pick the rule, rename.
- Fix daily-biting leaks only (NOT the rewrite): `~/collide-vault` hardcode,
  the `-g`→`-s` color bug, the permanent `/tmp/zen-debug.log`.

---

## Dev environment / sandbox

**The sandbox and the portability proof are the same artifact** — and that same
artifact is also the OSS "try it instantly" path. Three layers:

1. **Config-driven roots (mandatory; also Q-E).** Roots come from env/config,
   never hardcoded: code root, home/vault root, tmux socket, role variable.
   No sandbox and no portability without it — same indirection. Build first.
2. **Fixtures generator.** A deterministic, disposable synthetic world: fake
   code root + home tree covering every context shape (multi-repo with a
   worktree, single-repo, plain, both cockpits). Doubles as the test fixture.
3. **Docker clean-room = transferability proof + CI + instant demo.** A
   container with *only the declared deps*, the fixtures, a private tmux socket.
   If the picker works there and nothing else is installed, "minimal and
   transferable" is **proven, not asserted** — P-07 and S-08 made executable.

Notes: macOS (real target) vs Linux (container) divergence is a feature
(catches BSD-isms) but test both; the GUI layer (wezterm/aerospace) is out of
scope — we build the headless engine, not the cockpit glass; stub agent panes
for layout tests; use a dedicated tmux socket even on the host. This is the
test harness of the tool's own repo, not a separate project.

---

## Open source

The constraint that shapes everything: **clean engine in source, all soft spots
in config.** A newcomer installs, boots, and customizes without editing code.

- **Engine vs. config split.** Source = the universal model (contexts, lanes,
  source sets, identity, grammar, build, enforcement). Config = everything
  opinionated.
- **The soft spots (config, not code):** roots & which dirs are context sources;
  the tier/grouping labels; keybindings (live in `tmux.conf`); **picker backend**
  (fzf default, swappable for sk / fzy / television / gum); pane layout
  (author's `nvim+claude` is a choice; default = plain shell); glyphs/colors
  (nerd-font ↔ ASCII); the home-dir convention.
- **Zero-config defaults.** Works out of the box on a plain code root of git
  repos with no config; config only to customize. Progressive disclosure.
- **Pluggable picker by contract.** Anything that reads rows on stdin and
  returns a selection qualifies; the tool defines the row protocol, the user
  names the binary in config.
- **Clean install/boot.** One-command install; a quickstart to a working picker
  in minutes; the Docker image as instant try.
- **No personal data in source, ever.** Project names, vault paths, role paths
  are config/fixtures — never committed to the engine.

---

## Enforcement & error handling

The tool is the **enforcer of the model** and is **honest about failure** — this
is what makes the slider (P-04) safe and the cement hold.

- **Validate before acting.** Check preconditions (tmux up? container exists?
  branch free? home dir real?) before mutating — never half-build a session.
- **Fail loud and legible, never silent.** Every degraded path is reported, not
  hidden. (The audit found the opposite everywhere: silent zen degrade, blank
  compact picker, `??%` status.)
- **Idempotent recovery.** Re-running repairs a partial state rather than
  compounding it (ties to the durability thesis).
- **`doctor` / `check`.** A command that detects drift between model and reality
  — orphaned sessions, cruft repos, naming violations, archived-but-live
  sessions — and reports/repairs. Enforcement made continuous; the natural home
  for the lifecycle checks, and what semi-automates future cleanup passes.
- **Enforceable phrasing (S-06).** Every rule is stated so a reader can tell
  conformance from violation without guessing.

---

## Observability (logging & performance)

In due time, but designed in from the start (P-15).

- **Structured, leveled logging** to a known location; quiet by default, verbose
  on a flag/env.
- **Performance timing** of operations (scan, build, picker) so regressions are
  visible — and so the eventual Rust rewrite has a baseline to beat.
- Agent/automation actions leave a durable trace (P-15): what ran, when, on
  whose direction.

---

## Open questions

- **Q-B · Where a context's definition lives.** home + source set. A `.holmes`
  file at the context root, a registry entry, or pure convention? The
  spine model makes the *common* case derivable by convention; only
  multi-container cockpits need an explicit source list. _Keystone._
- **Q-C · Pointer mechanism.** Wikilinks in the home note, or an explicit field?
- **Q-D · Repo location — RESOLVED → standalone, named `grove`.** Open-source +
  the Docker/sandbox harness settle it: its own repo (`~/code/grove/grove/`),
  its own name `grove` (distinct from `holmes`, the author's manifest CLI),
  pip/brew-installable. The author's `holmes` / HOLMES OS setup *configures* it.
- **Q-E · Config-driven roots** — folded into Dev environment layer 1; now a
  prerequisite, not just open.
- **Q-F · Build / deploy / bootstrap.** PATH on a fresh machine (P-07): binary
  download / `brew` / `pipx` / chezmoi `run_once`? Migration entry (S-10). The
  Docker clean-room validates this path.
- **Q-G · Worktree dir naming.** `holmes worktree new` must create a directory
  (display-irrelevant but must exist). Slugify from the branch?
- **Q-H · Scope boundary.** Focus-tracking unification, daily-recap, status-line
  rewrite — in core or a later pass? _Lean: out of core._
- **Q-I · Starting language.** Endgame is Rust (Decided 17 makes the engine
  swappable behind contracts, so this is a *starting* choice, not a life
  sentence). OSS distribution favors a single static binary (Go/Rust →
  `brew install`, no runtime — the sesh/tms/zellij ecosystem is all compiled);
  Python is most familiar to the author but pipx/venv is more adopter friction.
  Driver is distribution + portfolio, not performance. _Lean: Go now (single
  binary, in-lane portfolio, closer to the Rust endgame than Python). Author's
  call; deferrable while high-level._

---

## Integration surface

| Surface | Direction | Mechanism |
|---|---|---|
| **tmux** | both | tmux invokes the tool via keybindings (`run-shell`); the tool drives tmux by shelling out to the `tmux` CLI. Shell-out, not control-mode — simpler, durable, P-08. |
| **git** | read | subprocess for worktree/branch/dirty state. |
| **filesystem** | read | scans the code root and the home tree. |
| **registry** | read | optional context defs; reuses `holmes_os.registry` in the author's config. |
| **config** | read | machine-variant + soft-spot config (roots, role, picker, layout). |
| **status line** | read | tmux `status-right` calls a segment subcommand. |
| **nvim / claude** | drive | `tmux send-keys` / `new-window -c` to lay out panes. Loose coupling. |

---

## Portability & durability

**Thesis: the session is a pure function of the backend.** The runtime is
*reconstructed from on-disk structure*, never remembered. Portability and
durability then become the same property: any machine, any time, after any
crash, `session open X` rebuilds the identical workspace from version-controlled
structure, not cached state.

- **Stateless by default** — derive from disk each call; persist only small,
  rebuildable niceties (frecency).
- **Idempotent ops** — attach-or-build; safe to re-run.
- **One role variable (S-08)** — no hardcoded personal paths.
- **Shell- & terminal-agnostic — tmux is the boundary.** Target `POSIX + tmux +
  git`. tmux abstracts the terminal above and the pane shell below, so the tool
  works across bash/zsh/fish and any terminal that runs tmux (macOS + Linux).
  Prefer tmux-native ops (`new-window -c <dir>`) over shell-string injection
  (`send-keys "cd …"`); where shell is unavoidable, emit POSIX. Optional
  niceties (popup, nerd glyphs) degrade to ASCII when absent.
- **Reconstructible (P-07 / S-03)** — code + declarative config in VC; the
  Docker clean-room is the live proof.
- **Language-agnostic contracts (Decided 17)** — the engine is swappable; the
  contracts and fixtures outlive any one implementation.
- **No implicit contracts** — set `@pinned_name` directly; validate the nvim
  socket or fail loud; no hand-counted cell budgets; no permanent `/tmp` logs.
- **Single source of truth** — one model, every surface derives from it.

---

## Build order

Strangler-fig: each script becomes a one-line delegation the moment its
subcommand lands. Shippable and interruptible at every step.

0. **Cleanup pass** — stabilize the current system. _Do first._
1. **Contracts + config-driven roots** — the config schema, the picker row
   protocol, the JSON/TSV boundary, the fixtures. The sandbox/portability/
   swappable-engine foundation (Q-E, Decided 17).
2. **`project list`** — scan the code root, apply the source-set model, emit
   JSON/TSV. The cornerstone.
3. **`session open <ctx>`** — build session idempotently.
4. **`worktree new <branch> [--repo]`** — sets `@pinned_name`, `.env` symlink.
5. **`window`** — picker; grammar/sort above.
6. **`status-segment`** — replace `tmux-claude-usage`.

Out of core (Q-H): focus unification, daily-recap, deeper status, full
observability build-out.

---

## Parking lot (bonus — after core)

- **Time-box timer with a global popup.** `timer 25m` → a background waiter
  fires a `tmux display-popup` over whatever session is attached when time's up,
  plus an optional always-visible status countdown. Ties into the existing
  `focus` script and the time-boxing discipline (self.md). Useful, not a toy.
- **Messaging via a tmux pane.** Needs a target first (message whom, via what —
  Telegram? the Mike agent? local?). Park until specified.
