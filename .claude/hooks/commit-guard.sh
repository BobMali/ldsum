#!/usr/bin/env bash
# PreToolUse hook on Bash: inspects `git commit` commands before they run.
# Exit 2 blocks the tool call and returns stderr to Claude.

set -uo pipefail

input="$(cat)"

if command -v jq >/dev/null 2>&1; then
  cmd="$(printf '%s' "$input" | jq -r '.tool_input.command // empty')"
else
  cmd="$(printf '%s' "$input" \
    | sed -n 's/.*"command"[[:space:]]*:[[:space:]]*"\(.*\)".*/\1/p' \
    | head -n 1)"
  # Minimal JSON unescaping; install jq to avoid this path.
  cmd="${cmd//\\n/$'\n'}"
  cmd="${cmd//\\t/$'\t'}"
  cmd="${cmd//\\\"/\"}"
  cmd="${cmd//\\\\/\\}"
fi

# Not a commit -> nothing to do.
printf '%s' "$cmd" | grep -qE '(^|[;&|[:space:]])git[[:space:]]+commit' || exit 0

block() {
  printf '%s\n' "$1" >&2
  exit 2
}

# Inspect only the git commit invocation's own arguments, so a -n belonging
# to grep, sort or echo elsewhere on the line does not trip the guard.
commit_segs="$(printf '%s' "$cmd" | tr ';&|' '\n' \
  | grep -E '(^|[[:space:]])git[[:space:]]+commit' || true)"

while IFS= read -r seg; do
  [ -n "$seg" ] || continue
  case " $seg " in
    *" --no-verify "*|*" -n "*)
      block "Refused: that flag skips the commit-msg hook. Fix the message instead."
      ;;
  esac
done <<< "$commit_segs"

# Extract the subject line from -m "..." / -m '...' / a heredoc.
subject=""
if printf '%s' "$cmd" | grep -qE "\-m[[:space:]]+\""; then
  subject="$(printf '%s' "$cmd" | sed -n 's/^[^"]*-m[[:space:]]*"\([^"]*\).*/\1/p' | head -n 1)"
elif printf '%s' "$cmd" | grep -qE "\-m[[:space:]]+'"; then
  subject="$(printf '%s' "$cmd" | sed -n "s/^[^']*-m[[:space:]]*'\([^']*\).*/\1/p" | head -n 1)"
elif printf '%s' "$cmd" | grep -q '<<'; then
  subject="$(printf '%s' "$cmd" | sed -n '/<</,$p' | sed -n '2p')"
fi

subject="${subject%%$'\n'*}"

# No message on the command line: the commit-msg hook will catch it.
[ -n "$subject" ] || exit 0

hook_dir="${CLAUDE_PROJECT_DIR:-.}/.githooks"
regex_file="$hook_dir/conventional-regex.txt"
[ -f "$regex_file" ] || exit 0
regex="$(head -n 1 "$regex_file")"

case "$subject" in
  "Merge "*|"Revert "*|"fixup!"*|"squash!"*) exit 0 ;;
esac

if printf '%s' "$subject" | grep -qE "$regex"; then
  exit 0
fi

block "Refused: commit subject is not a valid Conventional Commit.

  got: $subject

Format: <type>(<scope>): <description>
  types:  feat fix docs test refactor perf build ci chore revert
  scopes: hash checksums cmd run ci deps   (optional)
  desc:   lowercase, imperative, no trailing period, <= 66 chars
  break:  append ! before the colon

Rewrite the message and run the command again. Describe why the change was
made, not that a change was made ('fix: fix bug' is not acceptable)."
