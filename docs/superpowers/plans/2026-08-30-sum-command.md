# Sum Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `ldsum sum <path>...`, which prints the checksum of files and trees in GNU text, GNU binary, BSD tagged, or bare format.

**Architecture:** A new `internal/checksums` owns the line shapes: a `Format`, an `Entry`, and a `Render` that writes exactly one line. A new `internal/run/sum.go` walks the arguments, streams each regular file through the existing `hash.Sum`, and renders each entry as it is produced. `cmd/sum.go` parses flags and calls `run.Sum`. `internal/hash` gains one four-line predicate and nothing else.

**Tech Stack:** Go 1.27, `spf13/cobra`, standard library only (`crypto/sha256`, `crypto/sha512`, `path/filepath`, `io/fs`).

**Spec:** `docs/superpowers/specs/2026-08-30-sum-command-design.md`

## Global Constraints

- Standard library plus `spf13/cobra` only. Adding any module is a decision to raise with the author first, never an implementation detail.
- Hash algorithms come from `crypto/*`. No third-party hashing.
- `os.Exit` appears only in `main.go`. Everything else returns an `error`.
- Cobra commands parse flags and call `internal/run`. They do no real work.
- Commands write through `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, never `os.Stdout`.
- `internal/hash` and `internal/checksums` take readers and writers, never paths.
- Data to stdout, diagnostics to stderr.
- Exit codes: `0` success, `1` mismatch or missing target file during verify, `2` usage or I/O error. `sum` never produces `1`.
- Errors wrap with context they do not already carry. `os` and `io` return `*fs.PathError`, which already names the operation and the path — do not add a second copy.
- Never read a whole file into memory. Stream through a fixed buffer. `hash.Sum` already does this; do not accumulate rendered lines either.
- Comment only what the code cannot say: why, not what. One or two lines.
- Tests are table-driven with `t.Run` per case. Anything written goes in `t.TempDir()`.
- Digest constants are published FIPS 180-4 values. Never assert against a digest this code produced. The line *shapes* were captured from `shasum` 6.02 and Darwin `/sbin/sha256sum` and are recorded in the spec.
- Definition of done, run and reported rather than reasoned about: `gofmt -l .` prints nothing, `go vet ./...` passes, `go test ./...` passes. `golangci-lint run` must be clean if available locally; CI runs it either way.
- `golangci-lint` enables `perfsprint`: prefer `errors.New` with concatenation over `fmt.Errorf` when the message has no verbs or a single `%s`. `%q` and multi-argument `%d` formats are fine — `internal/hash/hash.go` already uses `fmt.Errorf("unknown algorithm %q: ...", a)` and passes.
- `revive` requires a doc comment on every exported identifier, including each new type and constant block.
- Commit messages are Conventional Commits: `<type>(<scope>): <description>`, lowercase, imperative, no trailing period, <= 66 characters. Scopes here: `hash`, `checksums`, `cmd`, `run`. One line, no body — the diff says the rest. No AI attribution trailers.

## Working with this repo's hooks

Three hooks will interrupt the cycle. None is a failure:

- **`go-check.sh`** runs `gofmt`/`go vet`/`go test ./...` after every file write. Right after you write a failing test it will report that the build and tests fail. **That is the red step's evidence**, not a problem to fix by abandoning the test. Read the message, confirm it failed for the reason the task predicts, then write the implementation.
- **`test-guard.sh`** turns any write to an *existing* `*_test.go` into a permission prompt. Creating a test file is free; appending whole new lines passes through. Several tasks below add a case to a test file an earlier task created — expect a prompt and wait for the answer. Never work around it, and never edit a test to make a red test pass.
- **`commit-guard.sh`** checks the commit message. A rejection is a message problem, never a reason for `--no-verify`.

## File Structure

| File | Responsibility |
|------|----------------|
| `internal/checksums/checksums.go` | Create. `Format`, `Entry`, `Render`, and the unexported escaping helpers. Knows line shapes; knows nothing about files. |
| `internal/checksums/checksums_test.go` | Create. Every format, every escaping case. |
| `internal/hash/hash.go` | Modify. Add `Supported(Algorithm) bool`. Nothing else changes. |
| `internal/hash/hash_test.go` | Modify. One test for `Supported`. |
| `internal/run/sum.go` | Create. `SumOptions`, `Sum`, and the unexported `sumPath` / `sumFile` / `walkDir`. |
| `internal/run/sum_test.go` | Create. Named files, missing files, directories, walking, skipping, verbose. |
| `cmd/sum.go` | Create. Flag wiring for the `sum` subcommand. |
| `cmd/sum_test.go` | Create. Flag behaviour and exit codes through the real command tree. |
| `cmd/root.go` | Modify. Register `newSumCmd()`; widen `Short` and `Long`. |
| `README.md` | Modify. Status note, `sum` usage, format table. |

## A clarification the spec leaves implicit

The spec says symlinks are never followed. That governs entries **discovered by walking**. A symlink named directly as an argument is followed: `sumPath` calls `os.Stat`, which resolves the link, so `ldsum sum link.txt` hashes what the link points at. This is what any caller means when they name a path, and it is how `sha256sum` behaves. Inside `walkDir`, `d.Type().IsRegular()` is consulted instead, which does not resolve links, so a discovered symlink is skipped. Both behaviours are tested.

---

### Task 1: Render GNU text and binary lines

**Files:**
- Create: `internal/checksums/checksums.go`
- Test: `internal/checksums/checksums_test.go`

**Interfaces:**
- Consumes: `hash.Digest` from `internal/hash` — a struct with `Algorithm Algorithm` and `Hex string`.
- Produces: `checksums.Format` (an int enum), `checksums.Entry{Digest hash.Digest; Path string}`, and `checksums.Render(w io.Writer, e Entry, f Format) error`. Later tasks call `Render` once per file.

- [ ] **Step 1: Write the failing test**

Create `internal/checksums/checksums_test.go`:

```go
package checksums

