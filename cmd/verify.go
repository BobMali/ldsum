package cmd

import (
	"github.com/BobMali/ldsum/internal/run"
	"github.com/spf13/cobra"
)

var verifyAlgorithm string

var verifyCmd = &cobra.Command{
	Use:   "verify <file> <checksum>",
	Short: "Verify a file against an expected checksum",
	Long: `Verify checks a file against a checksum given on the command line.

The algorithm is taken from the length of the checksum — 64 hex characters is
sha256, 128 is sha512 — unless --algo names one. It exits 0 when the digest
matches, 1 when it does not, and 2 when the command itself was wrong.`,
	Args: cobra.ExactArgs(2),
	// run already reports the verdict, so Cobra must not print it again.
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return run.Verify(cmd.OutOrStdout(), cmd.ErrOrStderr(), run.VerifyOptions{
			Path:      args[0],
			Expected:  args[1],
			Algorithm: verifyAlgorithm,
		})
	},
}

func init() {
	verifyCmd.Flags().StringVar(&verifyAlgorithm, "algo", "",
		"checksum algorithm: sha256 or sha512 (inferred from the checksum length when omitted)")
	rootCmd.AddCommand(verifyCmd)
}
