# Binary Harness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a root-level test that builds the real `ldsum` binary and executes it as a process, so that `main.go`, the process exit status, real argv and the real working directory stop being untested.

**Architecture:** One new file, `main_test.go`, in `package main_test` at the repository root. `TestMain` builds the binary once into a temp directory; a `run` helper execs it with a chosen working directory and reports both streams plus the exit status read from `*exec.ExitError`. Six cases, each present only because an in-process test structurally cannot observe what it observes. No production code changes at all.

**Tech Stack:** Go 1.27, standard library only for the harness (`os/exec`, `bytes`, `errors`). No new module. `gremlins` gates CI; `go-mutesting` is the acceptance check and is run by hand.

**Spec:** `docs/superpowers/specs/2026-09-02-binary-harness-design.md` — commit `6043923`. The plan argues from the spec; read both.

## Global Constraints

- Go 1.27 or newer. Module path `github.com/BobMali/ldsum`.
- Standard library plus `spf13/cobra` only. **No new module may be added by this plan.** `rogpeppe/go-internal/testscript` was considered and rejected on this rule.
- Never assert against a digest the implementation just produced. The sha256 of `"abc"` is `ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad`, already bound as `abcSHA256` in `cmd/verify_test.go`; this package binds its own copy.
- Anything written goes under `t.TempDir()`.
- **Do not edit, rename, delete or weaken an existing test.** This plan creates one new file and appends to it. Task 4 edits `README.md` and `CLAUDE.md` only.
- **Never exec the `./ldsum` in the working tree.** It is gitignored, untracked and may be any age. The harness always builds fresh.
- Do not touch `test.txt`, `test_dir/` or `checksum_file.txt` — untracked scratch files belonging to the author.
- Definition of done for every task: `gofmt -l .` prints nothing, and `go vet ./...`, `go test ./...` and `golangci-lint run` are clean. Run them; do not reason about them.
- Commit messages are Conventional Commits: `<type>(<scope>): <description>`, lowercase, imperative, no trailing period, 66 characters or fewer. No scope fits a root-level test, so these commits carry none. **No trailers of any kind.**

## Two deliberate deviations, so a reviewer does not have to guess

1. **Tasks 2 and 3 use explicit `t.Run` blocks rather than a struct table.** The house rule is table-driven, and Task 1 follows it. Tasks 2 and 3 do not, because their cases assert differently-shaped evidence — one exact stderr, one substring, one empty — and a shared row would need a per-case escape hatch that reads worse than the three blocks it replaces. Each case is still its own `t.Run`.
2. **The unknown-algorithm case asserts a prefix and a substring, not the whole sentence.** The wording belongs to `internal/run`, whose own tests already pin it. What this harness pins is that the diagnostic reached the real stderr with the `ldsum: ` prefix.
3. **Four commits, where the spec sketched two.** The spec was written before the work was decomposed. The harness splits cleanly into three logical increments, each with its own experiment and each independently revertable; the documentation commit stays separate exactly as the spec said, because it depends on evidence the first three produce.

## What "red" means here

`main.go` predates this harness, so red-green-refactor cannot start from an absent feature. Three experiments substitute for it, and each is a step in the plan rather than a note:

- **Task 1** breaks the build on purpose and confirms `TestMain`'s error path reports it, instead of leaving six cases to fail confusingly against a missing file.
- **Task 2** rewrites `main.go` on a throwaway copy to drop `os.Exit` and confirms the two exit-code cases go red. This is the experiment that proves the harness pins `main.go` at all.
- **Task 3** reintroduces cwd-relative path resolution on a throwaway copy and confirms the relative-entry case goes red — the shape of the original `selectTargets` defect.

All three must be **run**, and the observed output pasted into the task report. "It would fail" is not an acceptable report, and it is the specific failure mode a cheap implementer falls into: this plan wants a sonnet-tier model or better for exactly that reason.

---

### Task 1: Build the binary and prove the harness runs it

**Files:**
- Create: `main_test.go`

