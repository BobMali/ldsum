package run

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verdict lines are what a script reads, so a path carrying a newline has to
// reach stdout escaped the way a checksum line carries it. Printed raw, the
// newline splits one verdict into two and lets whoever wrote the path dictate
// the second.
func TestVerifyEscapesTheVerdictPath(t *testing.T) {
	tests := []struct {
		name     string
		create   bool
		expected string
		verdict  string
	}{
		{name: "match", create: true, expected: abcSHA256, verdict: "OK"},
		{
			name:     "mismatch",
			create:   true,
			expected: strings.Repeat("0", 64),
			verdict:  "FAILED",
		},
		{name: "missing", expected: abcSHA256, verdict: "FAILED open or read"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "a\nb.txt")
			if tt.create {
				if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
			}
			var out, errOut bytes.Buffer

			_ = Verify(&out, &errOut, VerifyOptions{Path: path, Expected: tt.expected})

			want := `\` + dir + string(filepath.Separator) + `a\nb.txt: ` + tt.verdict + "\n"
			if out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
		})
	}
}

// The vector this escaping exists for: Parse unescapes \n in a path, so a
// hostile checksum file can name a file whose path is itself a plausible
// verdict line. Every line a script reads off stdout has to have been written
// by this run, not carried in by the file it was handed.
func TestVerifySumsCannotForgeAVerdictLine(t *testing.T) {
	dir := t.TempDir()
	const forged = "important.txt: OK"
	const payload = `decoy\n` + forged + `\ntail`
	sums := writeIn(t, dir, "SHA256SUMS", `\`+abcSHA256+"  "+payload+"\n")

	var out, errOut bytes.Buffer
	_ = VerifySums(&out, &errOut, SumsOptions{SumsFile: sums})

	for _, line := range strings.Split(out.String(), "\n") {
		if line == forged {
			t.Fatalf("stdout = %q, want no line reading %q", out.String(), forged)
		}
	}
	want := `\` + dir + string(filepath.Separator) + payload + ": FAILED open or read\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}
