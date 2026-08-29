// Package run orchestrates the work behind each command. It returns errors
// and never exits.
package run

import (
	"fmt"
	"io"
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
	return fmt.Sprintf("%s: checksum mismatch", e.Path)
}

// Verify streams the file at opts.Path through the hasher and reports whether
// its digest matches opts.Expected.
func Verify(out, errOut io.Writer, opts VerifyOptions) error {
	expected, err := parseExpected(opts)
	if err != nil {
		return fmt.Errorf("verify %s: %w", opts.Path, err)
	}

	f, err := os.Open(opts.Path)
	if err != nil {
		return fmt.Errorf("open %s: %w", opts.Path, err)
	}
	defer f.Close()

	actual, err := hash.Sum(f, expected.Algorithm)
	if err != nil {
		return fmt.Errorf("read %s: %w", opts.Path, err)
	}

	if !actual.Equal(expected) {
		fmt.Fprintf(out, "%s: FAILED\n", opts.Path)
		fmt.Fprintf(errOut, "expected: %s\n", expected.Hex)
		fmt.Fprintf(errOut, "actual:   %s\n", actual.Hex)
		return &MismatchError{Path: opts.Path, Expected: expected, Actual: actual}
	}

	fmt.Fprintf(out, "%s: OK\n", opts.Path)
	return nil
}

func parseExpected(opts VerifyOptions) (hash.Digest, error) {
	if opts.Algorithm == "" {
		return hash.ParseDigest(opts.Expected)
	}
	return hash.ParseDigestAs(opts.Expected, hash.Algorithm(opts.Algorithm))
}
