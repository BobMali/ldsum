package checksums

import (
	"bufio"
	"errors"
	"io"
	"strings"

	"github.com/BobMali/ldsum/internal/hash"
)

// Listing is what one checksum file contained.
type Listing struct {
	Entries []Entry
	Bad     []BadLine
}

// BadLine is a line Parse could not read as a checksum.
type BadLine struct {
	Line int
	Err  error
}

var errNotChecksum = errors.New("not a checksum line")

// Parse reads r as a checksum file. Every line is recognised on its own, so
// one file may mix formats and algorithms. The returned error reports a
// failure to read r; a line that is not a checksum becomes a BadLine.
func Parse(r io.Reader) (Listing, error) {
	var l Listing
	s := bufio.NewScanner(r)
	for n := 1; s.Scan(); n++ {
		// Checksum files travel between systems, so a CRLF ending is not the
		// author saying the path ends in a carriage return.
		line := strings.TrimSuffix(s.Text(), "\r")
		if trimmed := strings.TrimLeft(line, " \t"); trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		e, err := parseLine(line)
		if err != nil {
			l.Bad = append(l.Bad, BadLine{Line: n, Err: err})
			continue
		}
		e.Line = n
		l.Entries = append(l.Entries, e)
	}
	if err := s.Err(); err != nil {
		return Listing{}, err
	}
	return l, nil
}

func parseLine(line string) (Entry, error) {
	hexRun := leadingHex(line)
	if hexRun == "" {
		return Entry{}, errNotChecksum
	}
	path, ok := gnuPath(line[len(hexRun):])
	if !ok {
		return Entry{}, errNotChecksum
	}
	if path == "" {
		return Entry{}, errors.New("checksum with no path")
	}
	d, err := hash.ParseDigest(hexRun)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Digest: d, Path: path}, nil
}

// gnuPath splits the path off what follows a GNU-format digest. Two spaces
// mean text and " *" means binary; a single space is accepted too, because
// tools that emit one separator instead of two are common. The two-space case
// is tested first, so a path that itself starts with an asterisk survives.
func gnuPath(rest string) (string, bool) {
	switch {
	case strings.HasPrefix(rest, "  "):
		return rest[2:], true
	case strings.HasPrefix(rest, " *"):
		return rest[2:], true
	case strings.HasPrefix(rest, " "):
		return rest[1:], true
	default:
		return "", false
	}
}

func leadingHex(s string) string {
	i := 0
	for i < len(s) && isHexDigit(s[i]) {
		i++
	}
	return s[:i]
}

func isHexDigit(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}
