# ldsum

Verify a file against an expected checksum.

The file comes from a local path or a URL, and the expected checksum is either
given inline or read from a checksum file — a `SHA256SUMS` published alongside
a release, say. `ldsum` prints whether the computed digest matches and exits
non-zero when it does not, so it drops into a script.

> **Status:** `sum` computes checksums for local files and directories.
> `verify` works with local files, given a checksum inline or read from a
> checksum file. URL input is not yet implemented.

## Usage

### Compute a checksum

```sh
ldsum sum dist.tar.gz
ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad  dist.tar.gz
```

The default is the GNU coreutils text format, so the output is a `SHA256SUMS`
line. Four formats are available, and they are mutually exclusive:

| Flag | Output |
|------|--------|
| `--text`, `-t` | `<digest>  <path>` — the default |
| `--binary`, `-b` | `<digest> *<path>` |
| `--tag` | `SHA256 (<path>) = <digest>` |
| `--bare` | `<digest>` alone, for `d=$(ldsum sum --bare f)` |

A path containing a backslash or a newline is escaped the way coreutils does
it: the line gains a leading `\`, a backslash becomes `\\`, and a newline
becomes `\n`. Tagged format has no such convention, so such a path is an error
there.

`--algo` picks the algorithm, defaulting to sha256:

```sh
ldsum sum --algo sha512 dist.tar.gz
```

Directories are walked only with `-r`, which is deliberate — hashing a tree is
too large an action to start because an argument happened to be a directory:

```sh
ldsum sum -r ./dist > SHA256SUMS
```

`-o` writes the lines to a file instead, truncating whatever is there:

```sh
ldsum sum -r ./dist -o SHA256SUMS
```

It behaves like the redirection above, which includes the one hazard: an output
file inside the tree being walked exists by the time the walk reaches it, and is
summed into its own listing. Write it outside the tree, or name the paths.

A symlink named as an argument is followed — naming it is how you ask for it.
Symlinks found by walking are not, so no file is summed twice and no cycle can
occur. Skipped entries are silent unless `-v` names them on stderr.

Verify a file against a checksum:

```sh
ldsum verify dist.tar.gz ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
dist.tar.gz: OK
```

When the digests differ, the expected and actual values go to stderr:

```sh
ldsum verify dist.tar.gz 0000000000000000000000000000000000000000000000000000000000000000
dist.tar.gz: FAILED
expected: 0000000000000000000000000000000000000000000000000000000000000000
actual:   ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
```

The algorithm comes from the length of the checksum — 64 hex characters is
sha256, 128 is sha512. Name it explicitly with `--algo` when you want the
length checked too:

```sh
ldsum verify --algo sha256 dist.tar.gz ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
```

### Verify against a checksum file

`--sums-file`, or `-c`, reads the expected checksums from a file:

```sh
ldsum verify -c SHA256SUMS
dist.tar.gz: OK
docs.pdf: OK
```

The format is recognised per line, so a file may mix them: the GNU text
(`<digest>  <path>`) and binary (`<digest> *<path>`) formats, the BSD tagged
format (`SHA256 (<path>) = <digest>`), and a file holding nothing but a
digest. Escaped paths are unescaped, so anything `ldsum sum` writes can be
read straight back.

Entries resolve relative to **the checksum file's directory**, not the working
directory — a published `SHA256SUMS` sits beside the files it describes, so
that is what its paths mean:

```sh
ldsum verify -c ~/downloads/SHA256SUMS      # no need to cd first
```

An entry spelled as an absolute path is used as it stands, since it already
says where its file is, so a listing `ldsum sum` wrote from absolute paths
reads back wherever it is kept:

```sh
ldsum sum /srv/dist/app.tar.gz > SUMS
ldsum verify -c SUMS
```

This is the main place `ldsum` differs from `sha256sum -c`, which resolves
against the working directory.

Naming files checks only those entries, in the order given:

```sh
ldsum verify -c SHA256SUMS dist.tar.gz
```

A file that holds a bare digest names nothing, so name the file yourself:

```sh
ldsum verify -c dist.tar.gz.sha256 dist.tar.gz
```

A symlink listed in a checksum file is followed, the same as one named on the
command line. The reason differs, though: `ldsum sum` follows a named symlink
because naming it is how you asked for it, and under `-c` it is the file doing
the naming rather than you.

A mismatch does not stop the run: every file is reported and the summary goes
to stderr.

```sh
ldsum verify -c SHA256SUMS
dist.tar.gz: OK
docs.pdf: FAILED
expected: 0000000000000000000000000000000000000000000000000000000000000000
actual:   ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
ldsum: 1 of 2 files failed
```

Lines that are not checksums are named on stderr and skipped; a file with no
usable lines is an error. `--algo` and `--sums-file` cannot be combined —
the file says which algorithm each entry uses.

## Exit codes

Both subcommands exit so they drop straight into a script:

| Code | Meaning |
|------|---------|
| 0 | every digest matched, or every file was summed |
| 1 | a digest did not match, or a file being verified is missing |
| 2 | the command could not be carried out: wrong argument count, bad checksum, unknown algorithm, an unreadable file, an unusable checksum file, an output file that cannot be written, a directory without `-r` |

## Requirements

- Go 1.27 or newer
- `git` and `bash` — the hook tests shell out to both

## Setup

```sh
git clone https://github.com/BobMali/ldsum.git
cd ldsum
git config core.hooksPath githooks
go build ./...
go test ./...
```

The `core.hooksPath` line is the one step that is easy to miss. It is
repository-local config, so a clone does not carry it, and without it the
`commit-msg` hook silently never runs.

## Commit messages

Commits follow [Conventional Commits](https://www.conventionalcommits.org),
enforced by `githooks/commit-msg`:

```
<type>(<scope>): <description>
```

Types are `feat` `fix` `docs` `test` `refactor` `perf` `build` `ci` `chore`
`revert`; the optional scopes are `hash` `checksums` `cmd` `run` `ci` `deps`.
The description is lowercase and imperative with no trailing period. The
pattern itself lives in `githooks/conventional-regex.txt`.

A rejected commit is a message problem — fix the message rather than skipping
the hook.

## Development

```sh
go build ./...
go vet ./...
gofmt -l .          # prints nothing when clean
go test ./...
```

All four pass before a change is done.

The root `main_test.go` is the one test that runs `ldsum` itself out of
process: it builds the binary into a temporary directory and executes it, so
`go test ./...` needs a working `go build` and pays one cached build. It is
what covers `main.go`, the process exit status, and a working directory chosen
per case. The `cmd/` tests drive the same command tree in process, which is
why none of those three are visible to them.

### Mutation testing

CI also runs [gremlins](https://github.com/go-gremlins/gremlins), which edits
the source in small ways and fails if the tests still pass. A surviving mutant
means some behaviour has no test holding it in place.

```sh
go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
gremlins unleash    # about 48 seconds; exit 10 (efficacy) or 11 (mcover) means a threshold was breached
```

Settings and the thresholds live in `.gremlins.yaml`. Run it locally before a
push that touches `internal/` or `cmd/`, or let CI find it.

A survivor is a missing test, not a reason to change an existing one. If you
believe a mutant is genuinely equivalent — the mutated code cannot behave
differently, whatever the test — say so on the pull request rather than lowering
the threshold. Rewriting the line so there is nothing left to mutate is usually
better than either.

### The deeper audit

Gremlins only rewrites operators, so a whole class of weak test survives it: it
has no way to delete a statement, drop an error return or remove a branch.
[go-mutesting](https://github.com/avito-tech/go-mutesting) does all three, and
on a green gremlins run it still found a dozen real gaps.

It is not in CI and should not be. It takes several minutes, always exits 0
whatever the score, and its survivors need reading one by one — many are
equivalent mutants that no test could ever kill.

`go-mutesting` mutates the files under test in place, restoring each one as it
goes rather than working from a copy of its own. Point it at your working tree
and an interrupted run leaves your checkout mutated, so run it on a throwaway
copy instead, and not concurrently with anything else that touches that copy.
Commit or stash first: `git archive HEAD` archives the last commit, not
uncommitted changes, so a dirty working tree gets audited against the last
commit rather than your changes.

```sh
go install github.com/avito-tech/go-mutesting/cmd/go-mutesting@latest
PROBE=$(mktemp -d)
git archive HEAD | tar -x -C "$PROBE"
(cd "$PROBE" && go-mutesting --do-not-remove-tmp-folder ./... | tee audit.log)
```

Each `FAIL` line is a mutant that lived. `--do-not-remove-tmp-folder` keeps the
mutated copy so you can see what changed:

```sh
diff -u "$PROBE/internal/hash/hash.go" /tmp/go-mutesting-<n>/internal/hash/hash.go.5
```

Run it when you have changed the code substantially, and treat the output as
a reading list rather than a score. There is deliberately no blacklist file:
`--blacklist` matches the MD5 of the *mutated file*, so every entry silently
stops matching the moment anything else in that file is edited, and a stale
blacklist hides real survivors.

Some survivors are permanent. A mutant is equivalent when the change cannot
alter behaviour, and unreachable when no test can drive the branch it sits in.
Both are expected; a survivor *not* on this list is worth investigating.

The table is the union of survivors observed across runs, not one run's
output: go-mutesting's mutant count is not stable between runs on identical
source — two runs against the same commit reported 284 and 288 total
mutants, both killing exactly 277, because the tool's own deduplication
varies from run to run. Any single run shows a subset of the rows below.

| Where | Why it survives |
|---|---|
| `internal/hash/hash.go` — `copyBufSize` arithmetic (4 mutants) | The buffer size cannot change the digest. Equivalent. |
| `internal/run/sum.go` — the `Flush` and `Close` error returns (2 mutants) | No portable way to make either fail on a regular file. `/dev/full` is Linux-only. |
| `internal/run/sum.go` — the `if err != nil` block after `WalkDir` returns (4 mutants) | Unreachable by construction: the callback returns `nil` for everything. The source comment says so and keeps it anyway. |
| `cmd/exit.go` — the `if worst == 0` guard in `exitCode` (2 mutants) | Unreachable by construction: `VerifyErrors.Errs` is never empty and every member maps to exit 1 or 2, so `worst` is never 0. The guard is deliberate defensive code; kept anyway. |

One run after the binary harness landed on 2026-09-02 scored 97% (0.965278,
278 passed, 10 failed, 11 duplicated, 288 total) — a run's numbers, not the
numbers, given the count above. A `FAIL` outside the table above is worth
investigating; the score itself is a rough indicator, not a tripwire.
