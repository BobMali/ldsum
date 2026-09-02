package run

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BobMali/ldsum/internal/checksums"
	"github.com/BobMali/ldsum/internal/hash"
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
			name: "a subdirectory sorting after the files comes last",
			files: map[string]string{
				"b.txt":     "abc",
				"a.txt":     "abc",
				"sub/c.txt": "abc",
			},
			order: []string{"a.txt", "b.txt", "sub/c.txt"},
		},
		{
			name: "a subdirectory sorting before a file comes first",
			files: map[string]string{
				"b.txt":    "abc",
				"aa/x.txt": "abc",
			},
			order: []string{"aa/x.txt", "b.txt"},
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

func TestSumWalksASymlinkedDirectoryArgument(t *testing.T) {
	tests := []struct {
		name string
		link string
	}{
		{name: "a link named as the argument is walked", link: "linkdir"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"real/a.txt": "abc"})
			link := filepath.Join(root, tt.link)
			if err := os.Symlink(filepath.Join(root, "real"), link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{link},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			// Named on purpose, so the tree is walked — and paths stay under the
			// name the caller gave rather than the link target's.
			if want := abcSHA256 + "  " + filepath.Join(link, "a.txt") + "\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty", errOut.String())
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

func TestSumKeepsGoingAfterATagRejection(t *testing.T) {
	tests := []struct {
		name string
		odd  string
	}{
		{name: "a path tagged format cannot carry", odd: "we\\ird.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{
				"a.txt": "abc",
				tt.odd:  "abc",
				"z.txt": "abc",
			})
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Tag,
				Recursive: true,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error after a rejected path")
			}
			// The other two files are still summed: a rejection is one more
			// per-file failure, not a reason to abandon the tree.
			want := "SHA256 (" + filepath.Join(root, "a.txt") + ") = " + abcSHA256 + "\n" +
				"SHA256 (" + filepath.Join(root, "z.txt") + ") = " + abcSHA256 + "\n"
			if out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if !strings.Contains(errOut.String(), "tagged format") {
				t.Errorf("stderr = %q, want it to explain the tagged-format limit", errOut.String())
			}
		})
	}
}

func TestSumWalksAnArgumentWithATrailingSeparator(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{name: "a plain directory", dir: "sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{tt.dir + "/c.txt": "abc"})
			// The caller's own trailing slash must not double with the one
			// walkDir appends; filepath.Join cleans it away.
			arg := filepath.Join(root, tt.dir) + string(filepath.Separator)
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{arg},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			want := abcSHA256 + "  " + filepath.Join(root, tt.dir, "c.txt") + "\n"
			if out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
		})
	}
}

func TestSumWritesToAnOutputFile(t *testing.T) {
	tests := []struct {
		name     string
		existing string
	}{
		{name: "a path that does not exist yet"},
		{name: "a file already there", existing: "stale contents\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			path := filepath.Join(root, "a.txt")
			outPath := filepath.Join(t.TempDir(), "SHA256SUMS")
			if tt.existing != "" {
				if err := os.WriteFile(outPath, []byte(tt.existing), 0o644); err != nil {
					t.Fatalf("write existing output file: %v", err)
				}
			}
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{path},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Output:    outPath,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			written, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("read output file: %v", err)
			}
			if want := abcSHA256 + "  " + path + "\n"; string(written) != want {
				t.Errorf("output file = %q, want %q", written, want)
			}
			if out.String() != "" {
				t.Errorf("stdout = %q, want empty", out.String())
			}
			if errOut.String() != "" {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}

func TestSumReportsAnUnusableOutputFile(t *testing.T) {
	tests := []struct {
		name   string
		output func(root string) string
	}{
		{
			name:   "a directory that does not exist",
			output: func(root string) string { return filepath.Join(root, "missing", "SHA256SUMS") },
		},
		{
			name:   "a directory in place of the file",
			output: func(root string) string { return root },
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
				Format:    checksums.Text,
				Output:    tt.output(root),
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error naming the output file")
			}
			// Nothing was summed, so neither stream saw a line.
			if out.String() != "" {
				t.Errorf("stdout = %q, want empty", out.String())
			}
			if errOut.String() != "" {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}

// The summary is the only place the attempted and failed counts are observable,
// so its exact text is what holds every counter in sum.go in place.
func TestSumSummaryCountsNamedPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths func(root string) []string
		want  string
	}{
		{
			name: "one missing beside one good file",
			paths: func(root string) []string {
				return []string{filepath.Join(root, "a.txt"), filepath.Join(root, "gone.txt")}
			},
			want: "could not sum 1 of 2 paths",
		},
		{
			name: "two missing beside one good file",
			paths: func(root string) []string {
				return []string{
					filepath.Join(root, "a.txt"),
					filepath.Join(root, "gone.txt"),
					filepath.Join(root, "also-gone.txt"),
				}
			},
			want: "could not sum 2 of 3 paths",
		},
		{
			name: "a directory without -r is one attempt and one failure",
			paths: func(root string) []string {
				return []string{root, filepath.Join(root, "a.txt")}
			},
			want: "could not sum 1 of 2 paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     tt.paths(root),
				Algorithm: "sha256",
				Format:    checksums.Text,
			})
			if err == nil {
				t.Fatalf("Sum() error = nil, want %q", tt.want)
			}
			if err.Error() != tt.want {
				t.Errorf("Sum() error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestSumSummaryCountsAWalk(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a file with mode 000")
	}
	tests := []struct {
		name       string
		unreadable string
		want       string
	}{
		{name: "one unreadable file in a tree of three", unreadable: "b-secret.txt", want: "could not sum 1 of 3 paths"},
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
				t.Fatalf("Sum() error = nil, want %q", tt.want)
			}
			if err.Error() != tt.want {
				t.Errorf("Sum() error = %q, want %q", err, tt.want)
			}
		})
	}
}

