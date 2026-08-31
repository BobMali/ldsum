// Package run orchestrates the work behind each command. It returns errors
// and never exits.
package run

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

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

// verifyEntry hashes one file and reports whether it matches. Every verdict a
// verification prints leaves through here, so a later change to how a verdict
// looks has one place to go.
func verifyEntry(out, errOut io.Writer, path string, expected hash.Digest) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &MissingTargetError{Path: path, Err: err}
		}
		return err
	}
	defer f.Close()

	actual, err := hash.Sum(f, expected.Algorithm)
	if err != nil {
		return err
	}

	if !actual.Equal(expected) {
		fmt.Fprintf(out, "%s: FAILED\n", path)
		fmt.Fprintf(errOut, "expected: %s\n", expected.Hex)
		fmt.Fprintf(errOut, "actual:   %s\n", actual.Hex)
		return &MismatchError{Path: path, Expected: expected, Actual: actual}
	}

	fmt.Fprintf(out, "%s: OK\n", path)
	return nil
}

func parseExpected(opts VerifyOptions) (hash.Digest, error) {
	if opts.Algorithm == "" {
		return hash.ParseDigest(opts.Expected)
	}
	return hash.ParseDigestAs(opts.Expected, hash.Algorithm(opts.Algorithm))
}
