#!/usr/bin/env bash
# dev-entrypoint.sh
#
# Starts a grove dev session inside the Docker clean-room.
# Drops you directly into the richest fixture context (coding-project-big)
# so window pick, worktree list, session delete, etc. are all immediately testable.
#
# The container has GROVE_CODE_ROOT and GROVE_HOME_ROOT already set to the
# fixture world generated at image build time.
set -e

echo "grove dev environment"
echo "  vault: $GROVE_HOME_ROOT"
echo "  code:  $GROVE_CODE_ROOT"
echo ""
echo "Available contexts:"
grove session list | grep -o '"name":"[^"]*"' | sed 's/"name":"//;s/"//' | sed 's/^/  /' 2>/dev/null || true
echo ""
echo "Opening coding-project-big..."

exec grove session open coding-project-big
