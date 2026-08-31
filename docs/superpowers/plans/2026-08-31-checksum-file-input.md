# Checksum File Input Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `ldsum verify` read expected checksums from a checksum file such as a published `SHA256SUMS`, recognising the line format automatically.

**Architecture:** `internal/checksums` gains a `Parse` that reads an `io.Reader` and recognises each line on its own — BSD tagged, GNU text, GNU binary, or a bare digest. `internal/run` gains `VerifySums` beside the existing `Verify`; both call one unexported `verifyEntry`, so the two input modes share the part that is the same. Several files means several verdicts and one status, so failures collect into a `VerifyErrors` that `cmd/exit.go` unwraps, worst code winning.

**Tech Stack:** Go 1.27, `spf13/cobra` v1.10.2, standard library only otherwise.

**Spec:** `docs/superpowers/specs/2026-08-31-checksum-file-input-design.md` — read it before Task 1. The plan argues from it and does not repeat its reasoning.

## Global Constraints

- Go 1.27.0. Module path `github.com/BobMali/ldsum`.
- Standard library plus `spf13/cobra` only. Adding a module is a decision, not an implementation detail — do not.
- **Test-first, one case at a time.** Write one failing test, run it, watch it fail for the right reason, then write the minimum code that passes it. Never write implementation before its failing test exists.
- **Do not edit an existing test.** Every test in this repo is green today and stays that way. New test *files* are free, and so is appending a new function to an existing test file. Rewriting, renaming, deleting or loosening existing test text needs the author's permission — `.claude/hooks/test-guard.sh` will stop you.
- `os.Exit` appears only in `main.go`. Everything else returns an `error`.
- Cobra commands parse flags and call `internal/run`. They do no work of their own.
- Commands write through `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`. `internal/hash` and `internal/checksums` take readers and writers, never paths.
- Data to stdout, diagnostics to stderr.
- `os` and `io` errors are already `*fs.PathError` carrying the operation and path. Do not wrap them with a second copy. Never log and return the same error.
- Comment only what the code cannot say — why, not what. One or two lines.
- Conventional Commits, one line, no body unless the reason lives outside the diff. Types `feat fix docs test refactor perf build ci chore revert`; scopes `hash checksums cmd run ci deps`. Lowercase imperative description, no trailing period, <= 66 characters. A rejected commit is a message problem; never reach for `--no-verify`.
- **Never add `Co-Authored-By` or `Claude-Session` trailers to commits.**
- Definition of done for every task: `gofmt -l .` prints nothing, `go vet ./...` passes, `go test ./...` passes. Run them and report what they printed. Never claim done from reasoning about the code.

---

### Task 1: Parse the GNU line formats

**Files:**
- Create: `internal/checksums/parse.go`
- Create: `internal/checksums/parse_test.go`
- Modify: `internal/checksums/checksums.go` — the package comment (line 1-2) and the `Entry` struct

**Interfaces:**
- Consumes: `hash.ParseDigest(string) (hash.Digest, error)`, `hash.Digest{Algorithm, Hex}` from `internal/hash`.
- Produces: `checksums.Parse(io.Reader) (Listing, error)`, `checksums.Listing{Entries []Entry; Bad []BadLine}`, `checksums.BadLine{Line int; Err error}`, and `Entry` gains a third field `Line int`.

- [ ] **Step 1: Write the failing test**

Create `internal/checksums/parse_test.go`. `abcSHA256` is already declared in `checksums_test.go` in this same package — reuse it, do not redeclare it.

```go
package checksums

import (
	"strings"
	"testing"

	"github.com/BobMali/ldsum/internal/hash"
)

// abcDigest is the sha256 of "abc" in the shape Parse must report.
var abcDigest = hash.Digest{Algorithm: hash.SHA256, Hex: abcSHA256}

func TestParseGNU(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Entry
	}{
		{
			name: "text is two spaces",
			in:   abcSHA256 + "  dist/a.txt\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "binary is a space then an asterisk",
			in:   abcSHA256 + " *dist/a.txt\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "a single separator space is accepted",
			in:   abcSHA256 + " dist/a.txt\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "two spaces win, so a path may start with an asterisk",
			in:   abcSHA256 + "  *odd.txt\n",
			want: Entry{Digest: abcDigest, Path: "*odd.txt", Line: 1},
		},
		{
			name: "uppercase hex is normalised",
			in:   strings.ToUpper(abcSHA256) + "  dist/a.txt\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "a file need not end in a newline",
			in:   abcSHA256 + "  dist/a.txt",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Bad) != 0 {
				t.Fatalf("Parse() Bad = %+v, want none", got.Bad)
			}
			if len(got.Entries) != 1 {
				t.Fatalf("Parse() Entries = %+v, want exactly one", got.Entries)
			}
			if got.Entries[0] != tt.want {
				t.Errorf("Parse() entry = %+v, want %+v", got.Entries[0], tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/checksums -run TestParseGNU`
Expected: FAIL to build, with exactly two errors — `undefined: Parse` and `unknown field Line in struct literal of type Entry`. The test never names `Listing`, so no error mentions it.

- [ ] **Step 3: Add `Line` to `Entry` and fix the package comment**

In `internal/checksums/checksums.go`, replace the package comment's last sentence and extend `Entry`:

```go
// Package checksums renders and reads the lines of a checksum file. It works
// on readers, writers and strings, and knows nothing about files.
```

```go
// Entry is one file's digest together with the path it belongs to. Line is
// the 1-based line Parse read it from, and is zero for an entry that did not
// come from a file.
type Entry struct {
	Digest hash.Digest
	Path   string
	Line   int
}
```

- [ ] **Step 4: Write the minimal implementation**

Create `internal/checksums/parse.go`:

```go
package checksums

import (
	"bufio"
	"errors"
	"io"
	"strings"

	"github.com/BobMali/ldsum/internal/hash"
)

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

var errNotChecksum = errors.New("not a checksum line")

// Parse reads r as a checksum file. Every line is recognised on its own, so
// one file may mix formats and algorithms. The returned error reports a
// failure to read r; a line that is not a checksum becomes a BadLine.
func Parse(r io.Reader) (Listing, error) {
	var l Listing
	s := bufio.NewScanner(r)
	for n := 1; s.Scan(); n++ {
		// Checksum files travel between systems, so a CRLF ending is not the
		// author saying the path ends in a carriage return.
		line := strings.TrimSuffix(s.Text(), "\r")
		if trimmed := strings.TrimLeft(line, " \t"); trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		e, err := parseLine(line)
		if err != nil {
			l.Bad = append(l.Bad, BadLine{Line: n, Err: err})
			continue
		}
		e.Line = n
		l.Entries = append(l.Entries, e)
	}
	if err := s.Err(); err != nil {
		return Listing{}, err
	}
	return l, nil
}

func parseLine(line string) (Entry, error) {
	hexRun := leadingHex(line)
	if hexRun == "" {
		return Entry{}, errNotChecksum
	}
	path, ok := gnuPath(line[len(hexRun):])
	if !ok {
		return Entry{}, errNotChecksum
	}
	if path == "" {
		return Entry{}, errors.New("checksum with no path")
	}
	d, err := hash.ParseDigest(hexRun)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Digest: d, Path: path}, nil
}

// gnuPath splits the path off what follows a GNU-format digest. Two spaces
// mean text and " *" means binary; a single space is accepted too, because
// tools that emit one separator instead of two are common. The two-space case
// is tested first, so a path that itself starts with an asterisk survives.
func gnuPath(rest string) (string, bool) {
	switch {
	case strings.HasPrefix(rest, "  "):
		return rest[2:], true
	case strings.HasPrefix(rest, " *"):
		return rest[2:], true
	case strings.HasPrefix(rest, " "):
		return rest[1:], true
	default:
		return "", false
	}
}

func leadingHex(s string) string {
	i := 0
	for i < len(s) && isHexDigit(s[i]) {
		i++
	}
	return s[:i]
}

func isHexDigit(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}
```

