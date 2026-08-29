# Verify a Single File Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `ldsum verify <file> <expected-hex>`, which streams a local file through sha256 or sha512 and reports whether the digest matches.

**Architecture:** `internal/hash` turns an `io.Reader` into a `Digest` and parses expected checksums from strings. `internal/run` opens the file, drives the hasher, prints the verdict, and returns a typed error. `cmd` parses flags and maps errors to exit codes. `os.Exit` stays in `main.go`.

**Tech Stack:** Go 1.27, `spf13/cobra`, standard library only (`crypto/sha256`, `crypto/sha512`, `encoding/hex`).

**Spec:** `docs/superpowers/specs/2026-08-29-verify-single-file-design.md`

## Global Constraints

- Standard library plus `spf13/cobra` only. Adding any module is a decision to raise with the author first, never an implementation detail.
- Hash algorithms come from `crypto/*`. No third-party hashing.
- `os.Exit` appears only in `main.go`. Everything else returns an `error`.
- Cobra commands parse flags and call `internal/run`. They do no real work.
- Commands write through `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, never `os.Stdout`.
- `internal/hash` takes readers and strings, never paths.
- Data to stdout, diagnostics to stderr.
- Exit codes: `0` success, `1` mismatch or missing target file, `2` usage or I/O error.
- Errors wrap with context — `fmt.Errorf("read %s: %w", path, err)` — and are never logged and returned at the same time.
- Never read a whole file into memory. Stream through a fixed buffer.
- Comment only what the code cannot say: why, not what. One or two lines.
- Tests are table-driven with `t.Run` per case. Anything written goes in `t.TempDir()`.
- Digest constants are published FIPS 180-4 values. Never assert against a digest this code produced.
- Definition of done, run and reported rather than reasoned about: `gofmt -l .` prints nothing, `go vet ./...` passes, `go test ./...` passes.
- Commit messages are Conventional Commits: `<type>(<scope>): <description>`, lowercase, imperative, no trailing period, ≤ 66 characters. Scopes here: `hash`, `cmd`, `run`. No AI attribution trailers.

## Working with this repo's hooks

Two hooks will interrupt the cycle. Neither is a failure:

- **`go-check.sh`** runs `gofmt`/`go vet`/`go test ./...` after every file write and reports failures back. Right after you write a failing test, it will report that vet and the tests fail. **That is the red step's evidence**, not a problem to fix by abandoning the test. Read the message, confirm it fails for the reason the task predicts, then write the implementation.
- **`test-guard.sh`** turns any write to an *existing* `*_test.go` into a permission prompt. Creating a test file is free. If a task's step 1 creates a new test file, it passes through; if it appends a case to a file an earlier task created, expect a prompt and wait for the answer. Never work around it, and never edit a test to make a red test pass.

## File Structure

| File | Responsibility |
|---|---|
| `internal/hash/hash.go` | `Algorithm`, `Digest`, `Sum`, digest parsing and comparison. Knows nothing about files. |
| `internal/hash/hash_test.go` | Known-answer vectors and parsing cases. |
| `internal/run/verify.go` | Opens the path, drives the hasher, prints the verdict, returns `*MismatchError` or a wrapped I/O error. |
| `internal/run/verify_test.go` | Match, mismatch, and file error paths. |
| `cmd/verify.go` | The `verify` subcommand: flags, args, one call into `run`. |
| `cmd/verify_test.go` | End-to-end through `rootCmd` with buffers. |
| `cmd/exit.go` | `exitCode(error) int` — the only place that knows exit codes. |
| `cmd/exit_test.go` | The error-to-code mapping table. |
| `cmd/root.go` | Modified: boilerplate removed, `Execute` returns `int`. |
| `main.go` | Modified: `os.Exit(cmd.Execute())`. |

## Deviation from the spec

The spec's exit-code table lists a `run.ErrUsage` sentinel for bad hex, bad
length, and an unknown `--algo`. This plan drops it: those all exit `2`, and
so does every other non-mismatch, non-missing-file error, so the sentinel
would classify nothing. Parse failures are returned as plain wrapped errors.
If a later feature needs to tell usage errors apart, add the sentinel then.

---

### Task 1: sha256 digest of a reader

**Files:**
- Create: `internal/hash/hash.go`
- Test: `internal/hash/hash_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `type Algorithm string`; `const SHA256 Algorithm = "sha256"`; `type Digest struct { Algorithm Algorithm; Hex string }`; `func Sum(r io.Reader, a Algorithm) (Digest, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/hash/hash_test.go`:

