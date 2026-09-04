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
- `--remote-targets`, which makes the *relative* entries a remote checksum
  file lists resolve against that file's URL and be fetched, rather than
  being read from the working directory.
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
  stop checking the transport is working against itself. This has a cost the
  test plan has to absorb — see *Testing*.
- Authentication of any kind: headers, `.netrc`, credentials in the URL.
- Retries, resume, response caching, conditional requests, a HEAD preflight,
  progress output, a custom `User-Agent`.
- A proxy *flag*. Proxy support itself is not out: the transport is cloned
  from `http.DefaultTransport`, so `ProxyFromEnvironment` comes with it and
  `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` work. That is a consequence of how
  the client is built, not something that happens by itself — see *The
  client*.

## Detection, not a `--url` flag

A reference is judged by this algorithm, in order. It is spelled out rather
than described because the loose description — "contains `://` with an
http scheme" — has readings that disagree on real inputs.

1. `url.Parse(ref)` fails → **path**. A reference with a control character
   in it is a filename that will fail to open, not a malformed URL. So
   `https://ex.org/a<newline>b` is a path, and a mistyped URL of that shape
   reports "no such file or directory".
2. The parsed scheme is empty → **path**. This is what makes
   `mirror/https://ex.org/f` a path: it has `://` in it, but the scheme
   before the *first* colon is empty.
3. `://` does not immediately follow the scheme → **path**. A file really
   called `weird:thing` parses with scheme `weird`, and `c:/x/https://y`
   with scheme `c`; requiring the slashes right after the scheme keeps both
   as filenames. The test is
   `strings.HasPrefix(ref[len(u.Scheme):], "://")`, indexing the *original*
   string by the scheme's length rather than matching its text: `url.Parse`
   lowercases the scheme, so `HTTPS://EX.ORG/f` would fail a text match.
4. Scheme is `http` or `https` → **remote**. `url.Parse` lowercases the
   scheme, so `HTTPS://EX.ORG/f` is remote.
5. Otherwise → **error**, `ftp://host/f: unsupported scheme "ftp"`, exit 2.
   Silently treating it as a relative path named `ftp:/host/f` would report
   "no such file" for something plainly meant as a URL.

A `--url` flag was the alternative. It was rejected because it can only
restate what the string already says, and it would have to exist twice —
once for the target, once for `-c` — or else declare a mode that applies to
both, which is wrong for the mixed case that is the point of
`--remote-targets`.

## Command surface

```
ldsum verify <file|url> <checksum> [--algo sha256|sha512]
ldsum verify -c <file|url> [<file>...] [--remote-targets]
```

Neither the argument count nor `--algo` changes. `--remote-targets` is a
bool, and it needs two guards:

- Without `-c`, it is meaningless: there is no listing whose entries could be
  remote. Exit 2, `--remote-targets needs --sums-file`. This is a `Changed`
  check in `verify.go`'s existing `Args` function, beside the empty-`-c`
  check already there.
- With a *local* `-c`, it is meaningless for the same reason and would
  otherwise silently do nothing. Exit 2,
  `--remote-targets needs a URL for --sums-file`. This one is in
  `internal/run`, where the predicate lives, so `cmd` does not import
  `internal/source` to ask one question.

`cmd.MarkFlagsRequiredTogether` is the wrong tool for the first guard: it is
symmetric, so it would also make a bare `-c` fail.

**The second guard runs before the checksum file is opened**, at the top of
`VerifySums` — not in `selectTargets` beside its siblings, which is where it
would naturally sit. `selectTargets` runs after the open and the parse, so a
guard there would report `no such file` for a missing local `-c`, and would
print a listing's bad-line warnings before saying the command was wrong.

## Resolution

**A URL entry is used as it is, in every row.** An entry that spells out a
full `http(s)` URL is fetched from where it points whatever `-c` was, and is
never handed to `filepath.Join`, which mangles it
(`filepath.Join("https://ex.org", "f")` is `https:/ex.org/f`).
`filepath.Clean` mangles identically, so the `byPath` lookup in
`selectTargets` must skip cleaning a remote entry too, or naming that entry
on the command line reports `no entry for`.

