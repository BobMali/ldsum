#!/usr/bin/env bash
# PreToolUse hook on Write|Edit: once a test file exists, changing it needs the
# user's explicit approval. Writing a test file that does not exist yet is
# creation, not a change, so it passes through untouched.

set -uo pipefail

input="$(cat)"

if command -v jq >/dev/null 2>&1; then
  file_path="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty')"
else
  file_path="$(printf '%s' "$input" \
    | sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n 1)"
fi

case "$file_path" in
  *_test.go) ;;
  *) exit 0 ;;
esac

[ -f "$file_path" ] || exit 0

name="$(basename "$file_path")"

cat <<JSON
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"$name already exists. Changing a test after it is written needs your explicit permission — approve only if you asked for this test to change, and deny if the implementation should be fixed instead."}}
JSON
