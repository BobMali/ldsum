package checksums

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BobMali/ldsum/internal/hash"
)

// abcDigest is the sha256 of "abc" in the shape Parse must report.
var abcDigest = hash.Digest{Algorithm: hash.SHA256, Hex: abcSHA256}

func TestParseGNU(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Entry
	}{
		{
			name: "text is two spaces",
			in:   abcSHA256 + "  dist/a.txt\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "binary is a space then an asterisk",
			in:   abcSHA256 + " *dist/a.txt\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "a single separator space is accepted",
			in:   abcSHA256 + " dist/a.txt\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "two spaces win, so a path may start with an asterisk",
			in:   abcSHA256 + "  *odd.txt\n",
			want: Entry{Digest: abcDigest, Path: "*odd.txt", Line: 1},
		},
		{
			name: "uppercase hex is normalised",
			in:   strings.ToUpper(abcSHA256) + "  dist/a.txt\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "a file need not end in a newline",
			in:   abcSHA256 + "  dist/a.txt",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Bad) != 0 {
				t.Fatalf("Parse() Bad = %+v, want none", got.Bad)
			}
			if len(got.Entries) != 1 {
				t.Fatalf("Parse() Entries = %+v, want exactly one", got.Entries)
			}
			if got.Entries[0] != tt.want {
				t.Errorf("Parse() entry = %+v, want %+v", got.Entries[0], tt.want)
			}
		})
	}
}

func TestParseGNUBadLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "not a checksum at all", in: "hello world\n"},
		{name: "a digest of an unusable length", in: "deadbeef  a.txt\n"},
		{name: "a separator with no path after it", in: abcSHA256 + "  \n"},
		{name: "a tab is not a separator", in: abcSHA256 + "\ta.txt\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Entries) != 0 {
				t.Fatalf("Parse() Entries = %+v, want none", got.Entries)
			}
			if len(got.Bad) != 1 {
				t.Fatalf("Parse() Bad = %+v, want exactly one", got.Bad)
			}
			if got.Bad[0].Line != 1 {
				t.Errorf("Bad[0].Line = %d, want 1", got.Bad[0].Line)
			}
			if got.Bad[0].Err == nil {
				t.Error("Bad[0].Err = nil, want an error")
			}
		})
	}
}

func TestParseNumbersEveryLine(t *testing.T) {
	in := abcSHA256 + "  a.txt\n" + "garbage\n" + abcSHA256 + "  c.txt\n"

	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Entries) != 2 || got.Entries[0].Line != 1 || got.Entries[1].Line != 3 {
		t.Errorf("entry lines = %+v, want lines 1 and 3", got.Entries)
	}
	if len(got.Bad) != 1 || got.Bad[0].Line != 2 {
		t.Errorf("bad lines = %+v, want line 2", got.Bad)
	}
}

func TestParseTagged(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Entry
	}{
		{
			name: "sha256",
			in:   "SHA256 (dist/a.txt) = " + abcSHA256 + "\n",
			want: Entry{Digest: abcDigest, Path: "dist/a.txt", Line: 1},
		},
		{
			name: "a lowercase algorithm name",
			in:   "sha256 (a.txt) = " + abcSHA256 + "\n",
			want: Entry{Digest: abcDigest, Path: "a.txt", Line: 1},
		},
		{
			name: "a path containing the separator",
			in:   "SHA256 (a) = b.txt) = " + abcSHA256 + "\n",
			want: Entry{Digest: abcDigest, Path: "a) = b.txt", Line: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Bad) != 0 {
				t.Fatalf("Parse() Bad = %+v, want none", got.Bad)
			}
			if len(got.Entries) != 1 {
				t.Fatalf("Parse() Entries = %+v, want exactly one", got.Entries)
			}
			if got.Entries[0] != tt.want {
				t.Errorf("Parse() entry = %+v, want %+v", got.Entries[0], tt.want)
			}
		})
	}
}

func TestParseTaggedUnknownAlgorithm(t *testing.T) {
	got, err := Parse(strings.NewReader("MD5 (a.txt) = d41d8cd98f00b204e9800998ecf8427e\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Entries) != 0 {
		t.Fatalf("Parse() Entries = %+v, want none", got.Entries)
	}
	if len(got.Bad) != 1 {
		t.Fatalf("Parse() Bad = %+v, want exactly one", got.Bad)
	}
	if !strings.Contains(got.Bad[0].Err.Error(), "md5") {
		t.Errorf("Bad[0].Err = %v, want it to name md5", got.Bad[0].Err)
	}
}

// A file whose whole content is a digest names no path. Parse reports that
// as an entry with an empty Path and lets the caller decide what it means.
func TestParseBareDigest(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Entry
	}{
		{
			name: "sha256",
			in:   abcSHA256 + "\n",
			want: Entry{Digest: abcDigest, Line: 1},
		},
		{
			name: "sha512",
			in:   abcSHA512 + "\n",
			want: Entry{
				Digest: hash.Digest{Algorithm: hash.SHA512, Hex: abcSHA512},
				Line:   1,
			},
		},
		{
			name: "no trailing newline",
			in:   abcSHA256,
			want: Entry{Digest: abcDigest, Line: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Bad) != 0 {
				t.Fatalf("Parse() Bad = %+v, want none", got.Bad)
			}
			if len(got.Entries) != 1 {
				t.Fatalf("Parse() Entries = %+v, want exactly one", got.Entries)
			}
			if got.Entries[0] != tt.want {
				t.Errorf("Parse() entry = %+v, want %+v", got.Entries[0], tt.want)
			}
		})
	}
}

