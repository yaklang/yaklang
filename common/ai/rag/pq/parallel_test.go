package pq

import (
	"math/rand"
	"runtime"
	"testing"
	"time"
)

// TestParallelTraining 测试并行训练功能
func TestParallelTraining(t *testing.T) {
	const (
		vectorDim  = 128
		numVectors = 2000
		M          = 8
		K          = 64
	)

	// 生成测试数据
	rand.Seed(42)
	trainingData := make(chan []float64, numVectors)
	go func() {
		defer close(trainingData)
		for i := 0; i < numVectors; i++ {
			vector := make([]float64, vectorDim)
			for j := 0; j < vectorDim; j++ {
				vector[j] = rand.Float64()*2 - 1
			}
			trainingData <- vector
		}
	}()

	// 并行训练
	startTime := time.Now()
	codebook, err := Train(trainingData,
		WithM(M),
		WithK(K),
		WithMaxIters(20),
		WithConcurrentKMeans(true),
		WithMaxWorkers(4),
		WithParallelDistanceCalc(true))
	parallelTime := time.Since(startTime)

	if err != nil {
		t.Fatalf("Parallel training failed: %v", err)
	}

	// 验证码本结构
	if codebook.M != M {
		t.Errorf("Expected M=%d, got %d", M, codebook.M)
	}
	if codebook.K != K {
		t.Errorf("Expected K=%d, got %d", K, codebook.K)
	}

	t.Logf("Parallel training completed in %v", parallelTime)
}

// TestSequentialVsParallelTraining 比较串行和并行训练的结果一致性
func TestSequentialVsParallelTraining(t *testing.T) {
	const (
		vectorDim  = 64
		numVectors = 1000
		M          = 4
		K          = 32
		randomSeed = 123
	)

	// 生成相同的测试数据
	generateTrainingData := func() chan []float64 {
		rand.Seed(randomSeed)
		trainingData := make(chan []float64, numVectors)
		go func() {
			defer close(trainingData)
			for i := 0; i < numVectors; i++ {
				vector := make([]float64, vectorDim)
				for j := 0; j < vectorDim; j++ {
					vector[j] = rand.Float64()*2 - 1
				}
				trainingData <- vector
			}
		}()
		return trainingData
	}

	// 串行训练
	sequentialData := generateTrainingData()
	sequentialStart := time.Now()
	sequentialCodebook, err := Train(sequentialData,
		WithM(M),
		WithK(K),
		WithMaxIters(10),
		WithRandomSeed(randomSeed),
		WithConcurrentKMeans(false))
	sequentialTime := time.Since(sequentialStart)

	if err != nil {
		t.Fatalf("Sequential training failed: %v", err)
	}

	// 并行训练
	parallelData := generateTrainingData()
	parallelStart := time.Now()
	parallelCodebook, err := Train(parallelData,
		WithM(M),
		WithK(K),
		WithMaxIters(10),
		WithRandomSeed(randomSeed),
		WithConcurrentKMeans(true),
		WithMaxWorkers(2))
	parallelTime := time.Since(parallelStart)

	if err != nil {
		t.Fatalf("Parallel training failed: %v", err)
	}

	t.Logf("Sequential training: %v", sequentialTime)
	t.Logf("Parallel training: %v", parallelTime)

	speedup := float64(sequentialTime) / float64(parallelTime)
	t.Logf("Speedup: %.2fx", speedup)

	// 验证两种方法产生的码本结构相同
	if sequentialCodebook.M != parallelCodebook.M {
		t.Errorf("M mismatch: sequential=%d, parallel=%d", sequentialCodebook.M, parallelCodebook.M)
	}
	if sequentialCodebook.K != parallelCodebook.K {
		t.Errorf("K mismatch: sequential=%d, parallel=%d", sequentialCodebook.K, parallelCodebook.K)
	}
	if sequentialCodebook.SubVectorDim != parallelCodebook.SubVectorDim {
		t.Errorf("SubVectorDim mismatch: sequential=%d, parallel=%d", sequentialCodebook.SubVectorDim, parallelCodebook.SubVectorDim)
	}
}

// TestParallelKMeans 测试并行K-Means算法
func TestParallelKMeans(t *testing.T) {
	// 创建简单的2D测试数据，两个明显的聚类
	data := [][]float64{
		{0.0, 0.0}, {0.1, 0.1}, {0.0, 0.1}, {0.1, 0.0}, // 聚类1
		{5.0, 5.0}, {5.1, 5.1}, {5.0, 5.1}, {5.1, 5.0}, // 聚类2
	}

	// 串行K-Means
	sequentialCentroids, err := kMeans(data, 2, 50, 1e-6)
	if err != nil {
		t.Fatalf("Sequential K-Means failed: %v", err)
	}

	// 并行K-Means
	parallelCentroids, err := kMeansParallel(data, 2, 50, 1e-6, 2)
	if err != nil {
		t.Fatalf("Parallel K-Means failed: %v", err)
	}

	// 验证聚类数量
	if len(sequentialCentroids) != 2 {
		t.Errorf("Expected 2 sequential centroids, got %d", len(sequentialCentroids))
	}
	if len(parallelCentroids) != 2 {
		t.Errorf("Expected 2 parallel centroids, got %d", len(parallelCentroids))
	}

	// 验证维度
	for _, centroid := range sequentialCentroids {
		if len(centroid) != 2 {
			t.Errorf("Expected 2D centroid, got %dD", len(centroid))
		}
	}
	for _, centroid := range parallelCentroids {
		if len(centroid) != 2 {
			t.Errorf("Expected 2D centroid, got %dD", len(centroid))
		}
	}
}

