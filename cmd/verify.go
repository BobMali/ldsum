package cmd

import (
	"errors"

	"github.com/BobMali/ldsum/internal/run"
	"github.com/spf13/cobra"
)

// newVerifyCmd builds the verify subcommand. Every flag binds to a local, so
// two trees in the same process never share them.
func newVerifyCmd() *cobra.Command {
	var (
		algorithm string
		sumsFile  string
	)

	cmd := &cobra.Command{
		Use:   "verify [<file> <checksum>]",
		Short: "Verify a file against an expected checksum",
		Long: `Verify checks files against the checksums they are expected to have.

Given a file and a checksum, it checks that one file. The algorithm is taken
from the length of the checksum — 64 hex characters is sha256, 128 is sha512 —
unless --algo names one.

Given --sums-file, it reads the expected checksums from a checksum file
instead, recognising the GNU text and binary formats, the BSD tagged format,
and a file holding a bare digest. Entries are resolved relative to the
checksum file, so the command works from any directory. Naming files after
the flag checks only those entries; naming none checks them all.

It exits 0 when every digest matched, 1 when one did not or a file is
missing, and 2 when the command itself was wrong.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("sums-file") {
				// An empty -c is the flag itself being wrong. Falling through
				// to inline mode would complain about the argument count and
				// never mention the flag.
				if sumsFile == "" {
					return errors.New("--sums-file needs a file name")
				}
				// With a checksum file the arguments pick entries out of it,
				// so any number is meaningful, including none.
				return nil
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Arguments have parsed by this point, so a later failure is not a usage
			// problem and usage text would only bury the verdict.
			cmd.SilenceUsage = true
			if sumsFile != "" {
				return run.VerifySums(cmd.OutOrStdout(), cmd.ErrOrStderr(), run.SumsOptions{
					SumsFile: sumsFile,
					Paths:    args,
				})
			}
			return run.Verify(cmd.OutOrStdout(), cmd.ErrOrStderr(), run.VerifyOptions{
				Path:      args[0],
				Expected:  args[1],
				Algorithm: algorithm,
			})
		},
	}

	cmd.Flags().StringVar(&algorithm, "algo", "",
		"checksum algorithm: sha256 or sha512 (inferred from the checksum length when omitted)")
	cmd.Flags().StringVarP(&sumsFile, "sums-file", "c", "",
		"read the expected checksums from this file")
	cmd.MarkFlagsMutuallyExclusive("algo", "sums-file")

	return cmd
}
