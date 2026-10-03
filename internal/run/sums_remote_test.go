package run

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// The guard has to run before the checksum file is opened, or a missing local
// file reports "no such file" and never mentions the flag that was wrong.
func TestVerifySumsRemoteTargetsNeedsARemoteSumsFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "SHA256SUMS")

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile:      missing,
		RemoteTargets: true,
	})
	if err == nil {
		t.Fatal("VerifySums() error = nil, want the flag to be refused")
	}
	if !strings.Contains(err.Error(), "--remote-targets") {
		t.Errorf("error = %q, want it to name the flag", err)
	}
	if strings.Contains(err.Error(), "no such file") {
		t.Errorf("error = %q, want the flag error rather than the open error", err)
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q, want both empty",
			out.String(), errOut.String())
	}
}

// An unfetchable scheme is IsRemote's own error, not "needs a URL": the user
// did give a URL, just one this tool cannot fetch, and conflating the two
// would blame the wrong thing.
func TestVerifySumsRemoteTargetsUnsupportedScheme(t *testing.T) {
	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile:      "ftp://host/SHA256SUMS",
		RemoteTargets: true,
	})
	if err == nil {
		t.Fatal("VerifySums() error = nil, want the unsupported scheme to be refused")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Errorf("error = %q, want it to name the unsupported scheme", err)
	}
	if strings.Contains(err.Error(), "needs a URL") {
		t.Errorf("error = %q, want the scheme error, not the flag error", err)
	}
}

// sumsServer serves body at /v1.2/SHA256SUMS and each of files at its own
// path. Anything else 404s, so a test that expects a fetch proves it by the
// file existing only here, and a test that expects a local read proves it by
// not asking for one.
func sumsServer(t *testing.T, body string, files map[string]string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1.2/SHA256SUMS" {
				_, _ = io.WriteString(w, body)
				return
			}
			contents, ok := files[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = io.WriteString(w, contents)
		}))
	t.Cleanup(ts.Close)
	return ts
}

// The default for a remote listing: entries name local files, so nothing is
// downloaded but the listing itself.
func TestVerifySumsRemoteListingChecksLocalFiles(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	ts := sumsServer(t, abcSHA256+"  a.txt\n", nil)

	// Run from the directory holding the file, since a remote listing's
	// relative entries resolve against the working directory.
	t.Chdir(dir)

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{SumsFile: ts.URL + "/v1.2/SHA256SUMS"})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := "a.txt: OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// With the flag, the same listing fetches each entry from the listing's URL.
func TestVerifySumsRemoteTargetsFetchesEntries(t *testing.T) {
	ts := sumsServer(t, abcSHA256+"  a.txt\n",
		map[string]string{"/v1.2/a.txt": "abc"})

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile:      ts.URL + "/v1.2/SHA256SUMS",
		RemoteTargets: true,
	})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	want := ts.URL + "/v1.2/a.txt: OK\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// Positional arguments filter a remote listing the same way they filter a
// local one: they name entries as the file spells them, not resolved forms.
func TestVerifySumsRemoteTargetsWithFilters(t *testing.T) {
	ts := sumsServer(t, abcSHA256+"  a.txt\n"+abcSHA256+"  b.txt\n",
		map[string]string{"/v1.2/a.txt": "abc", "/v1.2/b.txt": "abc"})

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile:      ts.URL + "/v1.2/SHA256SUMS",
		Paths:         []string{"b.txt"},
		RemoteTargets: true,
	})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	want := ts.URL + "/v1.2/b.txt: OK\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want only the named entry %q", out.String(), want)
	}
}

// An entry that spells out a full URL points where it says, in every row.
func TestVerifySumsURLEntryIsUsedAsItIs(t *testing.T) {
	files := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "abc")
		}))
	defer files.Close()
	entry := files.URL + "/elsewhere/a.txt"

	dir := t.TempDir()
	sums := writeIn(t, dir, "SHA256SUMS", abcSHA256+"  "+entry+"\n")

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sums})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := entry + ": OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q — a URL entry must not be joined to a directory",
			out.String(), want)
	}
}

// Naming that entry as an argument must find it and fetch it: the lookup is
// on the entry as the file spells it, and the match must not send a URL
// through filepath.Join on its way to being opened.
func TestVerifySumsURLEntryCanBeNamed(t *testing.T) {
	files := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "abc")
		}))
	defer files.Close()
	entry := files.URL + "/elsewhere/a.txt"

	dir := t.TempDir()
	writeIn(t, dir, "b.txt", "abc")
	sums := writeIn(t, dir, "SHA256SUMS",
		abcSHA256+"  "+entry+"\n"+abcSHA256+"  b.txt\n")

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile: sums,
		Paths:    []string{entry},
	})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := entry + ": OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want only the named entry %q", out.String(), want)
	}
}

// A bare-digest listing names no entries, so the flag has nothing to resolve.
// A flag that does nothing is refused rather than ignored.
func TestVerifySumsBareDigestRefusesRemoteTargets(t *testing.T) {
	ts := sumsServer(t, abcSHA256+"\n", nil)

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile:      ts.URL + "/v1.2/SHA256SUMS",
		Paths:         []string{"dist.tar.gz"},
		RemoteTargets: true,
	})
	if err == nil {
		t.Fatal("VerifySums() error = nil, want the flag to be refused")
	}
	if !strings.Contains(err.Error(), "--remote-targets") {
		t.Errorf("error = %q, want it to name the flag", err)
	}
}

