# Sum: print checksums for files and trees

Date: 2026-08-30
Status: implemented

The second command. `verify` answers "does this file match a checksum I was
given"; `sum` produces the checksum in the first place, in the formats other
tools already read.

It also creates `internal/checksums`, which the layout in `CLAUDE.md` has
reserved from the start. That matters more than the command itself: the line
shapes this change renders are the line shapes `verify --sums-file` will later
have to parse. Render and parse are one format decision, and this is where it
gets made.

## Scope

In:

- A `sum` subcommand that prints the digest of one or more local files.
- GNU coreutils text and binary formats, BSD/tagged format, and a bare
  digest with no path.
- Recursive directory walking behind an explicit `-r`.
- `internal/checksums` with a `Format`, an `Entry`, and `Render`.

Out, deliberately:

- `Parse`. It belongs in the same file as `Render` and reads the same
  `Entry` back, but nothing consumes it until `--sums-file` exists.
- URL sources. `run` takes paths today, here as in `verify`.
- Reading from stdin as `-`.
- Writing to a file with `-o`. Shell redirection covers it. (Reversed on
  2026-08-31 by request: `sum` now takes `-o`, with truncating semantics
  matching the redirection it replaces.)
- New hash algorithms. `--algo` exposes the sha256 and sha512 that
  `internal/hash` already has. Adding to that family is its own change,
  and is easier to justify once a command exists to exercise it.

## Command surface

```
ldsum sum [--algo sha256|sha512] [--text|--binary|--tag|--bare] [-r] [-v] <path>...
```

At least one positional path (`cobra.MinimumNArgs(1)`).

`--algo` defaults to `sha256`. This differs from `verify`, which infers the
algorithm from the length of the checksum it was given — here there is no
expected checksum to infer from, so a default is the only option.

The four format flags are mutually exclusive (`MarkFlagsMutuallyExclusive`).
`--text` is the default, matching `sha256sum` with no arguments.

`-r`/`--recursive` is required before a directory argument is walked. Without
it, a directory argument is a per-file failure (see **Failure rule**) whose
message names the flag. Hashing a tree is too large an action to start
because an argument happened to be a directory.

`-v`/`--verbose` is local to `sum` and does one thing: report skipped
entries. See **Walk**.

## Formats

Captured from real tools on 2026-08-30 — `shasum` 6.02 (perl) and Darwin's
`/sbin/sha256sum` — not from memory. `·` marks a literal space.

| Flag | Line |
|------|------|
| `--text` (default) | `<hex>··<path>` |
| `--binary` | `<hex>·*<path>` |
| `--tag` | `SHA256·(<path>)·=·<hex>` |
| `--bare` | `<hex>` |

The tag prefix is the algorithm name uppercased: `SHA256` or `SHA512`.

`--bare` prints no path. With several files that loses which digest belongs
to which file, which is the caller's business — the flag exists so
`d=$(ldsum sum f)` needs no `cut`.

`Render` writes exactly one line, including its trailing newline.

## Escaping

A path holding a backslash or a newline cannot go into a text or binary line
as itself: the line stops being parseable. The coreutils convention, verified
against `shasum`, is to mark such a line with a leading backslash and escape
inside the path — a backslash becomes two, a newline becomes backslash-`n`:

```
ba7816bf8f01…  dist/plain.txt
\9f86d0818847…  dist/we\\ird.txt
```

Ordinary lines carry no leading marker. The escape set is exactly those two
characters and nothing else; Darwin's `/sbin/sha256sum` escapes nothing at
all, and we follow the GNU behaviour because a manifest we emit should be
readable by `sha256sum -c`.

Tagged format has no escape convention upstream, so under `--tag` a path that
would need escaping is a per-file failure rather than a line we invent a
syntax for. `--bare` prints no path, so the question does not arise.

## Walk

`filepath.WalkDir`, which reads directory entries in lexical order, so output
is deterministic with no sorting of our own. Arguments are processed in the
order given; within a directory, lexical order. The golden-output tests
depend on that sentence.

Only regular files are hashed. A symlink *named as an argument* is followed:
naming a path is how the caller asks for it, so `sumPath` uses `Stat` rather
than `Lstat`, and `walkDir` passes its root with a trailing separator so that a
symlinked directory resolves rather than vanishing. Symlinks *found by walking*
are not followed — `d.Type()` does not resolve them — so there are still no
cycles and no path appears twice. Every other kind of entry (fifo, socket,
device) is skipped the same way as a walked symlink.