**An entry beginning `/` belongs to whichever namespace its base does.** In
rows 1 and 2 it is a local absolute path and keeps the short-circuit it has
today. In row 3 the base is a URL, so it is a URL path reference and
resolves against the host: `/other/f` against
`https://ex.org/v1.2/SHA256SUMS` is `https://ex.org/other/f`, not a local
file. The two rules do not conflict — a URL entry names its own namespace, a
`/` entry takes the base's — but an implementer reading only the first
paragraph would short-circuit the `/` case wrongly, so both are spelled out
and both are in the test list.

**Relative entries** resolve against the listing:

| `-c` is | relative entries resolve against |
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

Making the remote listing fetch its relative entries by default was the
alternative, on the grounds that it is the same rule as the local case. It
was rejected because a forty-entry `SHA256SUMS` would then start forty
downloads from one short command, and because the local-file case is the one
people are in.

**There is no exception for bare-digest files.** An earlier draft had
`--remote-targets` resolve the positional argument of a bare-digest file
against the sums URL. That is dropped. The flag governs entries the
*listing* names, and a bare-digest file names none; applying it to an
argument the *user* typed would be the only place the flag silently rewrites
`dist.tar.gz` — plainly meant as a file on disk — into a fetch. Anyone who
wants the remote file can name it: `ldsum verify https://ex.org/v1.2/dist.tar.gz`
with the digest, or with `-c` pointing at the `.sha256` beside it.

Combining the two is therefore an error rather than a no-op, since a flag
that does nothing is exactly what the second guard already refuses:

```
$ ldsum verify -c https://ex.org/v1.2/dist.tar.gz.sha256 dist.tar.gz
dist.tar.gz: OK                              # ./dist.tar.gz, flag or not

$ ldsum verify -c https://ex.org/v1.2/dist.tar.gz.sha256 dist.tar.gz --remote-targets
ldsum: https://ex.org/v1.2/dist.tar.gz.sha256: holds one checksum and no
entries for --remote-targets to resolve
```

Unlike the other two guards this one cannot be checked up front — whether a
checksum file is bare-digest is only known once it is parsed — so it lives
in `selectTargets`' bare branch beside `no paths in file` and
`holds one checksum`, which are its siblings in every sense.

A positional argument that is itself a URL is fetched with no flag at all,
in every mode. That is the headline feature — `ldsum verify <url> <checksum>`
— and `--remote-targets` has nothing to say about it.

**What `--remote-targets` is and is not.** It governs one thing: whether a
*relative* entry in a remote listing means a local file or a remote one. It
is not a network consent gate, and the spec should not be read as claiming
one: a URL the user typed is fetched, and so is a URL a listing spells out
in full. That is consistent with the decision the `feat/checksum-file-input`
review already made in rejecting **path confinement** — `verify` is
read-only and the digest oracle is self-directed, and refusing references
that leave their own directory would break the `sum`/`verify`
interoperability that exists on purpose. The flag exists because forty
implicit downloads from a short command is a bad default, not because
fetching needs permission.

## internal/source

The package that turns a reference into bytes, and the only non-test package
in the module that reaches the network. CLAUDE.md's rule that `hash` and
`checksums` take readers rather than paths is unaffected: `source` exists so
that something has to know about references, and it is not those two.

```go
// Package source opens what a reference names, whether that is a path or an
// http(s) URL. It is the only package here that reaches the network.
package source

// IsRemote reports whether ref names a URL this package fetches. It errors
// on a reference shaped like a URL whose scheme cannot be fetched.
func IsRemote(ref string) (bool, error)

// Open returns a reader over what ref names. The caller closes it.
func Open(ref string) (io.ReadCloser, error)

// JoinURL resolves the file name entry against the URL base.
func JoinURL(base, entry string) (string, error)
```

`Open` on a path is `os.Open`, **and returns its error unwrapped**. `os`
produces an `*fs.PathError` carrying the operation and path already, and
`TestVerifySumsLeavesTheOsErrorBare` type-asserts exactly that; any wrapping
here breaks it and `TestVerifyErrors`'s exact-message case.

