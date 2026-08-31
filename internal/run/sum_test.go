package run

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BobMali/ldsum/internal/checksums"
)

// sumTree writes each name/contents pair under a fresh temporary directory and
// returns its root. Names may contain slashes; parents are created.
func sumTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

func TestSumNamedFiles(t *testing.T) {
	tests := []struct {
		name   string
		format checksums.Format
		want   func(path string) string
	}{
		{
			name:   "text by default",
			format: checksums.Text,
			want:   func(p string) string { return abcSHA256 + "  " + p + "\n" },
		},
		{
			name:   "bare drops the path",
			format: checksums.Bare,
			want:   func(string) string { return abcSHA256 + "\n" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			path := filepath.Join(root, "a.txt")
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{path},
				Algorithm: "sha256",
				Format:    tt.format,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if want := tt.want(path); out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}

func TestSumArgumentOrder(t *testing.T) {
	tests := []struct {
		name  string
		order []string
	}{
		{name: "as given", order: []string{"b.txt", "a.txt"}},
		{name: "reversed", order: []string{"a.txt", "b.txt"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc", "b.txt": "abc"})
			var paths []string
			var want strings.Builder
			for _, name := range tt.order {
				p := filepath.Join(root, name)
				paths = append(paths, p)
				want.WriteString(abcSHA256 + "  " + p + "\n")
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     paths,
				Algorithm: "sha256",
				Format:    checksums.Text,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if out.String() != want.String() {
				t.Errorf("stdout = %q, want %q", out.String(), want.String())
			}
		})
	}
}

func TestSumKeepsGoingAfterAFailure(t *testing.T) {
	tests := []struct {
		name         string
		missing      string
		wantInErrOut string
	}{
		{name: "a missing file", missing: "gone.txt", wantInErrOut: "gone.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			good := filepath.Join(root, "a.txt")
			bad := filepath.Join(root, tt.missing)
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{bad, good},
				Algorithm: "sha256",
				Format:    checksums.Text,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error after a failed file")
			}
			if want := abcSHA256 + "  " + good + "\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q — the good file must still be summed",
					out.String(), want)
			}
			if !strings.Contains(errOut.String(), tt.wantInErrOut) {
				t.Errorf("stderr = %q, want it to name %q", errOut.String(), tt.wantInErrOut)
			}
		})
	}
}

func TestSumRejectsADirectoryWithoutRecursion(t *testing.T) {
	tests := []struct {
		name string
		sub  string
	}{
		{name: "a subdirectory", sub: "sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{tt.sub + "/a.txt": "abc"})
			dir := filepath.Join(root, tt.sub)
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{dir},
				Algorithm: "sha256",
				Format:    checksums.Text,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error for a directory without -r")
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", out.String())
			}
			if !strings.Contains(errOut.String(), "-r") {
				t.Errorf("stderr = %q, want it to name the -r flag", errOut.String())
			}
		})
	}
}

func TestSumRejectsAnUnknownAlgorithm(t *testing.T) {
	tests := []struct {
		name string
		algo string
	}{
		{name: "md5", algo: "md5"},
		{name: "empty", algo: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{filepath.Join(root, "a.txt")},
				Algorithm: tt.algo,
				Format:    checksums.Text,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error for an unknown algorithm")
			}
			// The check is up front, so nothing is hashed and nothing is
			// reported per file.
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", out.String())
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want nothing", errOut.String())
			}
		})
	}
}

func TestSumWalksInLexicalOrder(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		order []string
	}{
		{
			name: "files before subdirectories, each sorted",
			files: map[string]string{
				"b.txt":     "abc",
				"a.txt":     "abc",
				"sub/c.txt": "abc",
			},
			order: []string{"a.txt", "b.txt", "sub/c.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, tt.files)
			var want strings.Builder
			for _, name := range tt.order {
				want.WriteString(abcSHA256 + "  " + filepath.Join(root, name) + "\n")
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if out.String() != want.String() {
				t.Errorf("stdout = %q, want %q", out.String(), want.String())
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}

func TestSumWalkSkipsSymlinksSilently(t *testing.T) {
	tests := []struct {
		name string
		link string
	}{
		{name: "a link beside the file it points at", link: "link.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			if err := os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, tt.link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q — the link must not be followed",
					out.String(), want)
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty without --verbose", errOut.String())
			}
		})
	}
}

func TestSumWalksAnEmptyDirectory(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "nothing to sum is not a failure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", out.String())
			}
		})
	}
}

func TestSumWalkKeepsGoingAfterAnUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a file with mode 000")
	}
	tests := []struct {
		name       string
		unreadable string
	}{
		{name: "mode 000 mid-walk", unreadable: "b-secret.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{
				"a.txt":       "abc",
				tt.unreadable: "abc",
				"c.txt":       "abc",
			})
			if err := os.Chmod(filepath.Join(root, tt.unreadable), 0o000); err != nil {
				t.Fatalf("chmod: %v", err)
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error after an unreadable file")
			}
			want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n" +
				abcSHA256 + "  " + filepath.Join(root, "c.txt") + "\n"
			if out.String() != want {
				t.Errorf("stdout = %q, want %q — the walk must continue past the failure",
					out.String(), want)
			}
			if !strings.Contains(errOut.String(), tt.unreadable) {
				t.Errorf("stderr = %q, want it to name %q", errOut.String(), tt.unreadable)
			}
		})
	}
}

func TestSumVerboseNamesSkippedEntries(t *testing.T) {
	tests := []struct {
		name string
		link string
	}{
		{name: "a symlink is named", link: "link.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			if err := os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, tt.link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
				Verbose:   true,
			})
			// Skipping is not a failure, however loudly it is reported.
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if !strings.Contains(errOut.String(), tt.link) {
				t.Errorf("stderr = %q, want it to name the skipped %q", errOut.String(), tt.link)
			}
		})
	}
}
