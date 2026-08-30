#!/usr/bin/env bash
# PreToolUse hook. Creating a test file is free, and so is adding a case to one.
# Rewriting, weakening or deleting an existing test needs the author's explicit
# permission, so those turn into a prompt.
#
# Registered on Write|Edit, and on Bash so a shell write cannot route around it.
# The Bash side asks about anything it cannot positively identify as read-only,
# so an unusual interpreter or an `eval` wrapper still prompts. Its real limit
# is different: it only looks at commands that name a *_test.go path, so one
# that reaches the file without naming it — `git reset --hard`, a script — goes
# unseen. Those destroy the whole working tree rather than tests specifically,
# and belong to a guard of their own.

set -uo pipefail

input="$(cat)"

have_jq=0
command -v jq >/dev/null 2>&1 && have_jq=1

ask() {
  cat <<JSON
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"$1 Approve only if you asked for this test to change; deny if the implementation should be fixed instead."}}
JSON
  exit 0
}

if [ "$have_jq" = 1 ]; then
  tool="$(printf '%s' "$input" | jq -r '.tool_name // empty')"
  file_path="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty')"
  command_line="$(printf '%s' "$input" | jq -r '.tool_input.command // empty')"
else
  tool=""
  file_path="$(printf '%s' "$input" \
    | sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n 1)"
  command_line=""
  # Without jq a Bash payload cannot be parsed, so any mention of a test file asks.
  case "$input" in
    *_test.go*)
      [ -n "$file_path" ] || ask "A shell command mentions a test file and jq is not installed to inspect it."
      ;;
  esac
fi

# ---------------------------------------------------------------- Bash writes

if [ "$tool" = "Bash" ]; then
  # Nothing to guard unless a Go test file is named at all. `go test ./...`
  # never names one, so the common case exits here.
  case "$command_line" in
    *_test.go*) ;;
    *) exit 0 ;;
  esac

  # A redirection into a test file is a write whatever produced it.
  if printf '%s' "$command_line" | grep -qE '>>?[[:space:]]*[^[:space:];|&<>]*_test\.go'; then
    ask "A shell redirection writes to a test file."
  fi

  # Judge each pipeline segment that names a test file by its command word.
  # Reading one is fine; anything not known to be read-only is treated as a write.
  while IFS= read -r segment; do
    case "$segment" in
      *_test.go*) ;;
      *) continue ;;
    esac

    word="$(printf '%s' "$segment" | sed -E 's/^[[:space:]]*//; s/[[:space:]].*//')"
    word="${word##*/}"

    case "$word" in
      cat|head|tail|less|more|grep|rg|egrep|fgrep|wc|ls|stat|file|realpath|basename|dirname|sort|uniq|cut|tr|nl|column|bat|diff|cmp|shasum|md5|md5sum|echo|printf)
        ;;
      find)
        # Searching is read-only; -exec and friends make find arbitrary.
        # " -exec" also matches -execdir, and " -ok" matches -okdir.
        case " $segment " in
          *" -exec"*|*" -ok"*|*" -delete"*)
            ask "find runs a command over a test file."
            ;;
        esac
        ;;
      sed)
        case " $segment " in
          *" -i"*|*" --in-place"*) ask "sed edits a test file in place." ;;
        esac
        ;;
      awk)
        case " $segment " in
          *" -i"*|*inplace*) ask "awk edits a test file in place." ;;
        esac
        ;;
      gofmt|goimports)
        case " $segment " in
          *" -w"*) ask "gofmt rewrites a test file in place." ;;
        esac
        ;;
      go)
        # go test / go build / go vet read sources; they never rewrite them.
        ;;
      git)
        case " $segment " in
          *" diff "*|*" log "*|*" show "*|*" status "*|*" blame "*|*" grep "*|*" ls-files "*|*" cat-file "*) ;;
          *" add "*|*" commit "*) ;;
          *) ask "A git command may rewrite a test file." ;;
        esac
        ;;
      *)
        ask "A shell command may write to a test file."
        ;;
    esac
  done <<EOF
$(printf '%s' "$command_line" | tr ';|&' '\n')
EOF

  exit 0
fi

# ----------------------------------------------------------- Write and Edit

case "$file_path" in
  *_test.go) ;;
  *) exit 0 ;;
esac

# A file that does not exist yet is a new test, not a change to one.
[ -f "$file_path" ] || exit 0

name="$(basename "$file_path")"

# Without jq the payload cannot be parsed well enough to tell an addition from a
# rewrite, so every change to an existing test file asks.
[ "$have_jq" = 1 ] || ask "A test file is being modified in $name."

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
    ask "An existing test is being rewritten or removed in $name."
    ;;
  Write)
    content="$(printf '%s' "$input" | jq -r '.tool_input.content // ""')"
    existing="$(cat "$file_path")"
    if starts_with_block "$content" "$existing"; then
      exit 0
    fi
    ask "An existing test file is being overwritten: $name."
    ;;
esac

ask "A test file is being modified in $name."
