package run

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/hash"
)

// SumOptions is one request to print checksums.
type SumOptions struct {
	Paths     []string
	Algorithm string
	Format    checksums.Format
	Output    string
	Recursive bool
	Verbose   bool
}

// Sum prints the digest of each path in opts.Paths, to out or to the file named
// by opts.Output. A file that cannot be summed is reported on errOut and the
// rest still run; the returned error then reports how many failed.
func Sum(out, errOut io.Writer, opts SumOptions) error {
	algo := hash.Algorithm(opts.Algorithm)
	// Checked once up front: a bad algorithm is a wrong command, not a
	// per-file problem to repeat across a whole tree.
	if !hash.Supported(algo) {
		return fmt.Errorf("unknown algorithm %q: want sha256 or sha512", opts.Algorithm)
	}
	if opts.Output == "" {
		return sumAll(out, errOut, algo, opts)
	}

	// Truncating, like the shell redirection this replaces, and opened before
	// any hashing so an unwritable destination costs no work.
	f, err := os.Create(opts.Output)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	sumErr := sumAll(w, errOut, algo, opts)
	// A lost flush or close is silently truncated output, which for a command
	// asked to write a file outweighs whatever the per-file count says.
	flushErr := w.Flush()
	closeErr := f.Close()
	switch {
	case flushErr != nil:
		return flushErr
	case closeErr != nil:
		return closeErr
	}
	return sumErr
}

// sumAll sums every path in opts and reports how many could not be done.
func sumAll(out, errOut io.Writer, algo hash.Algorithm, opts SumOptions) error {
	var attempted, failures int
	for _, path := range opts.Paths {
		a, f := sumPath(out, errOut, path, algo, opts)
		attempted += a
		failures += f
	}
	if failures > 0 {
		return fmt.Errorf("could not sum %d of %d paths", failures, attempted)
	}
	return nil
}

// sumPath handles one argument and returns how many files it tried and how
// many of those failed.
func sumPath(out, errOut io.Writer, path string, algo hash.Algorithm, opts SumOptions) (int, int) {
	// Stat rather than Lstat: a symlink named as an argument was named on
	// purpose, so it is followed. Entries found by walking are not.
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1, 1
	}
	if info.IsDir() {
		if !opts.Recursive {
			fmt.Fprintf(errOut, "%s: is a directory (use -r to recurse)\n", path)
			return 1, 1
		}
		return walkDir(out, errOut, path, algo, opts)
	}
	return sumFile(out, errOut, path, algo, opts.Format)
}

// walkDir sums every regular file under root. WalkDir reads each directory in
// lexical order, so the output order needs no sorting of its own.
func walkDir(out, errOut io.Writer, root string, algo hash.Algorithm, opts SumOptions) (int, int) {
	var attempted, failures int

	// WalkDir lstats its root, so a symlinked directory named as an argument
	// would be skipped as non-regular. A trailing separator resolves that final
	// link; entries found inside the tree are still never followed.
	err := filepath.WalkDir(root+string(filepath.Separator), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintln(errOut, err)
			attempted++
			failures++
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// Type() does not resolve links, so a symlink found by walking is
		// skipped rather than followed. Devices and sockets go the same way.
		if !d.Type().IsRegular() {
			if opts.Verbose {
				fmt.Fprintf(errOut, "%s: skipped, not a regular file\n", path)
			}
			return nil
		}
		a, f := sumFile(out, errOut, path, algo, opts.Format)
		attempted += a
		failures += f
		return nil
	})
	// WalkDir routes even a root failure through the callback, which returns nil
	// for everything, so this is unreachable — kept so a future error is not lost.
	if err != nil {
		fmt.Fprintln(errOut, err)
		attempted++
		failures++
	}
	return attempted, failures
}

// sumFile streams one file through the hasher and renders its line.
func sumFile(out, errOut io.Writer, path string, algo hash.Algorithm, format checksums.Format) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1, 1
	}
	defer f.Close()

	digest, err := hash.Sum(f, algo)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1, 1
	}

	if err := checksums.Render(out, checksums.Entry{Digest: digest, Path: path}, format); err != nil {
		fmt.Fprintln(errOut, err)
		return 1, 1
	}
	return 1, 0
}