import (
	"bytes"
	"testing"

	"github.com/BobMali/ldsum/internal/hash"
)

// sha256 of "abc", from FIPS 180-4. The line shapes around it were captured
// from shasum 6.02 and Darwin /sbin/sha256sum, not from this package.
const abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

func abcEntry(path string) Entry {
	return Entry{
		Digest: hash.Digest{Algorithm: hash.SHA256, Hex: abcSHA256},
		Path:   path,
	}
}

func TestRenderGNU(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		path   string
		want   string
	}{
		{
			name:   "text is two spaces",
			format: Text,
			path:   "dist/a.txt",
			want:   abcSHA256 + "  dist/a.txt\n",
		},
		{
			name:   "binary is space then asterisk",
			format: Binary,
			path:   "dist/a.txt",
			want:   abcSHA256 + " *dist/a.txt\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := Render(&out, abcEntry(tt.path), tt.format); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("Render() = %q, want %q", out.String(), tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./internal/checksums/ -run TestRenderGNU -v`
Expected: FAIL — the package does not compile, because `Entry`, `Format`, `Text`, `Binary` and `Render` are undefined.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/checksums/checksums.go`:

```go
// Package checksums renders and parses the lines of a checksum file. It works
// on writers and strings and knows nothing about files.
package checksums

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/BobMali/ldsum/internal/hash"
)

// Format is one of the line shapes a checksum file can use.
type Format int

// The supported line shapes. Text and Binary are the GNU coreutils formats;
// they differ only in the marker between the digest and the path.
const (
	Text Format = iota
	Binary
)

// Entry is one file's digest together with the path it belongs to.
type Entry struct {
	Digest hash.Digest
	Path   string
}

// Render writes e as a single line, including its trailing newline.
func Render(w io.Writer, e Entry, f Format) error {
	switch f {
	case Text, Binary:
		marker := "  "
		if f == Binary {
			marker = " *"
		}
		_, err := fmt.Fprintf(w, "%s%s%s\n", e.Digest.Hex, marker, e.Path)
		return err
	default:
		return errors.New("unknown checksum format " + strconv.Itoa(int(f)))
	}
}
```

- [ ] **Step 4: Run the test and watch it pass**

Run: `go test ./internal/checksums/ -run TestRenderGNU -v`
Expected: PASS, both subtests.

- [ ] **Step 5: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; vet and tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/checksums/checksums.go internal/checksums/checksums_test.go
git commit -m "feat(checksums): render gnu text and binary lines"
```

---

### Task 2: Render tagged and bare output

**Files:**
- Modify: `internal/checksums/checksums.go`
- Test: `internal/checksums/checksums_test.go` (append — expect a `test-guard.sh` prompt)

**Interfaces:**
- Consumes: `Format`, `Entry`, `Render` from Task 1.
- Produces: two more `Format` constants, `Tag` and `Bare`. Task 8 maps command flags onto all four.

- [ ] **Step 1: Write the failing test**

Append to `internal/checksums/checksums_test.go`:

```go
func TestRenderTagAndBare(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		entry  Entry
		want   string
	}{
		{
			name:   "tag names the algorithm uppercased",
			format: Tag,
			entry:  abcEntry("dist/a.txt"),
			want:   "SHA256 (dist/a.txt) = " + abcSHA256 + "\n",
		},
		{
			name:   "tag uses the sha512 name for sha512",
			format: Tag,
			entry: Entry{
				Digest: hash.Digest{Algorithm: hash.SHA512, Hex: "cafe"},
				Path:   "dist/a.txt",
			},
			want: "SHA512 (dist/a.txt) = cafe\n",
		},
		{
			name:   "bare prints the digest alone",
			format: Bare,
			entry:  abcEntry("dist/a.txt"),
			want:   abcSHA256 + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := Render(&out, tt.entry, tt.format); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("Render() = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	tests := []struct {
		name   string
		format Format
	}{
		{name: "out of range", format: Format(99)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := Render(&out, abcEntry("dist/a.txt"), tt.format); err == nil {
				t.Fatal("Render() error = nil, want error for an unknown format")
			}
			if out.Len() != 0 {
				t.Errorf("wrote %q, want nothing", out.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./internal/checksums/ -run 'TestRenderTagAndBare|TestRenderUnknownFormat' -v`
Expected: FAIL — `Tag` and `Bare` are undefined, so the test package does not compile. (`TestRenderUnknownFormat` would pass on its own; it is here because it locks the default arm now that the enum has more than one real member.)

- [ ] **Step 3: Write the minimal implementation**

In `internal/checksums/checksums.go`, extend the constant block:

```go
const (
	Text Format = iota
	Binary
	Tag
	Bare
)
```

Add `"strings"` to the imports, and add the two arms to `Render`, before the `default`:

```go
	case Tag:
		_, err := fmt.Fprintf(w, "%s (%s) = %s\n",
			strings.ToUpper(string(e.Digest.Algorithm)), e.Path, e.Digest.Hex)
		return err
	case Bare:
		_, err := fmt.Fprintf(w, "%s\n", e.Digest.Hex)
		return err
```

- [ ] **Step 4: Run the tests and watch them pass**

Run: `go test ./internal/checksums/ -v`
Expected: PASS, all subtests including Task 1's.

- [ ] **Step 5: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; vet and tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/checksums/checksums.go internal/checksums/checksums_test.go
git commit -m "feat(checksums): render tagged and bare output"
```

---

### Task 3: Escape paths that cannot go in a line as themselves

**Files:**
- Modify: `internal/checksums/checksums.go`
- Test: `internal/checksums/checksums_test.go` (append — expect a `test-guard.sh` prompt)

**Interfaces:**
- Consumes: `Render`, all four `Format` constants.
- Produces: no new exported names. `Render` now returns an error for `Tag` when the path contains a backslash or a newline; Task 5 turns that error into a per-file failure.

The convention, verified against `shasum` 6.02: when the path contains a backslash or a newline, the line gains a leading backslash, a backslash in the path becomes two, and a newline becomes backslash-`n`. Ordinary lines gain no marker. Those two characters are the whole escape set.

- [ ] **Step 1: Write the failing test**

Append to `internal/checksums/checksums_test.go`:

```go
func TestRenderEscapesPaths(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		path   string
		want   string
	}{
		{
			name:   "a plain path gains no marker",
			format: Text,
			path:   "dist/a.txt",
			want:   abcSHA256 + "  dist/a.txt\n",
		},
		{
			name:   "a backslash doubles and marks the line",
			format: Text,
			path:   "dist/we\\ird.txt",
			want:   "\\" + abcSHA256 + "  dist/we\\\\ird.txt\n",
		},
		{
			name:   "a newline becomes backslash n",
			format: Text,
			path:   "dist/two\nlines.txt",
			want:   "\\" + abcSHA256 + "  dist/two\\nlines.txt\n",
		},
		{
			name:   "binary escapes the same way",
			format: Binary,
			path:   "dist/we\\ird.txt",
			want:   "\\" + abcSHA256 + " *dist/we\\\\ird.txt\n",
		},
		{
			name:   "both characters in one path",
			format: Text,
			path:   "dist/a\\b\nc.txt",
			want:   "\\" + abcSHA256 + "  dist/a\\\\b\\nc.txt\n",
		},
		{
			name:   "bare ignores the path entirely",
			format: Bare,
			path:   "dist/we\\ird.txt",
			want:   abcSHA256 + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := Render(&out, abcEntry(tt.path), tt.format); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("Render() = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestRenderTagRejectsEscapablePath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "backslash", path: "dist/we\\ird.txt"},
		{name: "newline", path: "dist/two\nlines.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			err := Render(&out, abcEntry(tt.path), Tag)
			if err == nil {
				t.Fatal("Render() error = nil, want error for a path needing escapes")
			}
			if out.Len() != 0 {
				t.Errorf("wrote %q, want nothing", out.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./internal/checksums/ -run 'TestRenderEscapesPaths|TestRenderTagRejectsEscapablePath' -v`
Expected: FAIL. The escaping subtests report the unescaped path with no leading marker; `TestRenderTagRejectsEscapablePath` reports `error = nil`.

- [ ] **Step 3: Write the minimal implementation**

In `internal/checksums/checksums.go`, add `"strings"` if it is not already imported, then add the helpers below `Render`:

```go
// escaper is stateless and safe to share; building it once keeps it off the
// per-line path.
var escaper = strings.NewReplacer("\\", "\\\\", "\n", "\\n")

// needsEscape reports whether p holds a character a line cannot carry as
// itself: a backslash would be ambiguous, and a newline would split the entry
// into two malformed lines.
func needsEscape(p string) bool {
	return strings.ContainsAny(p, "\\\n")
}
```

Replace the `Text, Binary` arm of `Render` with:

```go
	case Text, Binary:
		marker := "  "
		if f == Binary {
			marker = " *"
		}
		// A leading backslash is how coreutils marks a line whose path was
		// escaped, so a reader knows to unescape it.
		prefix, path := "", e.Path
		if needsEscape(path) {
			prefix, path = "\\", escaper.Replace(path)
		}
		_, err := fmt.Fprintf(w, "%s%s%s%s\n", prefix, e.Digest.Hex, marker, path)
		return err
```

Replace the `Tag` arm with:

```go
	case Tag:
		// Tagged format has no escape convention upstream, so there is no
		// correct line to write rather than one we invented.
		if needsEscape(e.Path) {
			return errors.New(e.Path +
				": tagged format cannot carry a backslash or newline in a path")
		}
		_, err := fmt.Fprintf(w, "%s (%s) = %s\n",
			strings.ToUpper(string(e.Digest.Algorithm)), e.Path, e.Digest.Hex)
		return err
```

- [ ] **Step 4: Run the tests and watch them pass**

Run: `go test ./internal/checksums/ -v`
Expected: PASS, every subtest in the package.

- [ ] **Step 5: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; vet and tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/checksums/checksums.go internal/checksums/checksums_test.go
git commit -m "feat(checksums): escape paths that need it"
```

---

### Task 4: Report whether an algorithm is supported

**Files:**
- Modify: `internal/hash/hash.go`
- Test: `internal/hash/hash_test.go` (append — expect a `test-guard.sh` prompt)

**Interfaces:**
- Consumes: the existing unexported `hexLen` map.
- Produces: `hash.Supported(a Algorithm) bool`. Task 5 calls it once, before opening anything, so a bad `--algo` fails as a usage error instead of once per file across a whole tree.

- [ ] **Step 1: Write the failing test**

Append to `internal/hash/hash_test.go`:

```go
func TestSupported(t *testing.T) {
	tests := []struct {
		name string
		algo Algorithm
		want bool
	}{
		{name: "sha256", algo: SHA256, want: true},
		{name: "sha512", algo: SHA512, want: true},
		{name: "md5 is not supported", algo: Algorithm("md5"), want: false},
		{name: "the empty algorithm", algo: Algorithm(""), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Supported(tt.algo); got != tt.want {
				t.Errorf("Supported(%q) = %v, want %v", tt.algo, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./internal/hash/ -run TestSupported -v`
Expected: FAIL — the package does not compile, because `Supported` is undefined.

- [ ] **Step 3: Write the minimal implementation**

Add to `internal/hash/hash.go`, directly below the `hexLen` map:

```go
// Supported reports whether a is an algorithm this package can compute.
func Supported(a Algorithm) bool {
	_, ok := hexLen[a]
	return ok
}
```

- [ ] **Step 4: Run the test and watch it pass**

Run: `go test ./internal/hash/ -run TestSupported -v`
Expected: PASS, all four subtests.

- [ ] **Step 5: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; vet and tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/hash/hash.go internal/hash/hash_test.go
git commit -m "feat(hash): report whether an algorithm is supported"
```

---

### Task 5: Sum named files

**Files:**
- Create: `internal/run/sum.go`
- Test: `internal/run/sum_test.go`

**Interfaces:**
- Consumes: `hash.Sum`, `hash.Supported`, `hash.Algorithm`, `checksums.Render`, `checksums.Entry`, `checksums.Format`.
- Produces: `run.SumOptions{Paths []string; Algorithm string; Format checksums.Format}` and `run.Sum(out, errOut io.Writer, opts SumOptions) error`. Task 6 adds a `Recursive bool` field, Task 7 adds `Verbose bool`, and Task 8 builds the struct from flags.

The failure rule, which every later task extends rather than changes: a per-file failure prints to `errOut`, the run keeps going, and `Sum` returns an ordinary error at the end. `cmd/exit.go` maps an ordinary error to 2 through its existing default arm, so nothing there changes.

Diagnostics carry no `ldsum:` prefix. `cmd/root.go`'s `execute` already prefixes the returned error, and `*fs.PathError` from `os.Open` already reads as `open dist/x: permission denied` on its own. Adding a prefix here would duplicate the program name in a second package.

- [ ] **Step 1: Write the failing test**

Create `internal/run/sum_test.go`. Note that `sum_test.go` and `verify_test.go` share package `run`, so `abcSHA256` is already declared — do not redeclare it.

```go
package run

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BobMali/ldsum/internal/checksums"
)

// sumTree writes each name/contents pair under a fresh temporary directory and
// returns its root. Names may contain slashes; parents are created.
func sumTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

func TestSumNamedFiles(t *testing.T) {
	tests := []struct {
		name   string
		format checksums.Format
		want   func(path string) string
	}{
		{
			name:   "text by default",
			format: checksums.Text,
			want:   func(p string) string { return abcSHA256 + "  " + p + "\n" },
		},
		{
			name:   "bare drops the path",
			format: checksums.Bare,
			want:   func(string) string { return abcSHA256 + "\n" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			path := filepath.Join(root, "a.txt")
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{path},
				Algorithm: "sha256",
				Format:    tt.format,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if want := tt.want(path); out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}

func TestSumArgumentOrder(t *testing.T) {
	tests := []struct {
		name  string
		order []string
	}{
		{name: "as given", order: []string{"b.txt", "a.txt"}},
		{name: "reversed", order: []string{"a.txt", "b.txt"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc", "b.txt": "abc"})
			var paths []string
			var want strings.Builder
			for _, name := range tt.order {
				p := filepath.Join(root, name)
				paths = append(paths, p)
				want.WriteString(abcSHA256 + "  " + p + "\n")
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     paths,
				Algorithm: "sha256",
				Format:    checksums.Text,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if out.String() != want.String() {
				t.Errorf("stdout = %q, want %q", out.String(), want.String())
			}
		})
	}
}

func TestSumKeepsGoingAfterAFailure(t *testing.T) {
	tests := []struct {
		name         string
		missing      string
		wantInErrOut string
	}{
		{name: "a missing file", missing: "gone.txt", wantInErrOut: "gone.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			good := filepath.Join(root, "a.txt")
			bad := filepath.Join(root, tt.missing)
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{bad, good},
				Algorithm: "sha256",
				Format:    checksums.Text,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error after a failed file")
			}
			if want := abcSHA256 + "  " + good + "\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q — the good file must still be summed",
					out.String(), want)
			}
			if !strings.Contains(errOut.String(), tt.wantInErrOut) {
				t.Errorf("stderr = %q, want it to name %q", errOut.String(), tt.wantInErrOut)
			}
		})
	}
}

func TestSumRejectsADirectoryWithoutRecursion(t *testing.T) {
	tests := []struct {
		name string
		sub  string
	}{
		{name: "a subdirectory", sub: "sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{tt.sub + "/a.txt": "abc"})
			dir := filepath.Join(root, tt.sub)
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{dir},
				Algorithm: "sha256",
				Format:    checksums.Text,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error for a directory without -r")
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", out.String())
			}
			if !strings.Contains(errOut.String(), "-r") {
				t.Errorf("stderr = %q, want it to name the -r flag", errOut.String())
			}
		})
	}
}

func TestSumRejectsAnUnknownAlgorithm(t *testing.T) {
	tests := []struct {
		name string
		algo string
	}{
		{name: "md5", algo: "md5"},
		{name: "empty", algo: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{filepath.Join(root, "a.txt")},
				Algorithm: tt.algo,
				Format:    checksums.Text,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error for an unknown algorithm")
			}
			// The check is up front, so nothing is hashed and nothing is
			// reported per file.
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", out.String())
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want nothing", errOut.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./internal/run/ -run TestSum -v`
Expected: FAIL — the package does not compile, because `Sum` and `SumOptions` are undefined.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/run/sum.go`:

```go
package run

import (
	"fmt"
	"io"
	"os"

	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/hash"
)

// SumOptions is one request to print checksums.
type SumOptions struct {
	Paths     []string
	Algorithm string
	Format    checksums.Format
}

// Sum prints the digest of each path in opts.Paths. A file that cannot be
// summed is reported on errOut and the rest still run; the returned error then
// reports how many failed.
func Sum(out, errOut io.Writer, opts SumOptions) error {
	algo := hash.Algorithm(opts.Algorithm)
	// Checked once up front: a bad algorithm is a wrong command, not a
	// per-file problem to repeat across a whole tree.
	if !hash.Supported(algo) {
		return fmt.Errorf("unknown algorithm %q: want sha256 or sha512", opts.Algorithm)
	}

	var attempted, failures int
	for _, path := range opts.Paths {
		a, f := sumPath(out, errOut, path, algo, opts)
		attempted += a
		failures += f
	}
	if failures > 0 {
		return fmt.Errorf("could not sum %d of %d files", failures, attempted)
	}
	return nil
}

// sumPath handles one argument and returns how many files it tried and how
// many of those failed.
func sumPath(out, errOut io.Writer, path string, algo hash.Algorithm, opts SumOptions) (int, int) {
	// Stat rather than Lstat: a symlink named as an argument was named on
	// purpose, so it is followed. Entries found by walking are not.
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1, 1
	}
	if info.IsDir() {
		fmt.Fprintf(errOut, "%s: is a directory (use -r to recurse)\n", path)
		return 1, 1
	}
	return sumFile(out, errOut, path, algo, opts.Format)
}

// sumFile streams one file through the hasher and renders its line.
func sumFile(out, errOut io.Writer, path string, algo hash.Algorithm, format checksums.Format) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1, 1
	}
	defer f.Close()

	digest, err := hash.Sum(f, algo)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", path, err)
		return 1, 1
	}

	if err := checksums.Render(out, checksums.Entry{Digest: digest, Path: path}, format); err != nil {
		fmt.Fprintln(errOut, err)
		return 1, 1
	}
	return 1, 0
}
```

The `-r` in that message names a flag Task 6 adds. It is part of the designed command surface, not an invention.

- [ ] **Step 4: Run the tests and watch them pass**

Run: `go test ./internal/run/ -v`
Expected: PASS, every subtest including the existing `verify` ones.

- [ ] **Step 5: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; vet and tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/run/sum.go internal/run/sum_test.go
git commit -m "feat(run): sum named files"
```

---

### Task 6: Walk directories under -r

**Files:**
- Modify: `internal/run/sum.go`
- Test: `internal/run/sum_test.go` (append — expect a `test-guard.sh` prompt)

**Interfaces:**
- Consumes: `SumOptions`, `Sum`, `sumPath`, `sumFile` from Task 5.
- Produces: a `Recursive bool` field on `SumOptions`, and an unexported `walkDir`. Task 7 adds the `Verbose` field beside it; Task 8 sets both from flags.

`filepath.WalkDir` reads each directory's entries in lexical order, so output order is deterministic with no sorting of our own. Only regular files are hashed; `d.Type().IsRegular()` does not resolve symlinks, so a discovered link is skipped rather than followed. Skips are silent until Task 7.

- [ ] **Step 1: Write the failing test**

Append to `internal/run/sum_test.go`:

```go
func TestSumWalksInLexicalOrder(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		order []string
	}{
		{
			name: "files before subdirectories, each sorted",
			files: map[string]string{
				"b.txt":     "abc",
				"a.txt":     "abc",
				"sub/c.txt": "abc",
			},
			order: []string{"a.txt", "b.txt", "sub/c.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, tt.files)
			var want strings.Builder
			for _, name := range tt.order {
				want.WriteString(abcSHA256 + "  " + filepath.Join(root, name) + "\n")
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if out.String() != want.String() {
				t.Errorf("stdout = %q, want %q", out.String(), want.String())
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}

func TestSumWalkSkipsSymlinksSilently(t *testing.T) {
	tests := []struct {
		name string
		link string
	}{
		{name: "a link beside the file it points at", link: "link.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			if err := os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, tt.link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q — the link must not be followed",
					out.String(), want)
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty without --verbose", errOut.String())
			}
		})
	}
}

func TestSumWalksAnEmptyDirectory(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "nothing to sum is not a failure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", out.String())
			}
		})
	}
}

func TestSumWalkKeepsGoingAfterAnUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a file with mode 000")
	}
	tests := []struct {
		name       string
		unreadable string
	}{
		{name: "mode 000 mid-walk", unreadable: "b-secret.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{
				"a.txt":       "abc",
				tt.unreadable: "abc",
				"c.txt":       "abc",
			})
			if err := os.Chmod(filepath.Join(root, tt.unreadable), 0o000); err != nil {
				t.Fatalf("chmod: %v", err)
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error after an unreadable file")
			}
			want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n" +
				abcSHA256 + "  " + filepath.Join(root, "c.txt") + "\n"
			if out.String() != want {
				t.Errorf("stdout = %q, want %q — the walk must continue past the failure",
					out.String(), want)
			}
			if !strings.Contains(errOut.String(), tt.unreadable) {
				t.Errorf("stderr = %q, want it to name %q", errOut.String(), tt.unreadable)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./internal/run/ -run 'TestSumWalk|TestSumWalks' -v`
Expected: FAIL — the package does not compile, because `SumOptions` has no `Recursive` field.

- [ ] **Step 3: Write the minimal implementation**

In `internal/run/sum.go`, add `"io/fs"` and `"path/filepath"` to the imports, add the field:

```go
type SumOptions struct {
	Paths     []string
	Algorithm string
	Format    checksums.Format
	Recursive bool
}
```

Replace the directory branch in `sumPath`:

```go
	if info.IsDir() {
		if !opts.Recursive {
			fmt.Fprintf(errOut, "%s: is a directory (use -r to recurse)\n", path)
			return 1, 1
		}
		return walkDir(out, errOut, path, algo, opts)
	}
```

Add `walkDir` below `sumPath`:

```go
// walkDir sums every regular file under root. WalkDir reads each directory in
// lexical order, so the output order needs no sorting of its own.
func walkDir(out, errOut io.Writer, root string, algo hash.Algorithm, opts SumOptions) (int, int) {
	var attempted, failures int

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintln(errOut, err)
			attempted++
			failures++
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// Type() does not resolve links, so a symlink found by walking is
		// skipped rather than followed. Devices and sockets go the same way.
		if !d.Type().IsRegular() {
			return nil
		}
		a, f := sumFile(out, errOut, path, algo, opts.Format)
		attempted += a
		failures += f
		return nil
	})
	// The callback always returns nil, so this can only fire if WalkDir could
	// not stat the root it was handed.
	if err != nil {
		fmt.Fprintln(errOut, err)
		attempted++
		failures++
	}
	return attempted, failures
}
```

- [ ] **Step 4: Run the tests and watch them pass**

Run: `go test ./internal/run/ -v`
Expected: PASS, every subtest.

- [ ] **Step 5: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; vet and tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/run/sum.go internal/run/sum_test.go
git commit -m "feat(run): walk directories under -r"
```

---

### Task 7: Report skipped entries under --verbose

**Files:**
- Modify: `internal/run/sum.go`
- Test: `internal/run/sum_test.go` (append — expect a `test-guard.sh` prompt)

**Interfaces:**
- Consumes: `SumOptions`, `walkDir` from Task 6.
- Produces: a `Verbose bool` field on `SumOptions`. Task 8 sets it from `-v`.

`--verbose` does exactly one thing: name each entry the walk skipped, on stderr. It never changes what is skipped, what reaches stdout, or the exit code.

- [ ] **Step 1: Write the failing test**

Append to `internal/run/sum_test.go`:

```go
func TestSumVerboseNamesSkippedEntries(t *testing.T) {
	tests := []struct {
		name string
		link string
	}{
		{name: "a symlink is named", link: "link.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			if err := os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, tt.link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
				Verbose:   true,
			})
			// Skipping is not a failure, however loudly it is reported.
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if !strings.Contains(errOut.String(), tt.link) {
				t.Errorf("stderr = %q, want it to name the skipped %q", errOut.String(), tt.link)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./internal/run/ -run TestSumVerbose -v`
Expected: FAIL — the package does not compile, because `SumOptions` has no `Verbose` field.

- [ ] **Step 3: Write the minimal implementation**

In `internal/run/sum.go`, add the field:

```go
type SumOptions struct {
	Paths     []string
	Algorithm string
	Format    checksums.Format
	Recursive bool
	Verbose   bool
}
```

and replace the non-regular branch inside `walkDir`'s callback:

```go
		// Type() does not resolve links, so a symlink found by walking is
		// skipped rather than followed. Devices and sockets go the same way.
		if !d.Type().IsRegular() {
			if opts.Verbose {
				fmt.Fprintf(errOut, "%s: skipped, not a regular file\n", path)
			}
			return nil
		}
```

- [ ] **Step 4: Run the tests and watch them pass**

Run: `go test ./internal/run/ -v`
Expected: PASS, every subtest — including Task 6's, which asserts stderr stays empty without `--verbose`.

- [ ] **Step 5: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; vet and tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/run/sum.go internal/run/sum_test.go
git commit -m "feat(run): report skipped entries under --verbose"
```

---

### Task 8: Add the sum subcommand

**Files:**
- Create: `cmd/sum.go`
- Modify: `cmd/root.go` (register the command only; the description text is Task 9)
- Test: `cmd/sum_test.go`

**Interfaces:**
- Consumes: `run.Sum`, `run.SumOptions` with all five fields, and the four `checksums.Format` constants.
- Produces: `newSumCmd() *cobra.Command`, registered on the root tree.

Built like `cmd/verify.go`: flags bind to locals so two trees in one process never share them, `SilenceUsage` is set once arguments have parsed, and the body does nothing but assemble options and call `run`.

- [ ] **Step 1: Write the failing test**

Create `cmd/sum_test.go`. Package `cmd` already declares `abcSHA256`, `runCLI` and `fixture` in `verify_test.go` — reuse them, do not redeclare.

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSumCommandFormats(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want func(path string) string
	}{
		{
			name: "text is the default",
			args: nil,
			want: func(p string) string { return abcSHA256 + "  " + p + "\n" },
		},
		{
			name: "binary",
			args: []string{"--binary"},
			want: func(p string) string { return abcSHA256 + " *" + p + "\n" },
		},
		{
			name: "tag",
			args: []string{"--tag"},
			want: func(p string) string { return "SHA256 (" + p + ") = " + abcSHA256 + "\n" },
		},
		{
			name: "bare",
			args: []string{"--bare"},
			want: func(string) string { return abcSHA256 + "\n" },
		},
		{
			name: "text stated explicitly",
			args: []string{"--text"},
			want: func(p string) string { return abcSHA256 + "  " + p + "\n" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "abc")
			args := append([]string{"sum"}, tt.args...)

			stdout, stderr, err := runCLI(t, append(args, path)...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if want := tt.want(path); stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
			if got := exitCode(err); got != 0 {
				t.Errorf("exitCode() = %d, want 0", got)
			}
		})
	}
}

func TestSumCommandFormatFlagsAreExclusive(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "text and binary", args: []string{"--text", "--binary"}},
		{name: "tag and bare", args: []string{"--tag", "--bare"}},
		{name: "binary and tag", args: []string{"--binary", "--tag"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "abc")
			args := append([]string{"sum"}, tt.args...)

			_, _, err := runCLI(t, append(args, path)...)
			if err == nil {
				t.Fatal("Execute() error = nil, want an error for two format flags")
			}
			if got := exitCode(err); got != 2 {
				t.Errorf("exitCode() = %d, want 2", got)
			}
		})
	}
}

func TestSumCommandExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		args     func(root string) []string
		wantCode int
	}{
		{
			name:     "no arguments",
			args:     func(string) []string { return []string{"sum"} },
			wantCode: 2,
		},
		{
			name: "a missing file",
			args: func(root string) []string {
				return []string{"sum", filepath.Join(root, "gone.txt")}
			},
			wantCode: 2,
		},
		{
			name:     "a directory without -r",
			args:     func(root string) []string { return []string{"sum", root} },
			wantCode: 2,
		},
		{
			name: "an unknown algorithm",
			args: func(root string) []string {
				return []string{"sum", "--algo", "md5", filepath.Join(root, "a.txt")}
			},
			wantCode: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("abc"), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			_, _, err := runCLI(t, tt.args(root)...)
			if err == nil {
				t.Fatal("Execute() error = nil, want an error")
			}
			if got := exitCode(err); got != tt.wantCode {
				t.Errorf("exitCode() = %d, want %d", got, tt.wantCode)
			}
		})
	}
}

func TestSumCommandRecursive(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{name: "long flag", flag: "--recursive"},
		{name: "short flag", flag: "-r"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("abc"), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			stdout, _, err := runCLI(t, "sum", tt.flag, root)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n"; stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestSumCommandAlgorithm(t *testing.T) {
	// sha512 of "abc", from FIPS 180-4.
	const abcSHA512 = "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a" +
		"2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "sha256 by default", args: nil, want: abcSHA256},
		{name: "sha512 named", args: []string{"--algo", "sha512"}, want: abcSHA512},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "abc")
			args := append([]string{"sum", "--bare"}, tt.args...)

			stdout, _, err := runCLI(t, append(args, path)...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if want := tt.want + "\n"; stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestSumCommandTagUsesTheAlgorithmName(t *testing.T) {
	tests := []struct {
		name string
		algo string
		want string
	}{
		{name: "sha512 tag", algo: "sha512", want: "SHA512 ("},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "abc")

			stdout, _, err := runCLI(t, "sum", "--tag", "--algo", tt.algo, path)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !strings.HasPrefix(stdout, tt.want) {
				t.Errorf("stdout = %q, want it to start with %q", stdout, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `go test ./cmd/ -run TestSumCommand -v`
Expected: FAIL — every subtest reports an error from Cobra along the lines of `unknown command "sum" for "ldsum"`.

- [ ] **Step 3: Write the minimal implementation**

Create `cmd/sum.go`:

```go
package cmd

import (
	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/run"
	"github.com/spf13/cobra"
)

// newSumCmd builds the sum subcommand. Every flag binds to a local, so two
// trees in the same process never share them.
func newSumCmd() *cobra.Command {
	var (
		algorithm string
		text      bool
		binary    bool
		tagged    bool
		bare      bool
		recursive bool
		verbose   bool
	)

	cmd := &cobra.Command{
		Use:   "sum <path>...",
		Short: "Print the checksum of a file or a tree",
		Long: `Sum prints the checksum of each file it is given.

The default output is the GNU coreutils text format — the digest, two spaces,
then the path — which is what a SHA256SUMS file contains. --binary marks the
path with an asterisk instead, --tag switches to the BSD tagged format, and
--bare prints the digest alone so it can be captured straight into a variable.

Directory arguments are walked only with -r. Symlinks are never followed, and
anything skipped is named on stderr under -v.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Arguments have parsed by this point, so a later failure is not a
			// usage problem and usage text would only bury the output.
			cmd.SilenceUsage = true

			format := checksums.Text
			switch {
			case binary:
				format = checksums.Binary
			case tagged:
				format = checksums.Tag
			case bare:
				format = checksums.Bare
			}

			return run.Sum(cmd.OutOrStdout(), cmd.ErrOrStderr(), run.SumOptions{
				Paths:     args,
				Algorithm: algorithm,
				Format:    format,
				Recursive: recursive,
				Verbose:   verbose,
			})
		},
	}

	cmd.Flags().StringVar(&algorithm, "algo", "sha256", "checksum algorithm: sha256 or sha512")
	cmd.Flags().BoolVarP(&text, "text", "t", false, "GNU text format: <digest>  <path> (the default)")
	cmd.Flags().BoolVarP(&binary, "binary", "b", false, "GNU binary format: <digest> *<path>")
	cmd.Flags().BoolVar(&tagged, "tag", false, "BSD tagged format: SHA256 (<path>) = <digest>")
	cmd.Flags().BoolVar(&bare, "bare", false, "the digest alone, with no path")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "walk directory arguments")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "name skipped entries on stderr")
	cmd.MarkFlagsMutuallyExclusive("text", "binary", "tag", "bare")

	return cmd
}
```

In `cmd/root.go`, register it next to the existing line:

```go
	root.AddCommand(newVerifyCmd())
	root.AddCommand(newSumCmd())
