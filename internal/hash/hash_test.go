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

func TestSumSHA512(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty input",
			input: "",
			want:  "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
		},
		{
			name:  "abc",
			input: "abc",
			want:  "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		},
		{
			name:  "one million a",
			input: strings.Repeat("a", 1000000),
			want:  "e718483d0ce769644e2e42c7bc15b4638e1f98b13b2044285632a803afa973ebde0ff244877ea60a4cb0432ce577c31beb009c5c2c49aa2e4eadb217ad8cc09b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sum(strings.NewReader(tt.input), SHA512)
			if err != nil {
				t.Fatalf("Sum() error = %v", err)
			}
			if got.Hex != tt.want {
				t.Errorf("Hex = %s, want %s", got.Hex, tt.want)
			}
			if got.Algorithm != SHA512 {
				t.Errorf("Algorithm = %s, want %s", got.Algorithm, SHA512)
			}
		})
	}
}
