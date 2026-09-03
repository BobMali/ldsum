# Verify files and checksum files given by URL

Date: 2026-09-03
Status: designed

`verify` reads only local files. This adds the input axis CLAUDE.md has
described since the first commit — "the file comes from a local path *or* a
URL" — to both halves of the command: the file being checked, and the
checksum file `-c` reads its expectations from. It is the last of the two
axes the design has been carrying a placeholder for, and the first time
ldsum touches the network.

## Scope

In:

- An `http` or `https` URL accepted wherever `verify` accepts a path: the
  positional target, and `--sums-file`/`-c`.
- `--remote-targets`, which makes the entries a remote checksum file lists
  resolve against that file's URL and be fetched, rather than being read
  from the working directory.
- A new `internal/source` package: the one place that turns a reference —
  path or URL — into an `io.ReadCloser`.

Out, deliberately:

- **A URL as the inline checksum.** There is nothing to build. A URL whose
  body is one bare digest is exactly the bare-digest checksum-file shape
  `-c` already parses, so `ldsum verify -c https://ex.org/f.tar.gz.sha256
  f.tar.gz` works the moment `-c` takes a URL. Adding a third way to supply
  an expectation would duplicate that.
- `sum <url>`. `sum` reports digests of things you have; fetching a file to
  print its digest and then discard it is a different command. The axis
  CLAUDE.md names belongs to `verify`.
- `--insecure` / certificate skipping. A checksum tool that can be told to
  stop checking the transport is working against itself.
- Authentication of any kind: headers, `.netrc`, credentials in the URL.
- Retries, resume, response caching, conditional requests, a HEAD preflight,
  progress output, a custom `User-Agent`.
- Proxy configuration. `http.Transport` already honours `HTTP_PROXY`,
  `HTTPS_PROXY` and `NO_PROXY`, so this comes free and needs no surface.

## Detection, not a `--url` flag

A reference is remote if and only if it contains `://` and the scheme before
it is `http` or `https`, compared case-insensitively.

Everything else is a path. That includes a reference `url.Parse` rejects
outright — a control character, say — which is a filename that will fail to
open, not a malformed URL worth its own error.

`://` present with any other scheme is the command being wrong:
`unsupported scheme "ftp"`, exit 2. Silently treating `ftp://host/f` as a
relative path named `ftp:/host/f` would report "no such file or directory"
for something the user plainly meant as a URL.

The `://` requirement is what keeps the rule from eating filenames. A file
really called `weird:thing` parses with scheme `weird`, and a rule keyed on
the scheme alone would reject it; requiring the slashes means only something
shaped like a URL is judged as one.

A `--url` flag was the alternative. It was rejected because it can only
restate what the string already says, and it would have to exist twice —
once for the target, once for `-c` — or else declare a mode that applies to
both, which is wrong for the mixed case that is the whole point of
`--remote-targets` being opt-in.

## Command surface

```
ldsum verify <file|url> <checksum> [--algo sha256|sha512]
ldsum verify -c <file|url> [<file>...] [--remote-targets]
```

Neither the argument count nor `--algo` changes. `--remote-targets` is a
bool, and it needs two guards:

- Without `-c`, it is meaningless: there is no listing whose entries could be
  remote. Exit 2, `--remote-targets needs --sums-file`.
- With a *local* `-c`, it is meaningless for the same reason and would
  otherwise silently do nothing. Exit 2,
  `--remote-targets needs a URL for --sums-file`.

`cmd.MarkFlagsRequiredTogether` is the wrong tool for the first guard: it is
symmetric, so it would also make a bare `-c` fail. The first guard is a
`Changed` check in `verify.go`'s existing `Args` function, beside the empty
`-c` check already there. The second belongs in `internal/run`, where
`--remote-targets needs a URL` sits naturally beside `no paths in file` and
`holds one checksum` — and where the predicate lives, so `cmd` does not
import `internal/source` to ask one question.

## Resolution: one rule and one exception

**The rule.** A file the *user* names is resolved from the working
directory. A file the *listing* names is resolved against the listing:

| `-c` is | entries resolve against |
|---|---|
| a local path | the checksum file's directory — unchanged, today's behaviour |
| a URL | the working directory |
| a URL, with `--remote-targets` | the checksum file's URL |

