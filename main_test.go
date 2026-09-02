// Package main_test executes the built binary. It is the only test in the
// tree that leaves the process boundary: every other one drives cobra in
// process, where the exit status, real argv and the real working directory
// cannot be observed.
package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
