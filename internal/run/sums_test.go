package run

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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
