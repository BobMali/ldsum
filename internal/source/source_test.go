package source

import (
	"strconv"
	"strings"
	"testing"
)

func TestIsRemote(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		want       bool
		wantErr    bool
		wantScheme string
	}{
		{name: "https URL", ref: "https://ex.org/f", want: true},
		{name: "http URL", ref: "http://ex.org/f", want: true},
		{name: "scheme is case insensitive", ref: "HTTPS://EX.ORG/f", want: true},
		{name: "a relative path", ref: "dist.tar.gz"},
		{name: "a nested relative path", ref: "sub/dist.tar.gz"},
		{name: "an absolute path", ref: "/var/tmp/dist.tar.gz"},
		{name: "a dotted path", ref: "./dist.tar.gz"},
		// A colon is legal in a file name. Only the slashes make it a URL.
		{name: "a colon in a file name", ref: "weird:thing"},
		{name: "a windows-looking path", ref: "c:/x/https://y"},
		// "://" appears, but the scheme before the first colon is empty.
		{name: "a path containing a URL", ref: "mirror/https://ex.org/f"},
		// url.Parse rejects control characters, so this is a file name.
		{name: "a newline in the reference", ref: "https://ex.org/a\nb"},
		{name: "an unfetchable scheme", ref: "ftp://host/f", wantErr: true, wantScheme: "ftp"},
		{name: "a file scheme", ref: "file:///etc/passwd", wantErr: true, wantScheme: "file"},
		{name: "an uppercase unfetchable scheme", ref: "FTP://HOST/F", wantErr: true, wantScheme: "ftp"},
		{name: "an empty reference", ref: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsRemote(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("IsRemote(%q) error = nil, want an error", tt.ref)
				}
				// The message has to name the reference and the scheme, or it
				// says nothing the user can act on.
				if !strings.Contains(err.Error(), tt.ref) {
					t.Errorf("error = %q, want it to name %q", err, tt.ref)
				}
				if !strings.Contains(err.Error(), strconv.Quote(tt.wantScheme)) {
					t.Errorf("error = %q, want it to name scheme %q", err, tt.wantScheme)
				}
				if got {
					t.Errorf("IsRemote(%q) = true, want false alongside the error", tt.ref)
				}
				return
			}
			if err != nil {
				t.Fatalf("IsRemote(%q) error = %v", tt.ref, err)
			}
			if got != tt.want {
				t.Errorf("IsRemote(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}