// sha512 of "abc", from FIPS 180-4.
const abcSHA512 = "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a" +
	"2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"

// A lone digest whose length matches no algorithm is not an entry: nothing
// downstream could hash a file with it.
func TestParseBareDigestOfAnUnusableLength(t *testing.T) {
	got, err := Parse(strings.NewReader("deadbeef\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Entries) != 0 {
		t.Fatalf("Parse() Entries = %+v, want none", got.Entries)
	}
	if len(got.Bad) != 1 {
		t.Fatalf("Parse() Bad = %+v, want exactly one", got.Bad)
	}
	if !strings.Contains(got.Bad[0].Err.Error(), "8 hex characters") {
		t.Errorf("Bad[0].Err = %v, want it to name the unusable length", got.Bad[0].Err)
	}
}

// Render marks an escaped line with a leading backslash and writes \\ for a
// backslash and \n for a newline. Parse has to undo exactly that and no more.
func TestParseUnescapesPaths(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a backslash in the path",
			in:   `\` + abcSHA256 + `  dist\\a.txt` + "\n",
			want: `dist\a.txt`,
		},
		{
			name: "a newline in the path",
			in:   `\` + abcSHA256 + `  a\nb` + "\n",
			want: "a\nb",
		},
		{
			name: "an escaped backslash is not the start of an escaped newline",
			in:   `\` + abcSHA256 + `  a\\nb` + "\n",
			want: `a\nb`,
		},
		{
			name: "an unmarked line is literal",
			in:   abcSHA256 + `  a\nb` + "\n",
			want: `a\nb`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if len(got.Entries) != 1 {
				t.Fatalf("Parse() Entries = %+v, Bad = %+v, want one entry", got.Entries, got.Bad)
			}
			if got.Entries[0].Path != tt.want {
				t.Errorf("path = %q, want %q", got.Entries[0].Path, tt.want)
			}
		})
	}
}

// The two halves of this package have to agree, so prove it rather than
// asserting each side's idea of the line shape separately.
func TestRenderParseRoundTrip(t *testing.T) {
	paths := []string{
		"dist/a.txt",
		`dist\a.txt`,
		"a\nb",
		`a\nb`,
		"*starts-with-an-asterisk",
		"  starts-with-spaces",
		"ends-with-spaces  ",
	}

	for _, format := range []Format{Text, Binary} {
		for _, p := range paths {
			t.Run(p, func(t *testing.T) {
				var buf bytes.Buffer
				if err := Render(&buf, Entry{Digest: abcDigest, Path: p}, format); err != nil {
					t.Fatalf("Render() error = %v", err)
				}
				got, err := Parse(&buf)
				if err != nil {
					t.Fatalf("Parse() error = %v", err)
				}
				if len(got.Entries) != 1 {
					t.Fatalf("Parse() Entries = %+v, Bad = %+v, want one entry", got.Entries, got.Bad)
				}
				if got.Entries[0].Path != p {
					t.Errorf("round trip gave %q, want %q", got.Entries[0].Path, p)
				}
			})
		}
	}
}

func TestParseSkipsNoise(t *testing.T) {
	in := "# a header comment\r\n" +
		"\r\n" +
		abcSHA256 + "  a.txt\r\n" +
		"   \n" +
		"  # an indented comment\n" +
		"SHA512 (b.txt) = " + abcSHA512 + "\n"

	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Bad) != 0 {
		t.Fatalf("Parse() Bad = %+v, want none", got.Bad)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("Parse() Entries = %+v, want two", got.Entries)
	}
	if got.Entries[0].Path != "a.txt" || got.Entries[0].Digest.Algorithm != hash.SHA256 {
		t.Errorf("first entry = %+v, want a.txt as sha256", got.Entries[0])
	}
	if got.Entries[1].Path != "b.txt" || got.Entries[1].Digest.Algorithm != hash.SHA512 {
		t.Errorf("second entry = %+v, want b.txt as sha512", got.Entries[1])
	}
}

func TestParseEmptyFile(t *testing.T) {
	got, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Entries) != 0 || len(got.Bad) != 0 {
		t.Errorf("Parse() = %+v, want an empty listing", got)
	}
}

// leadingHex and isHexDigit decide where a digest ends, so their boundaries are
// tested directly: through Parse every one of these cases is just a BadLine,
// which makes a wrong boundary invisible.
func TestLeadingHex(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty input", in: "", want: ""},
		{name: "every hex byte", in: "0123456789abcdefABCDEF", want: "0123456789abcdefABCDEF"},
		{name: "stops at the first non-hex byte", in: "abcz", want: "abc"},
		{name: "a non-hex first byte yields nothing", in: "zzz", want: ""},
		// Each of these is the byte immediately past one arm's upper bound.
		{name: "colon is one past 9", in: "9:", want: "9"},
		{name: "g is one past f", in: "fg", want: "f"},
		{name: "capital G is one past F", in: "FG", want: "F"},
		// A space is below every arm's lower bound, and is what separates a
		// real digest from its path.
		{name: "space is below every range", in: "0 1", want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leadingHex(tt.in); got != tt.want {
				t.Errorf("leadingHex(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