```go
package hash

import (
	"strings"
	"testing"
)

// Vectors are the published FIPS 180-4 examples, never values this package
// produced.
func TestSumSHA256(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty input",
			input: "",
			want:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:  "abc",
			input: "abc",
			want:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
		{
			name:  "one million a",
			input: strings.Repeat("a", 1000000),
			want:  "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sum(strings.NewReader(tt.input), SHA256)
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if got.Hex != tt.want {
				t.Errorf("Hex = %s, want %s", got.Hex, tt.want)
			}
			if got.Algorithm != SHA256 {
				t.Errorf("Algorithm = %s, want %s", got.Algorithm, SHA256)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/hash/ -run TestSumSHA256 -v`
Expected: FAIL — the package does not build, `undefined: Sum`, `undefined: SHA256`.

If instead it fails on a *digest mismatch* once the code exists, do not touch
the implementation until you have checked the constant itself:
`printf 'abc' | shasum -a 256`. A wrong constant in the test is the only
reason to ask about changing a test file.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/hash/hash.go`:

```go
// Package hash turns a stream of bytes into a digest. It works on readers and
// strings and knows nothing about files.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	stdhash "hash"
	"io"
)

// Algorithm names a supported hash function.
type Algorithm string

const SHA256 Algorithm = "sha256"

// Digest is a computed or expected checksum. Hex is always lowercase.
type Digest struct {
	Algorithm Algorithm
	Hex       string
}

// A fixed buffer keeps memory flat however large the input is.
const copyBufSize = 32 * 1024

// Sum streams r through a and returns the digest.
func Sum(r io.Reader, a Algorithm) (Digest, error) {
	h, err := newHash(a)
	if err != nil {
		return Digest{}, err
	}
	if _, err := io.CopyBuffer(h, r, make([]byte, copyBufSize)); err != nil {
		return Digest{}, err
	}
	return Digest{Algorithm: a, Hex: hex.EncodeToString(h.Sum(nil))}, nil
}

