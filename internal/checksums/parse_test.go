package checksums

import (
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
