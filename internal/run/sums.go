package run

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/hash"
)

// SumsOptions is one request to verify against a checksum file. An empty
// Paths means every entry the file lists.
type SumsOptions struct {
	SumsFile string
	Paths    []string
}

// VerifyErrors reports every file that failed in one run. Errs is never empty.
type VerifyErrors struct {
	Checked int
	Errs    []error
}

func (e *VerifyErrors) Error() string {
	return fmt.Sprintf("%d of %d files failed", len(e.Errs), e.Checked)
}

func (e *VerifyErrors) Unwrap() []error { return e.Errs }

// target is one file to verify and the digest it has to have.
type target struct {
	path   string
	digest hash.Digest
}

// VerifySums verifies the files a checksum file lists. A mismatch does not
// stop the run: every file is reported, and the returned error says how many
// failed.
func VerifySums(out, errOut io.Writer, opts SumsOptions) error {
	f, err := os.Open(opts.SumsFile)
	if err != nil {
		return err
	}
	defer f.Close()

	listing, err := checksums.Parse(f)
	if err != nil {
		return err
	}

	targets, err := selectTargets(errOut, listing, opts)
	if err != nil {
		return err
	}

	var errs []error
	for _, t := range targets {
		if err := verifyEntry(out, errOut, t.path, t.digest); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	// One file, one verdict: nothing to summarise, and the error stays the
	// same shape a `verify <file> <checksum>` run would have returned.
	if len(targets) == 1 {
		return errs[0]
	}
	return &VerifyErrors{Checked: len(targets), Errs: errs}
}

// selectTargets works out which files the listing asks for. Entries are
// relative to the file that lists them, so the command works from anywhere.
func selectTargets(_ io.Writer, listing checksums.Listing, opts SumsOptions) ([]target, error) {
	base := filepath.Dir(opts.SumsFile)

	if len(opts.Paths) == 0 {
		targets := make([]target, 0, len(listing.Entries))
		for _, e := range listing.Entries {
			targets = append(targets, target{
				path:   filepath.Join(base, e.Path),
				digest: e.Digest,
			})
		}
		return targets, nil
	}

	// Arguments name entries as the file spells them, so the lookup is on the
	// listed path, not on anything resolved against the working directory.
	byPath := make(map[string]checksums.Entry, len(listing.Entries))
	for _, e := range listing.Entries {
		byPath[filepath.Clean(e.Path)] = e
	}

	targets := make([]target, 0, len(opts.Paths))
	for _, p := range opts.Paths {
		e, ok := byPath[filepath.Clean(p)]
		if !ok {
			return nil, fmt.Errorf("%s: no entry for %s", opts.SumsFile, p)
		}
		targets = append(targets, target{
			path:   filepath.Join(base, e.Path),
			digest: e.Digest,
		})
	}
	return targets, nil
}
