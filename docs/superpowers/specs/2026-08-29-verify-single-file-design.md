# Verify a single file against an inline checksum

Date: 2026-08-29
Status: approved, not yet implemented

The first real feature in ldsum. It fixes the shape of `internal/hash` and
`internal/run`, so later features — URLs, checksum files, multiple targets —
have to fit the interfaces this one establishes.

## Scope

In:

- A `verify` subcommand that checks one local file against one expected
  digest given on the command line.
- sha256 and sha512.
- Exit codes that distinguish a mismatch from a usage error.

Out, deliberately:

- URL sources. `run` takes a path today; a source resolver goes behind the
  same call when URLs land.
- `internal/checksums`. Nothing parses checksum-file lines until
  `--sums-file` exists.
- Multiple targets, and any `ldsum sum` that just prints a digest.

Removed as part of this change: the commented-out `Run:` stub and the dummy
`--toggle` flag in `cmd/root.go`, which were placeholders until real
behaviour landed.

## Command surface

```
ldsum verify <file> <expected-hex> [--algo sha256|sha512]
```

Exactly two positional arguments. Without `--algo`, the algorithm comes from
the length of the expected hex: 64 characters is sha256, 128 is sha512. Any
other length is a usage error rather than a guess. `--algo` names the
algorithm explicitly, and then a contradicting length is an error too.

```
$ ldsum verify dist.tar.gz 9f86d081884c7d65...
dist.tar.gz: OK

$ ldsum verify dist.tar.gz deadbeef...
dist.tar.gz: FAILED
expected: deadbeef...
actual:   9f86d081...
```

`cmd/verify.go` holds the command. It reads flags and arguments and calls
`run.Verify`. It does no work of its own.

## Packages

### internal/hash

Knows digests. Knows nothing about files.

```go
type Algorithm string          // "sha256", "sha512"

type Digest struct {
    Algorithm Algorithm
    Hex       string           // always lowercase
}

func ParseDigest(s string) (Digest, error)                 // infers from length
func ParseDigestAs(s string, a Algorithm) (Digest, error)  // --algo given
func Sum(r io.Reader, a Algorithm) (Digest, error)
func (d Digest) Equal(o Digest) bool
```

`Sum` copies through a fixed 32 KiB buffer, so memory stays flat no matter
how large the input is. Nothing reads a whole file.

`Equal` is a plain string comparison over normalised lowercase hex. The
expected digest is public information, not a secret, so there is nothing for
a timing attack to learn and no reason for a constant-time compare.

Parsing normalises: surrounding whitespace is trimmed, hex is lowercased,
non-hex characters are rejected, and the length must match the algorithm.

### internal/run

Orchestration. Returns errors; never exits.

```go
type VerifyOptions struct {
    Path      string
    Expected  string
    Algorithm string   // empty means infer from length
}

func Verify(out, errOut io.Writer, opts VerifyOptions) error
```

`Verify` parses the expected digest, opens the path, streams it through
`hash.Sum`, compares, writes the verdict, and returns.

`run` owns the printing rather than `cmd` because the multi-file feature has
to keep going after a mismatch and report every file. That loop belongs next
to the logic that produces each verdict, not in the Cobra layer.

## Data flow

```
cmd/verify.go ── VerifyOptions ──▶ run.Verify ──▶ hash.ParseDigest
                                       │
                                  os.Open(path)
                                       │
                                       ▼
                                   hash.Sum ──▶ Digest.Equal ──▶ verdict
```

## Output

Match: `<file>: OK` on stdout, nil error.

Mismatch: `<file>: FAILED` on stdout; `expected: …` and `actual: …` on
stderr; returns `*run.MismatchError`.

The verdict is the data, so it goes to stdout; the detail explaining a
failure is a diagnostic, so it goes to stderr.

## Errors and exit codes

`run` returns typed errors. `cmd.Execute` maps them, and is the only place
that knows about exit codes.

| Condition                                          | Error                        | Code |
|----------------------------------------------------|------------------------------|------|
| digests match                                       | `nil`                        | 0    |
| digest mismatch                                     | `*run.MismatchError`         | 1    |
| target file missing                                 | wraps `fs.ErrNotExist`       | 1    |
| bad hex, bad length, unknown `--algo`                | wraps `run.ErrUsage`         | 2    |
| wrong argument count                                | Cobra's `ExactArgs(2)`       | 2    |
| unreadable file, read failure, path is a directory  | anything else                | 2    |

`Execute` changes signature to `func Execute() int`, and `main.go` becomes
`os.Exit(cmd.Execute())`. `os.Exit` still appears nowhere but `main.go`.

The verify command sets `SilenceErrors` and `SilenceUsage` so Cobra does not
print a second, uglier copy of what `run` already reported. `Execute` prints
`ldsum: <err>` to stderr for every error except a mismatch, whose detail is
already out.

Errors are wrapped with context — `fmt.Errorf("read %s: %w", path, err)` —
and never logged and returned at the same time.

## Testing

Red-green-refactor, one test at a time. Each numbered step is a single
failing test followed by the code that makes it pass.

1. `hash.Sum` for sha256 against published FIPS 180-4 vectors: empty input,
   `"abc"`, and 1,000,000 repetitions of `'a'`.
2. The same for sha512.
3. `hash.ParseDigest`: the inference table, plus non-hex characters, odd
   length, 40-character (sha1-length) input, and the empty string.
4. `hash.ParseDigestAs`: a length that contradicts the named algorithm, and
   an unknown algorithm name.
5. `run.Verify` on a match — `OK` line, nil error. Writers are
   `bytes.Buffer`; the file comes from `t.TempDir()`.
6. `run.Verify` on a mismatch — `FAILED`, the expected/actual detail, and a
   `*MismatchError`.
7. `run.Verify` error paths: missing file, path is a directory, unreadable
   file (chmod 000; skipped when the test runs as root).
8. `cmd` end to end through `rootCmd.SetOut` / `SetErr` / `SetArgs`:
   success, mismatch, wrong argument count.
9. The error-to-exit-code mapping.

Table-driven throughout, `t.Run` per case. Anything written goes in
`t.TempDir()`.

Digest vectors are copied from the standard. A digest the implementation
itself produced is never used as an expected value — that asserts only that
the code agrees with itself.

Once a test file exists, changing it needs explicit permission from the
author. A red test means the implementation is wrong until they say
otherwise. `.claude/hooks/test-guard.sh` enforces this: any write to an
existing `*_test.go` becomes a permission prompt.

## Definition of done

`gofmt -l .` prints nothing, `go vet ./...` passes, `go test ./...` passes —
run, not reasoned about.
