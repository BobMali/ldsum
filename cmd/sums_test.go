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
	writeIn(t, dir, "a.txt", "abc")
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
