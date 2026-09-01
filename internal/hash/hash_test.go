package hash

import (
	"errors"
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

func TestParseDigest(t *testing.T) {
	sha256Hex := strings.Repeat("a", 64)
	sha512Hex := strings.Repeat("b", 128)

	valid := []struct {
		name  string
		input string
		want  Digest
	}{
		{
			name:  "sha256 length",
			input: sha256Hex,
			want:  Digest{Algorithm: SHA256, Hex: sha256Hex},
		},
		{
			name:  "sha512 length",
			input: sha512Hex,
			want:  Digest{Algorithm: SHA512, Hex: sha512Hex},
		},
		{
			name:  "uppercase is lowered",
			input: strings.ToUpper(sha256Hex),
			want:  Digest{Algorithm: SHA256, Hex: sha256Hex},
		},
		{
			name:  "surrounding space is trimmed",
			input: "  " + sha256Hex + "\n",
			want:  Digest{Algorithm: SHA256, Hex: sha256Hex},
		},
	}

	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDigest(tt.input)
			if err != nil {
				t.Fatalf("ParseDigest() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseDigest() = %+v, want %+v", got, tt.want)
			}
		})
	}

	invalid := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "empty", input: "", wantErr: "empty checksum"},
		{name: "only whitespace", input: "   ", wantErr: "empty checksum"},
		{name: "non-hex characters", input: strings.Repeat("z", 64), wantErr: "not a hex checksum"},
		{name: "odd length", input: strings.Repeat("a", 63), wantErr: "not a hex checksum"},
		{name: "sha1 length", input: strings.Repeat("a", 40), wantErr: "cannot tell the algorithm"},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDigest(tt.input)
			if err == nil {
				t.Errorf("ParseDigest(%q) = nil error, want an error", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseDigest(%q) error = %v, want substring %q", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestDigestEqual(t *testing.T) {
	base := Digest{Algorithm: SHA256, Hex: strings.Repeat("a", 64)}

	tests := []struct {
		name  string
		other Digest
		want  bool
	}{
		{name: "same algorithm and hex", other: base, want: true},
		{
			name:  "different hex",
			other: Digest{Algorithm: SHA256, Hex: strings.Repeat("b", 64)},
			want:  false,
		},
		{
			name:  "different algorithm",
			other: Digest{Algorithm: SHA512, Hex: base.Hex},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.Equal(tt.other); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseDigestAs(t *testing.T) {
	sha256Hex := strings.Repeat("a", 64)

	t.Run("length matches the named algorithm", func(t *testing.T) {
		got, err := ParseDigestAs(sha256Hex, SHA256)
		if err != nil {
			t.Fatalf("ParseDigestAs() error = %v", err)
		}
		want := Digest{Algorithm: SHA256, Hex: sha256Hex}
		if got != want {
			t.Errorf("ParseDigestAs() = %+v, want %+v", got, want)
		}
	})

	invalid := []struct {
		name      string
		input     string
		algorithm Algorithm
		wantErr   string
	}{
		{
			name:      "length contradicts the named algorithm",
			input:     sha256Hex,
			algorithm: SHA512,
			wantErr:   "needs 128 hex characters",
		},
		{
			name:      "unknown algorithm",
			input:     sha256Hex,
			algorithm: Algorithm("md5"),
			wantErr:   "unknown algorithm",
		},
		{
			name:      "not hex",
			input:     strings.Repeat("z", 64),
			algorithm: SHA256,
			wantErr:   "not a hex checksum",
		},
		{
			name:      "unknown algorithm is reported before bad hex",
			input:     strings.Repeat("z", 64),
			algorithm: Algorithm("md5"),
			wantErr:   "unknown algorithm",
		},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDigestAs(tt.input, tt.algorithm)
			if err == nil {
				t.Fatalf("ParseDigestAs(%q, %q) = nil error, want an error",
					tt.input, tt.algorithm)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseDigestAs(%q, %q) error = %v, want substring %q",
					tt.input, tt.algorithm, err, tt.wantErr)
			}
		})
	}
}

func TestSupported(t *testing.T) {
	tests := []struct {
		name string
		algo Algorithm
		want bool
	}{
		{name: "sha256", algo: SHA256, want: true},
		{name: "sha512", algo: SHA512, want: true},
		{name: "md5 is not supported", algo: Algorithm("md5"), want: false},
		{name: "the empty algorithm", algo: Algorithm(""), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Supported(tt.algo); got != tt.want {
				t.Errorf("Supported(%q) = %v, want %v", tt.algo, got, tt.want)
			}
		})
	}
}

// errReader hands over prefix and then fails, which is how a read error
// partway through a stream reaches Sum. This is an io.Reader rather than a
// mock: the rules ask for readers, not stubs of the standard library.
type errReader struct {
	prefix string
	n      int
	err    error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.n < len(r.prefix) {
		n := copy(p, r.prefix[r.n:])
		r.n += n
		return n, nil
	}
	return 0, r.err
}

func TestSumReportsAReadFailure(t *testing.T) {
	wantErr := errors.New("the device went away")
	tests := []struct {
		name   string
		prefix string
	}{
		{name: "the reader fails immediately"},
		{name: "the reader fails partway through", prefix: "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sum(&errReader{prefix: tt.prefix, err: wantErr}, SHA256)
			if !errors.Is(err, wantErr) {
				t.Fatalf("Sum() error = %v, want %v", err, wantErr)
			}
			// A partial digest is worse than no digest: it looks like an answer.
			if got != (Digest{}) {
				t.Errorf("Digest = %+v, want the zero value on failure", got)
			}
		})
	}
}
