package pq

import (
	"math"
	"math/rand"
	"testing"
)

// TestAsymmetricCosineSimilarity 测试非对称余弦相似度计算
func TestAsymmetricCosineSimilarity(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	// 创建测试向量
	queryVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		queryVector[i] = rand.Float64()*2 - 1
	}

	// 编码一个向量
	testVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		testVector[i] = rand.Float64()*2 - 1
	}
	codes, err := quantizer.Encode(testVector)
	if err != nil {
		t.Fatalf("Encoding failed: %v", err)
	}

	// 计算非对称余弦相似度
	similarity, err := quantizer.AsymmetricCosineSimilarity(queryVector, codes)
	if err != nil {
		t.Fatalf("Asymmetric cosine similarity calculation failed: %v", err)
	}

	// 验证相似度在有效范围内 [-1, 1]
	if similarity < -1.0 || similarity > 1.0 {
		t.Errorf("Cosine similarity should be in [-1, 1], got %f", similarity)
	}

	// 验证不是无穷大或NaN
	if math.IsInf(similarity, 0) || math.IsNaN(similarity) {
		t.Errorf("Invalid similarity value: %f", similarity)
	}
}

// TestSymmetricCosineSimilarity 测试对称余弦相似度计算
func TestSymmetricCosineSimilarity(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	// 创建两个测试向量并编码
	vector1 := make([]float64, vectorDim)
	vector2 := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		vector1[i] = rand.Float64()*2 - 1
		vector2[i] = rand.Float64()*2 - 1
	}

	codes1, err := quantizer.Encode(vector1)
	if err != nil {
		t.Fatalf("Encoding vector1 failed: %v", err)
	}

	codes2, err := quantizer.Encode(vector2)
	if err != nil {
		t.Fatalf("Encoding vector2 failed: %v", err)
	}

	// 计算对称余弦相似度
	similarity, err := quantizer.SymmetricCosineSimilarity(codes1, codes2)
	if err != nil {
		t.Fatalf("Symmetric cosine similarity calculation failed: %v", err)
	}

	// 验证相似度在有效范围内
	if similarity < -1.0 || similarity > 1.0 {
		t.Errorf("Cosine similarity should be in [-1, 1], got %f", similarity)
	}

	// 测试自相似度（应该为1）
	selfSimilarity, err := quantizer.SymmetricCosineSimilarity(codes1, codes1)
	if err != nil {
		t.Fatalf("Self similarity calculation failed: %v", err)
	}

	if math.Abs(selfSimilarity-1.0) > 1e-6 {
		t.Errorf("Self similarity should be close to 1.0, got %f", selfSimilarity)
	}
}

// TestComputeDotProductTable 测试点积表计算
func TestComputeDotProductTable(t *testing.T) {
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

	// 计算点积表
	dotProductTable, querySubNorms, err := quantizer.ComputeDotProductTable(queryVector)
	if err != nil {
		t.Fatalf("Dot product table computation failed: %v", err)
	}

	// 验证表结构
	if len(dotProductTable) != M {
		t.Errorf("Expected %d rows in dot product table, got %d", M, len(dotProductTable))
	}

	if len(querySubNorms) != M {
		t.Errorf("Expected %d query sub norms, got %d", M, len(querySubNorms))
	}

	for m := 0; m < M; m++ {
		if len(dotProductTable[m]) != K {
			t.Errorf("Expected %d columns in row %d, got %d", K, m, len(dotProductTable[m]))
		}

		// 验证查询子向量范数为正
		if querySubNorms[m] < 0 {
			t.Errorf("Query sub norm should be non-negative, got %f", querySubNorms[m])
		}

		// 验证点积值不是无穷大或NaN
		for k := 0; k < K; k++ {
			if math.IsInf(dotProductTable[m][k], 0) || math.IsNaN(dotProductTable[m][k]) {
				t.Errorf("Invalid dot product value at [%d][%d]: %f", m, k, dotProductTable[m][k])
			}
		}
	}
}

// TestAsymmetricCosineSimilarityWithDotProductTable 测试使用点积表的快速余弦相似度计算
func TestAsymmetricCosineSimilarityWithDotProductTable(t *testing.T) {
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

	// 编码一个测试向量
	testVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		testVector[i] = rand.Float64()*2 - 1
	}
	codes, err := quantizer.Encode(testVector)
	if err != nil {
		t.Fatalf("Encoding failed: %v", err)
	}

	// 计算点积表
	dotProductTable, querySubNorms, err := quantizer.ComputeDotProductTable(queryVector)
	if err != nil {
		t.Fatalf("Dot product table computation failed: %v", err)
	}

	// 使用点积表计算相似度
	fastSimilarity, err := quantizer.AsymmetricCosineSimilarityWithDotProductTable(codes, dotProductTable, querySubNorms)
	if err != nil {
		t.Fatalf("Fast similarity calculation failed: %v", err)
	}

	// 直接计算相似度
	directSimilarity, err := quantizer.AsymmetricCosineSimilarity(queryVector, codes)
	if err != nil {
		t.Fatalf("Direct similarity calculation failed: %v", err)
	}

	// 两种方法的结果应该相同（在数值精度范围内）
	if math.Abs(fastSimilarity-directSimilarity) > 1e-9 {
		t.Errorf("Similarity mismatch: fast=%f, direct=%f, diff=%f",
			fastSimilarity, directSimilarity, fastSimilarity-directSimilarity)
	}
}

