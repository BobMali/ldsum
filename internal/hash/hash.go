// Package hash turns a stream of bytes into a digest. It works on readers and
// strings and knows nothing about files.
package hash

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	stdhash "hash"
	"io"
	"strings"
)

// Algorithm names a supported hash function.
type Algorithm string

// The supported algorithms. Their digest lengths are distinct, which is what
// lets a checksum be recognised without being told which one it is.
const (
	SHA256 Algorithm = "sha256"
	SHA512 Algorithm = "sha512"
)

// Digest is a computed or expected checksum. Hex is always lowercase.
type Digest struct {
	Algorithm Algorithm
	Hex       string
}

// A fixed buffer keeps memory flat however large the input is.
const copyBufSize = 32 * 1024

// Sum streams r through a and returns the digest.
func Sum(r io.Reader, a Algorithm) (Digest, error) {
	h, err := newHash(a)
	if err != nil {
		return Digest{}, err
	}
	if _, err := io.CopyBuffer(h, r, make([]byte, copyBufSize)); err != nil {
		return Digest{}, err
	}
	return Digest{Algorithm: a, Hex: hex.EncodeToString(h.Sum(nil))}, nil
}

func newHash(a Algorithm) (stdhash.Hash, error) {
	switch a {
	case SHA256:
		return sha256.New(), nil
	case SHA512:
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q", a)
	}
}

// hexLen is the digest length, in hex characters, of each algorithm. The
// lengths are distinct, which is what makes inference possible.
var hexLen = map[Algorithm]int{
	SHA256: 64,
	SHA512: 128,
}

// Supported reports whether a is an algorithm this package can compute.
func Supported(a Algorithm) bool {
	_, ok := hexLen[a]
	return ok
}

// ParseDigest normalises s and infers the algorithm from its length.
func ParseDigest(s string) (Digest, error) {
	norm, err := normalize(s)
	if err != nil {
		return Digest{}, err
	}
	for a, n := range hexLen {
		if len(norm) == n {
			return Digest{Algorithm: a, Hex: norm}, nil
		}
	}
	return Digest{}, fmt.Errorf(
		"cannot tell the algorithm from %d hex characters: want 64 (sha256) or 128 (sha512)",
		len(norm),
	)
}

// ParseDigestAs normalises s and checks it against the named algorithm.
func ParseDigestAs(s string, a Algorithm) (Digest, error) {
	n, ok := hexLen[a]
	if !ok {
		return Digest{}, fmt.Errorf("unknown algorithm %q: want sha256 or sha512", a)
	}
	norm, err := normalize(s)
	if err != nil {
		return Digest{}, err
	}
	if len(norm) != n {
		return Digest{}, fmt.Errorf("%s needs %d hex characters, got %d", a, n, len(norm))
	}
	return Digest{Algorithm: a, Hex: norm}, nil
}

// Equal reports whether two digests are the same. The expected checksum is
// public, so there is nothing here for a timing attack to learn.
func (d Digest) Equal(o Digest) bool {
	return d.Algorithm == o.Algorithm && d.Hex == o.Hex
}

func normalize(s string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(s))
	if norm == "" {
		return "", errors.New("empty checksum")
	}
	if _, err := hex.DecodeString(norm); err != nil {
		return "", fmt.Errorf("not a hex checksum: %q", norm)
	}
	return norm, nil
}
