#!/usr/bin/env bash
# smoke-test.sh — Exercise every grove command and print its output.
#
# Run this from the "home" lane of any grove session inside the Docker dev
# container (or any machine with GROVE_CODE_ROOT and GROVE_HOME_ROOT set):
#
#   bash ~/scripts/smoke-test.sh     (from inside tmux in the dev container)
#
# Each section prints a labelled header, runs the command, and shows the raw
# output. Failures print a red FAIL line but do not abort — all sections run
# so you can see the full picture at once.
set -uo pipefail

PASS=0
FAIL=0

# ── helpers ────────────────────────────────────────────────────────────────────

bold()  { printf '\033[1m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
dim()   { printf '\033[2m%s\033[0m\n' "$*"; }

# run_section <label> <cmd...>
# Runs the command, prints its output, and marks pass/fail.
run_section() {
    local label="$1"; shift
    echo ""
    bold "── $label"
    dim "$ $*"
    if output=$("$@" 2>&1); then
        echo "$output"
        green "  ✓ ok"
        ((PASS++))
    else
        echo "$output"
        red "  ✗ FAIL (exit $?)"
        ((FAIL++))
    fi
}

# run_expect_fail <label> <cmd...>
# Like run_section but expects a non-zero exit.
run_expect_fail() {
    local label="$1"; shift
    echo ""
    bold "── $label  (expected to fail)"
    dim "$ $*"
    if output=$("$@" 2>&1); then
        echo "$output"
        red "  ✗ FAIL — command succeeded, expected failure"
        ((FAIL++))
    else
        echo "$output"
        green "  ✓ ok (failed as expected)"
        ((PASS++))
    fi
}

# ── preamble ───────────────────────────────────────────────────────────────────

bold "grove smoke test"
dim "vault:  ${GROVE_HOME_ROOT:-<not set>}"
dim "code:   ${GROVE_CODE_ROOT:-<not set>}"
dim "tmux:   ${TMUX:-(not inside tmux)}"
echo ""

# ── grove session list ─────────────────────────────────────────────────────────

run_section "grove session list (human)" \
    grove session list

run_section "grove session list --json" \
    grove session list --json

# ── grove status ───────────────────────────────────────────────────────────────

if [ -n "${TMUX:-}" ]; then
    run_section "grove status (human)" \
        grove status

    run_section "grove status --short" \
        grove status --short

    run_section "grove status --json" \
        grove status --json
else
    echo ""
    bold "── grove status  (skipped — not inside tmux)"
    dim "  Run this script from inside a grove tmux session to test status output."
fi

# ── grove window list ──────────────────────────────────────────────────────────

run_section "grove window list coding-project-big" \
    grove window list coding-project-big

if [ -n "${TMUX:-}" ]; then
    run_section "grove window list (current session)" \
        grove window list
fi

# ── grove project list ─────────────────────────────────────────────────────────

run_section "grove project list (human)" \
    grove project list

run_section "grove project list --json" \
    grove project list --json

# ── grove project init / archive round-trip ────────────────────────────────────

SMOKE_PROJECT="smoke-$(date +%s)"

run_section "grove project init $SMOKE_PROJECT" \
    grove project init "$SMOKE_PROJECT"

run_section "grove project init $SMOKE_PROJECT --code --no-git" \
    grove project init "$SMOKE_PROJECT" --code --no-git

run_section "grove project list (after init)" \
    grove project list

run_section "grove project archive $SMOKE_PROJECT" \
    grove project archive "$SMOKE_PROJECT"

run_section "grove project list (after archive)" \
    grove project list

# ── grove window delete guards ─────────────────────────────────────────────────

if [ -n "${TMUX:-}" ]; then
    run_expect_fail "grove window delete home  (must fail)" \
        grove window delete home

    run_expect_fail "grove window delete nonexistent-window  (must fail with available list)" \
        grove window delete "this-window-does-not-exist"
fi

# ── grove doctor ───────────────────────────────────────────────────────────────

run_section "grove doctor" \
    grove doctor

# ── summary ────────────────────────────────────────────────────────────────────

echo ""
bold "── summary"
green "  passed: $PASS"
if [ "$FAIL" -gt 0 ]; then
    red "  failed: $FAIL"
    exit 1
else
    green "  failed: $FAIL"
fi
