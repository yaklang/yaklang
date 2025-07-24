package hnsw

import (
	"fmt"
	"math"
)

// CosineSimilarity calculates the cosine similarity between two vectors.
// It returns a value between -1 (perfectly opposite) and 1 (perfectly similar).
//
// The formula is: similarity = (A · B) / (||A|| * ||B||)
//
// It returns an error if the vectors have different lengths, are empty,
// or if either vector has a magnitude of zero (is a zero vector).
func CosineSimilarity(a, b []float32) (float32, error) {
	// 1. Validate inputs
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector lengths are not equal: %d != %d", len(a), len(b))
	}
	if len(a) == 0 {
		return 0, fmt.Errorf("vectors cannot be empty")
	}

	var dotProduct float32 = 0.0
	var sumSqA float32 = 0.0
	var sumSqB float32 = 0.0

	// 2. Calculate Dot Product and Sum of Squares for magnitudes in a single loop
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		sumSqA += a[i] * a[i]
		sumSqB += b[i] * b[i]
	}

	// 3. Calculate Magnitudes (L2 norm)
	// Note: Go's math.Sqrt works on float64, so we cast back and forth.
	magA := float32(math.Sqrt(float64(sumSqA)))
	magB := float32(math.Sqrt(float64(sumSqB)))

	// Check for zero-magnitude vectors to prevent division by zero
	if magA == 0 || magB == 0 {
		return 0, fmt.Errorf("one or both vectors have a magnitude of zero")
	}

	// 4. Calculate and return the similarity
	return dotProduct / (magA * magB), nil
}

// CosineDistance computes the cosine distance between two vectors.
// The distance is defined as 1 - cosine_similarity.
// It returns a value between 0 (perfectly similar) and 2 (perfectly opposite).
func CosineDistance(a, b []float32) float32 {
	similarity, err := CosineSimilarity(a, b)
	if err != nil {
		// Pass the error up from the similarity calculation
		return 1
	}
	return 1.0 - similarity
}