**Interfaces:**
- Produces, for Tasks 2 and 3:
  - `const abcSHA256 string` — the sha256 of `"abc"`.
  - `var binary string` — absolute path to the freshly built binary, set by `TestMain`.
  - `func run(t *testing.T, dir string, args ...string) (stdout, stderr string, code int)` — execs `binary` with `dir` as its working directory.
  - `func writeIn(t *testing.T, dir, name, contents string) string` — writes a fixture, returns its absolute path.

- [ ] **Step 1: Write the harness and the first case**

Create `main_test.go`:

```go
// Package main_test executes the built binary. It is the only test in the
// tree that leaves the process boundary: every other one drives cobra in
// process, where the exit status, real argv and the real working directory
// cannot be observed.
package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The sha256 of "abc" — a known-answer vector, not a digest this suite
// produced. cmd/verify_test.go binds the same value for the same reason.
const abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

// binary is the freshly built ldsum under test, set by TestMain.
var binary string

func TestMain(m *testing.M) {
	// t.TempDir needs a *testing.T, which TestMain does not have, so the
	// directory is made and removed by hand.
	dir, err := os.MkdirTemp("", "ldsum-harness")
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness: temp dir: %v\n", err)
		os.Exit(1)
	}

	// Built fresh rather than reusing the ./ldsum in the working tree, which
	// is gitignored and may be any age. Only the returned error decides
	// success: go build writes to stderr on a good run too — a sandboxed one
	// emits a module-cache warning and still exits 0.
	binary = filepath.Join(dir, "ldsum")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "harness: go build: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	// os.Exit skips deferred functions, so the cleanup is spelled out.
	os.RemoveAll(dir)
	os.Exit(code)
}

// run executes the built binary with dir as its working directory — the one
// thing this harness exists to vary — and reports both streams and the
// process exit status.
func run(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	var exit *exec.ExitError
	switch err := cmd.Run(); {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		// The binary could not be started at all: a broken harness rather
		// than a failing case.
		t.Fatalf("run %v: %v", args, err)
	}
	return out.String(), errOut.String(), code
}

func writeIn(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestBinarySumPrintsADigest(t *testing.T) {
	tests := []struct {
		name       string
		contents   string
		wantStdout string
	}{
		{"known-answer vector", "abc", abcSHA256 + "  payload.txt\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeIn(t, dir, "payload.txt", tt.contents)

			stdout, stderr, code := run(t, dir, "sum", "payload.txt")
			if code != 0 {
				t.Errorf("exit = %d, want 0\nstderr: %s", code, stderr)
			}
			if stdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tt.wantStdout)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
		})
	}
}
```

- [ ] **Step 2: Run it**

```sh
go test -run TestBinarySumPrintsADigest -v .
```

Expected: PASS. The digest is the known-answer vector for `"abc"`, and `sum` writes GNU text format — digest, **two** spaces, path as given on the command line.

- [ ] **Step 3: Break the build and confirm the harness says so**

`TestMain`'s build-failure branch is otherwise dead code, and a harness that fails obscurely when the build breaks wastes the next person's afternoon. Prove it:

```sh
printf '\nfunc broken(\n' >> main.go
go test -run TestBinarySumPrintsADigest . ; echo "exit=$?"
git checkout -- main.go
```

Expected: a `harness: go build:` line naming the syntax error, and a non-zero exit — **not** six confusing per-case failures. Paste the observed output into the task report.

- [ ] **Step 4: Confirm the tree is clean again and the gates pass**

```sh
git diff --stat main.go   # must print nothing
gofmt -l .
go vet ./...
go test ./...
golangci-lint run
```

Expected: `git diff` silent, `gofmt` silent, the rest clean.

- [ ] **Step 5: Commit**

```sh
git add main_test.go
git commit -m "test: execute the built binary end to end"
```

---

### Task 2: Pin the process exit codes

This is the task that makes the harness worth having: `Execute` returning an `int` is already tested in `cmd/`, but nothing has ever checked that the *process* reports it.

**Files:**
- Modify: `main_test.go` (append; add `"strings"` to the import block)

**Interfaces:**
- Consumes: `abcSHA256`, `run`, `writeIn` from Task 1.
- Produces: nothing later tasks depend on.

- [ ] **Step 1: Write the failing test**

Append to `main_test.go`, and add `"strings"` to its imports:

