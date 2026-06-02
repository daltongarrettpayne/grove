# grove

Weaves your knowledge tree and your code tree into tmux sessions, windows, and
panes — one model, enforced — so your terminal workspace stops shifting under
you.

> **Status:** early design, not yet implemented. The full design + decision log
> lives in [`docs/design.md`](docs/design.md).

## The idea

- A **context** is a tmux session; its **lanes** are windows; its **views** are
  panes.
- A context sources from one-or-more container directories of git repos and
  worktrees; each working tree becomes a lane. Window 0 is always `home`.
- A window's identity is `(repo, branch)` — worktrees fall out for free, since
  git forbids the same branch in two of them.
- **The session is a pure function of what's on disk** — stateless, idempotent,
  reconstructible. Portability and durability come for free.
- **Engine in source, soft spots in config.** Roots, keybindings, picker
  backend, pane layout, colors — all configurable, nothing personal hardcoded.

## Why

Built so the system can be *cement, not wood models*: a stable, predictable
workspace layer that leverages you for the real work instead of demanding to be
re-tinkered. Open-source from the start; one configuration of it runs the
author's HOLMES OS.

## Status

Design phase. Starting language not yet locked (Go-leaning; Rust the eventual
target — the engine sits behind language-agnostic contracts so the rewrite is
re-implementing a spec, not redesigning).
