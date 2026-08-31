# Read expected checksums from a checksum file

Date: 2026-08-31
Status: designed

`verify` can only be told a checksum inline. This adds the second input axis
CLAUDE.md has always described — reading the expectation from a checksum file
such as a published `SHA256SUMS` — and with it the first parsing in
`internal/checksums`, the first multi-file run, and therefore the first
exit code that has to be aggregated.

## Scope

In:

- `--sums-file` on `verify`, with positional arguments filtering which
  entries are checked.
- Parsing the three line shapes `ldsum sum` writes — GNU text, GNU binary,
  BSD tagged — plus a file holding a bare digest and nothing else. The shape
  is recognised per line, never declared.
- Aggregating several files' verdicts into one exit code.

Out, deliberately:

- URL sources. Still the next feature after this one.
- `openssl dgst` output (`SHA256(f)= d`), md5, sha1.
- A `--strict` mode. Malformed lines warn; see *Malformed lines* below.
- Colour. A later task adds it; this design only constrains where output is
  written so that task stays small.

## Why a flag and not a subcommand

`verify`'s contract is *a file, an expected checksum, exit 0/1/2*. The
checksum file is the second way to supply the second half of that, not a
different question, and CLAUDE.md states the axis exactly that way: the
expected checksum is "given inline or read from a checksum file". A separate
`check` subcommand would duplicate the exit-code mapping and the flag
handling, and split one verb in two. `sha256sum` reaches the same conclusion:
`-c` is a flag, not a second binary.

The repo had already voted. `internal/checksums`' package comment, the
`internal/checksums` line in CLAUDE.md's layout, and `cmd/root.go`'s long
help all name `--sums-file`; `MissingTargetError` exists because "once a
checksum file can also be named, only one of them means exit 1".

## Command surface

```
ldsum verify <file> <checksum> [--algo sha256|sha512]   # unchanged
ldsum verify --sums-file <F> [<file>...]                # new
```

`--sums-file`, short `-c`, names the checksum file. Cobra validates
positional arguments after flags are parsed, so `Args` is a function that
branches: without `--sums-file` it is `cobra.ExactArgs(2)` exactly as today,
with it `cobra.ArbitraryArgs`.

`--algo` and `--sums-file` are mutually exclusive
(`cmd.MarkFlagsMutuallyExclusive`). In a checksum file the algorithm belongs
to each entry — the tag names it, or its digest length does — so `--algo`
could only contradict what the file says. Treating it as a constraint to
check entries against was considered and dropped: length inference is already
unambiguous, so the flag would assert nothing the parse does not.

### What the positional arguments mean

They filter, and what they filter against depends on what the file turned out
to hold. This is two different resolutions in one argument position, and it
is deliberate — each is the only sensible reading of its own file shape.

**A list file** — every entry has a path. Arguments name *entries*, matched
against the entry's path after unescaping, both sides through
`filepath.Clean` so `./dist.tar.gz` matches `dist.tar.gz`. An argument with
no matching entry is a wrong command: exit 2, and that includes an absolute
path when the file spells the entry relatively — arguments name entries as
the file writes them. With no arguments, every entry is checked.

Entries open relative to **the checksum file's directory**, not the working
directory. A published `SHA256SUMS` sits beside the files it describes, so
that is what its relative paths mean, and it lets the command run from
anywhere. This is the one place ldsum diverges from `sha256sum -c`, which
resolves against the working directory and expects you to `cd` first; the
README has to say so. The path printed in each verdict is the resolved one.

```
$ ldsum verify --sums-file ~/dl/SHA256SUMS
/home/me/dl/dist.tar.gz: OK
/home/me/dl/docs.pdf: OK

$ ldsum verify --sums-file ~/dl/SHA256SUMS dist.tar.gz
/home/me/dl/dist.tar.gz: OK
```

**A bare-digest file** — one digest, no path, as `dist.tar.gz.sha256` files
in the wild contain. Nothing in the file says which file it describes, so
exactly one argument is required, and it names a real file resolved from the
working directory like any other path argument. Zero arguments or two or more
is exit 2.

```
$ ldsum verify dist.tar.gz --sums-file dist.tar.gz.sha256
dist.tar.gz: OK

$ ldsum verify --sums-file dist.tar.gz.sha256
ldsum: dist.tar.gz.sha256: no paths in file; name the file to verify
```

