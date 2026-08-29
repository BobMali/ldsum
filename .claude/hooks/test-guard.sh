#!/usr/bin/env bash
# PreToolUse hook on Write|Edit. Creating a test file is free, and so is adding
# a case to one. Rewriting, weakening or deleting an existing test needs the
# author's explicit permission, so those turn into a prompt.

set -uo pipefail

input="$(cat)"

have_jq=0
command -v jq >/dev/null 2>&1 && have_jq=1

if [ "$have_jq" = 1 ]; then
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

# A file that does not exist yet is a new test, not a change to one.
[ -f "$file_path" ] || exit 0

ask() {
  cat <<JSON
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"$1 in $(basename "$file_path"). Approve only if you asked for this test to change; deny if the implementation should be fixed instead."}}
JSON
  exit 0
}

# Without jq the payload cannot be parsed well enough to tell an addition from a
# rewrite, so every change to an existing test file asks.
[ "$have_jq" = 1 ] || ask "A test file is being modified"

tool="$(printf '%s' "$input" | jq -r '.tool_name // empty')"

# Additions must land on a line boundary. Bare containment would accept
# "want: 5" -> "want: 55", which weakens an assertion rather than adding to it.
starts_with_block() {
  local text="$1" anchor="$2" rest
  [ -n "$anchor" ] || return 1
  rest="${text#"$anchor"}"
  [ "$rest" != "$text" ] || return 1
  [ -z "$rest" ] || [ "${rest:0:1}" = $'\n' ]
}

ends_with_block() {
  local text="$1" anchor="$2" pre
  [ -n "$anchor" ] || return 1
  pre="${text%"$anchor"}"
  [ "$pre" != "$text" ] || return 1
  [ "${pre: -1}" = $'\n' ]
}

case "$tool" in
  Edit)
    old="$(printf '%s' "$input" | jq -r '.tool_input.old_string // ""')"
    new="$(printf '%s' "$input" | jq -r '.tool_input.new_string // ""')"
    if starts_with_block "$new" "$old" || ends_with_block "$new" "$old"; then
      exit 0
    fi
    ask "An existing test is being rewritten or removed"
    ;;
  Write)
    content="$(printf '%s' "$input" | jq -r '.tool_input.content // ""')"
    existing="$(cat "$file_path")"
    if starts_with_block "$content" "$existing"; then
      exit 0
    fi
    ask "An existing test file is being overwritten"
    ;;
esac

ask "A test file is being modified"
