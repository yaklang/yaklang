package hnswspec

import (
	"math"
	"math/rand"
	"testing"
)

func TestAcceleratedDotMatchesGo(t *testing.T) {
	for _, dims := range []int{384, 768, 1536} {
		rng := rand.New(rand.NewSource(int64(dims)))
		a := make([]float32, dims)
		b := make([]float32, dims)
		for i := range a {
			a[i] = rng.Float32()*2 - 1
			b[i] = rng.Float32()*2 - 1
		}
		want := dotFloat32Go(a, b)
		got := dotFloat32Accelerated(a, b)
		tolerance := math.Max(1, math.Abs(want)) * 1e-12
		if math.Abs(got-want) > tolerance {
			t.Fatalf("dims=%d: accelerated dot=%v, Go dot=%v", dims, got, want)
		}
	}
}

func TestBatchDotMatchesGo(t *testing.T) {
	const dims = 1024
	rng := rand.New(rand.NewSource(1024))
	query := make([]float32, dims)
	for i := range query {
		query[i] = rng.Float32()*2 - 1
	}
	vectors := make([][]float32, 17)
	for i := range vectors {
		vectors[i] = make([]float32, dims)
		for j := range vectors[i] {
			vectors[i][j] = rng.Float32()*2 - 1
		}
	}
	got := make([]float64, len(vectors))
	dotFloat32Batch(query, vectors, got)
	for i := range vectors {
		want := dotFloat32Go(query, vectors[i])
		tolerance := math.Max(1, math.Abs(want)) * 1e-12
		if math.Abs(got[i]-want) > tolerance {
			t.Fatalf("vector=%d: batch dot=%v, Go dot=%v", i, got[i], want)
		}
	}
}

func TestAcceleratedCosineMatchesLegacyCalculation(t *testing.T) {
	const dims = 1024
	rng := rand.New(rand.NewSource(2048))
	left := make([]float32, dims)
	right := make([]float32, dims)
	for i := range left {
		left[i] = rng.Float32()*2 - 1
		right[i] = rng.Float32()*2 - 1
	}
	leftNode := NewStandardLayerNode(1, func() []float32 { return left })
	rightNode := NewStandardLayerNode(2, func() []float32 { return right })
	got := CosineDistance[int](leftNode, rightNode)
	want := cosineDistanceRaw(left, right)
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("accelerated cosine=%v, legacy cosine=%v", got, want)
	}
}

func TestBatchCosineLoadsLazyNodeOnce(t *testing.T) {
	const dims = 1024
	queryVector := make([]float32, dims)
	nodeVector := make([]float32, dims)
	for i := 0; i < dims; i++ {
		queryVector[i] = float32(i%17) / 17
		nodeVector[i] = float32(i%23) / 23
	}
	target := NewStandardLayerNode(0, func() []float32 { return queryVector })
	underlying := NewStandardLayerNode(1, func() []float32 { return nodeVector })
	loads := 0
	lazy := NewLazyLayerNode[int](1, func(LazyNodeID) (LayerNode[int], error) {
		loads++
		return underlying, nil
	})
	vectors := make([][]float32, 1)
	norms := make([]float64, 1)
	distances := make([]float64, 1)
	if !BatchCosineDistances([]LayerNode[int]{lazy}, target, vectors, norms, distances) {
		t.Fatal("batch cosine unexpectedly rejected a standard lazy node")
	}
	if loads != 1 {
		t.Fatalf("lazy node loaded %d times, want 1", loads)
	}
	want := CosineDistance[int](underlying, target)
	if math.Abs(distances[0]-want) > 1e-12 {
		t.Fatalf("batch distance=%v, scalar distance=%v", distances[0], want)
	}
}

func TestScalarCosineLoadsEachLazyNodeOnce(t *testing.T) {
	leftVector := []float32{1, 2, 3, 4}
	rightVector := []float32{4, 3, 2, 1}
	leftNode := NewStandardLayerNode(1, func() []float32 { return leftVector })
	rightNode := NewStandardLayerNode(2, func() []float32 { return rightVector })
	leftLoads, rightLoads := 0, 0
	left := NewLazyLayerNode[int](1, func(LazyNodeID) (LayerNode[int], error) {
		leftLoads++
		return leftNode, nil
	})
	right := NewLazyLayerNode[int](2, func(LazyNodeID) (LayerNode[int], error) {
		rightLoads++
		return rightNode, nil
	})

	got := CosineDistance[int](left, right)
	want := cosineDistanceRaw(leftVector, rightVector)
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("lazy distance=%v, raw distance=%v", got, want)
	}
	if leftLoads != 1 || rightLoads != 1 {
		t.Fatalf("lazy nodes loaded left=%d right=%d, want one load each", leftLoads, rightLoads)
	}
}
