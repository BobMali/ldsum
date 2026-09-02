# Mutation Testing Gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the test gaps that the `go-mutesting` audit found and the gremlins CI gate structurally cannot see, so that a future audit's survivors are all known-equivalent rather than a mixture of noise and real holes.

**Architecture:** Every task adds test cases only. No production code changes, by decision — where a code path cannot be reached from a test, it is recorded as an accepted survivor rather than reshaped to be reachable. Tests go into the existing `_test.go` file for the package under test, appended as new functions; no existing test is edited.

**Tech Stack:** Go 1.27, standard library plus `spf13/cobra`. `go-mutesting` (`~/go/bin`, installed by hand) verifies the work; `gremlins` gates CI and will not notice most of it.

**Spec:** None. This plan is driven by the mutant inventory in Appendix A, produced by `go-mutesting --do-not-remove-tmp-folder ./...` at commit `86a1ed0`. Appendix A travels with the plan and is the thing to argue from.

## Global Constraints

- Go 1.27 or newer. Module path `github.com/BobMali/ldsum`.
- Standard library plus `spf13/cobra` only. **No new module may be added by this plan.**
- Table-driven tests, one `t.Run` per case — including where there is a single case, which is the existing house style.
- Anything written goes under `t.TempDir()`. Fixtures live in `testdata/`.
- No mocks for the standard library. Pass an `io.Reader` or `fs.FS` instead.
- Never assert against a digest the implementation just produced. The sha256 of `"abc"` is `ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad`, already bound to `abcSHA256` in each test package that needs it.
- **Do not edit, rename, delete or weaken an existing test.** Every task below appends new functions. If a new test disagrees with the implementation, the implementation is wrong until the author says otherwise — stop and report rather than adjusting either side.
- Definition of done for every task: `gofmt -l .` prints nothing, `go vet ./...`, `go test ./...` and `golangci-lint run` are all clean. Run them; do not reason about them.
- Commit messages are Conventional Commits: `<type>(<scope>): <description>`, lowercase, imperative, no trailing period, 66 characters or fewer. Scopes in play here: `hash`, `checksums`, `run`, `cmd`. No trailers of any kind.

## How to verify a task actually killed its mutants

Unit tests passing is not evidence. After the tests are green, run the audit against the package you changed and compare the survivor list to Appendix A:

```sh
go-mutesting ./internal/checksums/...   # or whichever package the task touched
```

The mutation score should rise and the specific `FAIL` lines named in the task should be gone. To see what a surviving mutant actually changed:

```sh
go-mutesting --do-not-remove-tmp-folder ./internal/hash/...
diff -u internal/hash/hash.go /tmp/go-mutesting-<n>/internal/hash/hash.go.5
```

`go-mutesting` always exits 0. The score line at the end is the result; the exit code means nothing.

---

### Task 1: `hash.Sum` reports a read failure

The `io.CopyBuffer` error return in `Sum` can be deleted and the whole suite still passes. In a tool whose one job is hashing a stream, a read that fails partway through must not yield a digest.

**Files:**
- Modify: `internal/hash/hash_test.go` (append; the file currently ends at `TestSumUnknownAlgorithm` and friends)
- Under test: `internal/hash/hash.go:41-43`

**Interfaces:**
- Produces: `errReader` in package `hash` — a test-local `io.Reader` that yields `prefix` and then fails with `err`. Task 3 defines a same-named type in package `checksums`; they are independent.

- [ ] **Step 1: Write the failing test**

Append to `internal/hash/hash_test.go`, and add `"errors"` to its import block:

```go
// errReader hands over prefix and then fails, which is how a read error
// partway through a stream reaches Sum. This is an io.Reader rather than a
// mock: the rules ask for readers, not stubs of the standard library.
type errReader struct {
	prefix string
	n      int
	err    error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.n < len(r.prefix) {
		n := copy(p, r.prefix[r.n:])
		r.n += n
		return n, nil
	}
	return 0, r.err
}

func TestSumReportsAReadFailure(t *testing.T) {
	wantErr := errors.New("the device went away")
	tests := []struct {
		name   string
		prefix string
	}{
		{name: "the reader fails immediately"},
		{name: "the reader fails partway through", prefix: "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sum(&errReader{prefix: tt.prefix, err: wantErr}, SHA256)
			if !errors.Is(err, wantErr) {
				t.Fatalf("Sum() error = %v, want %v", err, wantErr)
			}
			// A partial digest is worse than no digest: it looks like an answer.
			if got != (Digest{}) {
				t.Errorf("Digest = %+v, want the zero value on failure", got)
			}
		})
	}
}
```