`Open` on a URL issues a GET, checks the status, and returns `resp.Body` —
which streams, so CLAUDE.md's "never read a whole file into memory" rule is
satisfied by doing nothing special. `hash.Sum`'s 32 KiB copy buffer paces the
download.

`IsRemote` returning `(bool, error)` rather than a plain bool is what lets
one classifier serve three callers — `Open`, `run`'s `--remote-targets`
guard, and `resolve` — without each repeating the scheme judgement.

### JoinURL takes a file name, not a URL reference

A checksum-file entry is a file name. Handing it to `url.Parse` reads
punctuation that is legal in a name as URL syntax: `a#b.txt` becomes a
fragment and fetches `a`, `a?b.txt` becomes a query, `100%.txt` fails to
parse at all, and `weird:thing` becomes an absolute URL with scheme `weird`.

So the reference is built as a value, never parsed:

```go
base.ResolveReference(&url.URL{Path: entry})
```

which yields `a%23b.txt`, `a%3Fb.txt`, `100%25.txt` and `a%20b.txt`, while
still resolving `sub/f`, `/f` and `../f` the way URL references resolve.
`ResolveReference` drops any query string on the base, which is correct here:
a query on a `SHA256SUMS` URL is addressing that file, not its siblings.

### Status codes

The status is checked before a single byte is hashed. This is the bug that
matters here: a 404 whose body is an HTML error page hashes perfectly well,
and left unchecked the run would report `FAILED` with an expected/actual
pair, telling the user their file is corrupt when the truth is that it was
never there.