func newHash(a Algorithm) (stdhash.Hash, error) {
	switch a {
	case SHA256:
		return sha256.New(), nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q", a)
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/hash/ -run TestSumSHA256 -v`
Expected: PASS, three subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/hash/hash.go internal/hash/hash_test.go
git commit -m "feat(hash): add sha256 digest of a reader"
```

---

### Task 2: sha512 digest of a reader

**Files:**
- Modify: `internal/hash/hash.go`
- Test: `internal/hash/hash_test.go` (existing — expect the test-guard prompt)

**Interfaces:**
- Consumes: `Sum`, `Digest`, `Algorithm` from Task 1.
- Produces: `const SHA512 Algorithm = "sha512"`.

- [ ] **Step 1: Write the failing test**

Append to `internal/hash/hash_test.go`:

```go
func TestSumSHA512(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty input",
			input: "",
			want:  "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
		},
		{
			name:  "abc",
			input: "abc",
			want:  "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		},
		{
			name:  "one million a",
			input: strings.Repeat("a", 1000000),
			want:  "e718483d0ce769644e2e42c7bc15b4638e1f98b13b2044285632a803afa973ebde0ff244877ea60a4cb0432ce577c31beb009c5c2c49aa2e4eadb217ad8cc09b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sum(strings.NewReader(tt.input), SHA512)
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if got.Hex != tt.want {
				t.Errorf("Hex = %s, want %s", got.Hex, tt.want)
			}
			if got.Algorithm != SHA512 {
				t.Errorf("Algorithm = %s, want %s", got.Algorithm, SHA512)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/hash/ -run TestSumSHA512 -v`
Expected: FAIL — `undefined: SHA512`.

- [ ] **Step 3: Write the minimal implementation**

In `internal/hash/hash.go`, add the `crypto/sha512` import, the constant, and the switch case:

```go
const (
	SHA256 Algorithm = "sha256"
	SHA512 Algorithm = "sha512"
)
```

```go
func newHash(a Algorithm) (stdhash.Hash, error) {
	switch a {
	case SHA256:
		return sha256.New(), nil
	case SHA512:
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q", a)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/hash/ -v`
Expected: PASS, both vector tests.

- [ ] **Step 5: Commit**

```bash
git add internal/hash/hash.go internal/hash/hash_test.go
git commit -m "feat(hash): add sha512 digest of a reader"
```

---

### Task 3: parse a checksum and infer its algorithm

**Files:**
- Modify: `internal/hash/hash.go`
- Test: `internal/hash/hash_test.go` (existing — expect the test-guard prompt)

**Interfaces:**
- Consumes: `Digest`, `Algorithm`, `SHA256`, `SHA512`.
- Produces: `func ParseDigest(s string) (Digest, error)`; `func (d Digest) Equal(o Digest) bool`.

- [ ] **Step 1: Write the failing test**

Append to `internal/hash/hash_test.go`:

```go
func TestParseDigest(t *testing.T) {
	sha256Hex := strings.Repeat("a", 64)
	sha512Hex := strings.Repeat("b", 128)

	valid := []struct {
		name  string
		input string
		want  Digest
	}{
		{
			name:  "sha256 length",
			input: sha256Hex,
			want:  Digest{Algorithm: SHA256, Hex: sha256Hex},
		},
		{
			name:  "sha512 length",
			input: sha512Hex,
			want:  Digest{Algorithm: SHA512, Hex: sha512Hex},
		},
		{
			name:  "uppercase is lowered",
			input: strings.ToUpper(sha256Hex),
			want:  Digest{Algorithm: SHA256, Hex: sha256Hex},
		},
		{
			name:  "surrounding space is trimmed",
			input: "  " + sha256Hex + "\n",
			want:  Digest{Algorithm: SHA256, Hex: sha256Hex},
		},
	}

	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDigest(tt.input)
			if err != nil {
				t.Fatalf("ParseDigest() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseDigest() = %+v, want %+v", got, tt.want)
			}
		})
	}

	invalid := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "only whitespace", input: "   "},
		{name: "non-hex characters", input: strings.Repeat("z", 64)},
		{name: "odd length", input: strings.Repeat("a", 63)},
		{name: "sha1 length", input: strings.Repeat("a", 40)},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseDigest(tt.input); err == nil {
				t.Errorf("ParseDigest(%q) = nil error, want an error", tt.input)
			}
		})
	}
}

func TestDigestEqual(t *testing.T) {
	base := Digest{Algorithm: SHA256, Hex: strings.Repeat("a", 64)}

	tests := []struct {
		name  string
		other Digest
		want  bool
	}{
		{name: "same algorithm and hex", other: base, want: true},
		{
			name:  "different hex",
			other: Digest{Algorithm: SHA256, Hex: strings.Repeat("b", 64)},
			want:  false,
		},
		{
			name:  "different algorithm",
			other: Digest{Algorithm: SHA512, Hex: base.Hex},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.Equal(tt.other); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/hash/ -run 'TestParseDigest|TestDigestEqual' -v`
Expected: FAIL — `undefined: ParseDigest`, `base.Equal undefined`.

- [ ] **Step 3: Write the minimal implementation**

Add `"strings"` to the imports of `internal/hash/hash.go`, then add:

```go
// hexLen is the digest length, in hex characters, of each algorithm. The
// lengths are distinct, which is what makes inference possible.
var hexLen = map[Algorithm]int{
	SHA256: 64,
	SHA512: 128,
}

// ParseDigest normalises s and infers the algorithm from its length.
func ParseDigest(s string) (Digest, error) {
	norm, err := normalize(s)
	if err != nil {
		return Digest{}, err
	}
	for a, n := range hexLen {
		if len(norm) == n {
			return Digest{Algorithm: a, Hex: norm}, nil
		}
	}
	return Digest{}, fmt.Errorf(
		"cannot tell the algorithm from %d hex characters: want 64 (sha256) or 128 (sha512)",
		len(norm),
	)
}

// Equal reports whether two digests are the same. The expected checksum is
// public, so there is nothing here for a timing attack to learn.
func (d Digest) Equal(o Digest) bool {
	return d.Algorithm == o.Algorithm && d.Hex == o.Hex
}

func normalize(s string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(s))
	if norm == "" {
		return "", fmt.Errorf("empty checksum")
	}
	if _, err := hex.DecodeString(norm); err != nil {
		return "", fmt.Errorf("not a hex checksum: %q", norm)
	}
	return norm, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/hash/ -v`
Expected: PASS, all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/hash/hash.go internal/hash/hash_test.go
git commit -m "feat(hash): parse a checksum and infer its algorithm"
```

---

### Task 4: honour an explicitly named algorithm

**Files:**
- Modify: `internal/hash/hash.go`
- Test: `internal/hash/hash_test.go` (existing — expect the test-guard prompt)

**Interfaces:**
- Consumes: `normalize`, `hexLen`, `Digest`.
- Produces: `func ParseDigestAs(s string, a Algorithm) (Digest, error)`.

- [ ] **Step 1: Write the failing test**

Append to `internal/hash/hash_test.go`:

```go
func TestParseDigestAs(t *testing.T) {
	sha256Hex := strings.Repeat("a", 64)

	t.Run("length matches the named algorithm", func(t *testing.T) {
		got, err := ParseDigestAs(sha256Hex, SHA256)
		if err != nil {
			t.Fatalf("ParseDigestAs() error = %v", err)
		}
		want := Digest{Algorithm: SHA256, Hex: sha256Hex}
		if got != want {
			t.Errorf("ParseDigestAs() = %+v, want %+v", got, want)
		}
	})

	invalid := []struct {
		name      string
		input     string
		algorithm Algorithm
	}{
		{
			name:      "length contradicts the named algorithm",
			input:     sha256Hex,
			algorithm: SHA512,
		},
		{
			name:      "unknown algorithm",
			input:     sha256Hex,
			algorithm: Algorithm("md5"),
		},
		{
			name:      "not hex",
			input:     strings.Repeat("z", 64),
			algorithm: SHA256,
		},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseDigestAs(tt.input, tt.algorithm); err == nil {
				t.Errorf("ParseDigestAs(%q, %q) = nil error, want an error",
					tt.input, tt.algorithm)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/hash/ -run TestParseDigestAs -v`
Expected: FAIL — `undefined: ParseDigestAs`.

- [ ] **Step 3: Write the minimal implementation**

Add to `internal/hash/hash.go`:

```go
// ParseDigestAs normalises s and checks it against the named algorithm.
func ParseDigestAs(s string, a Algorithm) (Digest, error) {
	n, ok := hexLen[a]
	if !ok {
		return Digest{}, fmt.Errorf("unknown algorithm %q: want sha256 or sha512", a)
	}
	norm, err := normalize(s)
	if err != nil {
		return Digest{}, err
	}
	if len(norm) != n {
		return Digest{}, fmt.Errorf("%s needs %d hex characters, got %d", a, n, len(norm))
	}
	return Digest{Algorithm: a, Hex: norm}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/hash/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hash/hash.go internal/hash/hash_test.go
git commit -m "feat(hash): honour an explicitly named algorithm"
```

---

### Task 5: report a matching checksum

**Files:**
- Create: `internal/run/verify.go`
- Test: `internal/run/verify_test.go`

**Interfaces:**
- Consumes: `hash.ParseDigest`, `hash.ParseDigestAs`, `hash.Sum`, `hash.Digest.Equal`, `hash.Algorithm`.
- Produces: `type VerifyOptions struct { Path, Expected, Algorithm string }`; `func Verify(out, errOut io.Writer, opts VerifyOptions) error`.

- [ ] **Step 1: Write the failing test**

Create `internal/run/verify_test.go`:

```go
package run

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// sha256 and sha512 of "abc", from FIPS 180-4.
const (
	abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	abcSHA512 = "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a" +
		"2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"
)

func writeFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestVerifyMatch(t *testing.T) {
	tests := []struct {
		name      string
		expected  string
		algorithm string
	}{
		{name: "sha256 inferred", expected: abcSHA256},
		{name: "sha512 inferred", expected: abcSHA512},
		{name: "sha256 named", expected: abcSHA256, algorithm: "sha256"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFixture(t, "abc")
			var out, errOut bytes.Buffer

			err := Verify(&out, &errOut, VerifyOptions{
				Path:      path,
				Expected:  tt.expected,
				Algorithm: tt.algorithm,
			})
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if want := path + ": OK\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/run/ -run TestVerifyMatch -v`
Expected: FAIL — `undefined: Verify`, `undefined: VerifyOptions`.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/run/verify.go`:

```go
// Package run orchestrates the work behind each command. It returns errors
// and never exits.
package run

import (
	"fmt"
	"io"
	"os"

	"github.com/BobMali/ldsum/internal/hash"
)

// VerifyOptions is one verification request. An empty Algorithm means infer
// it from the length of Expected.
type VerifyOptions struct {
	Path      string
	Expected  string
	Algorithm string
}

// Verify streams the file at opts.Path through the hasher and reports whether
// its digest matches opts.Expected.
func Verify(out, errOut io.Writer, opts VerifyOptions) error {
	expected, err := parseExpected(opts)
	if err != nil {
		return err
	}

	f, err := os.Open(opts.Path)
	if err != nil {
		return fmt.Errorf("open %s: %w", opts.Path, err)
	}
	defer f.Close()

	actual, err := hash.Sum(f, expected.Algorithm)
	if err != nil {
		return fmt.Errorf("read %s: %w", opts.Path, err)
	}

	fmt.Fprintf(out, "%s: OK\n", opts.Path)
	_ = actual
	return nil
}

func parseExpected(opts VerifyOptions) (hash.Digest, error) {
	if opts.Algorithm == "" {
		return hash.ParseDigest(opts.Expected)
	}
	return hash.ParseDigestAs(opts.Expected, hash.Algorithm(opts.Algorithm))
}
```

The `_ = actual` line is deliberate: nothing yet compares the digests, because
no test demands it. Task 6 replaces it.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/run/ -v`
Expected: PASS, three subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/run/verify.go internal/run/verify_test.go
git commit -m "feat(run): report a matching checksum"
```

---

### Task 6: report a checksum mismatch

**Files:**
- Modify: `internal/run/verify.go`
- Test: `internal/run/verify_test.go` (existing — expect the test-guard prompt)

**Interfaces:**
- Consumes: `Verify`, `VerifyOptions`, `hash.Digest`.
- Produces: `type MismatchError struct { Path string; Expected, Actual hash.Digest }` with `func (e *MismatchError) Error() string`.

- [ ] **Step 1: Write the failing test**

Append to `internal/run/verify_test.go` (and add `"errors"`, `"strings"`, and `"github.com/BobMali/ldsum/internal/hash"` to its imports):

```go
func TestVerifyMismatch(t *testing.T) {
	path := writeFixture(t, "abc")
	wrong := strings.Repeat("0", 64)
	var out, errOut bytes.Buffer

	err := Verify(&out, &errOut, VerifyOptions{Path: path, Expected: wrong})

	var mismatch *MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("Verify() error = %v, want *MismatchError", err)
	}
	if mismatch.Path != path {
		t.Errorf("Path = %q, want %q", mismatch.Path, path)
	}
	if mismatch.Actual.Hex != abcSHA256 {
		t.Errorf("Actual.Hex = %q, want %q", mismatch.Actual.Hex, abcSHA256)
	}
	if mismatch.Expected.Hex != wrong {
		t.Errorf("Expected.Hex = %q, want %q", mismatch.Expected.Hex, wrong)
	}
	if mismatch.Actual.Algorithm != hash.SHA256 {
		t.Errorf("Actual.Algorithm = %q, want %q", mismatch.Actual.Algorithm, hash.SHA256)
	}

	if want := path + ": FAILED\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if !strings.Contains(errOut.String(), "expected: "+wrong) {
		t.Errorf("stderr = %q, want it to contain the expected digest", errOut.String())
	}
	if !strings.Contains(errOut.String(), "actual:   "+abcSHA256) {
		t.Errorf("stderr = %q, want it to contain the actual digest", errOut.String())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/run/ -run TestVerifyMismatch -v`
Expected: FAIL — `undefined: MismatchError`.

- [ ] **Step 3: Write the minimal implementation**

In `internal/run/verify.go`, add the type:

```go
// MismatchError reports a file whose digest differs from the expected one. It
// is the one failure that means the check ran fine and the answer was no.
type MismatchError struct {
	Path     string
	Expected hash.Digest
	Actual   hash.Digest
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("%s: checksum mismatch", e.Path)
}
```

Replace the tail of `Verify` (the `fmt.Fprintf` / `_ = actual` / `return nil`
lines) with:

```go
	if !actual.Equal(expected) {
		fmt.Fprintf(out, "%s: FAILED\n", opts.Path)
		fmt.Fprintf(errOut, "expected: %s\n", expected.Hex)
		fmt.Fprintf(errOut, "actual:   %s\n", actual.Hex)
		return &MismatchError{Path: opts.Path, Expected: expected, Actual: actual}
	}

	fmt.Fprintf(out, "%s: OK\n", opts.Path)
	return nil
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/run/ -v`
Expected: PASS — both the match and the mismatch tests.

- [ ] **Step 5: Commit**

```bash
git add internal/run/verify.go internal/run/verify_test.go
git commit -m "feat(run): report a checksum mismatch"
```

---

### Task 7: wrap file and input errors with context

**Files:**
- Test: `internal/run/verify_test.go` (existing — expect the test-guard prompt)
- Modify: `internal/run/verify.go` only if a case fails.

**Interfaces:**
- Consumes: `Verify`, `VerifyOptions`.
- Produces: nothing new. This task pins down behaviour Task 5 already wrote, which is why it may need no implementation change.

- [ ] **Step 1: Write the failing test**

Append to `internal/run/verify_test.go` (and add `"io"` and `"io/fs"` to its imports):

```go
func TestVerifyErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope.txt")

		err := Verify(io.Discard, io.Discard, VerifyOptions{
			Path:     missing,
			Expected: abcSHA256,
		})
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Verify() error = %v, want one wrapping fs.ErrNotExist", err)
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("error = %q, want it to name the path", err)
		}
	})

	t.Run("path is a directory", func(t *testing.T) {
		dir := t.TempDir()

		err := Verify(io.Discard, io.Discard, VerifyOptions{
			Path:     dir,
			Expected: abcSHA256,
		})
		if err == nil {
			t.Fatal("Verify() = nil error, want an error")
		}
		if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("error = %v, want it not to look like a missing file", err)
		}
	})

	t.Run("unreadable file", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads a file whatever its mode")
		}
		path := writeFixture(t, "abc")
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatalf("chmod: %v", err)
		}

		err := Verify(io.Discard, io.Discard, VerifyOptions{
			Path:     path,
			Expected: abcSHA256,
		})
		if err == nil {
			t.Fatal("Verify() = nil error, want a permission error")
		}
		if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("error = %v, want it not to look like a missing file", err)
		}
	})

	t.Run("checksum is not hex", func(t *testing.T) {
		path := writeFixture(t, "abc")

		err := Verify(io.Discard, io.Discard, VerifyOptions{
			Path:     path,
			Expected: strings.Repeat("z", 64),
		})
		if err == nil {
			t.Fatal("Verify() = nil error, want an error")
		}
		var mismatch *MismatchError
		if errors.As(err, &mismatch) {
			t.Errorf("error = %v, want a parse error rather than a mismatch", err)
		}
	})

	t.Run("checksum length matches no algorithm", func(t *testing.T) {
		path := writeFixture(t, "abc")

		err := Verify(io.Discard, io.Discard, VerifyOptions{
			Path:     path,
			Expected: strings.Repeat("a", 40),
		})
		if err == nil {
			t.Fatal("Verify() = nil error, want an error")
		}
	})

	t.Run("unknown algorithm", func(t *testing.T) {
		path := writeFixture(t, "abc")

		err := Verify(io.Discard, io.Discard, VerifyOptions{
			Path:      path,
			Expected:  abcSHA256,
			Algorithm: "md5",
		})
		if err == nil {
			t.Fatal("Verify() = nil error, want an error")
		}
	})
}
```

- [ ] **Step 2: Run the test and read what fails**

Run: `go test ./internal/run/ -run TestVerifyErrors -v`

Expected: the parse and missing-file cases pass on the code Task 5 already
wrote. If every subtest passes, the task's implementation step is genuinely
empty — say so and move to step 4 rather than inventing a change.

- [ ] **Step 3: Fix only what failed**

Change `internal/run/verify.go` only to satisfy a subtest that actually
failed. The likely candidate is a directory read whose error needs the
`read %s: %w` wrapper — which `Verify` already applies. Do not add error
handling no failing test asked for.

- [ ] **Step 4: Run the whole suite**

Run: `go test ./... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/run/verify_test.go internal/run/verify.go
git commit -m "test(run): cover file and checksum error paths"
```

If `verify.go` did not change, drop it from the `git add` line.

---

### Task 8: the verify subcommand

**Files:**
- Create: `cmd/verify.go`
- Test: `cmd/verify_test.go`
- Modify: `cmd/root.go` (remove the boilerplate placeholders)

**Interfaces:**
- Consumes: `run.Verify`, `run.VerifyOptions`.
- Produces: `verifyCmd`, registered on `rootCmd`; package-level `verifyAlgorithm string` bound to `--algo`.

- [ ] **Step 1: Remove the Cobra placeholders and commit**

In `cmd/root.go`, delete the commented-out `Run:` stub inside `rootCmd`:

```go
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
```

and delete the whole `init()` function — the dummy `--toggle` flag, the
comments around it, and the `func init() { … }` wrapper. `rootCmd` needs no
`init` of its own; `cmd/verify.go` brings its own in the next step.

Then:

```bash
go build ./... && go vet ./... && gofmt -l . && go test ./...
git add cmd/root.go
git commit -m "refactor(cmd): drop cobra scaffolding placeholders"
```

- [ ] **Step 2: Write the failing test**

Create `cmd/verify_test.go`:

```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sha256 of "abc", from FIPS 180-4.
const abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

