package run

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/hash"
	"github.com/BobMali/ldsum/internal/source"
)

// SumsOptions is one request to verify against a checksum file. An empty
// Paths means every entry the file lists.
type SumsOptions struct {
	SumsFile      string
	Paths         []string
	RemoteTargets bool
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
	// Checked before the file is opened: a guard further down would report a
	// missing local file and never mention the flag that was wrong.
	if opts.RemoteTargets {
		remote, err := source.IsRemote(opts.SumsFile)
		if err != nil {
			return err
		}
		if !remote {
			return errors.New("--remote-targets needs a URL for --sums-file")
		}
	}

	f, err := source.Open(opts.SumsFile)
	if err != nil {
		return err
	}
	defer f.Close()

	listing, err := checksums.Parse(f)
	if err != nil {
		// The scanner knows nothing about the file it was handed, so its own
		// errors arrive bare. A failed read is already an *fs.PathError and
		// must not gain a second copy of the operation and path.
		var pathErr *fs.PathError
		if !errors.As(err, &pathErr) {
			err = fmt.Errorf("read %s: %w", opts.SumsFile, err)
		}
		return err
	}

	targets, warnings, err := selectTargets(listing, opts)
	// The two passes find their complaints in different orders, but they are
	// complaints about one file, so they come out in that file's order.
	for _, b := range listing.Bad {
		warnings = append(warnings, warning{line: b.Line, msg: b.Err.Error()})
	}
	slices.SortStableFunc(warnings, func(a, b warning) int {
		return cmp.Compare(a.line, b.line)
	})
	for _, w := range warnings {
		warnLine(errOut, opts.SumsFile, w.line, w.msg)
	}
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

// warning is one unusable line of a checksum file, held until every pass has
// had its say so the whole set can be reported in line order.
type warning struct {
	line int
	msg  string
}

// warnLine reports a line of a checksum file that could not be used. Every
// such warning leaves through here.
func warnLine(errOut io.Writer, file string, line int, msg string) {
	fmt.Fprintf(errOut, "%s:%d: %s\n", file, line, msg)
}

// resolve places a listed entry against the base the listing came from.
// remote says whether that base is a URL.
//
// An entry that is itself a URL names its own host and is returned untouched:
// filepath.Join would turn it into "https:/host/f". A local absolute entry
// keeps the same short-circuit, but only while the base is local — under a URL
// base a leading "/" is a URL path and belongs to that host.
func resolve(base, p string, remote bool) (string, error) {
	entryRemote, err := source.IsRemote(p)
	if err != nil {
		return "", err
	}
	if entryRemote {
		return p, nil
	}
	if remote {
		return source.JoinURL(base, p)
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Join(base, p), nil
}

// selectTargets works out which files the listing asks for. The mode is a
// property of the whole listing, not of any one line: a single pathless entry
// is a bare-digest file, and a stray one among many is just a broken line.
func selectTargets(listing checksums.Listing, opts SumsOptions) ([]target, []warning, error) {
	if len(listing.Entries) == 1 && listing.Entries[0].Path == "" {
		// A bare-digest file names no entries, so there is nothing for the
		// flag to resolve. Silently ignoring it would hide a wrong command.
		if opts.RemoteTargets {
			return nil, nil, fmt.Errorf(
				"%s: holds one checksum and no entries for --remote-targets to resolve",
				opts.SumsFile)
		}
		if len(opts.Paths) == 0 {
			return nil, nil, fmt.Errorf(
				"%s: no paths in file; name the file to verify", opts.SumsFile)
		}
		if len(opts.Paths) > 1 {
			return nil, nil, fmt.Errorf(
				"%s: holds one checksum; name exactly one file", opts.SumsFile)
		}
		return []target{{path: opts.Paths[0], digest: listing.Entries[0].Digest}}, nil, nil
	}

	var warnings []warning
	named := make([]checksums.Entry, 0, len(listing.Entries))
	for _, e := range listing.Entries {
		if e.Path == "" {
			warnings = append(warnings,
				warning{line: e.Line, msg: "checksum without a path"})
			continue
		}
		named = append(named, e)
	}
	if len(named) == 0 {
		return nil, warnings, fmt.Errorf("%s: no checksum lines found", opts.SumsFile)
	}

	// Where a relative entry points: the checksum file's directory when it is
	// local, the working directory when it is a URL — a URL has no local
	// directory — and the URL itself when the caller asked for that.
	sumsRemote, err := source.IsRemote(opts.SumsFile)
	if err != nil {
		return nil, warnings, err
	}
	base := filepath.Dir(opts.SumsFile)
	switch {
	case sumsRemote && opts.RemoteTargets:
		base = opts.SumsFile
	case sumsRemote:
		base = "."
	}

	if len(opts.Paths) == 0 {
		targets := make([]target, 0, len(named))
		for _, e := range named {
			path, err := resolve(base, e.Path, sumsRemote && opts.RemoteTargets)
			if err != nil {
				return nil, warnings, err
			}
			targets = append(targets, target{path: path, digest: e.Digest})
		}
		return targets, warnings, nil
	}

	// Arguments name entries as the file spells them, so the lookup is on the
	// listed path, not on anything resolved against the working directory. A
	// path may be listed more than once, and naming it has to select every one
	// of those entries or the argument would change what gets checked.
	byPath := make(map[string][]checksums.Entry, len(named))
	for _, e := range named {
		key := filepath.Clean(e.Path)
		byPath[key] = append(byPath[key], e)
	}

	targets := make([]target, 0, len(opts.Paths))
	for _, p := range opts.Paths {
		entries, ok := byPath[filepath.Clean(p)]
		if !ok {
			return nil, warnings, fmt.Errorf("%s: no entry for %s", opts.SumsFile, p)
		}
		for _, e := range entries {
			path, err := resolve(base, e.Path, sumsRemote && opts.RemoteTargets)
			if err != nil {
				return nil, warnings, err
			}
			targets = append(targets, target{path: path, digest: e.Digest})
		}
	}
	return targets, warnings, nil
}
