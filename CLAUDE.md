# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Contributor-facing setup — cloning, enabling the hooks, the build commands — lives in `README.md`; keep it there rather than duplicating it here.

## What ldsum is

A CLI that verifies a file against an expected checksum. Two input axes, both of which the design has to accommodate:

- **The file** comes from a local path *or* a URL.
- **The expected checksum** is given inline *or* read from a checksum file (e.g. a `SHA256SUMS` published alongside a release).

It prints whether the computed digest matches and exits non-zero when it does not, so it can be dropped into a script.

## Development workflow

**Test-first, always.**  
Use the Red-Green-Refactor cycle:

1. Write a test that fails for the right reason. Run it. See it fail.
2. Write the minimum code that makes it pass.
3. Refactor with the test green.
   Do not write implementation code before a failing test exists. If
   implementation code was written first, delete it and start over.

**DO NOT** write all tests and then all implementation code. That is not test-first. Write one test for one use case, then implement that use case. Then write the next test, and so on.

## Commands

```sh
go build ./...                  # build
go vet ./...                    # vet
gofmt -l .                      # list unformatted files (should print nothing)
go run . --help                 # run the CLI
go test ./...                   # all tests
go test ./cmd -run TestFoo      # a single test
golangci-lint run               # lint; config in .golangci.yml
gremlins unleash                # mutation testing; config in .gremlins.yaml
go-mutesting ./...              # deeper mutation audit, by hand, not in CI
cobra-cli add <name>            # scaffold a new subcommand into cmd/
```

`golangci-lint` and `gremlins` are not required locally — CI runs both on every
push and pull request, alongside the three gates below.

`gremlins unleash` takes about 40 seconds and exits 10 on an efficacy breach,
11 on an mcover breach. Its thresholds only work from `.gremlins.yaml`; the
`--threshold-*` flags are accepted and silently ignored. A mutant that
survives means a behaviour has no test holding it in place — see the testing
rules below for what to do about one.

`go-mutesting` is the deeper audit and is deliberately outside CI: it takes
about four minutes, always exits 0 however bad the score, and its survivors have
to be read individually because many are equivalent. It mutates files in
place, so don't run the command above straight against your checkout — see
`README.md` for the throwaway-copy recipe and how to triage the output. Run
it after substantial work, never as a gate. It catches what gremlins
structurally cannot — a deleted statement, a dropped error return, a removed
branch.

`cobra-cli` is installed at `~/go/bin/cobra-cli`. Note that files it generates are **not** gofmt-clean — run `gofmt -w` on them afterwards.

Anything that reaches the network fails under the Bash sandbox with a TLS certificate error (`CAfile: /etc/ssl/cert.pem`): module downloads (`go get`, `go mod tidy` on a new dep), `git push` / `git fetch`, and `gh`. Those need `dangerouslyDisableSandbox`. Ordinary build/test/vet work fine sandboxed.

## Layout

`main.go` calls `os.Exit(cmd.Execute())` — `Execute` returns an `int` exit
code — and `main_test.go` beside it builds that binary and runs it as a real
process, which is the only place the exit status and the working directory are
tested against a separately launched process. The `cmd/` directory holds
`root.go` (the base command), `verify.go` and `sum.go` (the subcommands),
`exit.go` (error-to-exit-code mapping), and test files. The `internal/hash/`
package computes digests from an `io.Reader` and parses checksum strings. The
`internal/checksums/` package renders and parses checksum-file lines. The
`internal/run/` package orchestrates each command and returns errors.

The module path is `github.com/BobMali/ldsum`.

## Rules

### Testing

- Table-driven tests, `t.Run` for each case.
- Fixtures live in `testdata/`. Use `t.TempDir()` for anything written.
- Use known-answer vectors for hashing (empty input, `"abc"`, a long
  generated input). Never assert against a digest the implementation just
  produced.
- No mocks for the standard library. Pass `io.Reader` / `fs.FS` instead.
- Test error paths: missing file, unreadable file, malformed checksum
  line, digest length mismatch, unknown algorithm.
- **Once a test exists, changing it needs explicit permission.** A red
  test means the implementation is wrong until the author says
  otherwise. Ask before editing, renaming, deleting, skipping or
  loosening an existing test — including relaxing an assertion to make
  it pass. Writing a *new* test file is free, and so is adding a new
  case to an existing one. `.claude/hooks/test-guard.sh` draws that
  line mechanically: an edit that only adds whole lines passes through,
  while rewriting, renaming, deleting or overwriting existing test text
  becomes a permission prompt.
