package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSumCommandFormats(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want func(path string) string
	}{
		{
			name: "text is the default",
			args: nil,
			want: func(p string) string { return abcSHA256 + "  " + p + "\n" },
		},
		{
			name: "binary",
			args: []string{"--binary"},
			want: func(p string) string { return abcSHA256 + " *" + p + "\n" },
		},
		{
			name: "tag",
			args: []string{"--tag"},
			want: func(p string) string { return "SHA256 (" + p + ") = " + abcSHA256 + "\n" },
		},
		{
			name: "bare",
			args: []string{"--bare"},
			want: func(string) string { return abcSHA256 + "\n" },
		},
		{
			name: "text stated explicitly",
			args: []string{"--text"},
			want: func(p string) string { return abcSHA256 + "  " + p + "\n" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "abc")
			args := append([]string{"sum"}, tt.args...)

			stdout, stderr, err := runCLI(t, append(args, path)...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if want := tt.want(path); stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
			if got := exitCode(err); got != 0 {
				t.Errorf("exitCode() = %d, want 0", got)
			}
		})
	}
}

func TestSumCommandFormatFlagsAreExclusive(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "text and binary", args: []string{"--text", "--binary"}},
		{name: "tag and bare", args: []string{"--tag", "--bare"}},
		{name: "binary and tag", args: []string{"--binary", "--tag"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "abc")
			args := append([]string{"sum"}, tt.args...)

			_, _, err := runCLI(t, append(args, path)...)
			if err == nil {
				t.Fatal("Execute() error = nil, want an error for two format flags")
			}
			if got := exitCode(err); got != 2 {
				t.Errorf("exitCode() = %d, want 2", got)
			}
		})
	}
}

func TestSumCommandExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		args     func(root string) []string
		wantCode int
	}{
		{
			name:     "no arguments",
			args:     func(string) []string { return []string{"sum"} },
			wantCode: 2,
		},
		{
			name: "a missing file",
			args: func(root string) []string {
				return []string{"sum", filepath.Join(root, "gone.txt")}
			},
			wantCode: 2,
		},
		{
			name:     "a directory without -r",
			args:     func(root string) []string { return []string{"sum", root} },
			wantCode: 2,
		},
		{
			name: "an unknown algorithm",
			args: func(root string) []string {
				return []string{"sum", "--algo", "md5", filepath.Join(root, "a.txt")}
			},
			wantCode: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("abc"), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			_, _, err := runCLI(t, tt.args(root)...)
			if err == nil {
				t.Fatal("Execute() error = nil, want an error")
			}
			if got := exitCode(err); got != tt.wantCode {
				t.Errorf("exitCode() = %d, want %d", got, tt.wantCode)
			}
		})
	}
}

func TestSumCommandRecursive(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{name: "long flag", flag: "--recursive"},
		{name: "short flag", flag: "-r"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("abc"), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			stdout, _, err := runCLI(t, "sum", tt.flag, root)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n"; stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestSumCommandAlgorithm(t *testing.T) {
	// sha512 of "abc", from FIPS 180-4.
	const abcSHA512 = "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a" +
		"2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "sha256 by default", args: nil, want: abcSHA256},
		{name: "sha512 named", args: []string{"--algo", "sha512"}, want: abcSHA512},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "abc")
			args := append([]string{"sum", "--bare"}, tt.args...)

			stdout, _, err := runCLI(t, append(args, path)...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if want := tt.want + "\n"; stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
		})
	}
}

func TestSumCommandTagUsesTheAlgorithmName(t *testing.T) {
	tests := []struct {
		name string
		algo string
		want string
	}{
		{name: "sha512 tag", algo: "sha512", want: "SHA512 ("},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "abc")

			stdout, _, err := runCLI(t, "sum", "--tag", "--algo", tt.algo, path)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !strings.HasPrefix(stdout, tt.want) {
				t.Errorf("stdout = %q, want it to start with %q", stdout, tt.want)
			}
		})
	}
}
