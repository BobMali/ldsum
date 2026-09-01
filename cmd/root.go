/*
Copyright © 2026 Malek Olabi

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

// Package cmd wires the command line: it parses flags and arguments and
// hands the work to internal/run.
package cmd

import (
	"errors"
	"fmt"

	"github.com/BobMali/ldsum/internal/run"
	"github.com/spf13/cobra"
)

// newRootCmd builds a fresh command tree. Every invocation gets its own, so
// what one run mutates — a silenced usage flag, a bound flag value — cannot
// leak into the next.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "ldsum",
		Short: "Compute and verify file checksums",
		Long: `ldsum computes checksums and verifies files against them.

sum prints the digest of a file or a whole tree, in the GNU coreutils text and
binary formats, the BSD tagged format, or on its own.

verify checks a file against an expected checksum given on the command line.
Reading the file from a URL, and reading the expected checksum from a checksum
file such as a published SHA256SUMS, are planned and not yet implemented.

Both exit non-zero when the answer is no, so they drop straight into a script.`,
		// execute prints errors itself, so Cobra must not print a second copy.
		SilenceErrors: true,
	}
	root.AddCommand(newVerifyCmd())
	root.AddCommand(newSumCmd())
	return root
}

// Execute runs the command tree and returns the process exit code.
func Execute() int {
	return execute(newRootCmd())
}

// execute runs an already-built tree and maps its error to an exit code. It
// reports the error itself, except for a single mismatch, whose detail run
// has already printed. Tests build their own tree so they can capture the
// streams.
func execute(root *cobra.Command) int {
	err := root.Execute()
	if err != nil {
		var (
			mismatch *run.MismatchError
			multi    *run.VerifyErrors
		)
		// An aggregate is checked too, because errors.As walks Unwrap()
		// []error: one holding a mismatch would otherwise take the silent
		// path and lose the only line that says how many files failed.
		silent := errors.As(err, &mismatch) && !errors.As(err, &multi)
		if !silent {
			fmt.Fprintf(root.ErrOrStderr(), "ldsum: %v\n", err)
		}
	}
	return exitCode(err)
}