```go
func TestBinaryExitCodes(t *testing.T) {
	// A well-formed sha256 that nothing hashes to.
	const zeroSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"

	t.Run("a mismatch exits 1", func(t *testing.T) {
		dir := t.TempDir()
		writeIn(t, dir, "payload.txt", "abc")

		stdout, stderr, code := run(t, dir, "verify", "payload.txt", zeroSHA256)
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if want := "payload.txt: FAILED\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		want := "expected: " + zeroSHA256 + "\nactual:   " + abcSHA256 + "\n"
		if stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	})

	t.Run("an unknown algorithm exits 2", func(t *testing.T) {
		dir := t.TempDir()
		writeIn(t, dir, "payload.txt", "abc")

		stdout, stderr, code := run(t, dir, "verify", "--algo", "nonesuch", "payload.txt", abcSHA256)
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		// The sentence itself belongs to internal/run, whose tests own it.
		// What this case pins is that it reached the real stderr, prefixed.
		if !strings.HasPrefix(stderr, "ldsum: ") || !strings.Contains(stderr, "unknown algorithm") {
			t.Errorf("stderr = %q, want an ldsum-prefixed unknown-algorithm diagnostic", stderr)
		}
	})
}
```

The exact stderr for the mismatch comes from `internal/run/verify.go:84-85` — `expected: ` then `actual:` followed by **three** spaces, so the two digests line up.

- [ ] **Step 2: Run it**

```sh
go test -run TestBinaryExitCodes -v .
```

Expected: PASS, both subtests.

- [ ] **Step 3: The red experiment — drop `os.Exit` on a throwaway copy**

A harness never seen failing proves nothing. Run this exactly; it never touches the working tree:

```sh
PROBE="$(mktemp -d)"
cp -R . "$PROBE/ldsum"
cat > "$PROBE/ldsum/main.go" <<'EOF'
package main

import "github.com/BobMali/ldsum/cmd"

func main() {
	cmd.Execute()
}
EOF
(cd "$PROBE/ldsum" && go test -run TestBinaryExitCodes -v .) ; echo "exit=$?"
rm -rf "$PROBE"
```

`os.Exit` and the `os` import both go, because dropping the call alone leaves `os` unused and the copy would not compile.

Expected: **FAIL**, with `exit = 0, want 1` and `exit = 0, want 2`. Paste the observed output into the task report. If it passes, stop — the harness is not observing what this plan claims, and that is a finding, not a step to work around.

- [ ] **Step 4: Confirm the working tree is untouched, then run the gates**

```sh
git status --short          # only main_test.go, modified
gofmt -l .
go vet ./...
go test ./...
golangci-lint run
```

- [ ] **Step 5: Commit**

```sh
git add main_test.go
git commit -m "test: pin the process exit codes"
```

---

### Task 3: Cover the real working directory and real argv

These three cases are the ones aimed at the assumption that let the original `selectTargets` defect through: every fixture in the existing suite spells its paths relatively, and every in-process test inherits the package directory as its cwd.

**Files:**
- Modify: `main_test.go` (append)

**Interfaces:**
- Consumes: `abcSHA256`, `run`, `writeIn` from Task 1.

- [ ] **Step 1: Write the failing test**

Append to `main_test.go`:

```go
func TestBinaryResolvesPathsAgainstTheRightDirectory(t *testing.T) {
	t.Run("a relative entry resolves against the checksum file", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "w")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeIn(t, dir, "payload.txt", "abc")
		writeIn(t, dir, "SUMS", abcSHA256+"  payload.txt\n")

		// The working directory is the parent, where payload.txt does not
		// exist: resolving the entry against the process's own directory
		// would fail outright. Only a real process can tell the two apart.
		stdout, stderr, code := run(t, parent, "verify", "-c", filepath.Join("w", "SUMS"))
		if code != 0 {
			t.Errorf("exit = %d, want 0\nstderr: %s", code, stderr)
		}
		if want := filepath.Join("w", "payload.txt") + ": OK\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("an absolute entry is used as it stands", func(t *testing.T) {
		dir := t.TempDir()
		payload := writeIn(t, dir, "payload.txt", "abc")
		sums := writeIn(t, dir, "SUMS", abcSHA256+"  "+payload+"\n")

		// Run from a directory with no relation to either file.
		stdout, stderr, code := run(t, t.TempDir(), "verify", "-c", sums)
		if code != 0 {
			t.Errorf("exit = %d, want 0\nstderr: %s", code, stderr)
		}
		if want := payload + ": OK\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("a filename containing a space", func(t *testing.T) {
		dir := t.TempDir()
		writeIn(t, dir, "with space.txt", "abc")

		// There is no shell between the harness and the binary; this pins
		// that argv arrives unsplit. A space is not escaped on output —
		// only a backslash or a newline is.
		stdout, stderr, code := run(t, dir, "sum", "with space.txt")
		if code != 0 {
			t.Errorf("exit = %d, want 0\nstderr: %s", code, stderr)
		}
		if want := abcSHA256 + "  with space.txt\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})
}
```