The middle row is the new default, and it is not an inconsistency with the
first. "Against the listing" means the directory the listing sits in, and a
URL has no local directory; the working directory is the only thing left
that can be meant. It is also the workflow that actually happens: you
downloaded the release artifact, and now you want to check it against
upstream's published `SHA256SUMS` without downloading anything twice.

```
$ ldsum verify -c https://ex.org/v1.2/SHA256SUMS
ldsum-linux-amd64: OK          # ./ldsum-linux-amd64, from cwd
ldsum-darwin-arm64: OK         # only SHA256SUMS was fetched

$ ldsum verify -c https://ex.org/v1.2/SHA256SUMS --remote-targets
ldsum-linux-amd64: OK          # https://ex.org/v1.2/ldsum-linux-amd64
ldsum-darwin-arm64: OK         # fetched and hashed as it streams
```

Making the remote listing fetch its entries by default was the alternative,
on the grounds that it is the same rule as the local case. It was rejected
because a forty-entry `SHA256SUMS` would then start forty downloads from one
short command, and because the local-file case is the one people are in.

**The exception.** A bare-digest checksum file names no path, so the file to
check is a positional argument — a file the *user* named, which by the rule
above is cwd-relative. `--remote-targets` overrides that one case and
resolves it against the sums URL, because a user who has asked for remote
targets has asked for exactly that:

```
$ ldsum verify -c https://ex.org/v1.2/dist.tar.gz.sha256 dist.tar.gz
dist.tar.gz: OK                              # ./dist.tar.gz

$ ldsum verify -c https://ex.org/v1.2/dist.tar.gz.sha256 dist.tar.gz --remote-targets
https://ex.org/v1.2/dist.tar.gz: OK          # fetched
```

This is new behaviour reachable only through a new flag, so no existing
behaviour changes and no existing test moves.

**No path confinement.** An entry that is an absolute URL is fetched from
wherever it points. This is the decision the `feat/checksum-file-input`
review already made for local paths — `verify` is read-only and the digest
oracle is self-directed, and refusing entries that escape their own
directory would break the `sum`/`verify` interoperability that exists on
purpose. One thing is genuinely different and is worth stating rather than
glossing: fetching sends traffic to a third party, where reading a file did
not. `--remote-targets` is the consent gate for precisely that, which is the
second reason it is opt-in.

## internal/source

The package that turns a reference into bytes, and the only one in the
module that imports `net/http`. CLAUDE.md's rule that `hash` and `checksums`
take readers rather than paths is unaffected: `source` exists so that
something has to know about references, and it is not those two.

```go
// Package source opens what a reference names, whether that is a path or an
// http(s) URL. It is the only package that reaches the network.
package source

// IsRemote reports whether ref names a URL this package fetches. It errors
// on a reference shaped like a URL whose scheme cannot be fetched.
func IsRemote(ref string) (bool, error)

// Open returns a reader over what ref names. The caller closes it.
func Open(ref string) (io.ReadCloser, error)

// JoinURL resolves ref against the URL base, which must be a URL.
func JoinURL(base, ref string) (string, error)
```

`Open` on a path is `os.Open`. On a URL it issues a GET, checks the status,
and returns `resp.Body` — which streams, so CLAUDE.md's "never read a whole
file into memory" rule is satisfied by doing nothing special. `hash.Sum`'s
32 KiB copy buffer is what paces the download.

`IsRemote` returning `(bool, error)` rather than a plain bool is what lets
one classifier serve three callers — `Open`, `run`'s `--remote-targets`
guard, and `resolve` — without each repeating the scheme judgement.

### Status codes

The status is checked before a single byte is hashed. This is the bug that
matters here: a 404 whose body is an HTML error page hashes perfectly well,
and left unchecked the run would report `FAILED` with an expected/actual
pair, telling the user their file is corrupt when the truth is that it was
never there.

```go
// StatusError reports a response that was not a success.
type StatusError struct {
	URL    string
	Status string // resp.Status, e.g. "404 Not Found"
	Code   int
}

func (e *StatusError) Error() string { return e.URL + ": " + e.Status }

// Is reports a not-found response as fs.ErrNotExist, so a caller that
// already distinguishes a missing file from an unreadable one needs no
// second branch for URLs.
func (e *StatusError) Is(target error) bool {
	return target == fs.ErrNotExist && (e.Code == http.StatusNotFound ||
		e.Code == http.StatusGone)
}
```

