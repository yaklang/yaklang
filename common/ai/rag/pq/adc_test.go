package pq

import (
	"math"
	"math/rand"
	"testing"
)

func TestCosineDistanceFromDotTableMatchesAsymmetricCosine(t *testing.T) {
	const dimension = 32
	rng := rand.New(rand.NewSource(7))
	vectors := make([][]float64, 40)
	for i := range vectors {
		vectors[i] = make([]float64, dimension)
		for d := range vectors[i] {
			vectors[i][d] = rng.NormFloat64()
		}
	}
	input := make(chan []float64, len(vectors))
	for _, vector := range vectors {
		input <- vector
	}
	close(input)
	codebook, err := Train(input, WithM(4), WithK(8), WithMaxIters(8), WithRandomSeed(7), WithConcurrentKMeans(false), WithParallelDistanceCalc(false))
	if err != nil {
		t.Fatal(err)
	}
	quantizer := NewQuantizer(codebook)
	query := vectors[0]
	code, err := quantizer.Encode(vectors[1])
	if err != nil {
		t.Fatal(err)
	}
	table, norms, err := quantizer.ComputeDotProductTable(query)
	if err != nil {
		t.Fatal(err)
	}
	var queryNormSq float64
	for _, norm := range norms {
		queryNormSq += norm * norm
	}
	got, err := quantizer.CosineDistanceFromDotTable(code, table, queryNormSq)
	if err != nil {
		t.Fatal(err)
	}
	similarity, err := quantizer.AsymmetricCosineSimilarity(query, code)
	if err != nil {
		t.Fatal(err)
	}
	want := 1 - similarity
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("adc cosine distance = %.12f, asymmetric = %.12f", got, want)
	}
}