// The existing test asserts only that some error came back, which a dropped
// os.Create check still satisfies via the failing flush. The error's identity
// is what separates them.
func TestSumOutputFileErrorNamesTheFile(t *testing.T) {
	tests := []struct {
		name   string
		output func(root string) string
	}{
		{
			name:   "a directory that does not exist",
			output: func(root string) string { return filepath.Join(root, "missing", "SHA256SUMS") },
		},
		{
			name:   "a directory in place of the file",
			output: func(root string) string { return root },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{"a.txt": "abc"})
			outPath := tt.output(root)
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{filepath.Join(root, "a.txt")},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Output:    outPath,
			})
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("Sum() error = %v (%T), want an *fs.PathError naming the output file", err, err)
			}
			if pathErr.Path != outPath {
				t.Errorf("error path = %q, want %q", pathErr.Path, outPath)
			}
		})
	}
}

// An unreadable file fails at os.Open inside sumFile. Only an unreadable
// directory makes WalkDir hand an error to the callback, which is a separate
// branch with its own reporting and counting.
func TestSumWalkReportsAnUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a directory with mode 000")
	}
	tests := []struct {
		name   string
		locked string
	}{
		{name: "mode 000 subdirectory", locked: "b-locked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{
				"a.txt":                               "abc",
				filepath.Join(tt.locked, "inner.txt"): "abc",
				"c.txt":                               "abc",
			})
			locked := filepath.Join(root, tt.locked)
			if err := os.Chmod(locked, 0o000); err != nil {
				t.Fatalf("chmod: %v", err)
			}
			// t.TempDir cannot remove a directory it may not read.
			t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
			})
			if err == nil {
				t.Fatal("Sum() error = nil, want an error after an unreadable directory")
			}
			if want := "could not sum 1 of 3 paths"; err.Error() != want {
				t.Errorf("Sum() error = %q, want %q", err, want)
			}
			if !strings.Contains(errOut.String(), tt.locked) {
				t.Errorf("stderr = %q, want it to name %q", errOut.String(), tt.locked)
			}
			want := abcSHA256 + "  " + filepath.Join(root, "a.txt") + "\n" +
				abcSHA256 + "  " + filepath.Join(root, "c.txt") + "\n"
			if out.String() != want {
				t.Errorf("stdout = %q, want %q — the walk must continue past the failure",
					out.String(), want)
			}
		})
	}
}

// A directory is walked into, not skipped, so -v must not announce it. Removing
// the IsDir branch leaves the output identical and only this line differs.
func TestSumVerboseWalkIsSilentAboutDirectories(t *testing.T) {
	tests := []struct {
		name string
		sub  string
	}{
		{name: "a subdirectory says nothing", sub: "sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{filepath.Join(tt.sub, "a.txt"): "abc"})
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Text,
				Recursive: true,
				Verbose:   true,
			})
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if want := abcSHA256 + "  " + filepath.Join(root, tt.sub, "a.txt") + "\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if errOut.String() != "" {
				t.Errorf("stderr = %q, want empty — a directory is walked, not skipped", errOut.String())
			}
		})
	}
}

// The tag-rejection branch is reachable and already driven by
// TestSumKeepsGoingAfterATagRejection, but nothing there asserts the counts,
// so sumFile's return values on that path are free to change.
func TestSumSummaryCountsATagRejection(t *testing.T) {
	tests := []struct {
		name string
		odd  string
		want string
	}{
		{
			name: "a path tagged format cannot carry",
			odd:  "we\\ird.txt",
			want: "could not sum 1 of 3 paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sumTree(t, map[string]string{
				"a.txt": "abc",
				tt.odd:  "abc",
				"z.txt": "abc",
			})
			var out, errOut bytes.Buffer

			err := Sum(&out, &errOut, SumOptions{
				Paths:     []string{root},
				Algorithm: "sha256",
				Format:    checksums.Tag,
				Recursive: true,
			})
			if err == nil {
				t.Fatalf("Sum() error = nil, want %q", tt.want)
			}
			if err.Error() != tt.want {
				t.Errorf("Sum() error = %q, want %q", err, tt.want)
			}
		})
	}
}

// sumFile is called directly because a directory never reaches it through Sum:
// sumPath routes directories to walkDir and the walk callback skips them. The
// branch is reachable all the same — a directory opens and then fails to read.
func TestSumFileReportsAnUnreadableStream(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "a directory opens and then fails to read"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer

			attempted, failures := sumFile(&out, &errOut, t.TempDir(), hash.SHA256, checksums.Text)
			if attempted != 1 || failures != 1 {
				t.Errorf("sumFile() = (%d, %d), want (1, 1)", attempted, failures)
			}
			if out.String() != "" {
				t.Errorf("stdout = %q, want empty — nothing was hashed", out.String())
			}
			if !strings.Contains(errOut.String(), "is a directory") {
				t.Errorf("stderr = %q, want it to report the read failure", errOut.String())
			}
		})
	}
}
