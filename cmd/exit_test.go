package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/BobMali/ldsum/internal/run"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "success",
			err:  nil,
			want: 0,
		},
		{
			name: "checksum mismatch",
			err:  &run.MismatchError{Path: "dist.tar.gz"},
			want: 1,
		},
		{
			name: "mismatch wrapped further up",
			err:  fmt.Errorf("verify: %w", &run.MismatchError{Path: "dist.tar.gz"}),
			want: 1,
		},
		{
			name: "missing target file",
			err:  fmt.Errorf("open dist.tar.gz: %w", fs.ErrNotExist),
			want: 1,
		},
		{
			name: "bad checksum on the command line",
			err:  errors.New(`not a hex checksum: "zz"`),
			want: 2,
		},
		{
			name: "unreadable file",
			err:  fmt.Errorf("read dist.tar.gz: %w", fs.ErrPermission),
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
