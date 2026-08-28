package hnswspec

const acceleratedDotMinDimensions = 384

func dotFloat32Go(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}
