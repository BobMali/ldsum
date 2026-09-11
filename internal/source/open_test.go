package source

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	t.Run("reads the file", func(t *testing.T) {
		rc, err := Open(path)
		if err != nil {
			t.Fatalf("Open(%q) error = %v", path, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(b) != "abc" {
			t.Errorf("contents = %q, want %q", b, "abc")
		}
	})

	// os already produces an *fs.PathError carrying the operation and the
	// path. Wrapping it here would add a second copy and break the exact
	// message internal/run asserts.
	t.Run("a missing file keeps the os error bare", func(t *testing.T) {
		missing := filepath.Join(dir, "nope.txt")
		_, err := Open(missing)
		if err == nil {
			t.Fatal("Open() error = nil, want an error")
		}
		var pathErr *fs.PathError
		if !errors.As(err, &pathErr) {
			t.Fatalf("error = %T, want an *fs.PathError", err)
		}
		want := "open " + missing + ": no such file or directory"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("an unfetchable scheme is an error", func(t *testing.T) {
		if _, err := Open("ftp://host/f"); err == nil {
			t.Fatal("Open() error = nil, want an unsupported-scheme error")
		}
	})
}

func TestOpenURL(t *testing.T) {
	t.Run("streams the body", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, "abc")
			}))
		defer ts.Close()

		rc, err := Open(ts.URL + "/f")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(b) != "abc" {
			t.Errorf("contents = %q, want %q", b, "abc")
		}
	})

	// The bug this guards: an error page hashes perfectly well, so without a
	// status check a 404 is reported as a checksum mismatch.
	t.Run("status is judged before any body is returned", func(t *testing.T) {
		tests := []struct {
			name         string
			code         int
			wantNotExist bool
		}{
			{name: "404 is a missing file", code: 404, wantNotExist: true},
			{name: "410 is a missing file", code: 410, wantNotExist: true},
			{name: "500 is not", code: 500},
			{name: "403 is not", code: 403},
			{name: "300 is not", code: 300},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ts := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(tt.code)
						// A body that would hash cleanly if it were returned.
						_, _ = io.WriteString(w, "abc")
					}))
				defer ts.Close()

				rc, err := Open(ts.URL + "/f")
				if err == nil {
					_ = rc.Close()
					t.Fatalf("Open() error = nil for %d, want an error", tt.code)
				}
				if rc != nil {
					t.Error("Open() returned a reader alongside its error")
				}
				var statusErr *StatusError
				if !errors.As(err, &statusErr) {
					t.Fatalf("error = %T, want a *StatusError", err)
				}
				if got := errors.Is(err, fs.ErrNotExist); got != tt.wantNotExist {
					t.Errorf("errors.Is(err, fs.ErrNotExist) = %v, want %v",
						got, tt.wantNotExist)
				}
				if !strings.Contains(err.Error(), "/f") {
					t.Errorf("error = %q, want it to name the URL", err)
				}
				if statusErr.Code != tt.code {
					t.Errorf("StatusError.Code = %d, want %d", statusErr.Code, tt.code)
				}
				if statusErr.URL != ts.URL+"/f" {
					t.Errorf("StatusError.URL = %q, want %q", statusErr.URL, ts.URL+"/f")
				}
			})
		}
	})

	// A non-2xx body that is not closed holds its connection open, so a
	// forty-entry listing of 404s would leak forty of them. Counted through
	// ConnState rather than asserted directly: a closed body is returned to
	// the pool and reused, an unclosed one is not.
	t.Run("a non-2xx response closes its body", func(t *testing.T) {
		var opened int
		ts := httptest.NewUnstartedServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				// Bigger than a socket read buffer: an undrained Close then
				// can't have reached EOF, so the connection can't be pooled
				// for reuse. A short body like http.NotFound's often arrives
				// whole before Close runs regardless, which is what let a
				// missing drain pass this check most of the time.
				_, _ = w.Write(bytes.Repeat([]byte("x"), 64*1024))
			}))
		ts.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			if state == http.StateNew {
				opened++
			}
		}
		ts.Start()
		defer ts.Close()

		for i := 0; i < 5; i++ {
			rc, err := Open(ts.URL + "/gone.txt")
			if err == nil {
				_ = rc.Close()
				t.Fatal("Open() error = nil, want a 404 error")
			}
		}
		if opened != 1 {
			t.Errorf("opened %d connections for 5 requests, want 1 — a 404 body was left open",
				opened)
		}
	})

	t.Run("a refused connection is an error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(
			func(_ http.ResponseWriter, _ *http.Request) {}))
		ref := ts.URL + "/f"
		ts.Close()

		_, err := Open(ref)
		if err == nil {
			t.Fatal("Open() error = nil, want a connection error")
		}
		if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("error = %v, want it not to look like a missing file", err)
		}
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			t.Errorf("error = %T, want a transport error, not a *StatusError", err)
		}
		// Pins the "returned unwrapped" claim: *url.Error already names the
		// URL, so a caller that wrapped it again would still pass every
		// check above but would name the URL twice.
		var urlErr *url.Error
		if !errors.As(err, &urlErr) {
			t.Fatalf("error = %T, want a *url.Error", err)
		}
		if !strings.Contains(err.Error(), ref) {
			t.Errorf("error = %q, want it to name %q", err, ref)
		}
	})
}
