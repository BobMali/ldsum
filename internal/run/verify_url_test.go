package run

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve starts a test server answering every path with body, and returns the
// URL of "payload.txt" on it. The server closes itself when the test ends.
func serve(t *testing.T, body string) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
	t.Cleanup(ts.Close)
	return ts.URL + "/payload.txt"
}

func TestVerifyURLTarget(t *testing.T) {
	t.Run("a match prints OK and returns nil", func(t *testing.T) {
		url := serve(t, "abc")
		var out, errOut bytes.Buffer

		err := Verify(&out, &errOut, VerifyOptions{Path: url, Expected: abcSHA256})
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		if want := url + ": OK\n"; out.String() != want {
			t.Errorf("stdout = %q, want %q", out.String(), want)
		}
		if errOut.Len() != 0 {
			t.Errorf("stderr = %q, want empty", errOut.String())
		}
	})

	t.Run("a mismatch is a MismatchError", func(t *testing.T) {
		url := serve(t, "not abc")
		var out, errOut bytes.Buffer

		err := Verify(&out, &errOut, VerifyOptions{Path: url, Expected: abcSHA256})
		var mismatch *MismatchError
		if !errors.As(err, &mismatch) {
			t.Fatalf("Verify() error = %v, want a *MismatchError", err)
		}
		if want := url + ": FAILED\n"; out.String() != want {
			t.Errorf("stdout = %q, want %q", out.String(), want)
		}
		if !strings.Contains(errOut.String(), "expected: "+abcSHA256) {
			t.Errorf("stderr = %q, want the expected digest", errOut.String())
		}
	})

	t.Run("--algo applies to a URL", func(t *testing.T) {
		url := serve(t, "abc")
		var out, errOut bytes.Buffer

		err := Verify(&out, &errOut, VerifyOptions{
			Path: url, Expected: abcSHA512, Algorithm: "sha512",
		})
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		if want := url + ": OK\n"; out.String() != want {
			t.Errorf("stdout = %q, want %q", out.String(), want)
		}
	})

	// A missing remote target is the same answer as a missing local one:
	// exit 1, not exit 2.
	t.Run("a 404 is a MissingTargetError", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			}))
		defer ts.Close()
		url := ts.URL + "/gone.txt"
		var out, errOut bytes.Buffer

		err := Verify(&out, &errOut, VerifyOptions{Path: url, Expected: abcSHA256})
		var missing *MissingTargetError
		if !errors.As(err, &missing) {
			t.Fatalf("Verify() error = %T (%v), want a *MissingTargetError", err, err)
		}
		if want := url + ": FAILED open or read\n"; out.String() != want {
			t.Errorf("stdout = %q, want %q", out.String(), want)
		}
	})

	// A body cut short fails inside hash.Sum with a bare "unexpected EOF",
	// which names nothing. A local file would have failed with an
	// *fs.PathError carrying its path, so the URL has to be added here.
	t.Run("a truncated body names the URL", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "100")
				_, _ = io.WriteString(w, "abc")
			}))
		defer ts.Close()
		url := ts.URL + "/short.txt"
		var out, errOut bytes.Buffer

		err := Verify(&out, &errOut, VerifyOptions{Path: url, Expected: abcSHA256})
		if err == nil {
			t.Fatal("Verify() error = nil, want a read error")
		}
		if !strings.Contains(err.Error(), url) {
			t.Errorf("error = %q, want it to name %q", err, url)
		}
	})
}

// A directory opens cleanly through source.Open but fails inside hash.Sum
// with an *fs.PathError that already names it. The new guard must leave that
// error bare: wrapping it unconditionally would pass every other assertion
// in this package, since none of them pin the read-failure message.
func TestVerifyLeavesTheLocalReadErrorBare(t *testing.T) {
	dir := t.TempDir()

	err := Verify(io.Discard, io.Discard, VerifyOptions{Path: dir, Expected: abcSHA256})
	if _, ok := err.(*fs.PathError); !ok {
		t.Fatalf("Verify() error = %v (%T), want a bare *fs.PathError", err, err)
	}
}