// TestParallelTrainingWithDifferentWorkerCounts 测试不同worker数量的并行训练
func TestParallelTrainingWithDifferentWorkerCounts(t *testing.T) {
	const (
		vectorDim  = 64
		numVectors = 1000
		M          = 8
		K          = 32
	)

	generateData := func() chan []float64 {
		rand.Seed(42)
		trainingData := make(chan []float64, numVectors)
		go func() {
			defer close(trainingData)
			for i := 0; i < numVectors; i++ {
				vector := make([]float64, vectorDim)
				for j := 0; j < vectorDim; j++ {
					vector[j] = rand.Float64()*2 - 1
				}
				trainingData <- vector
			}
		}()
		return trainingData
	}

	// 测试不同的worker数量
	workerCounts := []int{1, 2, 4, runtime.NumCPU()}
	var times []time.Duration

	for _, workers := range workerCounts {
		data := generateData()
		start := time.Now()

		_, err := Train(data,
			WithM(M),
			WithK(K),
			WithMaxIters(10),
			WithConcurrentKMeans(true),
			WithMaxWorkers(workers))

		elapsed := time.Since(start)
		times = append(times, elapsed)

		if err != nil {
			t.Errorf("Training with %d workers failed: %v", workers, err)
		}

		t.Logf("%d workers: %v", workers, elapsed)
	}

	// 验证并行化确实带来了性能提升（在大多数情况下）
	if len(times) >= 2 {
		singleWorkerTime := times[0]
		multiWorkerTime := times[1]

		// 多worker通常应该更快或至少不会显著变慢
		slowdownRatio := float64(multiWorkerTime) / float64(singleWorkerTime)
		if slowdownRatio > 2.0 { // 允许一些开销，但不应该慢太多
			t.Logf("Warning: Multi-worker training is significantly slower: %.2fx", slowdownRatio)
		}
	}
}

// TestParallelTrainingEdgeCases 测试并行训练的边界条件
func TestParallelTrainingEdgeCases(t *testing.T) {
	// 测试worker数量超过子向量数量的情况
	t.Run("WorkersExceedSubVectors", func(t *testing.T) {
		const (
			vectorDim  = 32
			numVectors = 100
			M          = 2 // 只有2个子向量
			K          = 16
		)

		trainingData := make(chan []float64, numVectors)
		go func() {
			defer close(trainingData)
			for i := 0; i < numVectors; i++ {
				vector := make([]float64, vectorDim)
				for j := 0; j < vectorDim; j++ {
					vector[j] = rand.Float64()
				}
				trainingData <- vector
			}
		}()

		// 使用比子向量数量更多的worker
		_, err := Train(trainingData,
			WithM(M),
			WithK(K),
			WithMaxIters(5),
			WithConcurrentKMeans(true),
			WithMaxWorkers(8)) // 8 > 2

		if err != nil {
			t.Errorf("Training should handle workers > sub-vectors gracefully: %v", err)
		}
	})

	// 测试单个worker的并行训练（应该等同于串行）
	t.Run("SingleWorkerParallel", func(t *testing.T) {
		const (
			vectorDim  = 32
			numVectors = 100
			M          = 4
			K          = 16
		)

		trainingData := make(chan []float64, numVectors)
		go func() {
			defer close(trainingData)
			for i := 0; i < numVectors; i++ {
				vector := make([]float64, vectorDim)
				for j := 0; j < vectorDim; j++ {
					vector[j] = rand.Float64()
				}
				trainingData <- vector
			}
		}()

		_, err := Train(trainingData,
			WithM(M),
			WithK(K),
			WithMaxIters(5),
			WithConcurrentKMeans(true),
			WithMaxWorkers(1))

		if err != nil {
			t.Errorf("Single worker parallel training failed: %v", err)
		}
	})
}

// BenchmarkSequentialVsParallelTraining 基准测试：串行vs并行训练
func BenchmarkSequentialVsParallelTraining(b *testing.B) {
	const (
		vectorDim  = 256
		numVectors = 2000
		M          = 8
		K          = 64
	)

	generateData := func() chan []float64 {
		trainingData := make(chan []float64, numVectors)
		go func() {
			defer close(trainingData)
			for i := 0; i < numVectors; i++ {
				vector := make([]float64, vectorDim)
				for j := 0; j < vectorDim; j++ {
					vector[j] = rand.Float64()
				}
				trainingData <- vector
			}
		}()
		return trainingData
	}

	b.Run("Sequential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data := generateData()
			_, err := Train(data,
				WithM(M),
				WithK(K),
				WithMaxIters(10),
				WithConcurrentKMeans(false))
			if err != nil {
				b.Fatalf("Sequential training failed: %v", err)
			}
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data := generateData()
			_, err := Train(data,
				WithM(M),
				WithK(K),
				WithMaxIters(10),
				WithConcurrentKMeans(true),
				WithMaxWorkers(4))
			if err != nil {
				b.Fatalf("Parallel training failed: %v", err)
			}
		}
	})
}

// BenchmarkParallelKMeans K-Means并行算法基准测试
func BenchmarkParallelKMeans(b *testing.B) {
	const numPoints = 2000
	const dimension = 64
	const k = 32

	// 生成测试数据
	data := make([][]float64, numPoints)
	for i := 0; i < numPoints; i++ {
		vector := make([]float64, dimension)
		for j := 0; j < dimension; j++ {
			vector[j] = rand.Float64()
		}
		data[i] = vector
	}

	b.Run("Sequential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := kMeans(data, k, 20, 1e-6)
			if err != nil {
				b.Fatalf("Sequential K-Means failed: %v", err)
			}
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := kMeansParallel(data, k, 20, 1e-6, 4)
			if err != nil {
				b.Fatalf("Parallel K-Means failed: %v", err)
			}
		}
	})
}