```go
// StatusError reports a response that was not a success. URL is the URL
// that produced it — resp.Request.URL, so a redirect chain names where it
// ended rather than where it started.
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

**The body is closed before the error is returned.** A forty-entry listing
of 404s otherwise leaks forty connections.

The success test is written `resp.StatusCode/100 != 2`, not
`< 200 || >= 300`. The range form spawns four boundary mutants that need
four contrived responses to kill; the division form spawns two, and the 200
and 404 cases already in the test table kill both. This is the same move as
`worst = max(worst, code)` in `cmd/exit.go` — prefer a line with nothing
interesting to mutate over an exception in `.gremlins.yaml`.

### The client

Built by cloning `http.DefaultTransport`, not by constructing a bare
`&http.Transport{}`:

```go
// newClient returns a client over a clone of t. Cloning rather than
// building from nothing is the point: DefaultTransport is where
// ProxyFromEnvironment, HTTP/2 and the connection-pool settings live, and a
// bare Transport has none of them — including no TLS handshake timeout.
func newClient(t *http.Transport) *http.Client
var client = newClient(http.DefaultTransport.(*http.Transport))
```

The parameter is `*http.Transport`, not `http.RoundTripper`: `newClient`
sets fields, which only the concrete type has. A test therefore passes
`ts.Client().Transport.(*http.Transport)` — that assertion holds, `httptest`
builds its client's transport as one — and `newClient` cloning internally is
what stops it from mutating the test server's own client.

`newClient` sets, on its clone:

- the dialer's `Timeout` — `dialTimeout`
- `TLSHandshakeTimeout` — `tlsTimeout`
- `ResponseHeaderTimeout` — `headerTimeout`
- `DisableCompression = true`, for the reason below

and on the client: the redirect policy, and **no `Timeout`**. `Client.Timeout`
caps the whole request including the body read, so any value large enough for
a 4 GB release artifact is too large to be a timeout, and any value small
enough to be useful would abort legitimate downloads.

`client` is a package-level `var` so tests can replace it with one built
over a test server's transport. This is the smallest
seam that makes the TLS and timeout cases testable at all — see *Testing* —
and it is not the injected-`Fetcher` design that was considered and dropped:
`Open`'s signature is unchanged and nothing outside the package sees a client.

**The accepted consequence:** a server that sends headers and then stalls the
body hangs the command forever. `ResponseHeaderTimeout` bounds the wait *for*
headers and stops there; `TLSHandshakeTimeout` and the dial timeout bound the
two stages before it. Bounding a stalled body needs either an overall
deadline, which breaks large downloads, or a stall detector, which is a new
mechanism for a failure nobody has hit. A `--timeout` flag was offered and
declined. If this ever bites, that flag is the fix, and this paragraph is the
record of why it does not exist yet.

### Transparent compression is turned off

A default transport advertises `Accept-Encoding: gzip` and silently
decompresses the response. For a checksum tool that is a correctness bug, not
an optimisation: a server that sends `dist.tar.gz` with
`Content-Encoding: gzip` would have ldsum hash the *decompressed* bytes and
report a mismatch against a digest computed over the file as stored.
`DisableCompression = true` makes ldsum hash what `curl -O` would save.

### Redirects

The policy refuses two things: more than ten hops, and a downgrade.

```go
func checkRedirect(req *http.Request, via []*http.Request) error
```

The hop limit has to be written out. `Client.checkRedirect` uses Go's
`defaultCheckRedirect`, which carries the `len(via) >= 10` cap, **only when
`CheckRedirect` is nil** (`net/http/client.go`). Installing a policy that
only refuses downgrades therefore removes the cap entirely, and a handler
redirecting to itself loops until something else gives out. The policy
returns `stopped after 10 redirects`, matching Go's own wording.

The downgrade refusal is the reason the policy exists. The checksum file is
the trust anchor of the whole operation: a digest fetched over plain HTTP can
be rewritten in flight to match a file that was also rewritten in flight, and
the command would print `OK`. Go's default follows such a redirect silently.
`http` → `https` is fine, and so is `https` → `https`.

The user sees the policy error wrapped in the `*url.Error` the client builds,
whose `URL` field is the *redirect target*:

```
ldsum: Get "http://ex.org/SHA256SUMS": redirect from https://ex.org/SHA256SUMS downgrades to http
```

The wording is chosen so the sentence still reads once Go has prefixed it,
and the test asserts that whole line rather than the policy error alone.

The restriction can in principle break a mirror that redirects downward; that
mirror is providing no integrity guarantee, which is the one thing this tool
is for.

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

**`verifyEntry`** — `os.Open(path)` becomes `source.Open(path)`. Its existing
`errors.Is(err, fs.ErrNotExist)` branch now catches a 404 too, by way of
`StatusError.Is`, and wraps it in `MissingTargetError` exactly as it does a
missing local file. Its `FAILED open or read` verdict already covers a fetch
that fails.

One thing does change inside it. The `hash.Sum` error is returned bare today,
which is right for a local file — reading one yields an `*fs.PathError`
carrying the path — but a truncated response body yields a plain
`unexpected EOF`, and `ldsum: unexpected EOF` names nothing. So that return
gains the same guard `VerifySums` already uses: wrap with `read %s: %w`
**only when the error is not already an `*fs.PathError`**.

The local read-error cases are unaffected, and this was checked against the
tests rather than assumed. `TestVerifyErrors/"path is a directory"` and
`/"unreadable file"` assert only that the error is non-nil and is *not*
`fs.ErrNotExist` — neither pins a message, and a directory read yields an
`*fs.PathError` on macOS anyway, so the guard leaves it alone. The one
exact-message assertion in the package,
`want := "open " + missing + ": no such file or directory"`, is on the
*open* path, which `source.Open` returns unwrapped for exactly this reason.
`sum_test.go`'s `"is a directory"` assertion is on `sumFile`, a different
function this change does not touch.

**`VerifySums`** — `os.Open(opts.SumsFile)` becomes `source.Open(...)`,
returned bare exactly as it is today. It needs no new guard: a `*url.Error`
from the GET reaches that same bare return, and the `*fs.PathError` guard
further down is on the *parse* path, where a body-read failure produces a
plain error and not a `*url.Error` at all.

**`resolve` and `selectTargets`** — `resolve` gains the absolute-reference
short-circuit for URLs described above, and a remote branch calling
`source.JoinURL`. The base is computed once, by `selectTargets`: the checksum
file's directory when `-c` is local, `"."` when it is a URL, and the URL
itself under `--remote-targets`. `selectTargets` also stops running
`filepath.Clean` over a remote entry when building its `byPath` lookup.

Neither function is called by any test directly — both are exercised only
through `VerifySums` — so this is an internal change with no test to
renegotiate.

## Output

A verdict line carries whatever reference was verified, so a URL prints as a
URL:

```
$ ldsum verify https://ex.org/v1.2/dist.tar.gz $EXPECTED
https://ex.org/v1.2/dist.tar.gz: OK
```

`verdict` still escapes the reference through `checksums.EscapePath`. A URL
that reached this point cannot contain a raw newline — `url.Parse` rejects
control characters, so such a reference was classified as a path — but it can
contain a backslash, which `EscapePath` escapes and marks exactly as it does
for a path. Diagnostics on stderr keep printing references raw, which remains
the deliberate boundary it was: verdict lines are for scripts, diagnostics
are for people.

## Exit codes

`cmd/exit.go` does not change, and neither does `cmd/execute`. The mapping
falls out of the error types that already exist:

| failure | error | exit |
|---|---|---|
| target 404 or 410 | `*MissingTargetError`, via `StatusError.Is` | 1 |
| target 500, or any other non-2xx | `*StatusError` | 2 |
| target unreachable, TLS failure, DNS failure | `*url.Error` | 2 |
| redirect loop, or a refused downgrade | `*url.Error` | 2 |
| truncated response body | wrapped `read <ref>: unexpected EOF` | 2 |
| checksum file 404 | `*StatusError`, returned bare by `VerifySums` | 2 |
| unsupported scheme | plain error naming the reference | 2 |
| digest differs | `*MismatchError` — unchanged | 1 |

The 404 rows are the point. A missing target is the user's file to fix and
exits 1; a missing *checksum file* is the command being wrong and exits 2 —
the distinction `MissingTargetError`'s own comment says it was created for,
now doing that job across a second kind of source.

**`exitCode` gains no `*StatusError` arm, and must not.** A 404 checksum file
reaches it as a bare `*StatusError`, which matches neither `*MismatchError`
nor `*MissingTargetError`, so it falls to `default: 2` — which is the answer
wanted. Adding an arm "for symmetry" with the target case would turn a
missing checksum file into exit 1 and undo the whole distinction. Only
`verifyEntry`, which knows it is looking at a target, converts a 404 into
something that exits 1. A run mixing a local
mismatch and a remote 500 still exits 2, because `VerifyErrors` already
returns the worst code.

**An unsupported scheme in a positional target** reaches `verifyEntry`, which
prints `FAILED open or read` before returning. That verdict line is wanted: a
run over several files must name the one that could not be read rather than
only counting it, and this failure is no different.

## Testing

`internal/source` — `httptest`, which is standard library and a real server,
not a mock. Tests build their client with
`newClient(ts.Client().Transport.(*http.Transport))`, which is also how the
TLS cases work at all: `httptest.NewTLSServer`'s
certificate is not in the system pool, so a request from the package client
fails with `x509: certificate signed by unknown authority` before any policy
runs, and `--insecure` is out of scope.

- a 200 whose body streams back byte-identical
- 404 and 410 satisfy `errors.Is(err, fs.ErrNotExist)`; 500 and 403 do not
- the status is checked before hashing: a 404 with a plausible body produces
  a not-found error, never a digest
- a non-2xx response closes its body
- connection refused (a server started and immediately closed)
- a `Content-Encoding: gzip` response is hashed as sent, not as decompressed
- `IsRemote` over: `https://`, `HTTPS://EX.ORG/f`, a relative path, an
  absolute path, `weird:thing`, `c:/x/https://y`, `mirror/https://ex.org/f`,
  a reference containing a newline (all paths), and `ftp://host/f` (error)