- [ ] **Step 2: Run it**

```sh
go test -run TestBinaryResolvesPathsAgainstTheRightDirectory -v .
```

Expected: PASS, all three subtests. All three outputs were observed from a real binary on 2026-09-02 and are recorded in the spec's Cases section.

- [ ] **Step 3: The red experiment — resolve against the working directory instead**

The relative-entry case claims to distinguish "resolved against the checksum file's directory" from "resolved against the process's cwd". Prove it by reintroducing the second behaviour on a throwaway copy. `resolve` in `internal/run/sums.go:97-102` is where the join happens:

```sh
PROBE="$(mktemp -d)"
cp -R . "$PROBE/ldsum"
perl -pi -e 's/^\treturn filepath\.Join\(base, p\)$/\treturn p/' "$PROBE/ldsum/internal/run/sums.go"
sed -n '/^func resolve/,/^}/p' "$PROBE/ldsum/internal/run/sums.go"   # both arms should now return p
(cd "$PROBE/ldsum" && go build ./... && go test -run TestBinaryResolvesPathsAgainstTheRightDirectory -v .) ; echo "exit=$?"
rm -rf "$PROBE"
```

`base` becomes an unused parameter, which Go permits, and `filepath` is still used elsewhere in the file, so the copy compiles.

Expected: the relative-entry subtest **FAILS**. The entry now resolves to `payload.txt` against the parent directory, where no such file exists, so the discriminating evidence is that stdout no longer contains `w/payload.txt: OK` and the exit status is no longer 0. The verdict line should read `payload.txt: FAILED open or read` (`internal/run/verify.go:66`, reached from `sums.go:73`) and the status should be 1, since a missing target maps there — but assert the failure, not that wording; record whatever you actually observe, including anything the run adds on stderr.

The absolute-entry subtest should still pass, since `resolve` returns an absolute path unchanged either way. That contrast is the evidence that the two cases are testing different things.

Paste the observed output into the task report. If the relative-entry subtest passes under this mutation, stop and report — the case is not observing what it claims.

- [ ] **Step 4: Run the gates**

```sh
gofmt -l .
go vet ./...
go test ./...
golangci-lint run
```

- [ ] **Step 5: Commit**

```sh
git add main_test.go
git commit -m "test: cover the real cwd and argv shapes"
```

---

### Task 4: Run the audits and correct the documentation

**Files:**
- Modify: `README.md` — the go-mutesting survivor table and the score paragraph beneath it; the Development section
- Modify: `CLAUDE.md` — the Layout paragraph and the Structure file tree

**Interfaces:**
- Consumes: a committed `main_test.go` from Tasks 1-3, so `git archive HEAD` picks it up.

- [ ] **Step 1: Run gremlins and record the numbers**

```sh
gremlins unleash ; echo "exit=$?"
```

Expected: exit 0. `main.go` holds no operators gremlins can rewrite, so it should contribute no mutants and neither efficacy nor mcover should move. Record both figures. If gremlins now reports TIMED OUT mutants it did not before, the cause is that every mutant invalidates the build cache for `main`, so each test run pays a fresh link — raise `timeout-coefficient` in `.gremlins.yaml` and say so in the report. **Do not lower a threshold.**

- [ ] **Step 2: Run the deeper audit on a throwaway copy**