- [ ] **Step 2: Run it and watch it pass, then prove it is load-bearing**

Run: `go test ./internal/hash/ -run TestSumReportsAReadFailure -v`
Expected: PASS. The behaviour is already correct — this test exists to hold it there, which is the whole point of a mutation-driven gap.

Now prove the test can fail. Temporarily replace `internal/hash/hash.go:41-43` with:

```go
	if _, err := io.CopyBuffer(h, r, make([]byte, copyBufSize)); err != nil {
		_ = err
	}
```

Run the same command. Expected: FAIL on `Sum() error = <nil>, want the device went away`. **Restore the original three lines before continuing** — `git diff internal/hash/hash.go` must be empty.

- [ ] **Step 3: Confirm the mutant is dead**

Run: `go-mutesting ./internal/hash/...`
Expected: the score rises, and the `FAIL ... hash.go.5` line from Appendix A is gone. Four `FAIL` lines for `hash.go` remain — those are the `copyBufSize` arithmetic mutants, which are equivalent and stay (Task 10 records them).

- [ ] **Step 4: Run the full gates**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`
Expected: no output from `gofmt`, everything else clean.

- [ ] **Step 5: Commit**

```bash
git add internal/hash/hash_test.go
git commit -m "test(hash): cover a reader that fails mid-stream"
```

---

### Task 2: `leadingHex` stops where hex stops

Four mutants live in `internal/checksums/parse.go`'s two unexported helpers, and **none of them can be killed through `Parse`**. Traced by hand: `i := 0` → `i := 1` in `leadingHex` produces a `BadLine` for every input that the correct code also makes a `BadLine`, just by a different route, and the three `isHexDigit` upper-bound mutants only change which byte the scan stops on — which `Parse` then turns into the same rejection.

So this task tests the helpers directly. That is a departure from how this package is tested, and it is deliberate: these functions define what a checksum *is*, and nothing else pins their boundaries.

**Files:**
- Modify: `internal/checksums/parse_test.go` (append)
- Under test: `internal/checksums/parse.go:126-136`

- [ ] **Step 1: Write the failing test**

Append to `internal/checksums/parse_test.go`:

```go
// leadingHex and isHexDigit decide where a digest ends, so their boundaries are
// tested directly: through Parse every one of these cases is just a BadLine,
// which makes a wrong boundary invisible.
func TestLeadingHex(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty input", in: "", want: ""},
		{name: "every hex byte", in: "0123456789abcdefABCDEF", want: "0123456789abcdefABCDEF"},
		{name: "stops at the first non-hex byte", in: "abcz", want: "abc"},
		{name: "a non-hex first byte yields nothing", in: "zzz", want: ""},
		// Each of these is the byte immediately past one arm's upper bound.
		{name: "colon is one past 9", in: "9:", want: "9"},
		{name: "g is one past f", in: "fg", want: "f"},
		{name: "capital G is one past F", in: "FG", want: "F"},
		// A space is below every arm's lower bound, and is what separates a
		// real digest from its path.
		{name: "space is below every range", in: "0 1", want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leadingHex(tt.in); got != tt.want {
				t.Errorf("leadingHex(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/checksums/ -run TestLeadingHex -v`
Expected: PASS, all eight subtests.

- [ ] **Step 3: Prove each case is load-bearing**

Take the four mutations one at a time in `internal/checksums/parse.go`, run `go test ./internal/checksums/ -run TestLeadingHex`, confirm the named subtest fails, then restore the line before applying the next:

| Mutation | Subtest that must fail |
|---|---|
| `i := 0` → `i := 1` | `a_non-hex_first_byte_yields_nothing` (and `empty_input`, which panics) |
| `b >= '0' && b <= '9'` → `b >= '0' && true` | `colon_is_one_past_9` |
| `b >= 'a' && b <= 'f'` → `b >= 'a' && true` | `g_is_one_past_f` |
| `b >= 'A' && b <= 'F'` → `b >= 'A' && true` | `capital_G_is_one_past_F` |

After the last one, `git diff internal/checksums/parse.go` must be empty.

- [ ] **Step 4: Confirm the mutants are dead**

Run: `go-mutesting ./internal/checksums/...`
Expected: `parse.go.33`, `parse.go.35`, `parse.go.37` and `parse.go.58` are gone from the `FAIL` list. `parse.go.6` remains — that is Task 3.

- [ ] **Step 5: Run the full gates and commit**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`

```bash
git add internal/checksums/parse_test.go
git commit -m "test(checksums): pin where a hex run starts and ends"
```

---

### Task 3: `Parse` reports a read failure

`Parse`'s `s.Err()` check can be deleted: a checksum file that fails to read halfway would be reported as a short but successful listing, and `verify` would then say OK about the files it did manage to read.

**Files:**
- Modify: `internal/checksums/parse_test.go` (append; add `"errors"` to the import block)
- Under test: `internal/checksums/parse.go:57-59`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `errReader` in package `checksums`. Unrelated to Task 1's type of the same name in package `hash`.

- [ ] **Step 1: Write the failing test**

Append to `internal/checksums/parse_test.go`:

```go
// errReader fails on the first read. bufio.Scanner swallows the failure and
// simply stops, so only s.Err() distinguishes a truncated file from a short one.
type errReader struct{ err error }

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

func TestParseReportsAReadFailure(t *testing.T) {
	wantErr := errors.New("the file went away")
	tests := []struct {
		name string
	}{
		{name: "the reader fails"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(&errReader{err: wantErr})
			if !errors.Is(err, wantErr) {
				t.Fatalf("Parse() error = %v, want %v", err, wantErr)
			}
			// A partial listing is what makes this dangerous: verify would
			// report OK for the entries it did read and never mention the rest.
			if got.Entries != nil || got.Bad != nil {
				t.Errorf("Listing = %+v, want the zero value on failure", got)
			}
		})
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/checksums/ -run TestParseReportsAReadFailure -v`
Expected: PASS.

- [ ] **Step 3: Prove it is load-bearing**

Temporarily replace `internal/checksums/parse.go:57-59` with:

```go
	if err := s.Err(); err != nil {
		_ = err
	}
```

Run the same command. Expected: FAIL on `Parse() error = <nil>`. Restore, and confirm `git diff internal/checksums/parse.go` is empty.

- [ ] **Step 4: Confirm the mutant is dead**

Run: `go-mutesting ./internal/checksums/...`
Expected: no `FAIL` lines remain for `internal/checksums` at all.

- [ ] **Step 5: Run the full gates and commit**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`

```bash
git add internal/checksums/parse_test.go
git commit -m "test(checksums): cover a checksum file that fails to read"
```

---

### Task 4: the summary error counts every path

This is the biggest single gap: roughly twenty mutants live because nothing asserts the text of `could not sum %d of %d paths`. `attempted += a` can become `-=` or `=`, and every `return 1, 1` can become `return 0, 1` or `return 2, 1`, and no test notices. The counts are the only thing a script learns about how much of the run succeeded.

**Files:**
- Modify: `internal/run/sum_test.go` (append)
- Under test: `internal/run/sum.go:60-72` (`sumAll`), `:76-128` (`sumPath`, `walkDir`, `sumFile` return pairs)

- [ ] **Step 1: Write the first failing test**

Append to `internal/run/sum_test.go`. `sumTree` and `abcSHA256` already exist in this package:

```go
// The summary is the only place the attempted and failed counts are observable,
// so its exact text is what holds every counter in sum.go in place.
func TestSumSummaryCountsNamedPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths func(root string) []string
		want  string
	}{
		{
			name: "one missing beside one good file",
			paths: func(root string) []string {
				return []string{filepath.Join(root, "a.txt"), filepath.Join(root, "gone.txt")}
			},
			want: "could not sum 1 of 2 paths",
		},
		{
			name: "two missing beside one good file",
			paths: func(root string) []string {
				return []string{
					filepath.Join(root, "a.txt"),
					filepath.Join(root, "gone.txt"),
					filepath.Join(root, "also-gone.txt"),
				}
			},
			want: "could not sum 2 of 3 paths",
		},
		{
			name: "a directory without -r is one attempt and one failure",
			paths: func(root string) []string {
				return []string{root, filepath.Join(root, "a.txt")}
			},
			want: "could not sum 1 of 2 paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     tt.paths(root),
				Algorithm: "sha256",
				Format:    checksums.Text,
			})
			if err == nil {
				t.Fatalf("Sum() error = nil, want %q", tt.want)
			}
			if err.Error() != tt.want {
				t.Errorf("Sum() error = %q, want %q", err, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/run/ -run TestSumSummaryCountsNamedPaths -v`
Expected: PASS.

If a count is off by one, **stop and report** rather than editing the expectation. These numbers were derived by reading `sumPath`; a disagreement means either the reading or the implementation is wrong, and which one is the author's call.

- [ ] **Step 3: Write the walk test**

The named-path test never reaches `walkDir`'s own counters. Append:

```go
func TestSumSummaryCountsAWalk(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a file with mode 000")
	}
	tests := []struct {
		name       string
		unreadable string
		want       string
	}{
		{name: "one unreadable file in a tree of three", unreadable: "b-secret.txt", want: "could not sum 1 of 3 paths"},
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
				t.Fatalf("Sum() error = nil, want %q", tt.want)
			}
			if err.Error() != tt.want {
				t.Errorf("Sum() error = %q, want %q", err, tt.want)
			}
		})
	}
}
```

- [ ] **Step 4: Run it**

Run: `go test ./internal/run/ -run TestSumSummaryCounts -v`
Expected: PASS, both functions.

- [ ] **Step 5: Prove they are load-bearing**

In `internal/run/sum.go:64`, change `attempted += a` to `attempted -= a`. Run the same command. Expected: FAIL, reporting `could not sum 1 of -2 paths`. Restore, and confirm `git diff internal/run/sum.go` is empty.

- [ ] **Step 6: Confirm the mutants are dead**

Run: `go-mutesting ./internal/run/...`
Expected: the `sum.go` `FAIL` count drops from 38 to roughly 15 — about six belonging to Tasks 5, 6 and 7, and the rest to the accepted set in Appendix B (the flush and close returns, the four in the unreachable trailing block, and the three or four around `sumFile`'s `hash.Sum` branch). Record the actual number in the commit body if it is far from 15.

- [ ] **Step 7: Run the full gates and commit**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`

```bash
git add internal/run/sum_test.go
git commit -m "test(run): assert the counts in the sum summary"
```

---

### Task 5: an unusable output file is named in the error

`TestSumReportsAnUnusableOutputFile` already exists and still does not catch a deleted `os.Create` error check. With a nil file the rendered lines land in `bufio`'s buffer, succeed, and the *flush* fails instead — so `Sum` still returns non-nil and stderr is still empty, and the existing assertions all hold. What distinguishes the two is which error comes back: the real one is an `*fs.PathError` naming the output file, the mutant's is a bare `os.ErrInvalid`.

**Files:**
- Modify: `internal/run/sum_test.go` (append; add `"errors"` and `"io/fs"` to the import block)
- Under test: `internal/run/sum.go:40-43`

- [ ] **Step 1: Write the failing test**

```go
// The existing test asserts only that some error came back, which a dropped
// os.Create check still satisfies via the failing flush. The error's identity
// is what separates them.
func TestSumOutputFileErrorNamesTheFile(t *testing.T) {
	tests := []struct {
		name   string
		output func(root string) string
	}{
		{
			name:   "a directory that does not exist",
			output: func(root string) string { return filepath.Join(root, "missing", "SHA256SUMS") },
		},
		{
			name:   "a directory in place of the file",
			output: func(root string) string { return root },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			outPath := tt.output(root)
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{filepath.Join(root, "a.txt")},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Output:    outPath,
			})
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("Sum() error = %v (%T), want an *fs.PathError naming the output file", err, err)
			}
			if pathErr.Path != outPath {
				t.Errorf("error path = %q, want %q", pathErr.Path, outPath)
			}
		})
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/run/ -run TestSumOutputFileErrorNamesTheFile -v`
Expected: PASS.

- [ ] **Step 3: Prove it is load-bearing**

Temporarily replace `internal/run/sum.go:41-43` with:

```go
	f, err := os.Create(opts.Output)
	if err != nil {
		_ = err
	}
```

Run the same command. Expected: FAIL with `want an *fs.PathError` and a `*errors.errorString` or `*fs.PathError`-free type in the message. Restore, and confirm `git diff internal/run/sum.go` is empty.

- [ ] **Step 4: Confirm the mutant is dead**

Run: `go-mutesting ./internal/run/...`
Expected: `sum.go.13` gone. `sum.go.9` and `sum.go.10` — the flush and close returns — remain, and stay: see Appendix B.

- [ ] **Step 5: Run the full gates and commit**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`

```bash
git add internal/run/sum_test.go
git commit -m "test(run): pin which error an unusable -o reports"
```

---

### Task 6: an unreadable directory mid-walk is reported and counted

`walkDir`'s callback has an `if err != nil` branch — the diagnostic, the two counter bumps and the `return nil` can each be deleted with the suite still green, because no test makes `WalkDir` hand an error to the callback. An unreadable file does not do it: that fails later, at `os.Open` inside `sumFile`. An unreadable *directory* does.

**Files:**
- Modify: `internal/run/sum_test.go` (append)
- Under test: `internal/run/sum.go:102-107`

- [ ] **Step 1: Write the failing test**

```go
// An unreadable file fails at os.Open inside sumFile. Only an unreadable
// directory makes WalkDir hand an error to the callback, which is a separate
// branch with its own reporting and counting.
func TestSumWalkReportsAnUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a directory with mode 000")
	}
	tests := []struct {
		name   string
		locked string
	}{
		{name: "mode 000 subdirectory", locked: "b-locked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{
				"a.txt": "abc",
				filepath.Join(tt.locked, "inner.txt"): "abc",
				"c.txt": "abc",
			})
			locked := filepath.Join(root, tt.locked)
			if err := os.Chmod(locked, 0o000); err != nil {
				t.Fatalf("chmod: %v", err)
			}
			// t.TempDir cannot remove a directory it may not read.
			t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error after an unreadable directory")
			}
			if !strings.Contains(errOut.String(), tt.locked) {
				t.Errorf("stderr = %q, want it to name %q", errOut.String(), tt.locked)
			}
			want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n" +
				abcSHA256 + "  " + filepath.Join(root, "c.txt") + "\n"
			if out.String() != want {
				t.Errorf("stdout = %q, want %q — the walk must continue past the failure",
					out.String(), want)
			}
		})
	}
}
```

- [ ] **Step 2: Run it and read the count**

Run: `go test ./internal/run/ -run TestSumWalkReportsAnUnreadableDirectory -v`
Expected: PASS.

- [ ] **Step 3: Add the count assertion**

The failure count is what kills the two counter mutants in this branch, and it depends on how many times `WalkDir` invokes the callback for a directory it cannot read. Rather than guess, read it: add a temporary `t.Logf("summary: %v", err)` after the `err == nil` check, run with `-v`, and note the exact string.

Then replace the temporary log with the real assertion, using the string you observed:

```go
			if want := "could not sum 1 of 3 paths"; err.Error() != want {
				t.Errorf("Sum() error = %q, want %q", err, want)
			}
```

If the observed string is not `could not sum 1 of 3 paths`, use what you saw **and say so in the commit body** — a different count is a fact about `WalkDir`, not a licence to round the number.

- [ ] **Step 4: Prove it is load-bearing**

In `internal/run/sum.go`, delete the `fmt.Fprintln(errOut, err)` line from the callback's error branch. Run the same command. Expected: FAIL on `want it to name "b-locked"`. Restore, and confirm `git diff internal/run/sum.go` is empty.

- [ ] **Step 5: Confirm the mutants are dead**

Run: `go-mutesting ./internal/run/...`
Expected: the callback's error-branch mutants are gone. The trailing `if err != nil` block after `WalkDir` returns still has survivors, and stays — the source comment already explains that it is unreachable, and Appendix B records it.

- [ ] **Step 6: Run the full gates and commit**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`

```bash
git add internal/run/sum_test.go
git commit -m "test(run): cover a directory the walk cannot read"
```

---

### Task 7: a walked directory is not reported as skipped

`if d.IsDir() { return nil }` can have its body removed: directories then fall through to the `IsRegular` check, are still skipped, and still produce the right output — except that under `-v` every directory is announced on stderr as "skipped, not a regular file", which is wrong and which nothing catches.

**Files:**
- Modify: `internal/run/sum_test.go` (append)
- Under test: `internal/run/sum.go:109-111`

- [ ] **Step 1: Write the failing test**

```go
// A directory is walked into, not skipped, so -v must not announce it. Removing
// the IsDir branch leaves the output identical and only this line differs.
func TestSumVerboseWalkIsSilentAboutDirectories(t *testing.T) {
	tests := []struct {
		name string
		sub  string
	}{
		{name: "a subdirectory says nothing", sub: "sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{filepath.Join(tt.sub, "a.txt"): "abc"})
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
				Verbose:   true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if want := abcSHA256 + "  " + filepath.Join(root, tt.sub, "a.txt") + "\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if errOut.String() != "" {
				t.Errorf("stderr = %q, want empty — a directory is walked, not skipped", errOut.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/run/ -run TestSumVerboseWalkIsSilentAboutDirectories -v`
Expected: PASS.

- [ ] **Step 3: Prove it is load-bearing**

In `internal/run/sum.go`, delete the `return nil` from inside `if d.IsDir()`. Run the same command. Expected: FAIL on `stderr = "...: skipped, not a regular file\n", want empty`. Restore, and confirm `git diff internal/run/sum.go` is empty.

- [ ] **Step 4: Run the full gates and commit**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`

```bash
git add internal/run/sum_test.go
git commit -m "test(run): keep -v quiet about walked directories"
```

---

### Task 8: a missing checksum file is reported as an open failure

`VerifySums` opens the checksum file and returns the error bare, because `os.Open` already gives an `*fs.PathError` carrying the operation and path. Delete that return and the run still fails — but with `read <path>: invalid argument`, wrapped a second time, from `Parse` reading a nil file. Only the error's shape tells them apart. This also pins item 4 of the parked follow-up list, which asked for the bare error to be held in place.

**Files:**
- Modify: `internal/run/sums_test.go` (append; the file already imports `bytes`, `errors`, `os`, `path/filepath`, `strings` and `testing` — add only `"io/fs"`)
- Under test: `internal/run/sums.go:44-47`

- [ ] **Step 1: Write the failing test**

```go
// os.Open already returns an *fs.PathError carrying "open" and the path, which
// is why VerifySums returns it bare. Dropping that return still fails the run,
// but as a doubly wrapped "invalid argument" from reading a nil file.
func TestVerifySumsReportsAMissingChecksumFile(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "the checksum file does not exist", file: "SHA256SUMS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sumsPath := filepath.Join(t.TempDir(), tt.file)
			var out, errOut bytes.Buffer

			err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sumsPath})
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("VerifySums() error = %v (%T), want an *fs.PathError", err, err)
			}
			if pathErr.Op != "open" {
				t.Errorf("error op = %q, want %q", pathErr.Op, "open")
			}
			if pathErr.Path != sumsPath {
				t.Errorf("error path = %q, want %q", pathErr.Path, sumsPath)
			}
		})
	}
}
```

`SumsOptions` is `{SumsFile string; Paths []string}` (`internal/run/sums.go:17-20`), so naming only `SumsFile` leaves `Paths` nil, which is the "verify everything the file lists" case. The run never gets that far — it fails at the open.

- [ ] **Step 2: Run it**

Run: `go test ./internal/run/ -run TestVerifySumsReportsAMissingChecksumFile -v`
Expected: PASS.

- [ ] **Step 3: Prove it is load-bearing**

In `internal/run/sums.go`, replace the `return err` after `os.Open` with `_ = err`. Run the same command. Expected: FAIL, reporting a non-`*fs.PathError`. Restore, and confirm `git diff internal/run/sums.go` is empty.

- [ ] **Step 4: Confirm the mutant is dead**

Run: `go-mutesting ./internal/run/...`
Expected: `sums.go.0` gone from the `FAIL` list.

- [ ] **Step 5: Run the full gates and commit**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`

```bash
git add internal/run/sums_test.go
git commit -m "test(run): pin the bare open error for a checksum file"
```

---

### Task 9: `sum -v` is reachable from the command line

`internal/run` tests `Verbose` thoroughly, but the flag registration in `cmd/sum.go` can be deleted outright and every test still passes — nothing drives `sum` with `-v` through the cobra tree. The flag could silently stop existing.

**Files:**
- Modify: `cmd/sum_test.go` (append)
- Under test: `cmd/sum.go:71`

**Interfaces:**
- Consumes: `runCLI(t *testing.T, args ...string) (stdout, stderr string, err error)`, defined in `cmd/verify_test.go` and shared across the package's tests.

- [ ] **Step 1: Write the failing test**

`cmd/sum_test.go` has no tree-building helper — its tests call `t.TempDir()` and `os.WriteFile` inline, as `TestSumCommandRecursive` does, and the test below follows that. Its import block already has everything this needs (`os`, `path/filepath`, `strings`, `testing`).

```go
// internal/run covers what Verbose does. This covers that the flag exists at
// all: delete its registration and only a test that types -v notices.
func TestSumVerboseFlagIsWired(t *testing.T) {
	tests := []struct {
		name string
		link string
	}{
		{name: "-v names a skipped symlink", link: "link.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "a.txt")
			if err := os.WriteFile(file, []byte("abc"), 0o644); err != nil {
				t.Fatalf("write a.txt: %v", err)
			}
			if err := os.Symlink(file, filepath.Join(root, tt.link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			stdout, stderr, err := runCLI(t, "sum", "-r", "-v", root)
			if err != nil {
				t.Fatalf("runCLI() error = %v, stderr = %q", err, stderr)
			}
			if want := abcSHA256 + "  " + file + "\n"; stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
			if !strings.Contains(stderr, tt.link) {
				t.Errorf("stderr = %q, want it to name the skipped %q", stderr, tt.link)
			}
		})
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./cmd/ -run TestSumVerboseFlagIsWired -v`
Expected: PASS.

- [ ] **Step 3: Prove it is load-bearing**

In `cmd/sum.go`, delete the `cmd.Flags().BoolVarP(&verbose, ...)` line and add `_ = verbose` in its place so the package still builds. Run the same command. Expected: FAIL with `unknown shorthand flag: 'v'`. Restore, and confirm `git diff cmd/sum.go` is empty.

- [ ] **Step 4: Run the full gates and commit**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`

```bash
git add cmd/sum_test.go
git commit -m "test(cmd): drive sum -v through the command tree"
```

---

### Task 10: record the accepted survivors

The audit is only useful if its output can be read at a glance. After the nine tasks above, whatever still survives should be survivors *on purpose*, listed where the next person triaging the output will look — which is the README section that tells them how to run it.

**Files:**
- Modify: `README.md` (the "The deeper audit" subsection added in commit `86a1ed0`)

- [ ] **Step 1: Re-run the whole audit and capture the truth**

Run: `go-mutesting ./... | tee audit-after.log`
Then: `grep '^FAIL ' audit-after.log`

Write down every surviving mutant. Do not copy Appendix B into the README unexamined — it is a prediction made before the tests were written, and the point of this step is to replace it with the measurement.

- [ ] **Step 2: Add the list to the README**

Append to the "The deeper audit" subsection, filling in the mutants actually observed in Step 1 and the score from the final line:

```markdown
Some survivors are permanent. A mutant is equivalent when the change cannot
alter behaviour, and unreachable when no test can drive the branch it sits in.
Both are expected; a survivor *not* on this list is worth investigating.

| Where | Why it survives |
|---|---|
| `internal/hash/hash.go` — `copyBufSize` arithmetic (4 mutants) | The buffer size cannot change the digest. Equivalent. |
| `internal/run/sum.go` — the `Flush` and `Close` error returns | No portable way to make either fail on a regular file. `/dev/full` is Linux-only. |
| `internal/run/sum.go` — the `if err != nil` block after `WalkDir` returns | Unreachable by construction: the callback returns `nil` for everything. The source comment says so and keeps it anyway. |
| `internal/run/sum.go` — `sumFile`'s `hash.Sum` error branch | Reaching it needs a file that opens and then fails to read. No portable trigger. |
| `main.go` — `os.Exit(cmd.Execute())` | Nothing executes the built binary yet. Its own plan. |

The score after the gap-closing work of 2026-09-01 is <N>%. A drop below that,
or a `FAIL` outside this table, means a new gap rather than new noise.
```

- [ ] **Step 3: Correct the table against Step 1**

Any row above that Step 1 did **not** report as surviving must be deleted — a stale "expected survivor" entry is exactly the rot this table exists to prevent. Any survivor Step 1 found that has no row needs one, with a real reason. "Not sure" is not a reason: if you cannot say why it survives, it is a gap, not an accepted survivor, and it belongs in a follow-up rather than in this table.

- [ ] **Step 4: Run the full gates and commit**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`

```bash
git add README.md
git commit -m "docs: record which mutants are expected to survive"
```

---

## Appendix A: the mutant inventory

`go-mutesting --do-not-remove-tmp-folder ./...` at commit `86a1ed0`:

```
The mutation score is 0.822917 (237 passed, 51 failed, 11 duplicated, 0 skipped, total is 288)
```

Survivors by file:

| File | Survivors | Tasks |
|---|---|---|
| `internal/run/sum.go` | 38 | 4, 5, 6, 7 — plus the accepted set |
| `internal/hash/hash.go` | 5 | 1 (one), the other four accepted |
| `internal/checksums/parse.go` | 5 | 2 (four), 3 (one) |
| `internal/run/sums.go` | 1 | 8 |
| `cmd/sum.go` | 1 | 9 |
| `main.go` | 1 | out of scope — its own plan |

Gremlins, by contrast, reports **100% efficacy and zero survivors** on the same commit. Its mutators only rewrite operators, so it cannot delete a statement, drop an error return or remove a branch — which is the entire class of defect this plan closes. Both numbers are correct; they measure different things.

## Appendix B: the predicted accepted set

Written before the tests, to be replaced by measurement in Task 10.

- `internal/hash/hash.go` — four `copyBufSize` arithmetic mutants. Equivalent: the buffer size cannot change a digest.
- `internal/run/sum.go` — `return flushErr` and `return closeErr`. Making a flush or close fail on a regular file has no portable trigger; `/dev/full` exists only on Linux.
- `internal/run/sum.go` — the `if err != nil` block after `WalkDir` returns, and its two counter bumps. The callback returns `nil` for everything, so the block is unreachable; the source comment already says so.
- `internal/run/sum.go` — `sumFile`'s `hash.Sum` error branch, which needs a file that opens successfully and then fails to read.
- `main.go` — `os.Exit(cmd.Execute())`, pending the binary-harness plan.

## Out of scope

**The binary harness.** Killing `main.go`'s mutant means building the binary inside a test and executing it, which is a different kind of work from the nine tasks above and overlaps a follow-up already parked: golden files, absolute paths, an unrelated working directory, unicode filenames. Planning them together yields one harness rather than a sketch and then a rewrite. That plan comes next.

**Production code changes.** Decided at the outset: where a path cannot be reached from a test, it is recorded rather than reshaped. `Sum`'s output destination stays a path rather than becoming an injectable writer.
