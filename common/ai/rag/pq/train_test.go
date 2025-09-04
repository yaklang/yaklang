package pq

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// TestTrain 测试基本的训练功能
func TestTrain(t *testing.T) {
	// 设置固定随机种子以获得可重现的结果
	rand.Seed(42)

	const (
		vectorDim  = 256
		numVectors = 1000
		M          = 8
		K          = 32
	)

	// 生成测试数据
	trainingData := make(chan []float64, numVectors)
	go func() {
		defer close(trainingData)
		for i := 0; i < numVectors; i++ {
			vector := make([]float64, vectorDim)
			for j := 0; j < vectorDim; j++ {
				vector[j] = rand.Float64()*2 - 1 // [-1, 1] 范围
			}
			trainingData <- vector
		}
	}()

	// 训练PQ模型
	codebook, err := Train(trainingData, WithM(M), WithK(K), WithMaxIters(10))
	if err != nil {
		t.Fatalf("Training failed: %v", err)
	}

	// 验证码本结构
	if codebook.M != M {
		t.Errorf("Expected M=%d, got %d", M, codebook.M)
	}
	if codebook.K != K {
		t.Errorf("Expected K=%d, got %d", K, codebook.K)
	}
	if codebook.SubVectorDim != vectorDim/M {
		t.Errorf("Expected SubVectorDim=%d, got %d", vectorDim/M, codebook.SubVectorDim)
	}

	// 验证质心数量
	if len(codebook.Centroids) != M {
		t.Errorf("Expected %d codebooks, got %d", M, len(codebook.Centroids))
	}

	for m := 0; m < M; m++ {
		if len(codebook.Centroids[m]) != K {
			t.Errorf("Codebook %d: expected %d centroids, got %d", m, K, len(codebook.Centroids[m]))
		}
		for k := 0; k < K; k++ {
			if len(codebook.Centroids[m][k]) != vectorDim/M {
				t.Errorf("Centroid [%d][%d]: expected dimension %d, got %d",
					m, k, vectorDim/M, len(codebook.Centroids[m][k]))
			}
		}
	}
}

