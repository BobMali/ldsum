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

// The two arity errors say different things on purpose: with no argument the
// file simply names nothing, while with several the command named too many.
func TestVerifySumsBareDigestArityMessagesDiffer(t *testing.T) {
	dir := t.TempDir()
	sums := writeIn(t, dir, "dist.tar.gz.sha256", abcSHA256+"\n")

	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{name: "none", paths: nil, want: "no paths in file"},
		{name: "two", paths: []string{"a", "b"}, want: "name exactly one file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifySums(io.Discard, io.Discard, SumsOptions{SumsFile: sums, Paths: tt.paths})
			if err == nil {
				t.Fatal("VerifySums() = nil error, want one")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// `ldsum sum` writes an absolute path whenever it is given one, so a checksum
// file may spell its entries absolutely. Such an entry already says where the
// file is; joining it onto the checksum file's directory doubles it.
func TestVerifySumsAbsoluteEntry(t *testing.T) {
	dir := t.TempDir()
	target := writeIn(t, dir, "a.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS", abcSHA256+"  "+target+"\n")

	var out, errOut bytes.Buffer
	if err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sums}); err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := target + ": OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

func TestVerifySumsAbsoluteEntryNamedAsArgument(t *testing.T) {
	dir := t.TempDir()
	target := writeIn(t, dir, "a.txt", "abc")
	other := writeIn(t, dir, "b.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS",
		abcSHA256+"  "+target+"\n"+abcSHA256+"  "+other+"\n")

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sums, Paths: []string{target}})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := target + ": OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// With the checksum file named without a directory the base is ".", and
// joining an absolute entry onto it strips the leading separator instead of
// doubling it. The result is a working-directory-relative path, so the run
// would read whatever happens to sit there rather than the file the checksum
// file named.
func TestVerifySumsAbsoluteEntryWithBareSumsFileName(t *testing.T) {
	dir := t.TempDir()
	target := writeIn(t, dir, "a.txt", "abc")
	writeIn(t, dir, "SHA256SUMS", abcSHA256+"  "+target+"\n")

	t.Chdir(dir)

	var out, errOut bytes.Buffer
	if err := VerifySums(&out, &errOut, SumsOptions{SumsFile: "SHA256SUMS"}); err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := target + ": OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}
