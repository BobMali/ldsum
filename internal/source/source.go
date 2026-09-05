// Package source opens what a reference names, whether that is a local path
// or an http(s) URL. It is the only package here that reaches the network.
package source

import (
	"fmt"
	"net/url"
	"strings"
)

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