- [ ] **Step 5: Run the test and watch it pass**

Run: `go test ./internal/checksums -run TestParseGNU -v`
Expected: PASS, all six subtests.

- [ ] **Step 6: Write the failing test for lines that are not checksums**

Append to `internal/checksums/parse_test.go`:

```go
func TestParseGNUBadLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "not a checksum at all", in: "hello world\n"},
		{name: "a digest of an unusable length", in: "deadbeef  a.txt\n"},
		{name: "a separator with no path after it", in: abcSHA256 + "  \n"},
		{name: "a tab is not a separator", in: abcSHA256 + "\ta.txt\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Entries) != 0 {
				t.Fatalf("Parse() Entries = %+v, want none", got.Entries)
			}
			if len(got.Bad) != 1 {
				t.Fatalf("Parse() Bad = %+v, want exactly one", got.Bad)
			}
			if got.Bad[0].Line != 1 {
				t.Errorf("Bad[0].Line = %d, want 1", got.Bad[0].Line)
			}
			if got.Bad[0].Err == nil {
				t.Error("Bad[0].Err = nil, want an error")
			}
		})
	}
}

func TestParseNumbersEveryLine(t *testing.T) {
	in := abcSHA256 + "  a.txt\n" + "garbage\n" + abcSHA256 + "  c.txt\n"

	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Entries) != 2 || got.Entries[0].Line != 1 || got.Entries[1].Line != 3 {
		t.Errorf("entry lines = %+v, want lines 1 and 3", got.Entries)
	}
	if len(got.Bad) != 1 || got.Bad[0].Line != 2 {
		t.Errorf("bad lines = %+v, want line 2", got.Bad)
	}
}
```

- [ ] **Step 7: Run them**

Run: `go test ./internal/checksums -run 'TestParseGNUBadLines|TestParseNumbersEveryLine' -v`
Expected: PASS. The implementation from Step 4 already covers these — if any case fails, the implementation is wrong, not the test.

- [ ] **Step 8: Check the whole gate and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add internal/checksums/parse.go internal/checksums/parse_test.go internal/checksums/checksums.go
git commit -m "feat(checksums): parse the GNU text and binary line formats"
```

---

### Task 2: Parse the BSD tagged format and a bare digest

**Files:**
- Modify: `internal/checksums/parse.go`
- Modify: `internal/checksums/parse_test.go` (append only)

**Interfaces:**
- Consumes: `Parse`, `Listing`, `BadLine` from Task 1; `hash.ParseDigestAs(string, hash.Algorithm) (hash.Digest, error)`.
- Produces: nothing new. `Parse` recognises two more shapes; a bare digest becomes an `Entry` whose `Path` is `""`.

- [ ] **Step 1: Write the failing test for tagged lines**

Append to `internal/checksums/parse_test.go`:

```go
func TestParseTagged(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Entry
	}{
		{
			name: "sha256",
			in:   "SHA256 (dist/a.txt) = " + abcSHA256 + "\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "a lowercase algorithm name",
			in:   "sha256 (a.txt) = " + abcSHA256 + "\n",
			want: Entry{Digest: abcDigest, Path: "a.txt", Line: 1},
		},
		{
			name: "a path containing the separator",
			in:   "SHA256 (a) = b.txt) = " + abcSHA256 + "\n",
			want: Entry{Digest: abcDigest, Path: "a) = b.txt", Line: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Bad) != 0 {
				t.Fatalf("Parse() Bad = %+v, want none", got.Bad)
			}
			if len(got.Entries) != 1 {
				t.Fatalf("Parse() Entries = %+v, want exactly one", got.Entries)
			}
			if got.Entries[0] != tt.want {
				t.Errorf("Parse() entry = %+v, want %+v", got.Entries[0], tt.want)
			}
		})
	}
}