// TestTrainWithInvalidInputs 测试各种无效输入
func TestTrainWithInvalidInputs(t *testing.T) {
	tests := []struct {
		name        string
		vectorDim   int
		M           int
		K           int
		numVectors  int
		expectError bool
	}{
		{"Empty data", 128, 8, 256, 0, true},
		{"Invalid M", 128, 0, 256, 100, true},
		{"Invalid K", 128, 8, 0, 100, true},
		{"K too large", 128, 8, 300, 100, true},
		{"Dimension not divisible by M", 127, 8, 256, 100, true},
		{"Valid input", 128, 8, 32, 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trainingData := make(chan []float64, tt.numVectors)
			go func() {
				defer close(trainingData)
				for i := 0; i < tt.numVectors; i++ {
					vector := make([]float64, tt.vectorDim)
					for j := 0; j < tt.vectorDim; j++ {
						vector[j] = rand.Float64()
					}
					trainingData <- vector
				}
			}()

			_, err := Train(trainingData, WithM(tt.M), WithK(tt.K), WithMaxIters(5))

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestTrainWithInconsistentDimensions 测试维度不一致的输入
func TestTrainWithInconsistentDimensions(t *testing.T) {
	trainingData := make(chan []float64, 3)
	go func() {
		defer close(trainingData)
		trainingData <- make([]float64, 128)
		trainingData <- make([]float64, 64) // 不同维度
		trainingData <- make([]float64, 128)
	}()

	_, err := Train(trainingData, WithM(8), WithK(32))
	if err == nil {
		t.Errorf("Expected error for inconsistent dimensions")
	}
}

// TestTrainOptions 测试各种配置选项
func TestTrainOptions(t *testing.T) {
	const vectorDim = 64
	const numVectors = 100

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

	codebook, err := Train(trainingData,
		WithM(4),
		WithK(16),
		WithMaxIters(5),
		WithTolerance(1e-3),
		WithRandomSeed(123),
	)

	if err != nil {
		t.Fatalf("Training with options failed: %v", err)
	}

	if codebook.M != 4 {
		t.Errorf("Expected M=4, got %d", codebook.M)
	}
	if codebook.K != 16 {
		t.Errorf("Expected K=16, got %d", codebook.K)
	}
}

// TestKMeans 测试K-Means算法
func TestKMeans(t *testing.T) {
	// 创建简单的2D测试数据，两个明显的聚类
	data := [][]float64{
		{0.0, 0.0}, {0.1, 0.1}, {0.0, 0.1}, {0.1, 0.0}, // 聚类1
		{5.0, 5.0}, {5.1, 5.1}, {5.0, 5.1}, {5.1, 5.0}, // 聚类2
	}

	centroids, err := kMeans(data, 2, 50, 1e-6)
	if err != nil {
		t.Fatalf("K-Means failed: %v", err)
	}

	if len(centroids) != 2 {
		t.Errorf("Expected 2 centroids, got %d", len(centroids))
	}

	for _, centroid := range centroids {
		if len(centroid) != 2 {
			t.Errorf("Expected 2D centroid, got %dD", len(centroid))
		}
	}

	// 验证聚类中心是否合理（应该接近 [0.05, 0.05] 和 [5.05, 5.05]）
	center1Found := false
	center2Found := false

	for _, centroid := range centroids {
		dist1 := math.Sqrt(math.Pow(centroid[0]-0.05, 2) + math.Pow(centroid[1]-0.05, 2))
		dist2 := math.Sqrt(math.Pow(centroid[0]-5.05, 2) + math.Pow(centroid[1]-5.05, 2))

		if dist1 < 0.5 {
			center1Found = true
		}
		if dist2 < 0.5 {
			center2Found = true
		}
	}

	if !center1Found || !center2Found {
		t.Errorf("K-Means did not find expected cluster centers")
	}
}

// TestKMeansWithInvalidInputs 测试K-Means的边界条件
func TestKMeansWithInvalidInputs(t *testing.T) {
	tests := []struct {
		name        string
		data        [][]float64
		k           int
		expectError bool
	}{
		{"Empty data", [][]float64{}, 2, true},
		{"K <= 0", [][]float64{{1, 2}}, 0, true},
		{"K > data size", [][]float64{{1, 2}}, 5, true},
		{"Valid input", [][]float64{{1, 2}, {3, 4}, {5, 6}}, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := kMeans(tt.data, tt.k, 10, 1e-6)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestEuclideanDistanceSquared 测试欧氏距离计算
func TestEuclideanDistanceSquared(t *testing.T) {
	tests := []struct {
		name     string
		v1       []float64
		v2       []float64
		expected float64
	}{
		{"Same vectors", []float64{1, 2, 3}, []float64{1, 2, 3}, 0.0},
		{"Different vectors", []float64{0, 0}, []float64{3, 4}, 25.0},
		{"Single dimension", []float64{1}, []float64{4}, 9.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := euclideanDistanceSquared(tt.v1, tt.v2)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("Expected distance squared %.9f, got %.9f", tt.expected, result)
			}
		})
	}
}

// TestEuclideanDistanceSquaredWithDifferentLengths 测试不同长度向量的距离计算
func TestEuclideanDistanceSquaredWithDifferentLengths(t *testing.T) {
	v1 := []float64{1, 2, 3}
	v2 := []float64{1, 2}

	result := euclideanDistanceSquared(v1, v2)
	if result != math.MaxFloat64 {
		t.Errorf("Expected MaxFloat64 for different length vectors, got %f", result)
	}
}

// BenchmarkTrain 训练性能基准测试
func BenchmarkTrain(b *testing.B) {
	const vectorDim = 512
	const numVectors = 1000

	vectors := make([][]float64, numVectors)
	for i := 0; i < numVectors; i++ {
		vector := make([]float64, vectorDim)
		for j := 0; j < vectorDim; j++ {
			vector[j] = rand.Float64()
		}
		vectors[i] = vector
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		trainingData := make(chan []float64, numVectors)
		go func() {
			defer close(trainingData)
			for _, vector := range vectors {
				trainingData <- vector
			}
		}()

		_, err := Train(trainingData, WithM(8), WithK(64), WithMaxIters(5))
		if err != nil {
			b.Fatalf("Training failed: %v", err)
		}
	}
}

// BenchmarkKMeans K-Means算法性能基准测试
func BenchmarkKMeans(b *testing.B) {
	// 生成测试数据
	const numPoints = 1000
	const dimension = 64

	data := make([][]float64, numPoints)
	for i := 0; i < numPoints; i++ {
		vector := make([]float64, dimension)
		for j := 0; j < dimension; j++ {
			vector[j] = rand.Float64()
		}
		data[i] = vector
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := kMeans(data, 32, 20, 1e-6)
		if err != nil {
			b.Fatalf("K-Means failed: %v", err)
		}
	}
}

// TestTrainWithRealWorldData 使用更真实的数据测试训练
func TestTrainWithRealWorldData(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	const vectorDim = 1024
	const numVectors = 5000
	const M = 16
	const K = 256

	// 生成模拟真实世界的数据（具有一些结构）
	trainingData := make(chan []float64, numVectors)
	go func() {
		defer close(trainingData)
		for i := 0; i < numVectors; i++ {
			vector := make([]float64, vectorDim)

			// 模拟有结构的数据：前半部分偏向正值，后半部分偏向负值
			for j := 0; j < vectorDim/2; j++ {
				vector[j] = math.Abs(rand.NormFloat64()) // 正值偏向
			}
			for j := vectorDim / 2; j < vectorDim; j++ {
				vector[j] = -math.Abs(rand.NormFloat64()) // 负值偏向
			}

			trainingData <- vector
		}
	}()

	startTime := time.Now()
	codebook, err := Train(trainingData,
		WithM(M),
		WithK(K),
		WithMaxIters(30),
		WithTolerance(1e-5))
	trainingTime := time.Since(startTime)

	if err != nil {
		t.Fatalf("Training failed: %v", err)
	}

	t.Logf("Training completed in %v", trainingTime)
	t.Logf("Codebook: M=%d, K=%d, SubVectorDim=%d",
		codebook.M, codebook.K, codebook.SubVectorDim)

	// 验证训练结果的合理性
	if codebook.M != M || codebook.K != K {
		t.Errorf("Codebook parameters mismatch")
	}

	// 检查质心是否有合理的分布
	for m := 0; m < codebook.M; m++ {
		var sumMagnitude float64
		for k := 0; k < codebook.K; k++ {
			centroid := codebook.Centroids[m][k]
			magnitude := 0.0
			for _, val := range centroid {
				magnitude += val * val
			}
			sumMagnitude += math.Sqrt(magnitude)
		}
		avgMagnitude := sumMagnitude / float64(codebook.K)

		// 质心的平均幅度应该在合理范围内
		if avgMagnitude < 0.1 || avgMagnitude > 100.0 {
			t.Errorf("Unreasonable centroid magnitude for subspace %d: %f", m, avgMagnitude)
		}
	}
}
