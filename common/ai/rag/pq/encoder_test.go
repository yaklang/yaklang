package pq

import (
	"math"
	"math/rand"
	"testing"
)

// createTestCodebook 创建用于测试的码本
func createTestCodebook(M, K, subVectorDim int) *Codebook {
	codebook := &Codebook{
		M:            M,
		K:            K,
		SubVectorDim: subVectorDim,
		Centroids:    make([][][]float64, M),
	}

	// 生成固定的测试质心
	rand.Seed(42)
	for m := 0; m < M; m++ {
		codebook.Centroids[m] = make([][]float64, K)
		for k := 0; k < K; k++ {
			centroid := make([]float64, subVectorDim)
			for d := 0; d < subVectorDim; d++ {
				centroid[d] = rand.Float64()*2 - 1 // [-1, 1] 范围
			}
			codebook.Centroids[m][k] = centroid
		}
	}

	return codebook
}

// TestNewQuantizer 测试量化器创建
func TestNewQuantizer(t *testing.T) {
	// 测试有效码本
	codebook := createTestCodebook(8, 256, 32)
	quantizer := NewQuantizer(codebook)
	if quantizer == nil {
		t.Errorf("Expected non-nil quantizer for valid codebook")
	}

	// 测试空码本
	quantizer = NewQuantizer(nil)
	if quantizer != nil {
		t.Errorf("Expected nil quantizer for nil codebook")
	}
}

// TestEncode 测试向量编码
func TestEncode(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	// 创建测试向量
	vector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		vector[i] = rand.Float64()*2 - 1
	}

	// 测试编码
	codes, err := quantizer.Encode(vector)
	if err != nil {
		t.Fatalf("Encoding failed: %v", err)
	}

	// 验证编码结果
	if len(codes) != M {
		t.Errorf("Expected %d codes, got %d", M, len(codes))
	}

	// 验证每个码都在有效范围内
	for i, code := range codes {
		if int(code) >= K {
			t.Errorf("Code %d has invalid value %d (should be < %d)", i, code, K)
		}
	}
}

