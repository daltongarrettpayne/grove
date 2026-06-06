#!/usr/bin/env bash
# integration-test.sh — drive grove's real tmux/git surface and assert on the
# resulting state.
#
# Unlike smoke-test.sh (which runs each command and eyeballs its output), this
# harness sets up an isolated tmux server on a private socket, runs the
# interactive flows end to end, and asserts on observable state: which windows
# got built, which window became active after a pick, which worktree dirs exist,
# which sessions are live. It is the automated coverage for the layer that unit
# tests can't reach — the tmux + git + picker integration.
#
# It is self-contained and needs no attached client: every assertion is made
# against tmux server state (list-windows, has-session, active window) or the
# filesystem, all of which are observable without an interactive client. The two
# steps that genuinely need an attached client — the display-popup picker
# overlay and switch-client/attach focus changes — are exercised for their build
# side effects and otherwise noted; verify those interactively on a real machine.
#
# Requirements: grove on PATH, plus tmux, git, fzf. GROVE_CODE_ROOT and
# GROVE_HOME_ROOT must point at a fixture world (the Docker dev image sets both).
#
# Usage:
#   bash scripts/integration-test.sh
#   make integration            # runs it inside the Docker clean-room
set -uo pipefail

PASS=0
FAIL=0

green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
bold()  { printf '\033[1m%s\033[0m\n' "$*"; }
dim()   { printf '\033[2m%s\033[0m\n' "$*"; }

# ok <label> — record a pass.
ok()   { green "  ✓ $1"; ((PASS++)); }
# bad <label> <detail> — record a failure with context.
bad()  { red   "  ✗ $1"; [ -n "${2:-}" ] && printf '      %s\n' "$2"; ((FAIL++)); }

# assert_eq <label> <expected> <actual>
assert_eq() {
    if [ "$2" = "$3" ]; then ok "$1"; else bad "$1" "expected [$2], got [$3]"; fi
}
# assert_contains <label> <needle> <haystack>
assert_contains() {
    if printf '%s' "$3" | grep -qF -- "$2"; then ok "$1"; else bad "$1" "missing [$2] in: $3"; fi
}

# ── isolated tmux server ─────────────────────────────────────────────────────
# Export the socket BEFORE creating the server so panes inherit it and grove
# (which honors GROVE_TMUX_SOCKET) and the harness address the same server.
export GROVE_TMUX_SOCKET="${GROVE_TMUX_SOCKET:-/tmp/grove-integration.sock}"
SOCK="$GROVE_TMUX_SOCKET"
TM() { tmux -S "$SOCK" "$@"; }

cleanup() { TM kill-server 2>/dev/null || true; }
trap cleanup EXIT

TM kill-server 2>/dev/null || true
TM new-session -d -s driver -x 220 -y 50 -c "$GROVE_HOME_ROOT"

# send <window> <command> — type a command into a pane and wait for it to finish
# by polling a per-call sentinel file. Returns the captured stdout+stderr.
SENTINEL_DIR="$(mktemp -d)"
send() {
    local target="$1"; shift
    local cmd="$1"
    local out; out="$SENTINEL_DIR/out.$RANDOM"
    TM send-keys -t "$target" "{ $cmd ; } > $out 2>&1; touch $out.done" Enter
    for _ in $(seq 1 50); do
        [ -f "$out.done" ] && break
        sleep 0.1
    done
    cat "$out" 2>/dev/null
}

bold "grove integration test"
dim  "socket: $SOCK"
dim  "vault:  ${GROVE_HOME_ROOT:-<unset>}"
dim  "code:   ${GROVE_CODE_ROOT:-<unset>}"
echo

# ── session open: builds home + one window per lane, focus returns home ───────
bold "── session open builds the workspace"
send driver "grove session open coding-project-big" >/dev/null
windows="$(TM list-windows -t coding-project-big -F '#{window_index}:#{window_name}' 2>/dev/null)"
assert_contains "home is window 0"            "0:home"        "$windows"
assert_eq       "lane count (home + 5 lanes)" "6"             "$(printf '%s\n' "$windows" | grep -c .)"
assert_contains "repo-alpha · main lane"      "repo-alpha"    "$windows"
assert_contains "repo-beta · feat/dirty lane" "feat/dirty"    "$windows"
active="$(TM display-message -t coding-project-big -p '#{window_name}' 2>/dev/null)"
assert_eq       "focus returns to home"       "home"          "$active"

# ── session open is idempotent ────────────────────────────────────────────────
bold "── session open is idempotent"
before="$(TM list-windows -t coding-project-big -F '#{window_name}' | sort | tr '\n' ',')"
send driver "grove session open coding-project-big" >/dev/null
after="$(TM list-windows -t coding-project-big -F '#{window_name}' | sort | tr '\n' ',')"
assert_eq "re-open does not duplicate windows" "$before" "$after"

# ── session list marks the live session ───────────────────────────────────────
bold "── session list reflects live sessions"
list="$(grove session list)"
assert_contains "coding-project-big listed" "coding-project-big" "$list"
live_row="$(printf '%s\n' "$list" | grep 'coding-project-big')"
assert_contains "big shows session=yes" "yes" "$live_row"

# ── window list grammar ───────────────────────────────────────────────────────
bold "── window list grammar"
wl="$(grove window list coding-project-big)"
assert_eq       "home is first row" "home" "$(printf '%s\n' "$wl" | head -1)"
assert_contains "uses '·' separator" "·" "$wl"