func TestParseTaggedUnknownAlgorithm(t *testing.T) {
	got, err := Parse(strings.NewReader("MD5 (a.txt) = d41d8cd98f00b204e9800998ecf8427e\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Entries) != 0 {
		t.Fatalf("Parse() Entries = %+v, want none", got.Entries)
	}
	if len(got.Bad) != 1 {
		t.Fatalf("Parse() Bad = %+v, want exactly one", got.Bad)
	}
	if !strings.Contains(got.Bad[0].Err.Error(), "md5") {
		t.Errorf("Bad[0].Err = %v, want it to name md5", got.Bad[0].Err)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./internal/checksums -run TestParseTagged -v`
Expected: FAIL. `SHA256 (…) = …` has no leading hex run, so `parseLine` returns `errNotChecksum` and every case reports a `BadLine` where an `Entry` was wanted. `TestParseTaggedUnknownAlgorithm` fails on the message, which is `not a checksum line`.

- [ ] **Step 3: Recognise tagged lines**

In `internal/checksums/parse.go`, add `"regexp"` to the imports, add the pattern, and insert the tagged branch at the top of `parseLine`:

```go
// taggedRE matches the BSD line shape. The path group is greedy so that a
// path containing ") = " still leaves the real separator at the end.
var taggedRE = regexp.MustCompile(`^([A-Za-z0-9-]+) \((.+)\) = ([0-9A-Fa-f]+)$`)
```

```go
func parseLine(line string) (Entry, error) {
	if m := taggedRE.FindStringSubmatch(line); m != nil {
		d, err := hash.ParseDigestAs(m[3], hash.Algorithm(strings.ToLower(m[1])))
		if err != nil {
			return Entry{}, err
		}
		return Entry{Digest: d, Path: m[2]}, nil
	}

	hexRun := leadingHex(line)
	...unchanged from here...
}
```

- [ ] **Step 4: Run them and watch them pass**

Run: `go test ./internal/checksums -run TestParseTagged -v`
Expected: PASS.

- [ ] **Step 5: Write the failing test for a bare digest**

Append to `internal/checksums/parse_test.go`:

```go
// A file whose whole content is a digest names no path. Parse reports that
// as an entry with an empty Path and lets the caller decide what it means.
func TestParseBareDigest(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Entry
	}{
		{
			name: "sha256",
			in:   abcSHA256 + "\n",
			want: Entry{Digest: abcDigest, Line: 1},
		},
		{
			name: "sha512",
			in:   abcSHA512 + "\n",
			want: Entry{
				Digest: hash.Digest{Algorithm: hash.SHA512, Hex: abcSHA512},
				Line:   1,
			},
		},
		{
			name: "no trailing newline",
			in:   abcSHA256,
			want: Entry{Digest: abcDigest, Line: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Bad) != 0 {
				t.Fatalf("Parse() Bad = %+v, want none", got.Bad)
			}
			if len(got.Entries) != 1 {
				t.Fatalf("Parse() Entries = %+v, want exactly one", got.Entries)
			}
			if got.Entries[0] != tt.want {
				t.Errorf("Parse() entry = %+v, want %+v", got.Entries[0], tt.want)
			}
		})
	}
}
```

`abcSHA512` does not exist in this package's tests yet. Add it to `parse_test.go` as a new declaration — do not touch the existing const block in `checksums_test.go`:

```go
// sha512 of "abc", from FIPS 180-4.
const abcSHA512 = "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a" +
	"2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"
```

- [ ] **Step 6: Run it and watch it fail**

Run: `go test ./internal/checksums -run TestParseBareDigest -v`
Expected: FAIL. `gnuPath("")` returns false, so a lone digest is reported as `not a checksum line`.

- [ ] **Step 7: Recognise a lone digest**

In `parseLine`, between the `hexRun == ""` guard and the `gnuPath` call:

```go
	// A line that is nothing but a digest names no file; the caller has to.
	if len(line) == len(hexRun) {
		d, err := hash.ParseDigest(hexRun)
		if err != nil {
			return Entry{}, err
		}
		return Entry{Digest: d}, nil
	}
```

- [ ] **Step 8: Run it and watch it pass**

Run: `go test ./internal/checksums -run TestParseBareDigest -v`
Expected: PASS.

- [ ] **Step 9: Check the whole gate and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add internal/checksums/parse.go internal/checksums/parse_test.go
git commit -m "feat(checksums): parse the tagged format and a lone digest"
```

---

### Task 3: Escaped paths, CRLF, comments, and the round trip

**Files:**
- Modify: `internal/checksums/parse.go`
- Modify: `internal/checksums/parse_test.go` (append only)

**Interfaces:**
- Consumes: `Parse` from Tasks 1-2; `Render(io.Writer, Entry, Format) error` and the `Text` / `Binary` formats from `checksums.go`.
- Produces: nothing new. `Parse` now undoes what `Render` escapes, so the two round-trip.

- [ ] **Step 1: Write the failing test**

Append to `internal/checksums/parse_test.go`:

```go
// Render marks an escaped line with a leading backslash and writes \\ for a
// backslash and \n for a newline. Parse has to undo exactly that and no more.
func TestParseUnescapesPaths(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a backslash in the path",
			in:   `\` + abcSHA256 + `  dist\\a.txt` + "\n",
			want: `dist\a.txt`,
		},
		{
			name: "a newline in the path",
			in:   `\` + abcSHA256 + `  a\nb` + "\n",
			want: "a\nb",
		},
		{
			name: "an escaped backslash is not the start of an escaped newline",
			in:   `\` + abcSHA256 + `  a\\nb` + "\n",
			want: `a\nb`,
		},
		{
			name: "an unmarked line is literal",
			in:   abcSHA256 + `  a\nb` + "\n",
			want: `a\nb`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Entries) != 1 {
				t.Fatalf("Parse() Entries = %+v, Bad = %+v, want one entry", got.Entries, got.Bad)
			}
			if got.Entries[0].Path != tt.want {
				t.Errorf("path = %q, want %q", got.Entries[0].Path, tt.want)
			}
		})
	}
}

// The two halves of this package have to agree, so prove it rather than
// asserting each side's idea of the line shape separately.
func TestRenderParseRoundTrip(t *testing.T) {
	paths := []string{
		"dist/a.txt",
		`dist\a.txt`,
		"a\nb",
		`a\nb`,
		"*starts-with-an-asterisk",
		"  starts-with-spaces",
		"ends-with-spaces  ",
	}

	for _, format := range []Format{Text, Binary} {
		for _, p := range paths {
			t.Run(p, func(t *testing.T) {
				var buf bytes.Buffer
				if err := Render(&buf, Entry{Digest: abcDigest, Path: p}, format); err != nil {
					t.Fatalf("Render() error = %v", err)
				}
				got, err := Parse(&buf)
				if err != nil {
					t.Fatalf("Parse() error = %v", err)
				}
				if len(got.Entries) != 1 {
					t.Fatalf("Parse() Entries = %+v, Bad = %+v, want one entry", got.Entries, got.Bad)
				}
				if got.Entries[0].Path != p {
					t.Errorf("round trip gave %q, want %q", got.Entries[0].Path, p)
				}
			})
		}
	}
}

func TestParseSkipsNoise(t *testing.T) {
	in := "# a header comment\r\n" +
		"\r\n" +
		abcSHA256 + "  a.txt\r\n" +
		"   \n" +
		"  # an indented comment\n" +
		"SHA512 (b.txt) = " + abcSHA512 + "\n"

	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Bad) != 0 {
		t.Fatalf("Parse() Bad = %+v, want none", got.Bad)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("Parse() Entries = %+v, want two", got.Entries)
	}
	if got.Entries[0].Path != "a.txt" || got.Entries[0].Digest.Algorithm != hash.SHA256 {
		t.Errorf("first entry = %+v, want a.txt as sha256", got.Entries[0])
	}
	if got.Entries[1].Path != "b.txt" || got.Entries[1].Digest.Algorithm != hash.SHA512 {
		t.Errorf("second entry = %+v, want b.txt as sha512", got.Entries[1])
	}
}

func TestParseEmptyFile(t *testing.T) {
	got, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Entries) != 0 || len(got.Bad) != 0 {
		t.Errorf("Parse() = %+v, want an empty listing", got)
	}
}
```

Add `"bytes"` to the imports of `parse_test.go`. Insert it as a whole new line inside the existing block — rewriting the block counts as editing existing test text, and `.claude/hooks/test-guard.sh` will stop you for permission.

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./internal/checksums -run 'TestParseUnescapesPaths|TestRenderParseRoundTrip|TestParseSkipsNoise|TestParseEmptyFile' -v`
Expected: `TestParseUnescapesPaths` FAILs — the leading `\` is not stripped, so `leadingHex` returns `""` and the line is bad. `TestRenderParseRoundTrip` FAILs on the escaped paths for the same reason. `TestParseSkipsNoise` and `TestParseEmptyFile` should already PASS; if they do not, the Task 1 implementation is wrong.

- [ ] **Step 3: Undo the escapes**

In `internal/checksums/parse.go`, add the replacer beside the existing `escaper` idea and consume the marker at the top of `parseLine`:

```go
// unescaper undoes what escaper writes. Replacer makes one left-to-right pass
// with non-overlapping matches, so the two characters of an escaped backslash
// are never re-read as the start of an escaped newline.
var unescaper = strings.NewReplacer(`\\`, `\`, `\n`, "\n")
```

```go
func parseLine(line string) (Entry, error) {
	if m := taggedRE.FindStringSubmatch(line); m != nil {
		...unchanged...
	}

	// A leading backslash is how coreutils marks a line whose path was
	// escaped. Tagged lines never carry it: Render refuses such a path there.
	escaped := strings.HasPrefix(line, `\`)
	if escaped {
		line = line[1:]
	}

	hexRun := leadingHex(line)
	...
```

and just before the final `return`, after `path` is known good:

```go
	if escaped {
		path = unescaper.Replace(path)
	}
	return Entry{Digest: d, Path: path}, nil
```

- [ ] **Step 4: Run them and watch them pass**

Run: `go test ./internal/checksums -v`
Expected: PASS, including every test that existed before this task.

- [ ] **Step 5: Check the whole gate and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add internal/checksums/parse.go internal/checksums/parse_test.go
git commit -m "feat(checksums): unescape paths so render and parse round-trip"
```

---

### Task 4: Verify every entry in a checksum file

**Files:**
- Modify: `internal/run/verify.go`
- Create: `internal/run/sums.go`
- Create: `internal/run/sums_test.go`

**Interfaces:**
- Consumes: `checksums.Parse`, `checksums.Listing`, `checksums.Entry` from Tasks 1-3; `hash.Digest`, `hash.Sum`; the existing `MismatchError` and `MissingTargetError`.
- Produces: `run.VerifySums(out, errOut io.Writer, opts SumsOptions) error`, `run.SumsOptions{SumsFile string; Paths []string}`, `run.VerifyErrors{Checked int; Errs []error}` with `Error() string` and `Unwrap() []error`, and the unexported `verifyEntry(out, errOut io.Writer, path string, expected hash.Digest) error`.

- [ ] **Step 1: Refactor — split the verdict out of `Verify`**

No new test: the existing cases in `internal/run/verify_test.go` cover this exactly, and they must stay green with no edit. In `internal/run/verify.go`, replace the body of `Verify` after `parseExpected` with a call, and move what was there into a new function:

```go
// Verify streams the file at opts.Path through the hasher and reports whether
// its digest matches opts.Expected.
func Verify(out, errOut io.Writer, opts VerifyOptions) error {
	expected, err := parseExpected(opts)
	if err != nil {
		return fmt.Errorf("verify %s: %w", opts.Path, err)
	}
	return verifyEntry(out, errOut, opts.Path, expected)
}

// verifyEntry hashes one file and reports whether it matches. Every verdict a
// verification prints leaves through here, so a later change to how a verdict
// looks has one place to go.
func verifyEntry(out, errOut io.Writer, path string, expected hash.Digest) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &MissingTargetError{Path: path, Err: err}
		}
		return err
	}
	defer f.Close()

	actual, err := hash.Sum(f, expected.Algorithm)
	if err != nil {
		return err
	}

	if !actual.Equal(expected) {
		fmt.Fprintf(out, "%s: FAILED\n", path)
		fmt.Fprintf(errOut, "expected: %s\n", expected.Hex)
		fmt.Fprintf(errOut, "actual:   %s\n", actual.Hex)
		return &MismatchError{Path: path, Expected: expected, Actual: actual}
	}

	fmt.Fprintf(out, "%s: OK\n", path)
	return nil
}
```

- [ ] **Step 2: Prove the refactor changed nothing**

Run: `go test ./...`
Expected: PASS, with no test file touched. `git diff --stat` must show `internal/run/verify.go` and nothing else.

- [ ] **Step 3: Commit the refactor on its own**

```bash
git add internal/run/verify.go
git commit -m "refactor(run): split the per-file verdict out of Verify"
```

- [ ] **Step 4: Write the failing test**

Create `internal/run/sums_test.go`. `abcSHA256` and `abcSHA512` are already declared in `verify_test.go` in this same package — reuse them.

```go
package run

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeIn puts contents at dir/name and returns the full path, making any
// parent directories the name asks for.
func writeIn(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// The temp directory is never the working directory, so a run that finds
// these files has resolved them against the checksum file, not against cwd.
func TestVerifySumsEveryEntry(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	writeIn(t, dir, "nested/b.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS",
		abcSHA256+"  a.txt\n"+abcSHA256+"  nested/b.txt\n")

	var out, errOut bytes.Buffer
	if err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sums}); err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}

	want := filepath.Join(dir, "a.txt") + ": OK\n" +
		filepath.Join(dir, "nested/b.txt") + ": OK\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if errOut.String() != "" {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
}
```

- [ ] **Step 5: Run it and watch it fail**

Run: `go test ./internal/run -run TestVerifySumsEveryEntry`
Expected: FAIL to build — `undefined: VerifySums`, `undefined: SumsOptions`.

- [ ] **Step 6: Write the minimal implementation**

Create `internal/run/sums.go`:

```go
package run

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/hash"
)

