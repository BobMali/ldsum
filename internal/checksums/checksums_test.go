package checksums

import (
	"bytes"
	"testing"

	"github.com/BobMali/ldsum/internal/hash"
)

// sha256 of "abc", from FIPS 180-4. The line shapes around it were captured
// from shasum 6.02 and Darwin /sbin/sha256sum, not from this package.
const abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

func abcEntry(path string) Entry {
	return Entry{
		Digest: hash.Digest{Algorithm: hash.SHA256, Hex: abcSHA256},
		Path:   path,
	}
}

func TestRenderGNU(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		path   string
		want   string
	}{
		{
			name:   "text is two spaces",
			format: Text,
			path:   "dist/a.txt",
			want:   abcSHA256 + "  dist/a.txt\n",
		},
		{
			name:   "binary is space then asterisk",
			format: Binary,
			path:   "dist/a.txt",
			want:   abcSHA256 + " *dist/a.txt\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := Render(&out, abcEntry(tt.path), tt.format); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("Render() = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestRenderTagAndBare(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		entry  Entry
		want   string
	}{
		{
			name:   "tag names the algorithm uppercased",
			format: Tag,
			entry:  abcEntry("dist/a.txt"),
			want:   "SHA256 (dist/a.txt) = " + abcSHA256 + "\n",
		},
		{
			name:   "tag uses the sha512 name for sha512",
			format: Tag,
			entry: Entry{
				Digest: hash.Digest{Algorithm: hash.SHA512, Hex: "cafe"},
				Path:   "dist/a.txt",
			},
			want: "SHA512 (dist/a.txt) = cafe\n",
		},
		{
			name:   "bare prints the digest alone",
			format: Bare,
			entry:  abcEntry("dist/a.txt"),
			want:   abcSHA256 + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := Render(&out, tt.entry, tt.format); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("Render() = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	tests := []struct {
		name   string
		format Format
	}{
		{name: "out of range", format: Format(99)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := Render(&out, abcEntry("dist/a.txt"), tt.format); err == nil {
				t.Fatal("Render() error = nil, want error for an unknown format")
			}
			if out.Len() != 0 {
				t.Errorf("wrote %q, want nothing", out.String())
			}
		})
	}
}

func TestRenderEscapesPaths(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		path   string
		want   string
	}{
		{
			name:   "a plain path gains no marker",
			format: Text,
			path:   "dist/a.txt",
			want:   abcSHA256 + "  dist/a.txt\n",
		},
		{
			name:   "a backslash doubles and marks the line",
			format: Text,
			path:   "dist/we\\ird.txt",
			want:   "\\" + abcSHA256 + "  dist/we\\\\ird.txt\n",
		},
		{
			name:   "a newline becomes backslash n",
			format: Text,
			path:   "dist/two\nlines.txt",
			want:   "\\" + abcSHA256 + "  dist/two\\nlines.txt\n",
		},
		{
			name:   "binary escapes the same way",
			format: Binary,
			path:   "dist/we\\ird.txt",
			want:   "\\" + abcSHA256 + " *dist/we\\\\ird.txt\n",
		},
		{
			name:   "both characters in one path",
			format: Text,
			path:   "dist/a\\b\nc.txt",
			want:   "\\" + abcSHA256 + "  dist/a\\\\b\\nc.txt\n",
		},
		{
			name:   "bare ignores the path entirely",
			format: Bare,
			path:   "dist/we\\ird.txt",
			want:   abcSHA256 + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := Render(&out, abcEntry(tt.path), tt.format); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("Render() = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestRenderTagRejectsEscapablePath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "backslash", path: "dist/we\\ird.txt"},
		{name: "newline", path: "dist/two\nlines.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			err := Render(&out, abcEntry(tt.path), Tag)
			if err == nil {
				t.Fatal("Render() error = nil, want error for a path needing escapes")
			}
			if out.Len() != 0 {
				t.Errorf("wrote %q, want nothing", out.String())
			}
		})
	}
}
