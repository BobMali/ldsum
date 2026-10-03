package hash

import (
	"bytes"
	"fmt"
	"testing"
)

// The README's performance section quotes these. Run them with
//
//	go test -run '^$' -bench . ./internal/hash
//	go test -run '^$' -bench Parallel -cpu 1,2,4,8,16 ./internal/hash
//
// They read from memory on purpose: what they pin is the hash rate, which is
// the ceiling on any read, so a disk or a network can only sit below it.

func BenchmarkSum(b *testing.B) {
	for _, a := range []Algorithm{SHA256, SHA512} {
		for _, size := range []int{4 << 10, 1 << 20} {
			data := bytes.Repeat([]byte("x"), size)
			b.Run(fmt.Sprintf("%s/%dKiB", a, size>>10), func(b *testing.B) {
				b.SetBytes(int64(size))
				for b.Loop() {
					if _, err := Sum(bytes.NewReader(data), a); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// Read across -cpu values, this is how far hashing separate files at once
// could scale: each goroutine hashes its own stream, as a worker would.
func BenchmarkSumParallel(b *testing.B) {
	data := bytes.Repeat([]byte("x"), 1<<20)
	b.SetBytes(int64(len(data)))
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Sum(bytes.NewReader(data), SHA256); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

// Sum allocates its copy buffer on every call, so a small input pays for a
// buffer many times its own size. The allocation count is the point here.
func BenchmarkSumSmallInput(b *testing.B) {
	data := bytes.Repeat([]byte("x"), 4<<10)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Sum(bytes.NewReader(data), SHA256); err != nil {
			b.Fatal(err)
		}
	}
}
