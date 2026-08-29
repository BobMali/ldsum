#!/usr/bin/env bash
# PostToolUse hook: after Claude writes or edits a Go file, run the quality
# gate. Exit 2 blocks and feeds stderr back to Claude so it fixes the problem
# itself instead of moving on.

set -uo pipefail

input="$(cat)"

# Pull tool_input.file_path out of the hook JSON on stdin.
if command -v jq >/dev/null 2>&1; then
  file_path="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty')"
else
  file_path="$(printf '%s' "$input" \
    | sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n 1)"
fi

# Only care about Go sources.
case "$file_path" in
  *.go) ;;
  *) exit 0 ;;
esac

cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0

# Nothing to check before the module exists.
[ -f go.mod ] || exit 0

problems=""

unformatted="$(gofmt -l . 2>&1)"
if [ -n "$unformatted" ]; then
  problems="${problems}gofmt: these files are not formatted:
${unformatted}

"
fi

if ! vet_output="$(go vet ./... 2>&1)"; then
  problems="${problems}go vet failed:
${vet_output}

"
fi

if ! test_output="$(go test ./... 2>&1)"; then
  problems="${problems}go test failed:
${test_output}
"
fi

if [ -n "$problems" ]; then
  printf 'Quality gate failed after editing %s\n\n%s' "$file_path" "$problems" >&2
  printf 'Fix this before continuing. Do not start new work.\n' >&2
  exit 2
fi

exit 0
