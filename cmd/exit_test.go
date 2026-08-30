package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestExecute(t *testing.T) {
	// sha256 of "abc", from FIPS 180-4.
	const abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

	tests := []struct {
		name     string
		args     func(path string) []string // built from the fixture path
		setup    func(*testing.T) string    // returns a temp file path, or empty if not needed
		wantInt  int
		checkErr func(*testing.T, string) // function to check stderr
	}{
		{
			name:    "digests match",
			wantInt: 0,
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "payload.txt")
				if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return path
			},
			args: func(path string) []string {
				return []string{"verify", path, abcSHA256}
			},
			checkErr: func(t *testing.T, stderr string) {
				if stderr != "" {
					t.Errorf("stderr = %q, want empty", stderr)
				}
			},
		},
		{
			name:    "mismatch",
			wantInt: 1,
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "payload.txt")
				if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return path
			},
			args: func(path string) []string {
				return []string{"verify", path, strings.Repeat("0", 64)}
			},
			checkErr: func(t *testing.T, stderr string) {
				// Should have expected/actual lines but NO ldsum: line
				if !strings.Contains(stderr, "expected:") {
					t.Errorf("mismatch stderr should contain expected line, got %q", stderr)
				}
				if !strings.Contains(stderr, "actual:") {
					t.Errorf("mismatch stderr should contain actual line, got %q", stderr)
				}
				if strings.Contains(stderr, "ldsum:") {
					t.Errorf("mismatch stderr should not contain 'ldsum:' line, got %q", stderr)
				}
			},
		},
		{
			name:    "missing target file",
			wantInt: 1,
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nope.txt")
			},
			args: func(path string) []string {
				return []string{"verify", path, abcSHA256}
			},
			checkErr: func(t *testing.T, stderr string) {
				if !strings.Contains(stderr, "ldsum:") {
					t.Errorf("stderr = %q, want to contain 'ldsum:'", stderr)
				}
			},
		},
		{
			name:    "bad checksum",
			wantInt: 2,
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "payload.txt")
				if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return path
			},
			args: func(path string) []string {
				return []string{"verify", path, "zz"}
			},
			checkErr: func(t *testing.T, stderr string) {
				if !strings.Contains(stderr, "ldsum:") {
					t.Errorf("stderr = %q, want to contain 'ldsum:'", stderr)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			root := newRootCmd()
			root.SetOut(&out)
			root.SetErr(&errOut)

			filePath := tt.setup(t)
			root.SetArgs(tt.args(filePath))

			got := execute(root)

			if got != tt.wantInt {
				t.Errorf("Execute() = %d, want %d", got, tt.wantInt)
			}

			stderr := errOut.String()
			tt.checkErr(t, stderr)
		})
	}
}