Order of the verdicts follows the arguments when there are any, and the file
otherwise. A path named twice is verified twice; deduplicating would hide a
mistake rather than fix it.

## Parsing: internal/checksums

The package gains parsing and keeps its rule — readers and strings, never
paths.

```go
// Listing is what one checksum file contained.
type Listing struct {
	Entries []Entry
	Bad     []BadLine
}

// BadLine is a line Parse could not read as a checksum.
type BadLine struct {
	Line int
	Err  error
}

// Parse reads r as a checksum file. The returned error reports a failure to
// read r; a line that is not a checksum becomes a BadLine instead.
func Parse(r io.Reader) (Listing, error)
```

`Entry` gains `Line int` — the 1-based line it was parsed from, zero when the
entry was not parsed from a file. `Render` ignores it, so every existing
`Entry` literal still compiles.

`Parse` reports what it saw and judges nothing further: whether a pathless
entry is a usable bare-digest file, or which entries were asked for, is
`internal/run`'s decision.

### Recognising a line

Each line is handled alone, so one file may mix algorithms and shapes. A
trailing `\r` is stripped first — checksum files travel — then blank lines
and lines whose first non-space character is `#` are skipped without
comment.

A leading `\` is the coreutils marker for an escaped path. It is consumed,
and after the path has been split out, `\\` becomes `\` and `\n` becomes a
newline. A single left-to-right pass is required, not two sequential
replacements, or `\\n` would wrongly become a newline;
`strings.NewReplacer("\\\\", "\\", "\\n", "\n")` does exactly this pass.
The two escapes mirror what `Render` writes and nothing more, so
`Render` → `Parse` round-trips.

Then, in order:

1. **BSD tagged** — `^([A-Za-z0-9-]+) \((.+)\) = ([0-9A-Fa-f]+)$`. The
   algorithm is group 1 lowercased, checked with `hash.ParseDigestAs`, so an
   `MD5 (f) = …` line becomes a `BadLine` naming the unknown algorithm. The
   greedy path group is what lets a path containing `) = ` still parse.
2. **GNU text or binary** — a leading run of hex, then the remainder:
   - starts with two spaces → text, the path is the rest
   - starts with `" *"` → binary, the path is the rest
   - starts with one space → text, the path is the rest; lenient, because
     tools that emit a single separator space are common
   The two-space case is tested before the one-space case, so
   `<digest>  *odd` yields the path `*odd` rather than losing the asterisk.
   An empty path is a `BadLine`.
3. **Bare digest** — the whole line is hex. `Entry.Path` is `""`.
4. Anything else is a `BadLine`.

In cases 2 and 3 the algorithm comes from the digest's length through
`hash.ParseDigest`, which already reports an unusable length as an error;
that error becomes the `BadLine`'s.

### Malformed lines

Each `BadLine` is reported on stderr as `SHA256SUMS:7: not a checksum line`
and the run continues. They do not change the exit code on their own. That is
`sha256sum`'s default too — its `--strict` is opt-in, and we have no call for
one yet.

A file that yields no entries at all is a different thing: the command cannot
do what it was asked. That is exit 2, `SHA256SUMS: no checksum lines found`.

## Orchestration: internal/run

`Verify` keeps its exact signature and `VerifyOptions` keeps its exact
fields. A new `VerifySums` sits beside it and both call one unexported
helper:

```go
type SumsOptions struct {
	SumsFile string
	Paths    []string // empty means every entry in the file
}

func VerifySums(out, errOut io.Writer, opts SumsOptions) error

