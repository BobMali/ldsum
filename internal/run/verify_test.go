package run

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BobMali/ldsum/internal/hash"
)

// sha256 and sha512 of "abc", from FIPS 180-4.
const (
	abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	abcSHA512 = "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a" +
		"2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"
)

func writeFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestVerifyMatch(t *testing.T) {
	tests := []struct {
		name      string
		expected  string
		algorithm string
	}{
		{name: "sha256 inferred", expected: abcSHA256},
		{name: "sha512 inferred", expected: abcSHA512},
		{name: "sha256 named", expected: abcSHA256, algorithm: "sha256"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFixture(t, "abc")
			var out, errOut bytes.Buffer

			err := Verify(&out, &errOut, VerifyOptions{
				Path:      path,
				Expected:  tt.expected,
				Algorithm: tt.algorithm,
			})
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if want := path + ": OK\n"; out.String() != want {
				t.Errorf("stdout = %q, want %q", out.String(), want)
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want empty", errOut.String())
			}
		})
	}
}

func TestVerifyMismatch(t *testing.T) {
	path := writeFixture(t, "abc")
	wrong := strings.Repeat("0", 64)
	var out, errOut bytes.Buffer

	err := Verify(&out, &errOut, VerifyOptions{Path: path, Expected: wrong})

	var mismatch *MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("Verify() error = %v, want *MismatchError", err)
	}
	if mismatch.Path != path {
		t.Errorf("Path = %q, want %q", mismatch.Path, path)
	}
	if mismatch.Actual.Hex != abcSHA256 {
		t.Errorf("Actual.Hex = %q, want %q", mismatch.Actual.Hex, abcSHA256)
	}
	if mismatch.Expected.Hex != wrong {
		t.Errorf("Expected.Hex = %q, want %q", mismatch.Expected.Hex, wrong)
	}
	if mismatch.Actual.Algorithm != hash.SHA256 {
		t.Errorf("Actual.Algorithm = %q, want %q", mismatch.Actual.Algorithm, hash.SHA256)
	}

	if want := path + ": FAILED\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if !strings.Contains(errOut.String(), "expected: "+wrong) {
		t.Errorf("stderr = %q, want it to contain the expected digest", errOut.String())
	}
	if !strings.Contains(errOut.String(), "actual:   "+abcSHA256) {
		t.Errorf("stderr = %q, want it to contain the actual digest", errOut.String())
	}
}