That distinction was missed when this section was first written: it said
symlinks were never followed at all, which made `sum -r` on a symlinked
directory a silent no-op that exited 0. Corrected after the behaviour was
found by probe during implementation.

A skip is silent by default and does not affect the exit code. Under
`--verbose`, each skipped entry is named on stderr, which is where
diagnostics go. Skipping stays a non-event either way; `--verbose` only makes
it visible.

Paths print as `filepath.Join` produces them, so `ldsum sum -r ./dist` emits
`dist/a.txt`, not `./dist/a.txt`.

## Failure rule

One rule covers every per-file failure: report it on stderr, keep going, and
exit 2 at the end. It is the house rule `verify` already follows — report
every file, do not stop at the first problem — and it applies to all of:

- a named argument that does not exist
- a directory argument without `-r`
- a file that cannot be opened, or that vanishes mid-walk
- a path that would need escaping, under `--tag`

Digests for every file that could be read still go to stdout. Nothing here
means exit 1: the existing table scopes `1` to a mismatch or a missing target
*during verify*, and `sum` has no mismatch to report. `2` already reads as
"the command could not be carried out: … unreadable file", so `cmd/exit.go`
needs no new case — a plain error falls through to its default arm.

## Packages

### internal/checksums

New. Knows line shapes. Knows nothing about files or walking.

```go
type Format int   // Text, Binary, Tag, Bare

type Entry struct {
    Digest hash.Digest   // carries both Algorithm and Hex
    Path   string
}

func Render(w io.Writer, e Entry, f Format) error
```

Escaping helpers stay unexported. `Parse` lands here later, beside `Render`,
reading the same `Entry` back.

The reason this is a package with a type rather than a `fmt.Fprintf` inside
`run`: the format has a second consumer that does not exist yet. If `sum`
formats strings inline, the only definition of the format is a printf verb
inside an orchestration function, and the parser arrives with nothing to
agree with. Correcting the shape at that point would mean editing tests that
already passed, which this repo gates behind the author's permission.

### internal/run

New file, `sum.go`, beside `verify.go`.

```go
type SumOptions struct {
    Paths     []string
    Algorithm string
    Format    checksums.Format
    Recursive bool
    Verbose   bool
}

func Sum(out, errOut io.Writer, opts SumOptions) error
```

Walks, opens, streams each file through the existing `hash.Sum`, and renders
each entry as it is produced. Nothing accumulates: a tree of ten thousand
files holds one buffer, not ten thousand lines. Failures are counted; the
returned error reports the count and maps to exit 2.

### internal/hash

Unchanged. `Sum` and `Digest` are already the right shape.

### cmd

New file, `cmd/sum.go`, built like `cmd/verify.go`: flags bound to locals so
two trees in one process never share them, `SilenceUsage` set once arguments
have parsed, and a single call into `run.Sum`. No work of its own.

## Testing

`internal/checksums` — table-driven over the four formats, asserting exact
bytes against lines captured from `shasum` and `sha256sum`, never against a
digest this code produced. Escaping cases: a backslash, a newline, both, and
a plain path that must gain no marker. Tag with a path needing escapes
returns an error.

`internal/run` — a tree built in `t.TempDir()`. Cases: several files in
lexical order; several arguments in the order given; a directory without
`-r`; a file that cannot be opened, asserting the walk continues and the
other digests still print; an empty directory with `-r`, which prints nothing
and exits 0; a symlink skipped silently, and named on stderr under
`--verbose`.

An unreadable file is made with `chmod 000`, which does nothing when the
tests run as root — that case skips itself when `os.Geteuid() == 0`.

`cmd` — the format flags reject each other; `--algo` defaults to sha256; the
exit code is 2 for each failure in the rule above and 0 for a clean run.

Digest vectors come from the published values already in
`internal/hash/hash_test.go` rather than being restated here.

## Documentation

Both go stale the moment this lands, so both are part of the change:

- `README.md` — the status note says only `verify` works, and there is no
  `sum` usage section or mention of the formats.
- `cmd/root.go` — `Short` and `Long` describe ldsum as a verification tool.
  It now does two things.

## A note for `verify --sums-file`

`run.Sum` builds its aggregate error with `fmt.Errorf` and no `%w`, so nothing
is unwrapped from it. That is right here: a count of failures is genuinely a new
error, and `sum` has no exit-1 case for `errors.As` to find.

It becomes wrong the moment `verify --sums-file` aggregates several files. If it
copies this shape, an aggregate over a set of mismatches will not satisfy
`errors.As(err, &MismatchError{})`, and a real mismatch will exit 2 instead of
the documented 1. Aggregate there must preserve the worst error, not replace it.