// SumsOptions is one request to verify against a checksum file. An empty
// Paths means every entry the file lists.
type SumsOptions struct {
	SumsFile string
	Paths    []string
}

// VerifyErrors reports every file that failed in one run. Errs is never empty.
type VerifyErrors struct {
	Checked int
	Errs    []error
}

func (e *VerifyErrors) Error() string {
	return fmt.Sprintf("%d of %d files failed", len(e.Errs), e.Checked)
}

func (e *VerifyErrors) Unwrap() []error { return e.Errs }

// target is one file to verify and the digest it has to have.
type target struct {
	path   string
	digest hash.Digest
}

// VerifySums verifies the files a checksum file lists. A mismatch does not
// stop the run: every file is reported, and the returned error says how many
// failed.
func VerifySums(out, errOut io.Writer, opts SumsOptions) error {
	f, err := os.Open(opts.SumsFile)
	if err != nil {
		return err
	}
	defer f.Close()

	listing, err := checksums.Parse(f)
	if err != nil {
		return err
	}

	targets, err := selectTargets(errOut, listing, opts)
	if err != nil {
		return err
	}

	var errs []error
	for _, t := range targets {
		if err := verifyEntry(out, errOut, t.path, t.digest); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	// One file, one verdict: nothing to summarise, and the error stays the
	// same shape a `verify <file> <checksum>` run would have returned.
	if len(targets) == 1 {
		return errs[0]
	}
	return &VerifyErrors{Checked: len(targets), Errs: errs}
}

// selectTargets works out which files the listing asks for. Entries are
// relative to the file that lists them, so the command works from anywhere.
func selectTargets(errOut io.Writer, listing checksums.Listing, opts SumsOptions) ([]target, error) {
	base := filepath.Dir(opts.SumsFile)
	targets := make([]target, 0, len(listing.Entries))
	for _, e := range listing.Entries {
		targets = append(targets, target{
			path:   filepath.Join(base, e.Path),
			digest: e.Digest,
		})
	}
	return targets, nil
}
```

- [ ] **Step 7: Run it and watch it pass**

Run: `go test ./internal/run -run TestVerifySumsEveryEntry -v`
Expected: PASS.

- [ ] **Step 8: Write the failing test for carrying on past a failure**

Append to `internal/run/sums_test.go`, and widen its import block to
`"bytes"`, `"errors"`, `"io"`, `"os"`, `"path/filepath"`, `"strings"`,
`"testing"` — Go rejects an import nothing uses, so they arrive with the
tests that need them. Insert the three new ones as whole new lines; rewriting
the block counts as editing existing test text and the test guard will stop
you for permission:

```go
func TestVerifySumsKeepsGoingAfterAFailure(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	writeIn(t, dir, "b.txt", "not abc")
	writeIn(t, dir, "c.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS",
		abcSHA256+"  a.txt\n"+abcSHA256+"  b.txt\n"+abcSHA256+"  c.txt\n"+
			abcSHA256+"  gone.txt\n")

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sums})

	var multi *VerifyErrors
	if !errors.As(err, &multi) {
		t.Fatalf("VerifySums() error = %v, want a *VerifyErrors", err)
	}
	if multi.Checked != 4 {
		t.Errorf("Checked = %d, want 4", multi.Checked)
	}
	if len(multi.Errs) != 2 {
		t.Fatalf("Errs = %v, want two", multi.Errs)
	}
	var mismatch *MismatchError
	if !errors.As(multi.Errs[0], &mismatch) {
		t.Errorf("first failure = %v, want a *MismatchError", multi.Errs[0])
	}
	var missing *MissingTargetError
	if !errors.As(multi.Errs[1], &missing) {
		t.Errorf("second failure = %v, want a *MissingTargetError", multi.Errs[1])
	}

	// The run reported every file, not just the ones up to the first failure.
	for _, want := range []string{
		filepath.Join(dir, "a.txt") + ": OK",
		filepath.Join(dir, "b.txt") + ": FAILED",
		filepath.Join(dir, "c.txt") + ": OK",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout = %q, want it to contain %q", out.String(), want)
		}
	}
}