The success test is written `resp.StatusCode/100 != 2`, not
`< 200 || >= 300`. The range form spawns four boundary mutants that need
four contrived responses to kill; the division form spawns two, and the 200
and 404 cases already in the test table kill both. This is the same move as
`worst = max(worst, code)` in `cmd/exit.go` — prefer a line with nothing
interesting to mutate over an exception in `.gremlins.yaml`.

### Timeouts

A package-level client, with **no `Client.Timeout`**. That field caps the
whole request including the body read, so any value large enough for a 4 GB
release artifact is too large to be a timeout, and any value small enough to
be useful would abort legitimate downloads. Instead:

```go
// Overridable so a test can watch a stall without waiting for it.
var (
	dialTimeout    = 10 * time.Second
	headerTimeout  = 30 * time.Second
)
```

set as `Transport.DialContext`'s dialer timeout and
`Transport.ResponseHeaderTimeout`.

**The consequence, accepted deliberately:** a server that sends headers and
then stalls the body hangs the command forever. Bounding it needs either an
overall deadline, which breaks large downloads, or a stall detector, which
is a new mechanism for a failure mode nobody has hit. A `--timeout` flag was
offered and declined. If this ever bites, the fix is that flag, and this
paragraph is the record of why it does not exist yet.

The two literals are `var`s, not constants, for a reason beyond testing:
`30 * time.Second` under gremlins' arithmetic mutator becomes
`30 / time.Second`, which is zero, which is *no timeout at all* — a mutant
no test of correct behaviour can see. Tests set these directly and assert
the stall is cut short, which kills it.

### Redirects

Up to Go's default ten hops, with one restriction: a redirect from `https`
to `http` is refused.

The checksum file is the trust anchor of the whole operation. A digest
fetched over plain HTTP can be rewritten in flight to match a file that was
also rewritten in flight, and the command would print `OK`. Go's default
policy follows such a downgrade silently. `CheckRedirect` refuses it:

```
https://ex.org/SHA256SUMS: redirect to http://ex.org/SHA256SUMS downgrades https
```

`http` → `https` is fine, and so is `https` → `https`. The restriction can
in principle break a mirror that redirects downward; that mirror is
providing no integrity guarantee, which is the one thing this tool is for.

## internal/run

`Verify`, `VerifyOptions`, `VerifySums` and `SumsOptions` keep their
signatures. `SumsOptions` gains one field:

```go
type SumsOptions struct {
	SumsFile      string
	Paths         []string
	RemoteTargets bool
}
```

Three call sites change, and nothing else:

**`verifyEntry`** — `os.Open(path)` becomes `source.Open(path)`. Nothing
else in the function moves. Its existing `errors.Is(err, fs.ErrNotExist)`
branch now catches a 404 too, by way of `StatusError.Is`, and wraps it in
`MissingTargetError` exactly as it does a missing local file. Its
`FAILED open or read` verdict already covers a fetch that fails.

**`VerifySums`** — `os.Open(opts.SumsFile)` becomes
`source.Open(opts.SumsFile)`, and the guard that keeps an `*fs.PathError`
from gaining a second copy of its operation and path gains a sibling:
`*url.Error` carries the same two things for the same reason, so it is
returned bare as well.

**`resolve` and `selectTargets`** — `resolve` gains a remote branch, because
`filepath.Join` and `filepath.Clean` corrupt a URL rather than joining it
(`filepath.Join("https://ex.org", "f")` is `https:/ex.org/f`). The base is
computed once, by `selectTargets`, from the three-row table above: the
checksum file's directory when it is local, `"."` when it is a URL,
and the URL itself under `--remote-targets`. Only the third case takes the
remote branch, which calls `source.JoinURL`. An absolute local entry keeps
its existing short-circuit; under `--remote-targets` an entry beginning `/`
resolves against the URL's host, which is what a URL reference means.

Neither function is called by any test directly — both are exercised only
through `VerifySums` — so this is an internal change with no test to
renegotiate.

## Output

Unchanged, and that is the point: a verdict line carries whatever reference
was verified, so a URL prints as a URL.

```
$ ldsum verify https://ex.org/v1.2/dist.tar.gz $EXPECTED
https://ex.org/v1.2/dist.tar.gz: OK
```

`verdict` still escapes the reference through `checksums.EscapePath`. A URL
cannot contain a raw newline or backslash, so the escaping is a no-op here
and no new forgery vector arrives with this change; the call stays because
the same function prints local paths, where it does matter. Diagnostics on
stderr keep printing references raw, which remains the deliberate boundary
it was: verdict lines are for scripts, diagnostics are for people.

