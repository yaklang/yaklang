package hnswspec

import (
	"fmt"
	"math/rand"
	"testing"
)

func BenchmarkDotFloat32(b *testing.B) {
	for _, dims := range []int{128, 256, 384, 768, 1536} {
		a := make([]float32, dims)
		c := make([]float32, dims)
		rng := rand.New(rand.NewSource(1))
		for i := range a {
			a[i], c[i] = rng.Float32(), rng.Float32()
		}
		b.Run(fmt.Sprintf("auto/dims=%d", dims), func(b *testing.B) {
			var result float64
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if dims >= acceleratedDotMinDimensions {
					result = dotFloat32Accelerated(a, c)
				} else {
					result = dotFloat32Go(a, c)
				}
			}
			_ = result
		})
		b.Run(fmt.Sprintf("go/dims=%d", dims), func(b *testing.B) {
			var result float64
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result = dotFloat32Go(a, c)
			}
			_ = result
		})
	}
}

func BenchmarkDotFloat32Batch1024(b *testing.B) {
	const (
		dims  = 1024
		count = 32
	)
	rng := rand.New(rand.NewSource(1024))
	query := make([]float32, dims)
	vectors := make([][]float32, count)
	for i := range query {
		query[i] = rng.Float32()
	}
	for i := range vectors {
		vectors[i] = make([]float32, dims)
		for j := range vectors[i] {
			vectors[i][j] = rng.Float32()
		}
	}
	output := make([]float64, count)
	b.Run("single", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(count * dims * 2 * 4)
		for i := 0; i < b.N; i++ {
			for j := range vectors {
				output[j] = dotFloat32Accelerated(query, vectors[j])
			}
		}
	})
	b.Run("batch", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(count * dims * 2 * 4)
		for i := 0; i < b.N; i++ {
			dotFloat32Batch(query, vectors, output)
		}
	})
}
