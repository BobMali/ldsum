// Package source opens what a reference names, whether that is a local path
// or an http(s) URL. It is the only package here that reaches the network.
package source

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

// maxErrorBodyDrain bounds how much of a non-2xx body get reads before
// closing it. It mirrors net/http's own post-Close drain limit.
const maxErrorBodyDrain = 256 << 10

// drainTimeout bounds how long that drain may take. A byte bound alone is no
// bound at all against a server that trickles: the limit is never reached, so
// the read blocks for as long as it cares to stall. Abandoning the drain
// costs only a fresh connection, so this is short.
var drainTimeout = time.Second

// IsRemote reports whether ref names a URL this package fetches. A reference
// shaped like a URL whose scheme cannot be fetched is an error rather than a
// file name: reporting "no such file" for an ftp:// URL would hide what the
// user actually got wrong.
func IsRemote(ref string) (bool, error) {
	u, err := url.Parse(ref)
	// A reference url.Parse rejects — one holding a control character, say —
	// is treated as a file name, not a malformed URL.
	if err != nil || u.Scheme == "" {
		return false, nil
	}
	// Indexed by the scheme's length rather than matched against its text:
	// url.Parse lowercases the scheme, so HTTPS:// would fail a text match.
	// Requiring the slashes is what keeps "weird:thing" a file name.
	if !strings.HasPrefix(ref[len(u.Scheme):], "://") {
		return false, nil
	}
	switch u.Scheme {
	case "http", "https":
		return true, nil
	}
	return false, fmt.Errorf("%s: unsupported scheme %q", ref, u.Scheme)
}

// JoinURL resolves the file name entry against the URL base. The entry is
// built as a value rather than parsed: a checksum file lists file names, and
// url.Parse would read a '#' in one as a fragment and a '?' as a query.
func JoinURL(base, entry string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(&url.URL{Path: entry}).String(), nil
}

// Open returns a reader over what ref names. The caller closes it.
func Open(ref string) (io.ReadCloser, error) {
	remote, err := IsRemote(ref)
	if err != nil {
		return nil, err
	}
	if !remote {
		// Returned unwrapped: os produces an *fs.PathError that already
		// carries the operation and the path.
		f, err := os.Open(ref)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	return get(ref)
}

// get fetches ref and returns its body, having first judged the status. The
// order matters: an error page hashes as happily as a real file, so a 404
// left unchecked is reported as a checksum mismatch.
func get(ref string) (io.ReadCloser, error) {
	resp, err := client.Get(ref)
	if err != nil {
		// *url.Error already names the operation and the URL.
		return nil, err
	}
	// Division rather than a range compare: two mutants instead of four
	// boundary ones, and the 200 and 404 cases kill both.
	if resp.StatusCode/100 != 2 {
		drain(resp.Body)
		return nil, &StatusError{
			URL:    resp.Request.URL.String(),
			Status: resp.Status,
			Code:   resp.StatusCode,
		}
	}
	return resp.Body, nil
}

// drain reads a non-2xx body far enough to return its connection to the pool,
// then closes it. The transport drains on Close too, but asynchronously, so a
// following sequential request may not find the connection idle yet; doing it
// here makes reuse synchronous.
//
// The read is bounded in bytes and in time, and runs in a goroutine so that a
// stalled body is abandoned rather than waited out: closing the body is what
// unblocks the read, and nothing here is ever shown to anyone.
func drain(body io.ReadCloser) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.CopyN(io.Discard, body, maxErrorBodyDrain)
	}()
	select {
	case <-done:
	case <-time.After(drainTimeout):
	}
	_ = body.Close()
}