## Exit codes

`cmd/exit.go` does not change, and neither does `cmd/execute`. The mapping
falls out of the error types that already exist:

| failure | error | exit |
|---|---|---|
| target 404 or 410 | `*MissingTargetError`, via `StatusError.Is` | 1 |
| target 500, or any other non-2xx | `*StatusError` | 2 |
| target unreachable, TLS failure, DNS failure | `*url.Error` | 2 |
| checksum file 404 | `*StatusError`, returned bare by `VerifySums` | 2 |
| unsupported scheme | plain error | 2 |
| redirect downgrade | `*url.Error` wrapping the policy error | 2 |
| digest differs | `*MismatchError` — unchanged | 1 |

The 404 rows are the point. A missing target is the user's file to fix and
exits 1; a missing *checksum file* is the command being wrong and exits 2 —
which is the distinction `MissingTargetError`'s own comment says it was
created for, now doing that job across a second kind of source. A run mixing
a local mismatch and a remote 500 still exits 2, because `VerifyErrors`
already returns the worst code.

## Testing

`internal/source` — `httptest`, which is standard library and a real server,
not a mock:

- a 200 whose body streams back byte-identical
- 404 and 410 satisfy `errors.Is(err, fs.ErrNotExist)`; 500 and 403 do not
- the status is checked before hashing: a 404 with a plausible body produces
  a not-found error, never a digest
- connection refused (a server started and immediately closed)
- `IsRemote` over: `https://`, `HTTP://`, a plain relative path, an absolute
  path, `weird:thing`, `ftp://host/f` (error), and a reference `url.Parse`
  rejects (a path, not an error)
- `JoinURL` over a relative entry, an entry starting `/`, and a `..` entry
- a redirect chain that is followed; an `https` → `http` redirect that is
  refused, using `httptest.NewTLSServer` in front of a plain one
- a handler that sends headers and then blocks, with `headerTimeout` set
  small, asserting the call returns rather than hanging

`internal/run` — a `httptest` server, with real files under `t.TempDir()`:

- `Verify` with a URL target and a matching inline checksum: `OK`, exit 0
- the same with a checksum that differs: `FAILED`, a `*MismatchError`, and
  the expected/actual pair on stderr
- a URL target that 404s: `FAILED open or read` and a `*MissingTargetError`
- `--algo` applies to a URL target exactly as to a path
- a remote listing with relative entries checks local files, proven by a
  working directory that is not where the fixture files are
- the same listing under `RemoteTargets: true` fetches each entry, proven by
  recording the paths the handler was asked for
- a remote bare-digest file with one local positional argument
- the same with `RemoteTargets: true`, resolving the argument against the URL
- `RemoteTargets: true` with a local `-c` is an error
- a remote listing whose target 404s reports `FAILED open or read` and yields
  a `*MissingTargetError`
- a 404 checksum file is not a `*MissingTargetError`
- warnings from a remote listing carry the URL as the file part:
  `https://ex.org/SHA256SUMS:3: not a checksum line`

`cmd` — `--remote-targets` without `-c` is exit 2; the flag reaches
`SumsOptions`.

`main_test.go` — one case, an end-to-end verify against a `httptest` server
started in the harness, asserting exit 0, and one asserting exit 1 for a
404 target. This is the only place the real `http.Client` in a real process
is exercised.

### The sandbox

`httptest` binds a loopback port, and the Bash sandbox refuses `bind`
outright — `listen tcp6 [::1]:0: bind: operation not permitted`. Verified:
loopback TCP and unix sockets are both refused, and both succeed with
`dangerouslyDisableSandbox`. So from this change onward `go test ./...`
needs the sandbox disabled, and so do `gremlins unleash` and
`go-mutesting`, which shell out to `go test`. CI and an ordinary terminal
are unaffected. CLAUDE.md's paragraph on what the sandbox blocks has to say
this.

## Documentation

These say URL input is unimplemented and change with the code:

- `README.md`'s status note, plus a usage section for URLs, the
  `--remote-targets` semantics, and the three-row resolution table
- `cmd/root.go`'s long help — the sentence "Reading the file itself from a
  URL is planned and not yet implemented"
- `cmd/verify.go`'s long help
- `CLAUDE.md`: `internal/source/` in both the Layout prose and the Structure
  tree, with the clause that it is the one package whose job is reference to
  reader; and the sandbox note above
