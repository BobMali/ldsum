// Package hash turns a stream of bytes into a digest. It works on readers and
// strings and knows nothing about files.
package hash

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	stdhash "hash"
	"io"
)

// Algorithm names a supported hash function.
type Algorithm string

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