```

- [ ] **Step 4: Run the tests and watch them pass**

Run: `go test ./cmd/ -v`
Expected: PASS, every subtest including the existing `verify` and `exit` ones.

- [ ] **Step 5: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; vet and tests pass.

- [ ] **Step 6: Check it by hand**

Run: `go run . sum go.mod && go run . sum --tag go.mod && go run . sum -r internal | head -5`
Expected: a text line, then `SHA256 (go.mod) = ...`, then several lines under `internal/`. Cross-check the first against `shasum -a 256 go.mod` — the digests must match.

- [ ] **Step 7: Commit**

```bash
git add cmd/sum.go cmd/sum_test.go cmd/root.go
git commit -m "feat(cmd): add sum subcommand"
```

---

### Task 9: Describe the sum command

**Files:**
- Modify: `README.md`
- Modify: `cmd/root.go` (`Short` and `Long` only)

**Interfaces:**
- Consumes: the finished command from Task 8.
- Produces: nothing code depends on.

Both currently say ldsum only verifies, which stopped being true in Task 8.

- [ ] **Step 1: Widen the root description**

In `cmd/root.go`, replace `Short` and `Long`:

```go
		Use:   "ldsum",
		Short: "Compute and verify file checksums",
		Long: `ldsum computes checksums and verifies files against them.

sum prints the digest of a file or a whole tree, in the GNU coreutils text and
binary formats, the BSD tagged format, or on its own.

verify checks a file against an expected checksum. The file can be read from a
local path or fetched from a URL, and the expected checksum can be given inline
or read from a checksum file (for example the SHA256SUMS file published
alongside a release).

Both exit non-zero when the answer is no, so they drop straight into a script.`,
```

- [ ] **Step 2: Update the README**

In `README.md`:

- Replace the status note so it reads:

```markdown
> **Status:** `sum` computes checksums for local files and directories.
> `verify` works with local files and inline checksums. URL input and
> checksum-file input are not yet implemented.
```

- Add a `sum` section directly above the existing verify usage:

````markdown
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

Symlinks are never followed, so no file is summed twice and no cycle can
occur. Skipped entries are silent unless `-v` names them on stderr.
````

- Add a row to the exit-code table so it reads:

```markdown
| Code | Meaning |
|------|---------|
| 0 | the digest matched, or every file was summed |
| 1 | the digest did not match, or the file is missing (`verify` only) |
| 2 | the command could not be carried out: wrong argument count, bad checksum, unknown algorithm, unreadable file, a directory without `-r` |
```

- [ ] **Step 3: Verify every command in the README actually runs**

Run: `go build -o /dev/null ./... && go run . sum --help && go run . --help`
Expected: both help screens render, `sum` is listed on the root screen, and the flags match the table just written.

- [ ] **Step 4: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; vet and tests pass.

- [ ] **Step 5: Commit**

```bash
git add README.md cmd/root.go
git commit -m "docs: describe the sum command"
```

---

## Done when

```sh
gofmt -l .        # prints nothing
go vet ./...
go test ./...
golangci-lint run # if available locally; CI runs it regardless
```

Run them and report what they printed. Never mark this done by reasoning about the code.
