package source

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// Overridable so a test can watch a stall without waiting out a real one.
// They are vars for that reason alone; see the plan's mutation-testing note.
var (
	dialTimeout   = 10 * time.Second
	tlsTimeout    = 10 * time.Second
	headerTimeout = 30 * time.Second
	maxRedirects  = 10
)

// keepAlive preserves what DefaultTransport's own dialer set; replacing
// DialContext to add dialTimeout would otherwise silently drop it.
const keepAlive = 30 * time.Second

var client = newClient(http.DefaultTransport.(*http.Transport))

// newClient returns a client over a clone of t. Cloning rather than building
// from nothing is the point: DefaultTransport is where ProxyFromEnvironment,
// HTTP/2 and the connection-pool settings live, and a bare Transport has none
// of them — not even a TLS handshake timeout.
func newClient(t *http.Transport) *http.Client {
	tr := t.Clone()
	tr.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: keepAlive,
	}).DialContext
	tr.TLSHandshakeTimeout = tlsTimeout
	tr.ResponseHeaderTimeout = headerTimeout
	// A checksum describes the file as stored. Transparent gzip would have us
	// hash the decompressed bytes and call a good file corrupt.
	tr.DisableCompression = true
	// No Client.Timeout: it caps the body read too, so any value large enough
	// for a release artifact is too large to be a timeout.
	return &http.Client{Transport: tr, CheckRedirect: checkRedirect}
}

// checkRedirect refuses a downgrade, and re-imposes the hop limit that
// installing a policy at all removes — Client.checkRedirect uses Go's
// ten-hop default only while CheckRedirect is nil.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	prev := via[len(via)-1]
	if prev.URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("redirect from %s downgrades to %s",
			prev.URL, req.URL.Scheme)
	}
	return nil
}
