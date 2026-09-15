package source

import (
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// withClient points the package client at ts for the duration of one test.
// The tests here must not run in parallel: they share that variable.
func withClient(t *testing.T, ts *httptest.Server) {
	t.Helper()
	saved := client
	client = newClient(ts.Client().Transport.(*http.Transport))
	t.Cleanup(func() { client = saved })
}

// A checksum is computed over the file as stored. A default transport asks
// for gzip and silently decompresses, which would hash the wrong bytes and
// report a mismatch against the published digest.
func TestClientDoesNotDecompress(t *testing.T) {
	var stored []byte
	var buf strings.Builder
	zw := gzip.NewWriter(&buf)
	if _, err := io.WriteString(zw, "abc"); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	stored = []byte(buf.String())

	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				t.Errorf("Accept-Encoding = %q, want gzip not to be offered",
					r.Header.Get("Accept-Encoding"))
			}
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(stored)
		}))
	defer ts.Close()
	withClient(t, ts)

	rc, err := Open(ts.URL + "/dist.tar.gz")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(stored) {
		t.Errorf("got %d bytes, want the %d stored bytes — the body was decompressed",
			len(got), len(stored))
	}
}

func TestClientFollowsRedirects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/start" {
				http.Redirect(w, r, "/end", http.StatusFound)
				return
			}
			_, _ = io.WriteString(w, "abc")
		}))
	defer ts.Close()
	withClient(t, ts)

	rc, err := Open(ts.URL + "/start")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "abc" {
		t.Errorf("contents = %q, want %q", b, "abc")
	}
}

// StatusError.URL names where the request ended, not where it started, so a
// script chasing a broken link is shown the link's target. This pins the doc
// comment on that field, which Task 3 could not: it needs a redirect to test.
func TestClientReportsTheFinalURLOnAStatusError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/start" {
				http.Redirect(w, r, "/gone", http.StatusFound)
				return
			}
			http.NotFound(w, r)
		}))
	defer ts.Close()
	withClient(t, ts)

	_, err := Open(ts.URL + "/start")
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %v, want a *StatusError", err)
	}
	if want := ts.URL + "/gone"; statusErr.URL != want {
		t.Errorf("StatusError.URL = %q, want %q — the URL before the redirect", statusErr.URL, want)
	}
}

// Installing any CheckRedirect at all removes Go's ten-hop cap, because
// Client.checkRedirect only falls back to defaultCheckRedirect while the
// field is nil. Without re-imposing it, this handler loops forever.
func TestClientStopsARedirectLoop(t *testing.T) {
	var hops atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			hops.Add(1)
			http.Redirect(w, r, "/loop", http.StatusFound)
		}))
	defer ts.Close()
	withClient(t, ts)

	_, err := Open(ts.URL + "/loop")
	if err == nil {
		t.Fatal("Open() error = nil, want the redirect chain to be stopped")
	}
	if !strings.Contains(err.Error(), "stopped after") {
		t.Errorf("error = %q, want it to report the hop limit", err)
	}
	// checkRedirect runs before request N+1 with len(via) == N, so the policy
	// stops after exactly maxRedirects requests — not merely "at most".
	if n := int(hops.Load()); n != maxRedirects {
		t.Errorf("served %d requests, want exactly %d", n, maxRedirects)
	}
}

// The checksum file is the trust anchor: a digest fetched over plain HTTP can
// be rewritten in flight to match a file rewritten in flight.
func TestClientRefusesAnHTTPSDowngrade(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "abc")
		}))
	defer plain.Close()

	secure := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, plain.URL+"/SHA256SUMS", http.StatusFound)
		}))
	defer secure.Close()
	// The TLS server's certificate is not in the system pool, so the client
	// has to be built over its transport or the request fails before the
	// redirect policy ever runs.
	withClient(t, secure)

	_, err := Open(secure.URL + "/SHA256SUMS")
	if err == nil {
		t.Fatal("Open() error = nil, want the downgrade to be refused")
	}
	if !strings.Contains(err.Error(), "downgrades") {
		t.Errorf("error = %q, want it to say the redirect downgrades", err)
	}
	if !strings.Contains(err.Error(), secure.URL) {
		t.Errorf("error = %q, want it to name the https URL", err)
	}
	if !strings.Contains(err.Error(), plain.URL+"/SHA256SUMS") {
		t.Errorf("error = %q, want it to name the http target", err)
	}
}

// The stall has to happen BEFORE the headers are written. A handler that
// blocks after WriteHeader tests nothing: Get has already returned by then,
// and such a test passes with headerTimeout at zero.
func TestClientTimesOutWaitingForHeaders(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(
		func(_ http.ResponseWriter, _ *http.Request) {
			<-release
		}))
	defer func() {
		close(release)
		ts.Close()
	}()

	saved := headerTimeout
	headerTimeout = 50 * time.Millisecond
	t.Cleanup(func() { headerTimeout = saved })
	withClient(t, ts)

	done := make(chan error, 1)
	go func() {
		_, err := Open(ts.URL + "/f")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Open() error = nil, want a header timeout")
		}
		if !strings.Contains(err.Error(), "timeout awaiting response headers") {
			t.Errorf("error = %q, want a response-header timeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Open() did not return: the header timeout is not in effect")
	}
}

// A listener that accepts TCP and never handshakes is bounded by
// TLSHandshakeTimeout, which a bare http.Transport does not have.
func TestClientTimesOutOnTheTLSHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Accept and then say nothing at all.
			defer conn.Close()
		}
	}()

	saved := tlsTimeout
	tlsTimeout = 50 * time.Millisecond
	t.Cleanup(func() { tlsTimeout = saved })
	savedClient := client
	client = newClient(http.DefaultTransport.(*http.Transport))
	t.Cleanup(func() { client = savedClient })

	done := make(chan error, 1)
	go func() {
		_, err := Open("https://" + ln.Addr().String() + "/f")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Open() error = nil, want a TLS handshake timeout")
		}
		if !strings.Contains(err.Error(), "TLS handshake timeout") {
			t.Errorf("error = %q, want a TLS handshake timeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Open() did not return: TLSHandshakeTimeout is not set")
	}
}

// Proxy support is not a feature this package adds; it is a feature it must
// not lose by building a transport from nothing.
func TestClientKeepsTheDefaultTransportSettings(t *testing.T) {
	c := newClient(http.DefaultTransport.(*http.Transport))
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", c.Transport)
	}
	if tr.Proxy == nil {
		t.Error("Proxy is nil: the transport was not cloned from DefaultTransport")
	}
	if c.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0 — it would cap a large download", c.Timeout)
	}
	if !tr.DisableCompression {
		t.Error("DisableCompression = false, want true")
	}
	// The clone must not have mutated the shared default.
	if def := http.DefaultTransport.(*http.Transport); def.DisableCompression {
		t.Error("newClient mutated http.DefaultTransport")
	}
}
