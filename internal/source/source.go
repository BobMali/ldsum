// Package source opens what a reference names, whether that is a local path
// or an http(s) URL. It is the only package here that reaches the network.
package source

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var client = &http.Client{}

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
		return os.Open(ref)
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
		// Draining before Close lets the transport confirm the body was fully
		// consumed, which is what lets it return the connection to the pool
		// instead of tearing it down.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return nil, &StatusError{
			URL:    resp.Request.URL.String(),
			Status: resp.Status,
			Code:   resp.StatusCode,
		}
	}
	return resp.Body, nil
}
