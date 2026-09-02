// Package main_test executes the built binary. It is the only test that runs
// ldsum itself out of process: the cmd tests drive the same cobra tree in
// process, where the exit status and real argv cannot be observed, and where
// the working directory can only be changed by mutating the test binary's
// own.
package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The sha256 of "abc" — a known-answer vector, not a digest this suite
// produced. cmd/verify_test.go binds the same value for the same reason.
const abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

// binary is the freshly built ldsum under test, set by TestMain.
var binary string

func TestMain(m *testing.M) {
	// t.TempDir needs a *testing.T, which TestMain does not have, so the
	// directory is made and removed by hand.
	dir, err := os.MkdirTemp("", "ldsum-harness")
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness: temp dir: %v\n", err)
		os.Exit(1)
	}

	// Built fresh rather than reusing the ./ldsum in the working tree, which
	// is gitignored and may be any age. Only the returned error decides
	// success: go build writes to stderr on a good run too — a sandboxed one
	// emits a module-cache warning and still exits 0.
	binary = filepath.Join(dir, "ldsum")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "harness: go build: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	// os.Exit skips deferred functions, so the cleanup is spelled out.
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// run executes the built binary with dir as its working directory — the one
// thing this harness exists to vary — and reports both streams and the
// process exit status.
func run(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	var exit *exec.ExitError
	switch err := cmd.Run(); {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		// The binary could not be started at all: a broken harness rather
		// than a failing case.
		t.Fatalf("run %v: %v", args, err)
	}
	return out.String(), errOut.String(), code
}

func writeIn(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestBinarySumPrintsADigest(t *testing.T) {
	tests := []struct {
		name       string
		contents   string
		wantStdout string
	}{
		{"known-answer vector", "abc", abcSHA256 + "  payload.txt\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeIn(t, dir, "payload.txt", tt.contents)

			stdout, stderr, code := run(t, dir, "sum", "payload.txt")
			if code != 0 {
				t.Errorf("exit = %d, want 0\nstderr: %s", code, stderr)
			}
			if stdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tt.wantStdout)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestBinaryExitCodes(t *testing.T) {
	// A well-formed sha256 that nothing hashes to.
	const zeroSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"

	t.Run("a mismatch exits 1", func(t *testing.T) {
		dir := t.TempDir()
		writeIn(t, dir, "payload.txt", "abc")

		stdout, stderr, code := run(t, dir, "verify", "payload.txt", zeroSHA256)
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if want := "payload.txt: FAILED\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		want := "expected: " + zeroSHA256 + "\nactual:   " + abcSHA256 + "\n"
		if stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	})

	t.Run("an unknown algorithm exits 2", func(t *testing.T) {
		dir := t.TempDir()
		writeIn(t, dir, "payload.txt", "abc")

		stdout, stderr, code := run(t, dir, "verify", "--algo", "nonesuch", "payload.txt", abcSHA256)
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		// The sentence itself belongs to internal/run, whose tests own it.
		// What this case pins is that it reached the real stderr, prefixed.
		if !strings.HasPrefix(stderr, "ldsum: ") || !strings.Contains(stderr, "unknown algorithm") {
			t.Errorf("stderr = %q, want an ldsum-prefixed unknown-algorithm diagnostic", stderr)
		}
		if !strings.Contains(stderr, "nonesuch") {
			t.Errorf("stderr = %q, want the offending algorithm named", stderr)
		}
	})
}

func TestBinaryResolvesPathsAgainstTheRightDirectory(t *testing.T) {
	t.Run("a relative entry resolves against the checksum file", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "w")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeIn(t, dir, "payload.txt", "abc")
		writeIn(t, dir, "SUMS", abcSHA256+"  payload.txt\n")

		// The working directory is the parent, where payload.txt does not
		// exist: resolving the entry against the process's own directory
		// would fail outright. Setting it on a launched process rather than
		// with t.Chdir leaves the test binary's own directory alone.
		stdout, stderr, code := run(t, parent, "verify", "-c", filepath.Join("w", "SUMS"))
		if code != 0 {
			t.Errorf("exit = %d, want 0\nstderr: %s", code, stderr)
		}
		if want := filepath.Join("w", "payload.txt") + ": OK\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("an absolute entry is used as it stands", func(t *testing.T) {
		dir := t.TempDir()
		payload := writeIn(t, dir, "payload.txt", "abc")
		sums := writeIn(t, dir, "SUMS", abcSHA256+"  "+payload+"\n")

		// Run from a directory with no relation to either file.
		stdout, stderr, code := run(t, t.TempDir(), "verify", "-c", sums)
		if code != 0 {
			t.Errorf("exit = %d, want 0\nstderr: %s", code, stderr)
		}
		if want := payload + ": OK\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("a filename containing a space", func(t *testing.T) {
		dir := t.TempDir()
		writeIn(t, dir, "with space.txt", "abc")

		// There is no shell between the harness and the binary; this pins
		// that argv arrives unsplit. A space is not escaped on output —
		// only a backslash or a newline is.
		stdout, stderr, code := run(t, dir, "sum", "with space.txt")
		if code != 0 {
			t.Errorf("exit = %d, want 0\nstderr: %s", code, stderr)
		}
		if want := abcSHA256 + "  with space.txt\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})
}