// runCLI drives the real command tree with buffers in place of the process
// streams, and undoes the global state Cobra keeps between runs.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		verifyAlgorithm = ""
	})

	err = rootCmd.Execute()
	return out.String(), errOut.String(), err
}

func fixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestVerifyCommand(t *testing.T) {
	t.Run("match", func(t *testing.T) {
		path := fixture(t, "abc")

		stdout, stderr, err := runCLI(t, "verify", path, abcSHA256)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if want := path + ": OK\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("explicit algorithm", func(t *testing.T) {
		path := fixture(t, "abc")

		stdout, _, err := runCLI(t, "verify", "--algo", "sha256", path, abcSHA256)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if want := path + ": OK\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		path := fixture(t, "abc")
		wrong := strings.Repeat("0", 64)

		stdout, stderr, err := runCLI(t, "verify", path, wrong)
		if err == nil {
			t.Fatal("Execute() = nil error, want a mismatch error")
		}
		if want := path + ": FAILED\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if !strings.Contains(stderr, "expected: "+wrong) {
			t.Errorf("stderr = %q, want the expected digest", stderr)
		}
		if strings.Contains(stderr, "Usage:") {
			t.Errorf("stderr = %q, want no usage dump on a mismatch", stderr)
		}
	})

	t.Run("wrong argument count", func(t *testing.T) {
		path := fixture(t, "abc")

		if _, _, err := runCLI(t, "verify", path); err == nil {
			t.Error("Execute() = nil error, want an argument-count error")
		}
	})
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./cmd/ -run TestVerifyCommand -v`
Expected: FAIL — `undefined: verifyAlgorithm`, and once that compiles,
`unknown command "verify"`.

- [ ] **Step 4: Write the minimal implementation**

Create `cmd/verify.go`:

```go
package cmd

