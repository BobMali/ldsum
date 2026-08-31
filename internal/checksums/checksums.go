// Package checksums renders and reads the lines of a checksum file. It works
// on readers, writers and strings, and knows nothing about files.
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

// Entry is one file's digest together with the path it belongs to. Line is
// the 1-based line Parse read it from, and is zero for an entry that did not
// come from a file.
type Entry struct {
	Digest hash.Digest
	Path   string
	Line   int
}

// Render writes e as a single line, including its trailing newline.
func Render(w io.Writer, e Entry, f Format) error {
	switch f {
	case Text, Binary:
		marker := "  "
		if f == Binary {
			marker = " *"
		}
		// A leading backslash is how coreutils marks a line whose path was
		// escaped, so a reader knows to unescape it.
		prefix, path := "", e.Path
		if needsEscape(path) {
			prefix, path = "\\", escaper.Replace(path)
		}
		_, err := fmt.Fprintf(w, "%s%s%s%s\n", prefix, e.Digest.Hex, marker, path)
		return err
	case Tag:
		// Tagged format has no escape convention upstream, so there is no
		// correct line to write rather than one we invented.
		if needsEscape(e.Path) {
			return errors.New(e.Path +
				": tagged format cannot carry a backslash or newline in a path")
		}
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

// escaper is stateless and safe to share; building it once keeps it off the
// per-line path.
var escaper = strings.NewReplacer("\\", "\\\\", "\n", "\\n")

// needsEscape reports whether p holds a character a line cannot carry as
// itself: a backslash would be ambiguous, and a newline would split the entry
// into two malformed lines.
func needsEscape(p string) bool {
	return strings.ContainsAny(p, "\\\n")
}
