package pq

import (
	"math"
	"math/rand"
	"reflect"
	"sync"
	"testing"
)

func trainingInput(data [][]float64) <-chan []float64 {
	input := make(chan []float64, len(data))
	for _, vector := range data {
		input <- vector
	}
	close(input)
	return input
}

func TestTrainSingleClusterMean(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		for _, parallelDistance := range []bool{false, true} {
			book, err := Train(trainingInput([][]float64{{0, 2}, {4, 6}, {11, 13}}),
				WithM(1), WithK(1), WithRandomSeed(5), WithConcurrentKMeans(concurrent),
				WithParallelDistanceCalc(parallelDistance), WithMaxWorkers(2))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(book.Centroids, [][][]float64{{{5, 7}}}) {
				t.Errorf("concurrent=%v parallelDistance=%v: expected mean [5, 7], got %v", concurrent, parallelDistance, book.Centroids)
			}
		}
	}
}

func TestTrainSeedReproducibility(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	data := make([][]float64, 100)
	for i := range data {
		data[i] = make([]float64, 8)
		for j := range data[i] {
			data[i][j] = rng.Float64()
		}
	}
	train := func(concurrent, parallelDistance bool, workers int, seed int64) (*Codebook, error) {
		return Train(trainingInput(data), WithM(4), WithK(8), WithMaxIters(20),
			WithRandomSeed(seed), WithConcurrentKMeans(concurrent),
			WithParallelDistanceCalc(parallelDistance), WithMaxWorkers(workers))
	}
	want, err := train(false, false, 1, 42)
	if err != nil {
		t.Fatal(err)
	}
	for _, concurrent := range []bool{false, true} {
		for _, parallelDistance := range []bool{false, true} {
			for _, workers := range []int{-1, 0, 1, 2, 8} {
				got, err := train(concurrent, parallelDistance, workers, 42)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("seeded codebook depends on scheduling: concurrent=%v parallelDistance=%v workers=%d", concurrent, parallelDistance, workers)
				}
			}
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			seed := int64(42)
			if i%2 != 0 {
				seed = int64(i)
			}
			got, err := train(true, true, 4, seed)
			if err != nil {
				t.Error(err)
				return
			}
			if seed == 42 && !reflect.DeepEqual(got, want) {
				t.Error("another concurrent training call changed the seeded codebook")
			}
		}(i)
	}
	wg.Wait()
	// Recreate the fixture to verify that centroid updates did not mutate it.
	rng.Seed(10)
	for _, vector := range data {
		for _, value := range vector {
			if value != rng.Float64() {
				t.Fatal("Train modified its input vectors")
			}
		}
	}
}

func TestKMeansDuplicateVectors(t *testing.T) {
	for _, data := range [][][]float64{
		{{7, 9}, {7, 9}, {7, 9}, {7, 9}},
		{{0, 0}, {0, 0}, {10, 10}, {10, 10}},
	} {
		for _, workers := range []int{1, 2, 8} {
			centroids, err := kMeansWithRand(data, 3, 50, 1e-6, workers, rand.New(rand.NewSource(5)))
			if err != nil {
				t.Fatal(err)
			}
			if len(centroids) != 3 {
				t.Fatalf("expected three centers, got %v", centroids)
			}
			for _, point := range data {
				found := false
				for _, center := range centroids {
					if reflect.DeepEqual(point, center) {
						found = true
					}
				}
				if !found {
					t.Errorf("point %v is not represented by centers %v", point, centroids)
				}
			}
		}
	}
}

func TestKMeansPlusPlusCopiesInput(t *testing.T) {
	data := [][]float64{{0, 0}, {0, 0}, {10, 10}, {10, 10}}
	centers := initializeKMeansPlusPlus(data, 4, rand.New(rand.NewSource(5)))
	if reflect.DeepEqual(centers[0], centers[1]) {
		t.Fatalf("initializer repeated a center before representing both distinct points: %v", centers)
	}
	for _, center := range centers {
		center[0] = 99
	}
	if !reflect.DeepEqual(data, [][]float64{{0, 0}, {0, 0}, {10, 10}, {10, 10}}) {
		t.Fatal("initial centers alias the input")
	}
}

func TestKMeansInvalidTrainingParameters(t *testing.T) {
	for _, tc := range []struct {
		name      string
		data      [][]float64
		maxIters  int
		tolerance float64
	}{
		{"zero_iterations", [][]float64{{0}}, 0, 1e-6},
		{"negative_iterations", [][]float64{{0}}, -1, 1e-6},
		{"negative_tolerance", [][]float64{{0}}, 10, -1},
		{"nan_tolerance", [][]float64{{0}}, 10, math.NaN()},
		{"infinite_tolerance", [][]float64{{0}}, 10, math.Inf(1)},
		{"empty_vector", [][]float64{{}}, 10, 1e-6},
		{"inconsistent_dimensions", [][]float64{{0}, {0, 1}}, 10, 1e-6},
		{"nan_vector", [][]float64{{math.NaN()}}, 10, 1e-6},
		{"infinite_vector", [][]float64{{math.Inf(1)}}, 10, 1e-6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, workers := range []int{1, 2} {
				if _, err := kMeansWithRand(tc.data, 1, tc.maxIters, tc.tolerance, workers, rand.New(rand.NewSource(42))); err == nil {
					t.Fatalf("invalid input accepted with %d workers", workers)
				}
			}
			for _, concurrent := range []bool{false, true} {
				if _, err := Train(trainingInput(tc.data), WithM(1), WithK(1), WithMaxIters(tc.maxIters),
					WithTolerance(tc.tolerance), WithConcurrentKMeans(concurrent)); err == nil {
					t.Fatalf("invalid input accepted by Train with concurrent=%v", concurrent)
				}
			}
		})
	}
}

func TestTrainDoesNotReseedGlobalRandom(t *testing.T) {
	const seed int64 = 123
	want := rand.New(rand.NewSource(seed)).Int63()
	rand.Seed(seed)
	_, err := Train(trainingInput([][]float64{{0}, {1}, {2}}),
		WithM(1), WithK(1), WithRandomSeed(42), WithConcurrentKMeans(false))
	if err != nil {
		t.Fatal(err)
	}
	if got := rand.Int63(); got != want {
		t.Errorf("Train changed the global random stream: got %d, want %d", got, want)
	}
}

func BenchmarkTrainSeeded(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	data := make([][]float64, 1000)
	for i := range data {
		data[i] = make([]float64, 64)
		for j := range data[i] {
			data[i][j] = math.Sin(float64(i%16)) + rng.Float64()*0.1
		}
	}
	for _, mode := range []struct {
		name       string
		concurrent bool
	}{
		{"sequential", false},
		{"parallel", true},
	} {
		b.Run(mode.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := Train(trainingInput(data), WithM(4), WithK(16), WithMaxIters(20),
					WithRandomSeed(42), WithConcurrentKMeans(mode.concurrent),
					WithParallelDistanceCalc(false), WithMaxWorkers(4))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