import (
	"github.com/BobMali/ldsum/internal/run"
	"github.com/spf13/cobra"
)

var verifyAlgorithm string

var verifyCmd = &cobra.Command{
	Use:   "verify <file> <checksum>",
	Short: "Verify a file against an expected checksum",
	Long: `Verify checks a file against a checksum given on the command line.

The algorithm is taken from the length of the checksum — 64 hex characters is
sha256, 128 is sha512 — unless --algo names one. It exits 0 when the digest
matches, 1 when it does not, and 2 when the command itself was wrong.`,
	Args: cobra.ExactArgs(2),
	// run already reports the verdict, so Cobra must not print it again.
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return run.Verify(cmd.OutOrStdout(), cmd.ErrOrStderr(), run.VerifyOptions{
			Path:      args[0],
			Expected:  args[1],
			Algorithm: verifyAlgorithm,
		})
	},
}

func init() {
	verifyCmd.Flags().StringVar(&verifyAlgorithm, "algo", "",
		"checksum algorithm: sha256 or sha512 (inferred from the checksum length when omitted)")
	rootCmd.AddCommand(verifyCmd)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/verify.go cmd/verify_test.go
git commit -m "feat(cmd): add verify subcommand"
```

---

### Task 9: exit codes

**Files:**
- Create: `cmd/exit.go`
- Test: `cmd/exit_test.go`
- Modify: `cmd/root.go` (`Execute` returns `int`), `main.go`

**Interfaces:**
- Consumes: `run.MismatchError`.
- Produces: `func exitCode(err error) int`; `func Execute() int`.

- [ ] **Step 1: Write the failing test**

Create `cmd/exit_test.go`:

```go
package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/BobMali/ldsum/internal/run"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "success",
			err:  nil,
			want: 0,
		},
		{
			name: "checksum mismatch",
			err:  &run.MismatchError{Path: "dist.tar.gz"},
			want: 1,
		},
		{
			name: "mismatch wrapped further up",
			err:  fmt.Errorf("verify: %w", &run.MismatchError{Path: "dist.tar.gz"}),
			want: 1,
		},
		{
			name: "missing target file",
			err:  fmt.Errorf("open dist.tar.gz: %w", fs.ErrNotExist),
			want: 1,
		},
		{
			name: "bad checksum on the command line",
			err:  errors.New(`not a hex checksum: "zz"`),
			want: 2,
		},
		{
			name: "unreadable file",
			err:  fmt.Errorf("read dist.tar.gz: %w", fs.ErrPermission),
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/ -run TestExitCode -v`
Expected: FAIL — `undefined: exitCode`.

- [ ] **Step 3: Write the minimal implementation**

Create `cmd/exit.go`:

```go
package cmd

import (
	"errors"
	"io/fs"

	"github.com/BobMali/ldsum/internal/run"
)

// exitCode maps an error to a process status: 1 for a verification the user
// must act on, 2 for a command that was wrong to run.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var mismatch *run.MismatchError
	switch {
	case errors.As(err, &mismatch):
		return 1
	case errors.Is(err, fs.ErrNotExist):
		return 1
	default:
		return 2
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/ -run TestExitCode -v`
Expected: PASS, six subtests.

- [ ] **Step 5: Return the code from Execute and exit on it in main**

In `cmd/root.go`, replace `Execute`:

```go
// Execute runs the command tree and returns the process exit code. It reports
// the error itself, except for a mismatch, whose detail run has already
// printed.
func Execute() int {
	err := rootCmd.Execute()
	if err != nil {
		var mismatch *run.MismatchError
		if !errors.As(err, &mismatch) {
			fmt.Fprintf(rootCmd.ErrOrStderr(), "ldsum: %v\n", err)
		}
	}
	return exitCode(err)
}
```

Its imports become `errors`, `fmt`, `github.com/BobMali/ldsum/internal/run`
and `github.com/spf13/cobra` — `os` is no longer used there.

In `main.go`:

```go
package main

import (
	"os"

	"github.com/BobMali/ldsum/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
```

Keep the licence header comment at the top of both files.

- [ ] **Step 6: Run the whole suite and the real binary**

```bash
gofmt -l . && go vet ./... && go test ./...

printf 'abc' > "$TMPDIR/abc.txt"
go run . verify "$TMPDIR/abc.txt" ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad; echo "exit=$?"
go run . verify "$TMPDIR/abc.txt" 0000000000000000000000000000000000000000000000000000000000000000; echo "exit=$?"
go run . verify "$TMPDIR/missing.txt" ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad; echo "exit=$?"
go run . verify "$TMPDIR/abc.txt" zz; echo "exit=$?"
```

Expected: `gofmt` prints nothing; vet and tests pass; the four runs print
`OK` / `FAILED` / a missing-file error / a hex error and exit `0`, `1`, `1`,
`2`. Report what they actually printed.

- [ ] **Step 7: Commit**

```bash
git add cmd/exit.go cmd/exit_test.go cmd/root.go main.go
git commit -m "feat(cmd): exit 1 on mismatch and 2 on usage errors"
```

---

### Task 10: document the command

**Files:**
- Modify: `README.md`, `CLAUDE.md`

**Interfaces:**
- Consumes: the finished CLI.
- Produces: nothing code depends on.

- [ ] **Step 1: Update the README**

Add this section to `README.md`, above the contributor setup rather than
replacing it:

````markdown
## Usage

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
ldsum verify --algo sha256 dist.tar.gz ba7816bf...
```

Exit codes, so it drops into a script:

| Code | Meaning |
|------|---------|
| 0 | the digest matched |
| 1 | the digest did not match, or the file is missing |
| 2 | the command was wrong: bad checksum, unknown algorithm, unreadable file |
````

- [ ] **Step 2: Correct the stale claims in CLAUDE.md**

Two statements in the Layout section are now false and must be fixed:

- "There are no subcommands, no test files, and no non-Cobra packages so far — the checksum logic has not been written."
- "`cmd/root.go` still carries two pieces of Cobra boilerplate that were deliberately left in place: the commented-out `Run:` stub and a dummy `--toggle` flag registered in `init()`. Remove them when real behaviour lands."

Replace the first with a description of what now exists (`cmd/verify.go`,
`internal/hash`, `internal/run`; still no `internal/checksums`). Delete the
second — the placeholders are gone.

- [ ] **Step 3: Verify and commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add README.md CLAUDE.md
git commit -m "docs: describe the verify command"
```

---

## Done when

- `ldsum verify <file> <sha256|sha512 hex>` prints `OK` or `FAILED` and exits `0` / `1`.
- A missing file exits `1`; bad hex, an unknown `--algo`, and an unreadable file exit `2`.
- `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass — run, with output reported.
- No new module in `go.mod`.
