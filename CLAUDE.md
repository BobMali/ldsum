# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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
cobra-cli add <name>            # scaffold a new subcommand into cmd/
```

`cobra-cli` is installed at `~/go/bin/cobra-cli`. Note that files it generates are **not** gofmt-clean — run `gofmt -w` on them afterwards.

Module downloads (`go get`, `go mod tidy` on a new dep) fail under the Bash sandbox with a TLS certificate error; those specific commands need `dangerouslyDisableSandbox`. Ordinary build/test/vet work fine sandboxed.

## Layout

Standard Cobra shape and nothing more yet: `main.go` calls `cmd.Execute()`; `cmd/root.go` holds `rootCmd` and the `Execute` helper. There are no subcommands, no test files, and no non-Cobra packages so far — the checksum logic has not been written.

The module path is `github.com/BobMali/ldsum`.

`cmd/root.go` still carries two pieces of Cobra boilerplate that were deliberately left in place: the commented-out `Run:` stub and a dummy `--toggle` flag registered in `init()`. Remove them when real behaviour lands.

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
### Structure

```
main.go              // only: cmd.Execute() and os.Exit
cmd/                 // cobra command wiring, flag parsing
internal/hash/       // io.Reader -> digest; knows nothing about files
internal/checksums/  // parse and render checksum-file lines
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
- Errors are wrapped with context (`fmt.Errorf("read %s: %w", path, err)`)
  and never logged and returned at the same time.
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

If a linter is configured, `golangci-lint run` must be clean too.

Never mark work as done based on reasoning about the code. Run the
commands and report what they actually printed.


## Git

Identity is set repo-locally (`BobMali` / `BobMali@users.noreply.github.com`), not globally — don't rely on the global config.

The remote `origin` (`https://github.com/BobMali/ldsum.git`) still has `master` as its default branch, while local work is on `main`. `main` is a linear descendant of `master`, so it pushes without force; the default branch has yet to be switched on GitHub.

## Commit messages

Conventional Commits, enforced by `.githooks/commit-msg` and a PreToolUse
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

The body explains *why*, not *what* — the diff already shows what changed.

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