```sh
PROBE=$(mktemp -d)
git archive HEAD | tar -x -C "$PROBE"
(cd "$PROBE" && go-mutesting --do-not-remove-tmp-folder ./... | tee audit.log)
grep -n 'main\.go' "$PROBE/audit.log"
```

Expect this to take longer than the four minutes the README quotes: every mutant now invalidates `main`'s build cache, so each per-mutant `go test ./...` pays a link for the harness build. Note the observed wall-clock time — the README's estimate needs updating if it has moved much.

Never run `go-mutesting` against the working tree: it mutates files in place and an interrupted run leaves the checkout mutated.

- [ ] **Step 3: Decide from the evidence, not from the plan**

- **If no `main.go` survivor appears:** the harness did its job. Continue to Step 4.
- **If a `main.go` survivor still appears:** stop and report which mutant it is and what the mutated file looks like (`diff -u main.go /tmp/go-mutesting-<n>/main.go.<k>`). Do not edit the README to say the row is gone, and do not weaken a test to make a number look better. A survivor that turns out to be equivalent is a finding to write up, not a row to delete quietly.

- [ ] **Step 4: Update `README.md`**

Remove this row from the survivor table:

```
| `main.go` — `os.Exit(cmd.Execute())` (1 mutant) | Nothing executes the built binary yet. Its own plan. |
```

Restate the score paragraph beneath the table from **this run's** numbers — the passed/failed/duplicated/total counts and the score. Do not do arithmetic on the old figures: the tool's mutant count is not stable between runs, which is why that paragraph already says so.

Add to the Development section, after the `go test ./...` block:

```markdown
The root `main_test.go` is the one test that leaves the process: it builds
`ldsum` into a temporary directory and executes it, so `go test ./...` needs a
working `go build` and pays one cached build. It is what covers `main.go`,
the real exit status and the real working directory — everything else stops
at the package boundary.
```

- [ ] **Step 5: Update `CLAUDE.md`**

In the Layout section, extend the sentence about `main.go` so it reads:

```markdown
`main.go` calls `os.Exit(cmd.Execute())` — `Execute` returns an `int` exit
code — and `main_test.go` beside it builds that binary and runs it as a real
process, which is the only place the exit status and the real working
directory are tested.
```

In the Structure code block, add the harness under `main.go`:

```
main.go              // only: cmd.Execute() and os.Exit
main_test.go         // builds the binary and execs it; the only process-level test
```

- [ ] **Step 6: Run the gates and commit**

```sh
gofmt -l .
go vet ./...
go test ./...
golangci-lint run
git add README.md CLAUDE.md
git commit -m "docs: drop the main.go row from the audit table"
```

---

## Verification

End to end, after Task 4:

```sh
gofmt -l .            # prints nothing
go vet ./...
go test ./...         # includes the harness; the build happens inside TestMain
golangci-lint run
gremlins unleash      # exits 0
```

The acceptance criterion is not any of those. It is that a `go-mutesting` run on a throwaway copy no longer lists a `main.go` survivor, and that the three experiments — the broken build in Task 1, the dropped `os.Exit` in Task 2, the cwd-relative `resolve` in Task 3 — were each observed failing rather than argued about. All three observations belong in the task reports.

Then, once, by hand: build the binary and drive it with inputs deliberately unlike any fixture — a path with unicode in it, `-c` pointing at a file in a directory you are not standing in, an argument list that starts with a flag. The harness closes the scripted gap; it does not replace typing paths the way a person does.

## Risks

- **The audit gets slower.** Every mutant invalidates `main`'s build cache, so each per-mutant test run now links the harness binary. Expect the go-mutesting run to exceed the four minutes the README quotes, and watch gremlins for new TIMED OUT mutants. The fix, if needed, is `timeout-coefficient`, never a threshold.
- **Inner-loop cost.** `.claude/hooks/go-check.sh` runs `go test ./...` after every Go edit, so the build lands in the edit loop — a few tenths of a second warm, a second or two cold.
- **Case 4 can rot silently.** It turns on `cmd.Dir` being somewhere other than the fixture directory. Task 3 Step 3 exists so that this is written down rather than rediscovered: if a later edit lets the directory default back, the case keeps passing while testing nothing.