// verifyEntry hashes the file at path, compares it with expected, and prints
// the verdict.
func verifyEntry(out, errOut io.Writer, path string, expected hash.Digest) error
```

`verifyEntry` is today's `Verify` body from `os.Open` onward, unchanged.
`Verify` becomes parse-then-call.

Widening `VerifyOptions.Path` into a `Paths []string` and running both modes
through one entry point was the obvious alternative. It was rejected because
every case in `internal/run/verify_test.go` constructs `VerifyOptions{Path:
…}`, and widening the field would rewrite all of them — a green test rewritten
to accommodate a new feature is exactly the edit CLAUDE.md asks permission
for. Two entry points also read honestly: the modes differ only in where the
expectation comes from, and they share the part that is the same.

`VerifySums` opens the file, parses it, warns about every `BadLine`, decides
between bare and list mode, selects the entries, and verifies each in turn.

The mode is decided by the whole listing, not by any one line: it is **bare
mode when the listing holds exactly one entry and that entry's path is
empty**, and list mode otherwise. So a pathless digest among several entries
does not turn a `SHA256SUMS` into a bare-digest file. Such an entry is
reported on stderr like a malformed line — `SHA256SUMS:7: checksum without a
path` — then skipped, and like a malformed line it does not change the exit
code on its own. Its line number is what `Entry.Line` is for.
A checksum file that cannot be opened or read returns its error unwrapped —
`os` already produces an `*fs.PathError` carrying the operation and path —
and lands on exit 2. Only a missing *target* is exit 1, which is what
`MissingTargetError` has been waiting to distinguish.

## Exit codes

Several files means several verdicts and one status. `exit.go` already
anticipated this: *"Order is unobservable while both arms return 1. It stops
being so once a run can report several files at once: the worse code should
win then."*

```go
// VerifyErrors reports every file that failed in one run. Errs is never
// empty.
type VerifyErrors struct {
	Checked int
	Errs    []error
}

func (e *VerifyErrors) Error() string   // "2 of 5 files failed"
func (e *VerifyErrors) Unwrap() []error
```

`exitCode` tests for `*VerifyErrors` **first**, recurses into `Errs`, and
returns the highest code — so one unreadable file among several mismatches
still exits 2. A `*VerifyErrors` that somehow yielded 0 returns 2 instead: a
non-nil error must never report success.

When exactly one file was verified, `VerifySums` returns that file's error
bare rather than wrapping it, so single-target output and exit codes stay
byte-identical to a `verify <file> <checksum>` run.

### The trap in execute()

`errors.As` walks `Unwrap() []error`. A `*VerifyErrors` wrapping even one
`*MismatchError` therefore *matches* the `errors.As(err, &mismatch)` test in
`cmd.execute`, whose whole job is to suppress the message because `run`
already printed the detail. Left alone, the summary line would vanish in the
commonest failure of all. `execute` must test `*VerifyErrors` first and print
its summary; the suppression then applies only to a bare single-file
mismatch, as it does today.

## Output

Data on stdout, diagnostics on stderr, unchanged:

- `path: OK` and `path: FAILED` on stdout, one per file
- `expected:` / `actual:` on stderr under each `FAILED`
- `file:line: …` warnings for malformed lines, on stderr
- the summary, printed by `execute` from the `VerifyErrors` message

Colour is a later task, and it constrains this one in exactly one way: every
verdict, warning and summary must leave through a small, single set of write
calls — `verifyEntry` for the verdicts, one helper for the sums-file
warnings — so that task changes those and nothing else. No colour code, flag
or interface belongs in this change.

## Testing

`internal/checksums` — table-driven, one `t.Run` per case:

- each shape: GNU text, GNU binary, BSD tagged, bare digest
- a file mixing shapes, and one mixing sha256 with sha512
- escaped paths: a backslash, a newline, and `\\n` proving the single pass
- an explicit `Render` → `Parse` round-trip over the paths `Render` escapes
- CRLF line endings, blank lines, `#` comments
- `BadLine` cases: not a checksum at all, an unknown tagged algorithm, a
  digest of an unusable length, and a digest followed by a separator with no
  path after it
- the empty file

`internal/run` — real files under `t.TempDir()`, with a checksum file written
beside them:

- every entry verified with no arguments; arguments filtering to a subset
- a mismatch in the middle: the run keeps going and reports every file
- a missing target is a `*MissingTargetError`; a missing checksum file is not
- entries resolved against the checksum file's directory, proven by running
  with a working directory that is not it
- an argument naming no entry
- a pathless digest among several entries: warned about, skipped, and the
  run still exits 0 when every real entry matched
- bare digest: one argument works, zero and two do not
- a checksum file with no parseable lines

`cmd` — `exitCode` over a `*VerifyErrors`: mismatches alone give 1, one hard
error among them gives 2, and an empty one gives 2. `execute` prints the
summary for a `*VerifyErrors` wrapping a single mismatch, and still prints
nothing extra for a bare one.

## Documentation

`README.md`'s status note, `cmd/root.go`'s long help and `cmd/verify.go`'s
own, and the `internal/checksums` line in CLAUDE.md all say checksum-file
input is not implemented yet. They change with the code. The README also has
to state the base-directory divergence from `sha256sum -c` and the exit-code
table's new multi-file meaning.