- **A surviving mutant is a missing test.** Add the test that kills it.
  Lowering a threshold in `.gremlins.yaml`, excluding the file, or weakening
  an existing assertion are all the wrong move. Where a mutant is genuinely
  equivalent — the mutated code cannot behave differently — prefer rewriting
  the line so there is nothing to mutate (`worst = max(worst, code)` in
  `cmd/exit.go` is the worked example) over recording an exception.
### Structure

```
main.go              // only: cmd.Execute() and os.Exit
main_test.go         // builds the binary and execs it; the only process-level test
cmd/                 // cobra command wiring, flag parsing
internal/hash/       // io.Reader -> digest; knows nothing about files
internal/checksums/  // render and parse checksum-file lines
internal/run/        // orchestration; returns errors, never exits
testdata/
```

- `os.Exit` appears only in `main.go`. Everything else returns an `error`.
- Cobra commands must not do real work. They parse flags and call
  `internal/run`.
- Commands write through `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, never
  to `os.Stdout` directly, so output is testable.
- `internal/hash` and `internal/checksums` take readers and writers, not
  paths.
### Behaviour

- Stream file contents through the hasher with a fixed-size buffer. Never
  read a whole file into memory.
- Data goes to stdout, diagnostics go to stderr.
- Exit codes: `0` success, `1` checksum mismatch or missing target file
  during verify, `2` usage or I/O error.
- On `verify`, keep going after a mismatch and report every file, then
  exit non-zero. Don't stop at the first failure.
- Errors are wrapped with context that they do not already carry — for example,
  `fmt.Errorf("read %s: %w", path, err)` is right when the error has no path.
  But `os` and `io` operations return `*fs.PathError` which already carries the
  operation and path, so a second copy must not be added. Errors are never
  logged and returned at the same time.
### Comments

Comment only what the code cannot say: why a choice was made, or what an
otherwise baffling construct is for. One or two lines. Don't narrate what
the next line does.
### Dependencies

- Standard library plus `spf13/cobra` only.
- Any new module is a decision, not an implementation detail. Ask first.
- Hash algorithms come from `crypto/*`. No third-party hashing.
## Definition of done

All of these pass before a change is considered complete:

```sh
gofmt -l .        # prints nothing
go vet ./...
go test ./...
```

`golangci-lint run` must be clean too. It is configured (`.golangci.yml`:
the default linters, with errcheck excluded on writer and close calls) and
runs in CI, so a push is checked even if you do not run it locally.

`gremlins unleash` must exit 0 as well. It runs in CI on the same terms; a
change that touches `internal/` or `cmd/` is worth running it against locally
first.

Never mark work as done based on reasoning about the code. Run the
commands and report what they actually printed.


## Git

Identity is set repo-locally (`BobMali` / `BobMali@users.noreply.github.com`), not globally — don't rely on the global config.

The remote `origin` (`https://github.com/BobMali/ldsum.git`) has `main` as its default branch, and local `main` tracks `origin/main`. The old `master` branch has been deleted; its history is contained in `main`.

## Commit messages

Conventional Commits, enforced by `githooks/commit-msg` and a PreToolUse
hook. A rejected commit is a message problem, never a reason to reach for
`--no-verify`.

```
<type>(<scope>): <description>

<body: why the change was made>
```

- **types:** `feat` `fix` `docs` `test` `refactor` `perf` `build` `ci`
  `chore` `revert`
- **scopes** (optional): `hash` `checksums` `cmd` `run` `ci` `deps`
- **description:** lowercase, imperative, no trailing period, <= 66 chars
- **breaking change:** `!` before the colon, plus a `BREAKING CHANGE:`
  footer explaining the migration

One logical change per commit. A test and the code it drives belong in the
same commit; unrelated formatting does not.

One line by default. Before adding a body, apply the test: **would a reader
who has the diff open still be puzzled?** If not, delete it. A body that
restates the subject, enumerates what changed, or narrates the work is noise
— whoever reads the message has the diff.

Add a body only when the reason lives *outside* the diff: a constraint a
planned feature imposes, why an obvious alternative was rejected, how a
subtle bug was found.

```
# earns a body
feat(run): report a missing target as its own error type

exitCode classified exit 1 by sniffing fs.ErrNotExist, which will misread a
missing checksum file as a missing target once --sums-file exists.

# does not — the diff says all of this
fix: address the linter findings

errors.New for a format-free message, concatenation over Sprintf, a doc
comment on the algorithm constants...
```

```
# good
feat(cmd): add verify subcommand
fix(checksums): handle lines with trailing whitespace
test(hash): cover empty input vector
refactor(run): return errors instead of exiting

# rejected
fix: fix bug                 # says nothing
feat: Added verify command.  # past tense, capitalised, trailing period
update code                  # no type
chore: misc                  # bundles unrelated changes
```