- `JoinURL` over `sub/f`, `../f`, `a b.txt`, `a#b.txt`, `a?b.txt`,
  `100%.txt`, and a base carrying a query string; plus `/other/f` against
  `https://ex.org/v1.2/SHA256SUMS`, pinned to `https://ex.org/other/f` so
  the row-3 reading of a leading `/` is held in place
- a redirect chain that is followed
- a redirect loop stops after ten hops
- an `https` → `http` redirect is refused, asserting the full `url.Error`
  line the user sees
- a handler that blocks **before** writing headers, with `headerTimeout`
  small, returns `timeout awaiting response headers`. A handler that stalls
  after `WriteHeader` is the *other* case and must not be used here: `Get`
  has already returned by then, so such a test passes even with the timeout
  at zero
- a TLS listener that accepts the connection and never handshakes returns
  once `tlsTimeout` elapses

`internal/run` — a `httptest` server, with real files under `t.TempDir()`:

- `Verify` with a URL target and a matching inline checksum: `OK`, exit 0
- the same with a checksum that differs: `FAILED`, a `*MismatchError`, and
  the expected/actual pair on stderr
- a URL target that 404s: `FAILED open or read` and a `*MissingTargetError`
- a URL target whose body is cut short: the error names the URL
- `--algo` applies to a URL target exactly as to a path
- a remote listing with relative entries checks local files, proven by a
  working directory that is not where the fixture files are