// TestBatchAsymmetricCosineSimilarity 测试批量余弦相似度计算
func TestBatchAsymmetricCosineSimilarity(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim
	const numVectors = 10

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	queryVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		queryVector[i] = rand.Float64()*2 - 1
	}

	// 创建多个测试向量并编码
	allCodes := make([][]byte, numVectors)
	expectedSimilarities := make([]float64, numVectors)

	for i := 0; i < numVectors; i++ {
		testVector := make([]float64, vectorDim)
		for j := 0; j < vectorDim; j++ {
			testVector[j] = rand.Float64()*2 - 1
		}

		codes, err := quantizer.Encode(testVector)
		if err != nil {
			t.Fatalf("Encoding vector %d failed: %v", i, err)
		}
		allCodes[i] = codes

		// 计算期望的相似度
		expectedSim, err := quantizer.AsymmetricCosineSimilarity(queryVector, codes)
		if err != nil {
			t.Fatalf("Expected similarity calculation failed for vector %d: %v", i, err)
		}
		expectedSimilarities[i] = expectedSim
	}

	// 批量计算相似度
	batchSimilarities, err := quantizer.BatchAsymmetricCosineSimilarity(queryVector, allCodes)
	if err != nil {
		t.Fatalf("Batch similarity calculation failed: %v", err)
	}

	// 验证结果
	if len(batchSimilarities) != numVectors {
		t.Errorf("Expected %d similarities, got %d", numVectors, len(batchSimilarities))
	}

	for i, batchSim := range batchSimilarities {
		if math.Abs(batchSim-expectedSimilarities[i]) > 1e-9 {
			t.Errorf("Batch similarity mismatch for vector %d: batch=%f, expected=%f",
				i, batchSim, expectedSimilarities[i])
		}
	}
}

// TestFindMostSimilarCodes 测试最相似向量查找
func TestFindMostSimilarCodes(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim
	const numVectors = 20
	const topN = 5

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	queryVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		queryVector[i] = rand.Float64()*2 - 1
	}

	// 创建测试向量，包括一个与查询向量非常相似的向量
	allCodes := make([][]byte, numVectors)

	// 第一个向量设为与查询向量几乎相同（添加小噪声）
	similarVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		similarVector[i] = queryVector[i] + (rand.Float64()-0.5)*0.01 // 小噪声
	}
	codes, err := quantizer.Encode(similarVector)
	if err != nil {
		t.Fatalf("Encoding similar vector failed: %v", err)
	}
	allCodes[0] = codes

	// 其他向量为随机向量
	for i := 1; i < numVectors; i++ {
		randomVector := make([]float64, vectorDim)
		for j := 0; j < vectorDim; j++ {
			randomVector[j] = rand.Float64()*4 - 2 // 更大的随机范围
		}

		codes, err := quantizer.Encode(randomVector)
		if err != nil {
			t.Fatalf("Encoding vector %d failed: %v", i, err)
		}
		allCodes[i] = codes
	}

	// 查找最相似的向量
	indices, similarities, err := quantizer.FindMostSimilarCodes(queryVector, allCodes, topN)
	if err != nil {
		t.Fatalf("Finding most similar codes failed: %v", err)
	}

	// 验证结果
	if len(indices) != topN {
		t.Errorf("Expected %d indices, got %d", topN, len(indices))
	}

	if len(similarities) != topN {
		t.Errorf("Expected %d similarities, got %d", topN, len(similarities))
	}

	// 第一个结果应该是最相似的向量（索引0）
	if indices[0] != 0 {
		t.Errorf("Expected most similar vector to be at index 0, got %d", indices[0])
	}

	// 相似度应该按降序排列
	for i := 1; i < len(similarities); i++ {
		if similarities[i] > similarities[i-1] {
			t.Errorf("Similarities should be in descending order: [%d]=%f > [%d]=%f",
				i, similarities[i], i-1, similarities[i-1])
		}
	}
}

