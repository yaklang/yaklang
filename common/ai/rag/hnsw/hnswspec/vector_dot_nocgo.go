//go:build !cgo || hnsw_purego

package hnswspec

func dotFloat32Accelerated(a, b []float32) float64 {
	return dotFloat32Go(a, b)
}

func dotFloat32Batch(query []float32, vectors [][]float32, output []float64) {
	for i := range vectors {
		output[i] = dotFloat32Go(query, vectors[i])
	}
}