// A checksum file that is not there is a command that cannot run, not a
// verification that failed, so it must not look like a missing target.
func TestVerifySumsMissingSumsFile(t *testing.T) {
	err := VerifySums(io.Discard, io.Discard, SumsOptions{
		SumsFile: filepath.Join(t.TempDir(), "SHA256SUMS"),
	})
	if err == nil {
		t.Fatal("VerifySums() = nil error, want one")
	}
	var missing *MissingTargetError
	if errors.As(err, &missing) {
		t.Errorf("error = %v, want it not to be a *MissingTargetError", err)
	}
	var multi *VerifyErrors
	if errors.As(err, &multi) {
		t.Errorf("error = %v, want it not to be a *VerifyErrors", err)
	}
}
```

- [ ] **Step 9: Run them**

Run: `go test ./internal/run -run 'TestVerifySumsKeepsGoingAfterAFailure|TestVerifySumsMissingSumsFile' -v`
Expected: PASS. Step 6 already implements this — if either fails, the implementation is wrong.

- [ ] **Step 10: Check the whole gate and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add internal/run/sums.go internal/run/sums_test.go
git commit -m "feat(run): verify the files a checksum file lists"
```

---

### Task 5: Filter the entries with positional arguments

**Files:**
- Modify: `internal/run/sums.go` — `selectTargets`
- Modify: `internal/run/sums_test.go` (append only)

**Interfaces:**
- Consumes: `VerifySums`, `SumsOptions`, `VerifyErrors`, `target`, `selectTargets` from Task 4.
- Produces: nothing new. `SumsOptions.Paths` now selects entries.

- [ ] **Step 1: Write the failing test**

Append to `internal/run/sums_test.go`:

```go
func TestVerifySumsFiltersByArgument(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	writeIn(t, dir, "b.txt", "abc")
	writeIn(t, dir, "c.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS",
		abcSHA256+"  a.txt\n"+abcSHA256+"  b.txt\n"+abcSHA256+"  c.txt\n")

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile: sums,
		// Out of file order, and one of them spelled differently, to show
		// arguments drive the order and are matched after cleaning.
		Paths: []string{"c.txt", "./a.txt"},
	})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}

	want := filepath.Join(dir, "c.txt") + ": OK\n" + filepath.Join(dir, "a.txt") + ": OK\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// Naming a file the checksum file says nothing about is a wrong command, not
// a failed check, so it must not look like a mismatch or a missing target.
func TestVerifySumsArgumentWithNoEntry(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS", abcSHA256+"  a.txt\n")

	err := VerifySums(io.Discard, io.Discard, SumsOptions{
		SumsFile: sums,
		Paths:    []string{"b.txt"},
	})
	if err == nil {
		t.Fatal("VerifySums() = nil error, want one")
	}
	if !strings.Contains(err.Error(), "b.txt") {
		t.Errorf("error = %v, want it to name b.txt", err)
	}
	var mismatch *MismatchError
	var missing *MissingTargetError
	var multi *VerifyErrors
	if errors.As(err, &mismatch) || errors.As(err, &missing) || errors.As(err, &multi) {
		t.Errorf("error = %v, want a plain error", err)
	}
}

// Filtering down to one file must give back exactly what verifying that one
// file inline would have: the bare error, with no summary wrapped around it.
// The file lists two entries on purpose — with only one, the run would return
// the bare error whether or not the filter worked, and the test would prove
// nothing.
func TestVerifySumsOneTargetReturnsItsOwnError(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "not abc")
	writeIn(t, dir, "b.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS", abcSHA256+"  a.txt\n"+abcSHA256+"  b.txt\n")

	err := VerifySums(io.Discard, io.Discard, SumsOptions{
		SumsFile: sums,
		Paths:    []string{"a.txt"},
	})
	var multi *VerifyErrors
	if errors.As(err, &multi) {
		t.Fatalf("error = %v, want it not to be wrapped in a *VerifyErrors", err)
	}
	var mismatch *MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %v, want a *MismatchError", err)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./internal/run -run 'TestVerifySumsFiltersByArgument|TestVerifySumsArgumentWithNoEntry|TestVerifySumsOneTargetReturnsItsOwnError' -v`
Expected: `TestVerifySumsFiltersByArgument` FAILs — `selectTargets` ignores `Paths`, so all three files are reported in file order. `TestVerifySumsArgumentWithNoEntry` FAILs with a nil error. `TestVerifySumsOneTargetReturnsItsOwnError` FAILs because both of its entries are verified rather than the one asked for, so the run wraps them in a `*VerifyErrors`.

- [ ] **Step 3: Write the minimal implementation**

Replace `selectTargets` in `internal/run/sums.go`:

```go
// selectTargets works out which files the listing asks for. Entries are
// relative to the file that lists them, so the command works from anywhere.
func selectTargets(errOut io.Writer, listing checksums.Listing, opts SumsOptions) ([]target, error) {
	base := filepath.Dir(opts.SumsFile)

	if len(opts.Paths) == 0 {
		targets := make([]target, 0, len(listing.Entries))
		for _, e := range listing.Entries {
			targets = append(targets, target{
				path:   filepath.Join(base, e.Path),
				digest: e.Digest,
			})
		}
		return targets, nil
	}

	// Arguments name entries as the file spells them, so the lookup is on the
	// listed path, not on anything resolved against the working directory.
	byPath := make(map[string]checksums.Entry, len(listing.Entries))
	for _, e := range listing.Entries {
		byPath[filepath.Clean(e.Path)] = e
	}

	targets := make([]target, 0, len(opts.Paths))
	for _, p := range opts.Paths {
		e, ok := byPath[filepath.Clean(p)]
		if !ok {
			return nil, fmt.Errorf("%s: no entry for %s", opts.SumsFile, p)
		}
		targets = append(targets, target{
			path:   filepath.Join(base, e.Path),
			digest: e.Digest,
		})
	}
	return targets, nil
}
```

- [ ] **Step 4: Run them and watch them pass**

Run: `go test ./internal/run -v`
Expected: PASS, including every test that existed before this task.