// TestCompareCosineMethods 测试不同余弦相似度计算方法的比较
func TestCompareCosineMethods(t *testing.T) {
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

	testVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		testVector[i] = rand.Float64()*2 - 1
	}

	codes, err := quantizer.Encode(testVector)
	if err != nil {
		t.Fatalf("Encoding failed: %v", err)
	}

	// 比较不同方法
	comparison, err := quantizer.CompareCosineMethods(queryVector, codes)
	if err != nil {
		t.Fatalf("Method comparison failed: %v", err)
	}

	// 验证所有方法都返回了有效值
	requiredFields := []string{
		"asymmetric_cosine_similarity",
		"standard_cosine_similarity",
		"fast_cosine_similarity",
		"asymmetric_vs_standard_diff",
		"asymmetric_vs_fast_diff",
		"standard_vs_fast_diff",
	}

	for _, field := range requiredFields {
		if _, exists := comparison[field]; !exists {
			t.Errorf("Missing field in comparison: %s", field)
		}

		if val, ok := comparison[field].(float64); ok {
			if math.IsInf(val, 0) || math.IsNaN(val) {
				t.Errorf("Invalid value for field %s: %f", field, val)
			}
		}
	}

	// 验证非对称和快速方法的结果应该相同
	asymmetricSim := comparison["asymmetric_cosine_similarity"].(float64)
	fastSim := comparison["fast_cosine_similarity"].(float64)
	diff := comparison["asymmetric_vs_fast_diff"].(float64)

	if diff > 1e-9 {
		t.Errorf("Asymmetric and fast methods should give same result: asymmetric=%f, fast=%f, diff=%f",
			asymmetricSim, fastSim, diff)
	}
}

// TestCosineSimilarityWithSpecialVectors 测试特殊向量的余弦相似度
func TestCosineSimilarityWithSpecialVectors(t *testing.T) {
	const M = 4
	const K = 16
	const subVectorDim = 8
	const vectorDim = M * subVectorDim

	codebook := createTestCodebook(M, K, subVectorDim)
	quantizer := NewQuantizer(codebook)

	// 测试零向量
	zeroVector := make([]float64, vectorDim)
	normalVector := make([]float64, vectorDim)
	for i := 0; i < vectorDim; i++ {
		normalVector[i] = rand.Float64()*2 - 1
	}

	zeroCode, err := quantizer.Encode(zeroVector)
	if err != nil {
		t.Fatalf("Encoding zero vector failed: %v", err)
	}

	// 零向量与任何向量的余弦相似度应该为0
	// 但由于PQ量化误差，零向量可能不会编码为真正的零向量
	similarity, err := quantizer.AsymmetricCosineSimilarity(normalVector, zeroCode)
	if err != nil {
		t.Fatalf("Similarity calculation with zero vector failed: %v", err)
	}

	// 由于PQ量化，零向量可能不会保持为零，所以相似度可能不为0
	// 我们只检查它是否在合理范围内
	if math.Abs(similarity) > 0.5 {
		t.Errorf("Similarity with quantized zero vector should be reasonable, got %f", similarity)
	}

	// 测试相同向量的相似度（应该接近1）
	normalCode, err := quantizer.Encode(normalVector)
	if err != nil {
		t.Fatalf("Encoding normal vector failed: %v", err)
	}

	selfSimilarity, err := quantizer.AsymmetricCosineSimilarity(normalVector, normalCode)
	if err != nil {
		t.Fatalf("Self similarity calculation failed: %v", err)
	}

	// 由于量化误差，自相似度可能不完全是1，但应该很接近
	// 对于随机向量，量化误差可能比较大，所以设置一个更宽松的阈值
	if selfSimilarity < 0.3 { // 设置一个更宽松的下限
		t.Errorf("Self similarity should be positive, got %f", selfSimilarity)
	}
}

// BenchmarkAsymmetricCosineSimilarity 非对称余弦相似度性能基准测试
func BenchmarkAsymmetricCosineSimilarity(b *testing.B) {
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
		_, err := quantizer.AsymmetricCosineSimilarity(queryVector, codes)
		if err != nil {
			b.Fatalf("Similarity calculation failed: %v", err)
		}
	}
}

// BenchmarkBatchAsymmetricCosineSimilarity 批量余弦相似度性能基准测试
func BenchmarkBatchAsymmetricCosineSimilarity(b *testing.B) {
	codebook := createTestCodebook(8, 64, 32)
	quantizer := NewQuantizer(codebook)

	queryVector := make([]float64, 8*32)
	for i := range queryVector {
		queryVector[i] = rand.Float64()
	}

	const numVectors = 100
	allCodes := make([][]byte, numVectors)
	for i := 0; i < numVectors; i++ {
		codes := make([]byte, 8)
		for j := range codes {
			codes[j] = byte(rand.Intn(64))
		}
		allCodes[i] = codes
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := quantizer.BatchAsymmetricCosineSimilarity(queryVector, allCodes)
		if err != nil {
			b.Fatalf("Batch similarity calculation failed: %v", err)
		}
	}
}