// TestEncodeWithInvalidInputs 测试编码的边界条件
func TestEncodeWithInvalidInputs(t *testing.T) {
	codebook := createTestCodebook(4, 16, 8)
	quantizer := NewQuantizer(codebook)

	tests := []struct {
		name        string
		vector      []float64
		expectError bool
	}{
		{"Correct dimension", make([]float64, 32), false},
		{"Wrong dimension", make([]float64, 31), true},
		{"Empty vector", []float64{}, true},
		{"Too large dimension", make([]float64, 64), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := quantizer.Encode(tt.vector)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestEncodeWithNilQuantizer 测试未初始化的量化器
func TestEncodeWithNilQuantizer(t *testing.T) {
	quantizer := NewQuantizer(nil)
	if quantizer != nil {
		t.Fatalf("Expected nil quantizer")
	}

	// 创建一个空的量化器来测试错误处理
	quantizer = &Quantizer{codebook: nil}
	vector := make([]float64, 32)

	_, err := quantizer.Encode(vector)
	if err == nil {
		t.Errorf("Expected error for nil codebook")
	}
}

// TestDecode 测试向量解码
func TestDecode(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	// 创建测试码
	codes := []byte{0, 5, 10, 15}

	// 测试解码
	decodedVector, err := quantizer.Decode(codes)
	if err != nil {
		t.Fatalf("Decoding failed: %v", err)
	}

	// 验证解码结果
	expectedDim := M * subVectorDim
	if len(decodedVector) != expectedDim {
		t.Errorf("Expected decoded vector dimension %d, got %d", expectedDim, len(decodedVector))
	}

	// 验证解码向量是否由正确的质心组成
	for m := 0; m < M; m++ {
		centroidIndex := int(codes[m])
		expectedCentroid := codebook.Centroids[m][centroidIndex]

		start := m * subVectorDim
		end := start + subVectorDim
		actualSegment := decodedVector[start:end]

		for i, expected := range expectedCentroid {
			if math.Abs(actualSegment[i]-expected) > 1e-9 {
				t.Errorf("Decoded segment mismatch at [%d][%d]: expected %f, got %f",
					m, i, expected, actualSegment[i])
			}
		}
	}
}

// TestDecodeWithInvalidInputs 测试解码的边界条件
func TestDecodeWithInvalidInputs(t *testing.T) {
	codebook := createTestCodebook(4, 16, 8)
	quantizer := NewQuantizer(codebook)

	tests := []struct {
		name        string
		codes       []byte
		expectError bool
	}{
		{"Correct length", []byte{0, 1, 2, 3}, false},
		{"Wrong length", []byte{0, 1, 2}, true},
		{"Empty codes", []byte{}, true},
		{"Invalid code value", []byte{0, 1, 2, 20}, true}, // 20 >= K=16
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := quantizer.Decode(tt.codes)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestEncodeDecodeRoundTrip 测试编码-解码往返
func TestEncodeDecodeRoundTrip(t *testing.T) {
	const M = 8
	const K = 64
	const subVectorDim = 16
	const vectorDim = M * subVectorDim

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	// 创建多个测试向量
	for testCase := 0; testCase < 10; testCase++ {
		originalVector := make([]float64, vectorDim)
		for i := 0; i < vectorDim; i++ {
			originalVector[i] = rand.Float64()*4 - 2 // [-2, 2] 范围
		}

		// 编码
		codes, err := quantizer.Encode(originalVector)
		if err != nil {
			t.Fatalf("Encoding failed for test case %d: %v", testCase, err)
		}

		// 解码
		decodedVector, err := quantizer.Decode(codes)
		if err != nil {
			t.Fatalf("Decoding failed for test case %d: %v", testCase, err)
		}

		// 验证维度
		if len(decodedVector) != len(originalVector) {
			t.Errorf("Dimension mismatch: original=%d, decoded=%d",
				len(originalVector), len(decodedVector))
		}

		// 计算量化误差（应该是有限的）
		errorSum := 0.0
		for i := 0; i < len(originalVector); i++ {
			diff := originalVector[i] - decodedVector[i]
			errorSum += diff * diff
		}
		quantizationError := math.Sqrt(errorSum)

		// 量化误差应该在合理范围内（不能是无穷大或NaN）
		if math.IsInf(quantizationError, 0) || math.IsNaN(quantizationError) {
			t.Errorf("Invalid quantization error: %f", quantizationError)
		}

		// 对于我们的测试数据，量化误差应该相对较小
		if quantizationError > 100.0 { // 设置一个合理的上限
			t.Errorf("Quantization error too large: %f", quantizationError)
		}
	}
}

// TestAsymmetricDistance 测试非对称距离计算
func TestAsymmetricDistance(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	// 创建查询向量和测试码
	queryVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		queryVector[i] = rand.Float64()*2 - 1
	}

	codes := []byte{0, 5, 10, 15}

	// 计算非对称距离
	distance, err := quantizer.AsymmetricDistance(queryVector, codes)
	if err != nil {
		t.Fatalf("Asymmetric distance calculation failed: %v", err)
	}

	// 验证距离是非负的
	if distance < 0 {
		t.Errorf("Distance should be non-negative, got %f", distance)
	}

	// 验证距离不是无穷大或NaN
	if math.IsInf(distance, 0) || math.IsNaN(distance) {
		t.Errorf("Invalid distance value: %f", distance)
	}
}

// TestAsymmetricDistanceWithInvalidInputs 测试非对称距离计算的边界条件
func TestAsymmetricDistanceWithInvalidInputs(t *testing.T) {
	codebook := createTestCodebook(4, 16, 8)
	quantizer := NewQuantizer(codebook)

	tests := []struct {
		name        string
		queryVector []float64
		codes       []byte
		expectError bool
	}{
		{"Valid inputs", make([]float64, 32), []byte{0, 1, 2, 3}, false},
		{"Wrong query dimension", make([]float64, 31), []byte{0, 1, 2, 3}, true},
		{"Wrong codes length", make([]float64, 32), []byte{0, 1, 2}, true},
		{"Invalid code value", make([]float64, 32), []byte{0, 1, 2, 20}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := quantizer.AsymmetricDistance(tt.queryVector, tt.codes)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestComputeDistanceTable 测试距离表计算
func TestComputeDistanceTable(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	queryVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		queryVector[i] = rand.Float64()*2 - 1
	}

	// 计算距离表
	distanceTable, err := quantizer.ComputeDistanceTable(queryVector)
	if err != nil {
		t.Fatalf("Distance table computation failed: %v", err)
	}

	// 验证距离表结构
	if len(distanceTable) != M {
		t.Errorf("Expected %d rows in distance table, got %d", M, len(distanceTable))
	}

	for m := 0; m < M; m++ {
		if len(distanceTable[m]) != K {
			t.Errorf("Expected %d columns in row %d, got %d", K, m, len(distanceTable[m]))
		}

		// 验证所有距离都是非负的
		for k := 0; k < K; k++ {
			if distanceTable[m][k] < 0 {
				t.Errorf("Distance should be non-negative at [%d][%d]: %f", m, k, distanceTable[m][k])
			}
			if math.IsInf(distanceTable[m][k], 0) || math.IsNaN(distanceTable[m][k]) {
				t.Errorf("Invalid distance value at [%d][%d]: %f", m, k, distanceTable[m][k])
			}
		}
	}
}

// TestAsymmetricDistanceWithTable 测试使用距离表的快速距离计算
func TestAsymmetricDistanceWithTable(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	queryVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		queryVector[i] = rand.Float64()*2 - 1
	}

	codes := []byte{0, 5, 10, 15}

	// 计算距离表
	distanceTable, err := quantizer.ComputeDistanceTable(queryVector)
	if err != nil {
		t.Fatalf("Distance table computation failed: %v", err)
	}

	// 使用距离表计算距离
	fastDistance, err := quantizer.AsymmetricDistanceWithTable(codes, distanceTable)
	if err != nil {
		t.Fatalf("Fast distance calculation failed: %v", err)
	}

	// 直接计算距离
	directDistance, err := quantizer.AsymmetricDistance(queryVector, codes)
	if err != nil {
		t.Fatalf("Direct distance calculation failed: %v", err)
	}

	// 两种方法的结果应该相同
	if math.Abs(fastDistance-directDistance) > 1e-9 {
		t.Errorf("Distance mismatch: fast=%f, direct=%f, diff=%f",
			fastDistance, directDistance, fastDistance-directDistance)
	}
}

// TestBatchEncode 测试批量编码
func TestBatchEncode(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim
	const numVectors = 10

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	// 创建多个测试向量
	vectors := make([][]float64, numVectors)
	for i := 0; i < numVectors; i++ {
		vector := make([]float64, vectorDim)
		for j := 0; j < vectorDim; j++ {
			vector[j] = rand.Float64()*2 - 1
		}
		vectors[i] = vector
	}

	// 批量编码
	allCodes, err := quantizer.BatchEncode(vectors)
	if err != nil {
		t.Fatalf("Batch encoding failed: %v", err)
	}

	// 验证结果
	if len(allCodes) != numVectors {
		t.Errorf("Expected %d encoded vectors, got %d", numVectors, len(allCodes))
	}

	for i, codes := range allCodes {
		if len(codes) != M {
			t.Errorf("Vector %d: expected %d codes, got %d", i, M, len(codes))
		}

		// 验证与单独编码的结果一致
		singleCodes, err := quantizer.Encode(vectors[i])
		if err != nil {
			t.Fatalf("Single encoding failed for vector %d: %v", i, err)
		}

		for j, code := range codes {
			if code != singleCodes[j] {
				t.Errorf("Batch encoding mismatch for vector %d, code %d: batch=%d, single=%d",
					i, j, code, singleCodes[j])
			}
		}
	}
}

// TestBatchEncodeWithEmptyInput 测试空输入的批量编码
func TestBatchEncodeWithEmptyInput(t *testing.T) {
	codebook := createTestCodebook(4, 16, 8)
	quantizer := NewQuantizer(codebook)

	allCodes, err := quantizer.BatchEncode([][]float64{})
	if err != nil {
		t.Fatalf("Batch encoding of empty input failed: %v", err)
	}

	if len(allCodes) != 0 {
		t.Errorf("Expected 0 codes for empty input, got %d", len(allCodes))
	}
}

// TestGetCompressionRatio 测试压缩比计算
func TestGetCompressionRatio(t *testing.T) {
	codebook := createTestCodebook(8, 256, 16)
	quantizer := NewQuantizer(codebook)

	ratio := quantizer.GetCompressionRatio()
	expectedRatio := float64(8*16*8) / float64(8*1) // (M*SubVectorDim*8) / (M*1)

	if math.Abs(ratio-expectedRatio) > 1e-9 {
		t.Errorf("Expected compression ratio %f, got %f", expectedRatio, ratio)
	}
}

// TestGetCodebookInfo 测试码本信息获取
func TestGetCodebookInfo(t *testing.T) {
	const M = 8
	const K = 256
	const subVectorDim = 16

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	info := quantizer.GetCodebookInfo()
	if info == nil {
		t.Fatalf("Expected non-nil codebook info")
	}

	// 验证各个字段
	if info["M"] != M {
		t.Errorf("Expected M=%d, got %v", M, info["M"])
	}
	if info["K"] != K {
		t.Errorf("Expected K=%d, got %v", K, info["K"])
	}
	if info["SubVectorDim"] != subVectorDim {
		t.Errorf("Expected SubVectorDim=%d, got %v", subVectorDim, info["SubVectorDim"])
	}
	if info["TotalDimension"] != M*subVectorDim {
		t.Errorf("Expected TotalDimension=%d, got %v", M*subVectorDim, info["TotalDimension"])
	}
}

// TestEstimateQuantizationError 测试量化误差估计
func TestEstimateQuantizationError(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	vector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		vector[i] = rand.Float64()*2 - 1
	}

	error, err := quantizer.EstimateQuantizationError(vector)
	if err != nil {
		t.Fatalf("Quantization error estimation failed: %v", err)
	}

	// 验证误差是非负的且有限的
	if error < 0 {
		t.Errorf("Quantization error should be non-negative, got %f", error)
	}
	if math.IsInf(error, 0) || math.IsNaN(error) {
		t.Errorf("Invalid quantization error: %f", error)
	}
}

// BenchmarkEncode 编码性能基准测试
func BenchmarkEncode(b *testing.B) {
	codebook := createTestCodebook(16, 256, 64)
	quantizer := NewQuantizer(codebook)

	vector := make([]float64, 16*64)
	for i := range vector {
		vector[i] = rand.Float64()
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := quantizer.Encode(vector)
		if err != nil {
			b.Fatalf("Encoding failed: %v", err)
		}
	}
}

// BenchmarkDecode 解码性能基准测试
func BenchmarkDecode(b *testing.B) {
	codebook := createTestCodebook(16, 256, 64)
	quantizer := NewQuantizer(codebook)

	codes := make([]byte, 16)
	for i := range codes {
		codes[i] = byte(rand.Intn(256))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := quantizer.Decode(codes)
		if err != nil {
			b.Fatalf("Decoding failed: %v", err)
		}
	}
}

// BenchmarkAsymmetricDistance 非对称距离计算性能基准测试
func BenchmarkAsymmetricDistance(b *testing.B) {
	codebook := createTestCodebook(16, 256, 64)
	quantizer := NewQuantizer(codebook)

	queryVector := make([]float64, 16*64)
	for i := range queryVector {
		queryVector[i] = rand.Float64()
	}

	codes := make([]byte, 16)
	for i := range codes {
		codes[i] = byte(rand.Intn(256))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := quantizer.AsymmetricDistance(queryVector, codes)
		if err != nil {
			b.Fatalf("Distance calculation failed: %v", err)
		}
	}
}

// BenchmarkBatchEncode 批量编码性能基准测试
func BenchmarkBatchEncode(b *testing.B) {
	codebook := createTestCodebook(8, 64, 32)
	quantizer := NewQuantizer(codebook)

	const numVectors = 100
	vectors := make([][]float64, numVectors)
	for i := 0; i < numVectors; i++ {
		vector := make([]float64, 8*32)
		for j := range vector {
			vector[j] = rand.Float64()
		}
		vectors[i] = vector
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := quantizer.BatchEncode(vectors)
		if err != nil {
			b.Fatalf("Batch encoding failed: %v", err)
		}
	}
}