// Without the flag, a remote bare-digest file checks the local file named.
func TestVerifySumsRemoteBareDigestChecksALocalFile(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "dist.tar.gz", "abc")
	ts := sumsServer(t, abcSHA256+"\n", nil)
	t.Chdir(dir)

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile: ts.URL + "/v1.2/SHA256SUMS",
		Paths:    []string{"dist.tar.gz"},
	})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := "dist.tar.gz: OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// A missing checksum file is the command being wrong (exit 2), while a
// missing target is the user's file to fix (exit 1). A 404 must not blur that.
func TestVerifySums404ChecksumFileIsNotAMissingTarget(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
	defer ts.Close()

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{SumsFile: ts.URL + "/SHA256SUMS"})
	if err == nil {
		t.Fatal("VerifySums() error = nil, want a 404 error")
	}
	var missing *MissingTargetError
	if errors.As(err, &missing) {
		t.Errorf("error = %v, want it not to be a *MissingTargetError", err)
	}
}

// Warnings name the reference the listing came from, whatever kind it is.
func TestVerifySumsRemoteWarningsNameTheURL(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	ts := sumsServer(t, "not a checksum line\n"+abcSHA256+"  a.txt\n", nil)
	t.Chdir(dir)

	var out, errOut bytes.Buffer
	if err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile: ts.URL + "/v1.2/SHA256SUMS",
	}); err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	want := ts.URL + "/v1.2/SHA256SUMS:1:"
	if !strings.Contains(errOut.String(), want) {
		t.Errorf("stderr = %q, want it to start a warning with %q", errOut.String(), want)
	}
}

// Under a URL base with --remote-targets, a leading-"/" entry is a URL path
// on that host, not a local absolute path: resolve must reach JoinURL before
// the filepath.IsAbs short-circuit. A full-URL entry in the same listing is
// fetched from where it points, same as in every other row.
func TestVerifySumsRemoteTargetsRootPathAndURLEntry(t *testing.T) {
	files := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "abc")
		}))
	defer files.Close()
	entry := files.URL + "/elsewhere/g"

	ts := sumsServer(t, abcSHA256+"  /other/f\n"+abcSHA256+"  "+entry+"\n",
		map[string]string{"/other/f": "abc"})

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile:      ts.URL + "/v1.2/SHA256SUMS",
		RemoteTargets: true,
	})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	want := ts.URL + "/other/f: OK\n" + entry + ": OK\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// A remote listing's target can 404 too: the mismatch between "the checksum
// file was unreachable" (exit 2) and "the named file is missing" (exit 1)
// has to hold for a fetched target as well as a local one.
func TestVerifySumsRemoteTargetsMissingTargetIs404(t *testing.T) {
	ts := sumsServer(t, abcSHA256+"  a.txt\n", nil)

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile:      ts.URL + "/v1.2/SHA256SUMS",
		RemoteTargets: true,
	})
	var missing *MissingTargetError
	if !errors.As(err, &missing) {
		t.Fatalf("VerifySums() error = %T (%v), want a *MissingTargetError", err, err)
	}
	want := ts.URL + "/v1.2/a.txt: FAILED open or read\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// A URL entry is used as it is under a remote listing too, without
// --remote-targets: the entryRemote short-circuit in resolve runs before the
// base switch, so row 2 needs nothing extra to hold.
func TestVerifySumsRemoteListingURLEntryIsUsedAsItIs(t *testing.T) {
	files := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "abc")
		}))
	defer files.Close()
	entry := files.URL + "/elsewhere/a.txt"

	ts := sumsServer(t, abcSHA256+"  "+entry+"\n", nil)

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{SumsFile: ts.URL + "/v1.2/SHA256SUMS"})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := entry + ": OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// The same resolution as the test above, but reached through the named-paths
// branch. Naming entries must not move where they resolve, and the
// all-entries branch cannot stand in for this one: only one of the two is
// taken per run.
func TestVerifySumsRemoteListingChecksNamedLocalFiles(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, "a.txt", "abc")
	writeIn(t, dir, "b.txt", "abc")
	ts := sumsServer(t, abcSHA256+"  a.txt\n"+abcSHA256+"  b.txt\n", nil)

	// As above: a remote listing's relative entries resolve against the
	// working directory unless --remote-targets says otherwise.
	t.Chdir(dir)

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{
		SumsFile: ts.URL + "/v1.2/SHA256SUMS",
		Paths:    []string{"a.txt"},
	})
	if err != nil {
		t.Fatalf("VerifySums() error = %v", err)
	}
	if want := "a.txt: OK\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

// An entry naming a scheme this package cannot fetch is the entry's problem,
// not the listing's. resolve classifies every entry it is given, and the
// refusal has to reach the caller rather than becoming a local file name.
func TestVerifySumsEntryWithAnUnsupportedScheme(t *testing.T) {
	sums := writeIn(t, t.TempDir(), "SHA256SUMS", abcSHA256+"  ftp://host/f\n")

	var out, errOut bytes.Buffer
	err := VerifySums(&out, &errOut, SumsOptions{SumsFile: sums})
	if err == nil {
		t.Fatal("VerifySums() error = nil, want the entry's scheme to be refused")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Errorf("error = %q, want it to name the unsupported scheme", err)
	}
	if !strings.Contains(err.Error(), `"ftp"`) {
		t.Errorf("error = %q, want it to quote the offending scheme", err)
	}
}
