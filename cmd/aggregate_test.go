package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/BobMali/ldsum/internal/run"
	"github.com/spf13/cobra"
)

func TestExitCodeAggregates(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "mismatches alone",
			err: &run.VerifyErrors{Checked: 3, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				&run.MismatchError{Path: "b.txt"},
			}},
			want: 1,
		},
		{
			name: "a missing target among mismatches",
			err: &run.VerifyErrors{Checked: 2, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				&run.MissingTargetError{Path: "b.txt", Err: fs.ErrNotExist},
			}},
			want: 1,
		},
		{
			name: "one unreadable file outweighs any number of mismatches",
			err: &run.VerifyErrors{Checked: 2, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				fmt.Errorf("read b.txt: %w", fs.ErrPermission),
			}},
			want: 2,
		},
		{
			name: "an aggregate holding nothing is still a failure",
			err:  &run.VerifyErrors{},
			want: 2,
		},
		{
			name: "wrapped in context",
			err: fmt.Errorf("verify: %w", &run.VerifyErrors{Checked: 2, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				errors.New("read failed"),
			}}),
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCode(tt.err); got != tt.want {
				t.Errorf("exitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// errors.As walks Unwrap() []error, so an aggregate holding a mismatch also
// matches the arm whose whole job is to stay silent. Without the ordering in
// execute, the summary vanishes in the commonest failure there is.
func TestExecutePrintsTheAggregateSummary(t *testing.T) {
	root := &cobra.Command{Use: "ldsum", SilenceErrors: true}
	root.AddCommand(&cobra.Command{
		Use:          "stub",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return &run.VerifyErrors{Checked: 3, Errs: []error{
				&run.MismatchError{Path: "a.txt"},
				&run.MismatchError{Path: "b.txt"},
			}}
		},
	})
	var errOut bytes.Buffer
	root.SetOut(io.Discard)
	root.SetErr(&errOut)
	root.SetArgs([]string{"stub"})

	if code := execute(root); code != 1 {
		t.Errorf("execute() = %d, want 1", code)
	}
	if want := "ldsum: 2 of 3 files failed\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}
