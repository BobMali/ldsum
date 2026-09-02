# Binary harness for `main.go`

**Date:** 2026-09-02
**Status:** design approved in chat; spec awaiting review

## Why

Every test in the tree stops at the package boundary. `cmd/verify_test.go`'s
`runCLI` builds a fresh cobra tree, captures its streams and calls
`root.Execute()` in process — thorough about behaviour, blind to everything
the operating system contributes. Nothing has ever executed the built
binary, so four things are untested:

- `main.go` itself, all two lines of it;
- the process exit status, as opposed to the `int` `Execute` returns;
- real argv, arriving from the kernel rather than from `SetArgs`;
- the real working directory, and `os.Stdout` / `os.Stderr` as separate
  file descriptors rather than two `bytes.Buffer`s cobra was told to use.

`README.md`'s go-mutesting survivor table records the gap as a known
survivor: *`main.go` — `os.Exit(cmd.Execute())` (1 mutant) — Nothing
executes the built binary yet. Its own plan.* This is that plan.

The cost of the blind spot is not hypothetical. A Critical defect in
`selectTargets` survived thirty-odd tests, three mutation-testing reviews and
two advisor passes, and was found in thirty seconds by running the binary by
hand. It survived because every fixture in the suite spelled its paths
relatively — one shared assumption across thirty tests. What broke it was
typing a path the way a person does.

## Scope

Process-level only. The harness earns its keep exclusively on what an
in-process test structurally cannot observe. It does not re-test the CLI
surface: `cmd/` owns that, and duplicating those tables would buy a second
maintenance burden and no new information.

**Out of scope, deliberately:**

- A golden-file end-to-end suite covering every subcommand, flag and format.
- Coverage instrumentation. A subprocess contributes nothing to the parent
  test's profile, so `main.go` stays at 0% in `go test -cover`. Building with
  `-cover` and merging `GOCOVERDIR` profiles would move a number that nothing
  gates on, in exchange for version-sensitive build flags. The mutation audit
  is the evidence that this harness bites; a coverage percentage is not.
- Any new dependency. `rogpeppe/go-internal/testscript` is the obvious
  off-the-shelf answer and is rejected on the standard-library-plus-cobra
  rule. The harness is small enough that `os/exec` is not a hardship.
- Executing the `./ldsum` binary in the working tree. It is gitignored,
  untracked and possibly weeks old; a harness that picks it up silently tests
  the wrong bytes.

## Structure

One new file, `main_test.go`, at the repository root, in `package main_test`.
An external test package: the harness has no business reaching into `main`,
and `main` exports nothing to reach for.

```
main.go              // unchanged: cmd.Execute() and os.Exit
main_test.go         // NEW: builds the binary, execs it, asserts real exit status
```

It runs under a plain `go test ./...`, with no build tag and no `-short`
guard. Both alternatives were considered and rejected: this harness exists
because a whole layer was going unrun, so making it opt-in reproduces the
problem it is meant to solve. A build tag would additionally drop it out of
the definition of done in `CLAUDE.md` unless CI and
`.claude/hooks/go-check.sh` both learned to pass `-tags`.

### Building once

`TestMain` builds the binary before any case runs:

```go
var binary string

func TestMain(m *testing.M) {
    dir, err := os.MkdirTemp("", "ldsum-harness")
    // t.TempDir needs a *testing.T, which TestMain does not have, so the
    // directory is removed by hand below.
    if err != nil { ... os.Exit(1) }

    binary = filepath.Join(dir, "ldsum")
    build := exec.Command("go", "build", "-o", binary, ".")
    if out, err := build.CombinedOutput(); err != nil { ... os.Exit(1) }

    code := m.Run()
    os.RemoveAll(dir)
    os.Exit(code)
}
```

A build failure must report the compiler output and exit non-zero, never
leave the cases to fail one by one against a missing file. Key that decision
on the returned error alone: `go build` writes to stderr on success too — a
sandboxed run of it emits a module-cache warning and still exits 0 — so a
harness that treated any output as failure would refuse to start.
`os.RemoveAll` runs before `os.Exit`, because deferred functions do not run
through it.

### Running a case

A single helper wraps `os/exec`, so every case reads the same way:

```go
func run(t *testing.T, dir string, args ...string) (stdout, stderr string, code int)
```

- `dir` becomes `cmd.Dir`, which is how the harness controls the real working
  directory. Passing `""` inherits the test's own.
- stdout and stderr get separate `bytes.Buffer`s, so every case can assert
  that data went to one and diagnostics to the other.
- The status comes out of `*exec.ExitError` via `errors.As`, mirroring
  `exitCode` in `githooks/hooks_test.go` rather than inventing a second idiom
  for the same job. Anything that is not an `*exec.ExitError` is `t.Fatalf` —
  a binary that could not be started is a broken harness, not a failing case.

Fixtures are written under `t.TempDir()`. Digests are known-answer vectors
only: `"abc"` hashes to
`ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad`, the value
`cmd/` already uses. Nothing is asserted against a digest the implementation
just produced.

## Cases

Table-driven with `t.Run` per case. Expectations are inline, not golden
files: every line of output echoes an absolute temp path, so a golden file
would need templating and would buy nothing over a comparison against a
composed string.

Each case is present because it observes something in-process tests cannot.
The argv and output below were observed from a real run of a freshly built
binary on 2026-09-02; `<sha>` stands for the known-answer digest of `"abc"`,
`ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad`.

