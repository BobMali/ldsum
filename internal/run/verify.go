// Package run orchestrates the work behind each command. It returns errors
// and never exits.
package run

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/hash"
)

// VerifyOptions is one verification request. An empty Algorithm means infer
// it from the length of Expected.
type VerifyOptions struct {
	Path      string
	Expected  string
	Algorithm string
}

// MismatchError reports a file whose digest differs from the expected one. It
// is the one failure that means the check ran fine and the answer was no.
type MismatchError struct {
	Path     string
	Expected hash.Digest
	Actual   hash.Digest
}

func (e *MismatchError) Error() string {
	return e.Path + ": checksum mismatch"
}

// MissingTargetError reports that the file being verified does not exist. It
// says which file was missing, which a bare fs.ErrNotExist cannot: once a
// checksum file can also be named, only one of them means exit 1.
type MissingTargetError struct {
	Path string
	Err  error
}

// Error delegates so the message stays exactly what the os layer produced.
func (e *MissingTargetError) Error() string { return e.Err.Error() }

func (e *MissingTargetError) Unwrap() error { return e.Err }

// Verify streams the file at opts.Path through the hasher and reports whether
// its digest matches opts.Expected.
func Verify(out, errOut io.Writer, opts VerifyOptions) error {
	expected, err := parseExpected(opts)
	if err != nil {
		return fmt.Errorf("verify %s: %w", opts.Path, err)
	}
	return verifyEntry(out, errOut, opts.Path, expected)
}

// verifyEntry hashes one file and reports whether it matches.
func verifyEntry(out, errOut io.Writer, path string, expected hash.Digest) error {
	f, err := os.Open(path)
	if err != nil {
		// A file that cannot be read gets a verdict like any other, so a run
		// over many files names it rather than only counting it.
		verdict(out, path, "FAILED open or read")
		if errors.Is(err, fs.ErrNotExist) {
			return &MissingTargetError{Path: path, Err: err}
		}
		return err
	}
	defer f.Close()

	actual, err := hash.Sum(f, expected.Algorithm)
	if err != nil {
		// A directory opens cleanly and fails only here, so this site needs the
		// same verdict as the one above.
		verdict(out, path, "FAILED open or read")
		return err
	}

	if !actual.Equal(expected) {
		verdict(out, path, "FAILED")
		fmt.Fprintf(errOut, "expected: %s\n", expected.Hex)
		fmt.Fprintf(errOut, "actual:   %s\n", actual.Hex)
		return &MismatchError{Path: path, Expected: expected, Actual: actual}
	}

	verdict(out, path, "OK")
	return nil
}

// verdict writes one file's result. Every verdict a verification prints leaves
// through here, and the path goes out escaped the way a checksum line carries
// it: printed raw, a newline in a path forges a second verdict line for
// whatever reads stdout.
func verdict(out io.Writer, path, result string) {
	escaped, marked := checksums.EscapePath(path)
	prefix := ""
	if marked {
		prefix = `\`
	}
	fmt.Fprintf(out, "%s%s: %s\n", prefix, escaped, result)
}

func parseExpected(opts VerifyOptions) (hash.Digest, error) {
	if opts.Algorithm == "" {
		return hash.ParseDigest(opts.Expected)
	}
	return hash.ParseDigestAs(opts.Expected, hash.Algorithm(opts.Algorithm))
}
