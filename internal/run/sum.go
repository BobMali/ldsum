package run

import (
	"fmt"
	"io"
	"os"

	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/hash"
)

// SumOptions is one request to print checksums.
type SumOptions struct {
	Paths     []string
	Algorithm string
	Format    checksums.Format
}

// Sum prints the digest of each path in opts.Paths. A file that cannot be
// summed is reported on errOut and the rest still run; the returned error then
// reports how many failed.
func Sum(out, errOut io.Writer, opts SumOptions) error {
	algo := hash.Algorithm(opts.Algorithm)
	// Checked once up front: a bad algorithm is a wrong command, not a
	// per-file problem to repeat across a whole tree.
	if !hash.Supported(algo) {
		return fmt.Errorf("unknown algorithm %q: want sha256 or sha512", opts.Algorithm)
	}

	var attempted, failures int
	for _, path := range opts.Paths {
		a, f := sumPath(out, errOut, path, algo, opts)
		attempted += a
		failures += f
	}
	if failures > 0 {
		return fmt.Errorf("could not sum %d of %d files", failures, attempted)
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
		fmt.Fprintf(errOut, "%s: is a directory (use -r to recurse)\n", path)
		return 1, 1
	}
	return sumFile(out, errOut, path, algo, opts.Format)
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
		fmt.Fprintf(errOut, "%s: %v\n", path, err)
		return 1, 1
	}

	if err := checksums.Render(out, checksums.Entry{Digest: digest, Path: path}, format); err != nil {
		fmt.Fprintln(errOut, err)
		return 1, 1
	}
	return 1, 0
}
