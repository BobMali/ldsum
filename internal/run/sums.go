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

	for _, b := range listing.Bad {
		warnLine(errOut, opts.SumsFile, b.Line, b.Err.Error())
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

// warnLine reports a line of a checksum file that could not be used. Every
// such warning leaves through here.
func warnLine(errOut io.Writer, file string, line int, msg string) {
	fmt.Fprintf(errOut, "%s:%d: %s\n", file, line, msg)
}

// selectTargets works out which files the listing asks for. The mode is a
// property of the whole listing, not of any one line: a single pathless entry
// is a bare-digest file, and a stray one among many is just a broken line.
func selectTargets(errOut io.Writer, listing checksums.Listing, opts SumsOptions) ([]target, error) {
	if len(listing.Entries) == 1 && listing.Entries[0].Path == "" {
		if len(opts.Paths) == 0 {
			return nil, fmt.Errorf(
				"%s: no paths in file; name the file to verify", opts.SumsFile)
		}
		if len(opts.Paths) > 1 {
			return nil, fmt.Errorf(
				"%s: holds one checksum; name exactly one file", opts.SumsFile)
		}
		return []target{{path: opts.Paths[0], digest: listing.Entries[0].Digest}}, nil
	}

	named := make([]checksums.Entry, 0, len(listing.Entries))
	for _, e := range listing.Entries {
		if e.Path == "" {
			warnLine(errOut, opts.SumsFile, e.Line, "checksum without a path")
			continue
		}
		named = append(named, e)
	}
	if len(named) == 0 {
		return nil, fmt.Errorf("%s: no checksum lines found", opts.SumsFile)
	}

	base := filepath.Dir(opts.SumsFile)

	if len(opts.Paths) == 0 {
		targets := make([]target, 0, len(named))
		for _, e := range named {
			targets = append(targets, target{
				path:   filepath.Join(base, e.Path),
				digest: e.Digest,
			})
		}
		return targets, nil
	}

	// Arguments name entries as the file spells them, so the lookup is on the
	// listed path, not on anything resolved against the working directory.
	byPath := make(map[string]checksums.Entry, len(named))
	for _, e := range named {
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
