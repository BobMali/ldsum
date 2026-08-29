package hash

import (
	"strings"
	"testing"
)

// Vectors are the published FIPS 180-4 examples, never values this package
// produced.
func TestSumSHA256(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty input",
			input: "",
			want:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:  "abc",
			input: "abc",
			want:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
		{
			name:  "one million a",
			input: strings.Repeat("a", 1000000),
			want:  "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sum(strings.NewReader(tt.input), SHA256)
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if got.Hex != tt.want {
				t.Errorf("Hex = %s, want %s", got.Hex, tt.want)
			}
			if got.Algorithm != SHA256 {
				t.Errorf("Algorithm = %s, want %s", got.Algorithm, SHA256)
			}
		})
	}
}

func TestSumUnknownAlgorithm(t *testing.T) {
	tests := []struct {
		name string
		algo Algorithm
	}{
		{name: "unsupported algorithm", algo: Algorithm("md5")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Sum(strings.NewReader(""), tt.algo); err == nil {
				t.Fatal("Sum() error = nil, want error for unknown algorithm")
			}
		})
	}
}