- [ ] **Step 5: Check the whole gate and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add internal/run/sums.go internal/run/sums_test.go
git commit -m "feat(run): let arguments pick which entries to verify"
```

---

### Task 6: Bare-digest files, pathless entries, and unusable files

**Files:**
- Modify: `internal/run/sums.go` — `VerifySums` and `selectTargets`
- Modify: `internal/run/sums_test.go` (append only)

**Interfaces:**
- Consumes: everything from Tasks 4-5; `checksums.Listing.Bad` and `checksums.BadLine{Line, Err}` from Task 1.
- Produces: nothing new. `VerifySums` now reports malformed lines, handles a bare-digest file, and rejects a file it cannot use.

- [ ] **Step 1: Write the failing test**

Append to `internal/run/sums_test.go`:

```go
// A file whose whole content is a digest names no file, so the caller must.
func TestVerifySumsBareDigest(t *testing.T) {
	dir := t.TempDir()
	target := writeIn(t, dir, "dist.tar.gz", "abc")
	sums := writeIn(t, dir, "dist.tar.gz.sha256", abcSHA256+"\n")

	var out, errOut bytes.Buffer
	if err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sums, Paths: []string{target}}); err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := target + ": OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

func TestVerifySumsBareDigestNeedsExactlyOneArgument(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "dist.tar.gz", "abc")
	sums := writeIn(t, dir, "dist.tar.gz.sha256", abcSHA256+"\n")

	tests := []struct {
		name  string
		paths []string
	}{
		{name: "none", paths: nil},
		{name: "two", paths: []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifySums(io.Discard, io.Discard, SumsOptions{SumsFile: sums, Paths: tt.paths})
			if err == nil {
				t.Fatal("VerifySums() = nil error, want one")
			}
			if !strings.Contains(err.Error(), sums) {
				t.Errorf("error = %v, want it to name the checksum file", err)
			}
		})
	}
}

// A stray pathless digest does not turn a listing into a bare-digest file.
// It is reported like a malformed line and skipped.
func TestVerifySumsPathlessEntryAmongMany(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	writeIn(t, dir, "b.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS",
		abcSHA256+"  a.txt\n"+abcSHA256+"\n"+abcSHA256+"  b.txt\n")

	var out, errOut bytes.Buffer
	if err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sums}); err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if !strings.Contains(errOut.String(), sums+":2:") {
		t.Errorf("stderr = %q, want it to name line 2 of the checksum file", errOut.String())
	}
	want := filepath.Join(dir, "a.txt") + ": OK\n" + filepath.Join(dir, "b.txt") + ": OK\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// A line that is not a checksum is warned about and skipped; it does not by
// itself make the run fail.
func TestVerifySumsWarnsAboutMalformedLines(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS", "not a checksum\n"+abcSHA256+"  a.txt\n")

	var out, errOut bytes.Buffer
	if err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sums}); err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if !strings.Contains(errOut.String(), sums+":1:") {
		t.Errorf("stderr = %q, want it to name line 1 of the checksum file", errOut.String())
	}
	if want := filepath.Join(dir, "a.txt") + ": OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// A file with nothing usable in it is a command that cannot run.
