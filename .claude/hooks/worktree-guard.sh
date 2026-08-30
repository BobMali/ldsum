#!/usr/bin/env bash
# PreToolUse hook on Bash. Two git commands throw away work that nothing can
# bring back: `reset --hard` drops every uncommitted change, and `clean -f`
# deletes untracked files — including the git-ignored scratch directories that
# agents keep their state in. Both are rare in honest use, so a prompt costs
# almost nothing and buys back the one mistake with no undo.
#
# Deliberately narrow. `checkout -- <path>`, `restore <path>` and `stash` also
# discard, but they are the ordinary "undo my mess" commands, their blast
# radius is the path you name, and a stash comes back with `stash pop`. Guard
# everything and the prompts stop being read.
#
# Known limit: the subcommand is found by skipping git's global flags, and only
# -C and -c are known to take an argument. An exotic global flag with its own
# argument could hide the subcommand.

set -uo pipefail

input="$(cat)"

if command -v jq >/dev/null 2>&1; then
  tool="$(printf '%s' "$input" | jq -r '.tool_name // empty')"
  command_line="$(printf '%s' "$input" | jq -r '.tool_input.command // empty')"
else
  # Without jq the payload cannot be parsed, so fall back to the raw text: a
  # destructive command still trips the checks below.
  tool="Bash"
  command_line="$input"
fi

[ "$tool" = "Bash" ] || exit 0

case "$command_line" in
  *git*) ;;
  *) exit 0 ;;
esac

ask() {
  cat <<JSON
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"$1 Approve only if losing that work is what you intend."}}
JSON
  exit 0
}

has_flag() { # segment, extended regex for the flag
  printf '%s' "$1" | grep -qE "$2"
}

while IFS= read -r segment; do
  case " $segment " in
    *" git "*|"git "*) ;;
    *) continue ;;
  esac

  # The subcommand is the first non-flag word after `git`.
  read -r -a words <<<"$segment"
  i=0
  while [ "$i" -lt "${#words[@]}" ] && [ "${words[$i]}" != "git" ]; do
    i=$((i + 1))
  done
  i=$((i + 1))

  sub=""
  while [ "$i" -lt "${#words[@]}" ]; do
    case "${words[$i]}" in
      -C | -c) i=$((i + 2)) ;;
      -*) i=$((i + 1)) ;;
      *)
        sub="${words[$i]}"
        break
        ;;
    esac
  done

  case "$sub" in
    reset)
      if has_flag "$segment" '(^|[[:space:]])--hard([[:space:]]|$)'; then
        ask "git reset --hard discards every uncommitted change in the tree."
      fi
      ;;
    clean)
      # A dry run lists what would go and removes nothing.
      if has_flag "$segment" '(^|[[:space:]])(-[a-zA-Z]*n|--dry-run)'; then
        continue
      fi
      if has_flag "$segment" '(^|[[:space:]])(-[a-zA-Z]*f|--force)'; then
        ask "git clean deletes untracked files, ignored scratch directories included."
      fi
      ;;
  esac
done <<EOF
$(printf '%s' "$command_line" | tr ';|&' '\n')
EOF

exit 0