**1 — success carries status 0.** `ldsum sum payload.txt`, run in the fixture
directory. Status 0, stdout `<sha>  payload.txt`, stderr empty.

**2 — a mismatch carries status 1.** `ldsum verify payload.txt 0000…0000`
(sixty-four zeroes). Status 1, stdout `payload.txt: FAILED`, stderr the
`expected:` / `actual:` pair. One of the two cases that pin `main.go`.

**3 — a usage error carries status 2.** `ldsum verify --algo nonesuch
payload.txt <sha>`. Status 2, stdout empty, stderr `ldsum: verify
payload.txt: unknown algorithm "nonesuch": want sha256 or sha512`. The other
pinning case. Assert the leading `ldsum: ` and the substring `unknown
algorithm`, not the whole sentence — the wording belongs to `internal/run`,
whose own tests already own it, and duplicating it here buys a second place
to update.

**4 — the working directory is real.** Write `SUMS` beside the fixtures with
a *relative* entry — `ldsum sum payload.txt > SUMS`, run in the fixture
directory, yields `<sha>  payload.txt` — then run `ldsum verify -c w/SUMS`
with `cmd.Dir` set to the **parent** of the fixture directory. Status 0,
stdout `w/payload.txt: OK`: the entry resolved against the checksum file's
directory, and the verdict line carries the joined path rather than the bare
entry. The case is only meaningful because `payload.txt` does not exist in
the parent — resolution against the process's cwd would fail outright, which
is the shape of the historical `selectTargets` defect. A relative entry is
what makes this bite; an absolute one resolves identically from anywhere and
proves nothing here, which is case 5's job.

**5 — an absolute entry is used as it stands.** `ldsum sum
/abs/path/payload.txt > SUMS` writes `<sha>  /abs/path/payload.txt`;
`ldsum verify -c SUMS` from an unrelated directory then prints
`/abs/path/payload.txt: OK` and exits 0. Every existing fixture in the suite
spells its paths relatively, which is the shared assumption that let the
original defect through.

**6 — argv arrives as argv.** A fixture named `with space.txt`, summed by
name. Status 0, the path echoed intact on stdout. There is no shell between
the harness and the binary, and this pins that.

Cases 2 and 3 are the ones that pin `main.go`: with `os.Exit` removed the
process would report 0 and both would go red.

Stream separation is not a case of its own. Every case asserts both buffers,
so `os.Stdout` and `os.Stderr` being genuinely distinct descriptors is proven
six times over rather than once.

## Test-first, when the code already exists

`main.go` predates this harness, so red-green-refactor cannot start from an
absent feature. The equivalent step, and it is not optional:

> On a throwaway copy of the tree, rewrite `main.go` as `cmd.Execute()` with
> the `os.Exit` dropped, run the harness, and confirm cases 2 and 3 fail.
> Restore the copy. Record the observed output in the task report.

A harness that has never been seen failing proves nothing. This is an
experiment, not an argument: "it would fail" is not an acceptable report.

## Verification

The definition of done, unchanged:

```sh
gofmt -l .        # prints nothing
go vet ./...
go test ./...
golangci-lint run
gremlins unleash  # exits 0
```

Gremlins is expected to be unmoved. `os.Exit(cmd.Execute())` contains no
operators for it to rewrite, so `main.go` contributes no mutants and neither
efficacy nor mcover should shift. That is a prediction to confirm with a run,
not a claim to make in a commit message.

The acceptance criterion is the deeper audit. Following the throwaway-copy
recipe in `README.md`:

```sh
PROBE=$(mktemp -d)
git archive HEAD | tar -x -C "$PROBE"
(cd "$PROBE" && go-mutesting --do-not-remove-tmp-folder ./... | tee audit.log)
```

The harness is done when the `main.go` survivor no longer appears. The run
takes about four minutes, always exits 0 whatever the score, and mutates
files in place — throwaway copy only, never the working tree.

## Documentation follow-up

Once the survivor is gone:

- Remove the `main.go` row from the survivor table in `README.md` and
  re-measure the audit figures quoted beneath it. The table is a union across
  runs and the score is a rough indicator, so both need restating from the
  new run rather than arithmetic on the old one.
- Add a sentence to `README.md`'s Development section noting that the root
  test builds and executes the real binary, and that it therefore needs a
  working `go build` and adds a cached build to every `go test ./...`.
- Update the layout notes in `CLAUDE.md`: the file tree gains `main_test.go`,
  and the `main.go` line gains a clause saying the binary is exercised end to
  end from there.

## Commits

Two, in order:

1. `test: execute the built binary end to end` — `main_test.go` alone.
2. `docs: drop the main.go row from the audit table` — the README and
   CLAUDE.md edits, once a real audit run has confirmed the row is stale.

They are separate because the second depends on evidence the first produces,
and because a documentation edit that turns out to be premature should be
revertable without taking the harness with it.

## Risks

- **Inner-loop cost.** `.claude/hooks/go-check.sh` runs `go test ./...` after
  every Go edit, so the build lands in the edit loop. Cached it is a few
  tenths of a second; cold, a second or two. Accepted as the price of the
  harness always running.
- **CI without a build cache.** The first CI run pays a full build. The
  existing job already builds the module to test it, so the marginal cost is
  small.
- **Path assumptions in the harness itself.** Case 4 turns on `cmd.Dir`
  being somewhere other than the fixture directory. If a future edit lets it
  default back — to the fixture directory, or to the package directory — the
  case keeps passing while testing nothing. The brief should say so, so a
  reviewer checks it rather than reading a green run as proof.