- the same listing under `RemoteTargets: true` fetches each entry, proven by
  recording the paths the handler was asked for
- `RemoteTargets: true` together with positional arguments filtering the
  listing
- a URL entry spelled out in full is fetched in all three rows, and can be
  named as a positional argument without being mangled by `filepath.Clean`
- a remote bare-digest file with one local positional argument, which
  behaves the same whether or not `RemoteTargets` is set — because setting
  it is an error, asserted separately and naming the flag
- `RemoteTargets: true` with a local `-c` is an error, raised before the
  file is opened — proven by pointing `-c` at a path that does not exist and
  getting the flag error, not `no such file`
- a remote listing whose target 404s yields a `*MissingTargetError`; a 404
  checksum file does not
- warnings from a remote listing carry the URL as the file part:
  `https://ex.org/SHA256SUMS:3: not a checksum line`

`cmd` — the flag reaches `SumsOptions`; and `--remote-targets` without `-c`
is exit 2. That second case is asserted through `execute`, not by calling
`run`: the guard is in `Args`, which Cobra runs *before* `RunE` sets
`SilenceUsage`, so this is one of the few errors that still prints usage
text. The test checks that it does, which is what stops a later refactor
from moving the guard into `RunE` and silently changing the output.

`main_test.go` — an end-to-end verify against a `httptest` server started in
the harness, asserting exit 0, and one asserting exit 1 for a 404 target.
This is the only place the real package client in a real process is
exercised, so these use a plain `httptest.NewServer`, not a TLS one.

### Mutation testing

The duration and hop-count values are package-level `var`s so tests can
shrink them. That is their whole justification: an earlier draft claimed the
`var` also protects `30 * time.Second` from gremlins' arithmetic mutator,
and that is wrong twice over — a test that assigns the var cannot observe
the mutant, and gremlins reports a package-level initialiser as
`NOT COVERED` because it lies outside the coverage profile. Each initialiser
therefore adds one uncovered mutant to the mcover denominator;
`.gremlins.yaml` sets that threshold at 85 against a current 92.59, so a
handful is affordable, but they are a cost rather than a defence.

### The sandbox

`httptest` binds a loopback port, and the Bash sandbox refuses `bind`
outright — `listen tcp6 [::1]:0: bind: operation not permitted`. Verified:
loopback TCP and unix sockets are both refused, and both succeed with
`dangerouslyDisableSandbox`. So from this change onward `go test ./...`
needs the sandbox disabled, and so do `gremlins unleash` and `go-mutesting`,
which shell out to `go test`. CI and an ordinary terminal are unaffected.

CLAUDE.md's paragraph on the sandbox opens "Anything that reaches the
network fails under the Bash sandbox with a TLS certificate error", and that
is no longer the whole truth: a loopback listener never reaches the network
and fails earlier, at `bind`, with a different error. That sentence is the
one to change, and the list after it gains the test commands.

## Documentation

These say URL input is unimplemented and change with the code:

- `README.md`'s status note, plus a usage section for URLs, the
  `--remote-targets` semantics, and the resolution table
- `cmd/root.go`'s long help — the sentence "Reading the file itself from a
  URL is planned and not yet implemented"
- `cmd/verify.go`'s long help
- `CLAUDE.md`: `internal/source/` in both the Layout prose and the Structure
  tree, with the clause that it is the one package whose job is reference to
  reader; and the sandbox note above
