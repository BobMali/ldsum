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
