// Package checksums renders and parses the lines of a checksum file. It works
// on writers and strings and knows nothing about files.
package checksums

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/BobMali/ldsum/internal/hash"
)

// Format is one of the line shapes a checksum file can use.
type Format int

// The supported line shapes. Text and Binary are the GNU coreutils formats;
// they differ only in the marker between the digest and the path. Tag is the
// BSD-style "ALGO (path) = digest" line, and Bare prints only the digest.
const (
	Text Format = iota
	Binary
	Tag
	Bare
)

// Entry is one file's digest together with the path it belongs to.
type Entry struct {
	Digest hash.Digest
	Path   string
}

// Render writes e as a single line, including its trailing newline.
func Render(w io.Writer, e Entry, f Format) error {
	switch f {
	case Text, Binary:
		marker := "  "
		if f == Binary {
			marker = " *"
		}
		_, err := fmt.Fprintf(w, "%s%s%s\n", e.Digest.Hex, marker, e.Path)
		return err
	case Tag:
		_, err := fmt.Fprintf(w, "%s (%s) = %s\n",
			strings.ToUpper(string(e.Digest.Algorithm)), e.Path, e.Digest.Hex)
		return err
	case Bare:
		_, err := fmt.Fprintf(w, "%s\n", e.Digest.Hex)
		return err
	default:
		return errors.New("unknown checksum format " + strconv.Itoa(int(f)))
	}
}