func TestVerifySumsNoUsableLines(t *testing.T) {
	dir := t.TempDir()
	sums := writeIn(t, dir, "SHA256SUMS", "not a checksum\n# nor this\n")

	err := VerifySums(io.Discard, io.Discard, SumsOptions{SumsFile: sums})
	if err == nil {
		t.Fatal("VerifySums() = nil error, want one")
	}
	if !strings.Contains(err.Error(), "no checksum lines") {
		t.Errorf("error = %v, want it to say no checksum lines were found", err)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./internal/run -run 'TestVerifySumsBareDigest|TestVerifySumsBareDigestNeedsExactlyOneArgument|TestVerifySumsPathlessEntryAmongMany|TestVerifySumsWarnsAboutMalformedLines|TestVerifySumsNoUsableLines' -v`
Expected: every parent test FAILs. A pathless entry currently becomes `filepath.Join(base, "")`, which is the directory itself, so the bare cases verify the wrong thing; nothing warns about `Bad` lines; and an unusable file returns nil rather than an error. One subtest is already green: `TestVerifySumsBareDigestNeedsExactlyOneArgument/two` passes under Task 5's code, which files the pathless entry under the key `"."` and so reports `no entry for a`. Its sibling `none` is what makes the parent red.

- [ ] **Step 3: Write the minimal implementation**

In `internal/run/sums.go`, add the one place a checksum file's own lines are complained about — the spec keeps these together so that colouring them later is a change to this function and nothing else:

```go
// warnLine reports a line of a checksum file that could not be used. Every
// such warning leaves through here.
func warnLine(errOut io.Writer, file string, line int, msg string) {
	fmt.Fprintf(errOut, "%s:%d: %s\n", file, line, msg)
}
```

Then report the bad lines inside `VerifySums`, immediately after `checksums.Parse` returns:

```go
	for _, b := range listing.Bad {
		warnLine(errOut, opts.SumsFile, b.Line, b.Err.Error())
	}
```

Then replace `selectTargets` with the version that decides the mode first:

```go
// selectTargets works out which files the listing asks for. The mode is a
// property of the whole listing, not of any one line: a single pathless entry
// is a bare-digest file, and a stray one among many is just a broken line.
func selectTargets(errOut io.Writer, listing checksums.Listing, opts SumsOptions) ([]target, error) {
	if len(listing.Entries) == 1 && listing.Entries[0].Path == "" {
		if len(opts.Paths) == 0 {
			return nil, fmt.Errorf(
				"%s: no paths in file; name the file to verify", opts.SumsFile)
		}
		if len(opts.Paths) > 1 {
			return nil, fmt.Errorf(
				"%s: holds one checksum; name exactly one file", opts.SumsFile)
		}
		return []target{{path: opts.Paths[0], digest: listing.Entries[0].Digest}}, nil
	}

	named := make([]checksums.Entry, 0, len(listing.Entries))
	for _, e := range listing.Entries {
		if e.Path == "" {
			warnLine(errOut, opts.SumsFile, e.Line, "checksum without a path")
			continue
		}
		named = append(named, e)
	}
	if len(named) == 0 {
		return nil, fmt.Errorf("%s: no checksum lines found", opts.SumsFile)
	}

	base := filepath.Dir(opts.SumsFile)

	if len(opts.Paths) == 0 {
		targets := make([]target, 0, len(named))
		for _, e := range named {
			targets = append(targets, target{
				path:   filepath.Join(base, e.Path),
				digest: e.Digest,
			})
		}
		return targets, nil
	}

	// Arguments name entries as the file spells them, so the lookup is on the
	// listed path, not on anything resolved against the working directory.
	byPath := make(map[string]checksums.Entry, len(named))
	for _, e := range named {
		byPath[filepath.Clean(e.Path)] = e
	}

	targets := make([]target, 0, len(opts.Paths))
	for _, p := range opts.Paths {
		e, ok := byPath[filepath.Clean(p)]
		if !ok {
			return nil, fmt.Errorf("%s: no entry for %s", opts.SumsFile, p)
		}
		targets = append(targets, target{
			path:   filepath.Join(base, e.Path),
			digest: e.Digest,
		})
	}
	return targets, nil
}
```

A bare-digest file's argument is a real path from the working directory, so it is used as given — that is the one place `base` does not apply, because the file says nothing about where its subject lives.

- [ ] **Step 4: Run them and watch them pass**

Run: `go test ./internal/run -v`
Expected: PASS, including every earlier test.

- [ ] **Step 5: Check the whole gate and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add internal/run/sums.go internal/run/sums_test.go
git commit -m "feat(run): handle bare digests and unusable checksum files"
```

---

### Task 7: One exit code for many verdicts

**Files:**
- Modify: `cmd/exit.go` — `exitCode`
- Modify: `cmd/root.go` — `execute`
- Create: `cmd/aggregate_test.go`

**Interfaces:**
- Consumes: `run.VerifyErrors{Checked, Errs}` with `Unwrap() []error` from Task 4; the existing `run.MismatchError` and `run.MissingTargetError`.
- Produces: nothing new. `exitCode` and `execute` both understand an aggregate.

- [ ] **Step 1: Write the failing test**

Create `cmd/aggregate_test.go`:

```go
package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/BobMali/ldsum/internal/run"
	"github.com/spf13/cobra"
)

func TestExitCodeAggregates(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "mismatches alone",
			err: &run.VerifyErrors{Checked: 3, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				&run.MismatchError{Path: "b.txt"},
			}},
			want: 1,
		},
		{
			name: "a missing target among mismatches",
			err: &run.VerifyErrors{Checked: 2, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				&run.MissingTargetError{Path: "b.txt", Err: fs.ErrNotExist},
			}},
			want: 1,
		},
		{
			name: "one unreadable file outweighs any number of mismatches",
			err: &run.VerifyErrors{Checked: 2, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				fmt.Errorf("read b.txt: %w", fs.ErrPermission),
			}},
			want: 2,
		},
		{
			name: "an aggregate holding nothing is still a failure",
			err:  &run.VerifyErrors{},
			want: 2,
		},
		{
			name: "wrapped in context",
			err: fmt.Errorf("verify: %w", &run.VerifyErrors{Checked: 2, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				errors.New("read failed"),
			}}),
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCode(tt.err); got != tt.want {
				t.Errorf("exitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// errors.As walks Unwrap() []error, so an aggregate holding a mismatch also
// matches the arm whose whole job is to stay silent. Without the ordering in
// execute, the summary vanishes in the commonest failure there is.
func TestExecutePrintsTheAggregateSummary(t *testing.T) {
	root := &cobra.Command{Use: "ldsum", SilenceErrors: true}
	root.AddCommand(&cobra.Command{
		Use:          "stub",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return &run.VerifyErrors{Checked: 3, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				&run.MismatchError{Path: "b.txt"},
			}}
		},
	})
	var errOut bytes.Buffer
	root.SetOut(io.Discard)
	root.SetErr(&errOut)
	root.SetArgs([]string{"stub"})

	if code := execute(root); code != 1 {
		t.Errorf("execute() = %d, want 1", code)
	}
	if want := "ldsum: 2 of 3 files failed\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./cmd -run 'TestExitCodeAggregates|TestExecutePrintsTheAggregateSummary' -v`
Expected: FAIL. `exitCode` reaches its `errors.As(err, &mismatch)` arm through the aggregate's `Unwrap`, so the unreadable-file case returns 1 instead of 2 and the empty aggregate returns 2 only by accident of ordering. `TestExecutePrintsTheAggregateSummary` fails with an empty stderr, which is the bug this ordering exists to prevent.

- [ ] **Step 3: Make `exitCode` unwrap the aggregate first**

Replace `exitCode` in `cmd/exit.go`. The existing comment about order being unobservable goes: this is the change it was anticipating.

```go
// exitCode maps an error to a process status: 1 for a verification the user
// must act on, 2 for a command that was wrong to run.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	// Tested before the single-error arms, not after: errors.As walks
	// Unwrap() []error, so an aggregate holding a mismatch matches those arms
	// too and would report 1 however bad the rest of the run went.
	var multi *run.VerifyErrors
	if errors.As(err, &multi) {
		worst := 0
		for _, e := range multi.Errs {
			if code := exitCode(e); code > worst {
				worst = code
			}
		}
		if worst == 0 {
			// A non-nil error must never report success.
			return 2
		}
		return worst
	}
	var (
		mismatch *run.MismatchError
		missing  *run.MissingTargetError
	)
	switch {
	case errors.As(err, &mismatch):
		return 1
	case errors.As(err, &missing):
		return 1
	default:
		return 2
	}
}
```

- [ ] **Step 4: Make `execute` print the summary**

Replace the body of `execute` in `cmd/root.go`:

```go
// execute runs an already-built tree and maps its error to an exit code. It
// reports the error itself, except for a single mismatch, whose detail run
// has already printed. Tests build their own tree so they can capture the
// streams.
func execute(root *cobra.Command) int {
	err := root.Execute()
	if err != nil {
		var (
			mismatch *run.MismatchError
			multi    *run.VerifyErrors
		)
		// An aggregate is checked too, because errors.As walks Unwrap()
		// []error: one holding a mismatch would otherwise take the silent
		// path and lose the only line that says how many files failed.
		silent := errors.As(err, &mismatch) && !errors.As(err, &multi)
		if !silent {
			fmt.Fprintf(root.ErrOrStderr(), "ldsum: %v\n", err)
		}
	}
	return exitCode(err)
}
```

- [ ] **Step 5: Run the tests and watch them pass**

Run: `go test ./cmd -v`
Expected: PASS, including `TestExitCode` and `TestExecute`, which must be untouched.

- [ ] **Step 6: Check the whole gate and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add cmd/exit.go cmd/root.go cmd/aggregate_test.go
git commit -m "feat(cmd): fold many verdicts into one exit code"
```

---

### Task 8: Wire `--sums-file` onto verify

**Files:**
- Modify: `cmd/verify.go`
- Create: `cmd/sums_test.go`

**Interfaces:**
- Consumes: `run.VerifySums`, `run.SumsOptions` from Tasks 4-6; `execute` and `newRootCmd` from `cmd`.
- Produces: the `--sums-file` / `-c` flag on `ldsum verify`.

- [ ] **Step 1: Write the failing test**

Create `cmd/sums_test.go`. `runCLI` and `abcSHA256` already exist in `verify_test.go` in this same package — reuse them.

```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIn(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// runExit drives a fresh tree through execute, which is where the exit code
// and the top-level message are decided.
func runExit(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	// execute has to run before the buffers are read: a return statement
	// evaluates its expressions in order, so reading them inline gives back
	// two empty strings.
	code = execute(root)
	return out.String(), errOut.String(), code
}

func TestVerifySumsFile(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	writeIn(t, dir, "b.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS", abcSHA256+"  a.txt\n"+abcSHA256+"  b.txt\n")

	stdout, stderr, err := runCLI(t, "verify", "--sums-file", sums)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := filepath.Join(dir, "a.txt") + ": OK\n" + filepath.Join(dir, "b.txt") + ": OK\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestVerifySumsFileShortFlagAndFilter(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	writeIn(t, dir, "b.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS", abcSHA256+"  a.txt\n"+abcSHA256+"  b.txt\n")

	stdout, _, err := runCLI(t, "verify", "-c", sums, "b.txt")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := filepath.Join(dir, "b.txt") + ": OK\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// Without --sums-file the command still takes exactly two arguments; with it,
// any number of them, including none.
func TestVerifyArgumentCounts(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS", abcSHA256+"  a.txt\n")

	if _, _, err := runCLI(t, "verify", "one", "two", "three"); err == nil {
		t.Error("three arguments and no --sums-file: want an argument-count error")
	}
	if _, _, err := runCLI(t, "verify", "-c", sums); err != nil {
		t.Errorf("no arguments with --sums-file: unexpected error %v", err)
	}
}

// The algorithm of a checksum file's entries comes from the file itself, so
// naming one on the command line could only contradict it.
func TestVerifyAlgoAndSumsFileAreExclusive(t *testing.T) {
	dir := t.TempDir()
	sums := writeIn(t, dir, "SHA256SUMS", abcSHA256+"  a.txt\n")

	if _, _, err := runCLI(t, "verify", "--algo", "sha256", "-c", sums); err == nil {
		t.Error("Execute() = nil error, want the flags to be rejected together")
	}
}

func TestVerifySumsFileExitCodes(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "ok.txt", "abc")
	writeIn(t, dir, "bad.txt", "not abc")
	sums := writeIn(t, dir, "SHA256SUMS", abcSHA256+"  ok.txt\n"+abcSHA256+"  bad.txt\n")

	stdout, stderr, code := runExit(t, "verify", "-c", sums)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "bad.txt: FAILED") {
		t.Errorf("stdout = %q, want bad.txt reported FAILED", stdout)
	}
	if !strings.Contains(stderr, "ldsum: 1 of 2 files failed") {
		t.Errorf("stderr = %q, want the summary line", stderr)
	}

	_, _, code = runExit(t, "verify", "-c", filepath.Join(dir, "nope"))
	if code != 2 {
		t.Errorf("missing checksum file: exit code = %d, want 2", code)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./cmd -run 'TestVerifySumsFile|TestVerifyArgumentCounts|TestVerifyAlgoAndSumsFileAreExclusive' -v`
Expected: FAIL — `unknown flag: --sums-file`.

- [ ] **Step 3: Write the minimal implementation**

Replace `newVerifyCmd` in `cmd/verify.go`:

```go
// newVerifyCmd builds the verify subcommand. Every flag binds to a local, so
// two trees in the same process never share them.
func newVerifyCmd() *cobra.Command {
	var (
		algorithm string
		sumsFile  string
	)

	cmd := &cobra.Command{
		Use:   "verify [<file> <checksum>]",
		Short: "Verify a file against an expected checksum",
		Long: `Verify checks files against the checksums they are expected to have.

Given a file and a checksum, it checks that one file. The algorithm is taken
from the length of the checksum — 64 hex characters is sha256, 128 is sha512 —
unless --algo names one.

Given --sums-file, it reads the expected checksums from a checksum file
instead, recognising the GNU text and binary formats, the BSD tagged format,
and a file holding a bare digest. Entries are resolved relative to the
checksum file, so the command works from any directory. Naming files after
the flag checks only those entries; naming none checks them all.

It exits 0 when every digest matched, 1 when one did not or a file is
missing, and 2 when the command itself was wrong.`,
		Args: func(cmd *cobra.Command, args []string) error {
			// With a checksum file the arguments pick entries out of it, so
			// any number is meaningful, including none.
			if sumsFile != "" {
				return nil
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Arguments have parsed by this point, so a later failure is not a usage
			// problem and usage text would only bury the verdict.
			cmd.SilenceUsage = true
			if sumsFile != "" {
				return run.VerifySums(cmd.OutOrStdout(), cmd.ErrOrStderr(), run.SumsOptions{
					SumsFile: sumsFile,
					Paths:    args,
				})
			}
			return run.Verify(cmd.OutOrStdout(), cmd.ErrOrStderr(), run.VerifyOptions{
				Path:      args[0],
				Expected:  args[1],
				Algorithm: algorithm,
			})
		},
	}

	cmd.Flags().StringVar(&algorithm, "algo", "",
		"checksum algorithm: sha256 or sha512 (inferred from the checksum length when omitted)")
	cmd.Flags().StringVarP(&sumsFile, "sums-file", "c", "",
		"read the expected checksums from this file")
	cmd.MarkFlagsMutuallyExclusive("algo", "sums-file")

	return cmd
}
```

- [ ] **Step 4: Run the whole suite and watch it pass**

Run: `go test ./... -v`
Expected: PASS, including `TestVerifyCommand`'s "wrong argument count" case, which still routes through `cobra.ExactArgs(2)` because no `--sums-file` is given.

- [ ] **Step 5: Check the whole gate and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add cmd/verify.go cmd/sums_test.go
git commit -m "feat(cmd): add --sums-file to read expected checksums"
```

---

### Task 9: Say so in the documentation

**Files:**
- Modify: `README.md`
- Modify: `cmd/root.go` — the root `Long` text
- Modify: `CLAUDE.md` — the "What ldsum is", "Layout" and `internal/checksums` lines

**Interfaces:**
- Consumes: the finished behaviour of Tasks 1-8.
- Produces: nothing code depends on.

- [ ] **Step 1: Find every claim that is now false**

Run: `grep -rn 'sums-file\|not yet implemented\|Parsing arrives\|planned' README.md CLAUDE.md cmd/ internal/`
Expected: hits in `README.md` (the status note), `cmd/root.go` (the root `Long`), `CLAUDE.md` (the layout paragraph), and `internal/checksums/checksums.go` if Task 1's package-comment edit was missed.

- [ ] **Step 2: Update `cmd/root.go`**

In the root `Long`, replace the paragraph beginning "verify checks a file":

```
verify checks files against the checksums they are expected to have, given
inline or read from a checksum file such as a published SHA256SUMS. Reading
the file itself from a URL is planned and not yet implemented.
```

- [ ] **Step 3: Update `CLAUDE.md`**

In the Layout paragraph, replace "The `internal/checksums` package renders checksum-file lines; parsing them arrives with `--sums-file`." with:

```
The `internal/checksums` package renders and parses checksum-file lines.
```

In "What ldsum is", the two input axes stay as they are — they described this feature all along. Only the note that checksum-file input is unimplemented, if one is present, comes out.

- [ ] **Step 4: Update `README.md`**

Change the status note to say that `verify` now takes checksum files and that URL input is what remains. Add a section after the inline-checksum examples:

````markdown
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

This is the one place `ldsum` differs from `sha256sum -c`, which resolves
against the working directory.

Naming files checks only those entries, in the order given:

```sh
ldsum verify -c SHA256SUMS dist.tar.gz
```

A file that holds a bare digest names nothing, so name the file yourself:

```sh
ldsum verify -c dist.tar.gz.sha256 dist.tar.gz
```

A mismatch does not stop the run: every file is reported and the summary goes
to stderr.

```sh
ldsum verify -c SHA256SUMS
dist.tar.gz: OK
docs.pdf: FAILED
ldsum: 1 of 2 files failed
```

Lines that are not checksums are named on stderr and skipped; a file with no
usable lines is an error. `--algo` and `--sums-file` cannot be combined —
the file says which algorithm each entry uses.
````

Update the exit-code table's rows to cover several files:

```
| 0 | every digest matched, or every file was summed |
| 1 | a digest did not match, or a file being verified is missing |
| 2 | the command could not be carried out: wrong argument count, bad checksum, unknown algorithm, an unreadable file, an unusable checksum file, an output file that cannot be written, a directory without `-r` |
```

- [ ] **Step 5: Check the gate and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add README.md CLAUDE.md cmd/root.go
git commit -m "docs: describe reading checksums from a checksum file"
```

- [ ] **Step 6: Run the linter if it is installed**

Run: `golangci-lint run`
Expected: clean. It is not required locally — CI runs it — but a finding here is cheaper than one there.