# ── window pick selection (POPUP_ACTIVE bypass + send-keys drives fzf) ─────────
# select-window is client-free, so the active window genuinely changes.
bold "── window pick selects and switches window"
TM select-window -t coding-project-big:0
TM send-keys -t coding-project-big:0 "GROVE_POPUP_ACTIVE=1 grove window pick" Enter
sleep 1.2
TM send-keys -t coding-project-big:0 "feat/stale"
sleep 0.8
TM send-keys -t coding-project-big:0 Enter
sleep 1.2
picked="$(TM display-message -t coding-project-big -p '#{window_name}')"
assert_contains "window pick switched to feat/stale" "feat/stale" "$picked"

# ── session pick selection (builds the chosen session) ────────────────────────
bold "── session pick opens the chosen context"
TM kill-session -t coding-project-small 2>/dev/null || true
TM new-window -t driver -n pick -c "$GROVE_HOME_ROOT"
TM send-keys -t driver:pick "GROVE_POPUP_ACTIVE=1 grove session pick" Enter
sleep 1.2
TM send-keys -t driver:pick "small"
sleep 0.8
TM send-keys -t driver:pick Enter
sleep 1.5
if TM has-session -t coding-project-small 2>/dev/null; then
    ok "session pick built coding-project-small"
else
    bad "session pick built coding-project-small" "session not created"
fi

# ── worktree new / list / delete round-trip ───────────────────────────────────
bold "── worktree lifecycle"
REPO="$GROVE_CODE_ROOT/coding-project-small/repo"
# Pre-clean so reruns against a shared fixture world start fresh (git worktree
# remove leaves the branch behind, which would collide on the next `new`).
send driver "cd $REPO && git worktree remove --force ../repo-itest 2>/dev/null; git branch -D feat/itest 2>/dev/null; true" >/dev/null
wnew="$(send driver "cd $REPO && grove worktree new feat/itest")"
if [ -d "$GROVE_CODE_ROOT/coding-project-small/repo-itest" ]; then
    ok "worktree new created repo-itest dir"
else
    bad "worktree new created repo-itest dir" "$wnew"
fi
wlist="$(send driver "cd $REPO && grove worktree list")"
assert_contains "worktree list shows feat/itest" "feat/itest" "$wlist"
assert_contains "worktree list marks (main)"     "(main)"     "$wlist"
send driver "cd $REPO && grove worktree delete feat/itest" >/dev/null
if [ ! -d "$GROVE_CODE_ROOT/coding-project-small/repo-itest" ]; then
    ok "worktree delete removed the dir"
else
    bad "worktree delete removed the dir" "dir still present"
fi
# Leave the fixture branch tidy for the next run.
send driver "cd $REPO && git branch -D feat/itest 2>/dev/null; true" >/dev/null

# ── worktree error paths ──────────────────────────────────────────────────────
bold "── worktree error handling"
err="$(send driver "cd $REPO && grove worktree new 'Bad/Name!!'; echo RC=\$?")"
assert_contains "bad branch name rejected" "RC=1" "$err"
err="$(send driver "cd /root && grove worktree list; echo RC=\$?")"
assert_contains "not-a-repo rejected"      "RC=1" "$err"

# ── window delete guards ──────────────────────────────────────────────────────
bold "── window delete guards"
g1="$(send coding-project-big:0 "grove window delete home; echo RC=\$?")"
assert_contains "cannot delete home"        "cannot delete the home window" "$g1"
g2="$(send coding-project-big:0 "grove window delete nope; echo RC=\$?")"
assert_contains "missing window rejected"   "not found"                     "$g2"
assert_contains "missing window lists opts" "Available windows"             "$g2"

# ── project init / list / archive round-trip ──────────────────────────────────
bold "── project lifecycle"
P="itest-proj-$$"
send driver "grove project init $P" >/dev/null
pl="$(grove project list)"
assert_contains "project init created context" "$P" "$pl"
send driver "grove project archive $P" >/dev/null
pl2="$(grove project list)"
if printf '%s' "$pl2" | grep -qF "$P"; then
    bad "project archive removed from active list" "still listed"
else
    ok "project archive removed from active list"
fi

# ── status / status-bar in a lane window ──────────────────────────────────────
bold "── status reflects the focused window"
TM select-window -t coding-project-big:0
sb_home="$(send coding-project-big:0 "grove status-bar")"
assert_eq "status-bar at home = session name" "coding-project-big" "$sb_home"

# ── doctor detects fixture drift ──────────────────────────────────────────────
bold "── doctor finds the seeded violations"
doc="$(grove doctor; echo RC=$?)"
assert_contains "doctor reports stale-worktree" "stale-worktree" "$doc"
assert_contains "doctor reports detached-head"  "detached-head"  "$doc"
assert_contains "doctor exits non-zero on drift" "RC=1"          "$doc"

# ── session delete ────────────────────────────────────────────────────────────
bold "── session delete"
del="$(send driver "grove session delete coding-project-small; echo RC=\$?")"
assert_contains "delete succeeds" "RC=0" "$del"
if TM has-session -t coding-project-small 2>/dev/null; then
    bad "session is gone after delete" "still alive"
else
    ok "session is gone after delete"
fi
delx="$(send driver "grove session delete no-such; echo RC=\$?")"
assert_contains "delete missing session fails" "RC=1" "$delx"

# ── summary ───────────────────────────────────────────────────────────────────
rm -rf "$SENTINEL_DIR"
echo
bold "── summary"
green "  passed: $PASS"
if [ "$FAIL" -gt 0 ]; then
    red "  failed: $FAIL"
    exit 1
fi
green "  failed: 0"
