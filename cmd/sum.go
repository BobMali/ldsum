package cmd

import (
	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/run"
	"github.com/spf13/cobra"
)

// newSumCmd builds the sum subcommand. Every flag binds to a local, so two
// trees in the same process never share them.
func newSumCmd() *cobra.Command {
	var (
		algorithm string
		text      bool
		binary    bool
		tagged    bool
		bare      bool
		recursive bool
		verbose   bool
	)

	cmd := &cobra.Command{
		Use:   "sum <path>...",
		Short: "Print the checksum of a file or a tree",
		Long: `Sum prints the checksum of each file it is given.

The default output is the GNU coreutils text format — the digest, two spaces,
then the path — which is what a SHA256SUMS file contains. --binary marks the
path with an asterisk instead, --tag switches to the BSD tagged format, and
--bare prints the digest alone so it can be captured straight into a variable.

Directory arguments are walked only with -r. A symlink named as an argument is
followed; symlinks found by walking are not. Anything skipped is named on
stderr under -v.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Arguments have parsed by this point, so a later failure is not a
			// usage problem and usage text would only bury the output.
			cmd.SilenceUsage = true

			format := checksums.Text
			switch {
			case binary:
				format = checksums.Binary
			case tagged:
				format = checksums.Tag
			case bare:
				format = checksums.Bare
			}

			return run.Sum(cmd.OutOrStdout(), cmd.ErrOrStderr(), run.SumOptions{
				Paths:     args,
				Algorithm: algorithm,
				Format:    format,
				Recursive: recursive,
				Verbose:   verbose,
			})
		},
	}

	cmd.Flags().StringVar(&algorithm, "algo", "sha256", "checksum algorithm: sha256 or sha512")
	cmd.Flags().BoolVarP(&text, "text", "t", false, "GNU text format: <digest>  <path> (the default)")
	cmd.Flags().BoolVarP(&binary, "binary", "b", false, "GNU binary format: <digest> *<path>")
	cmd.Flags().BoolVar(&tagged, "tag", false, "BSD tagged format: SHA256 (<path>) = <digest>")
	cmd.Flags().BoolVar(&bare, "bare", false, "the digest alone, with no path")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "walk directory arguments")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "name skipped entries on stderr")
	cmd.MarkFlagsMutuallyExclusive("text", "binary", "tag", "bare")

	return cmd
}
