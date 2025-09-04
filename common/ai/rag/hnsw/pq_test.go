package hnsw

import (
	"math"
	"math/rand"
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/ai/rag/pq"
)

func TestHNSWWithPQCodebook(t *testing.T) {
	// 生成测试数据
	numVectors := 1000
	dimension := 128
	testVectors := generateTestVectors(numVectors, dimension, 42)

	// 训练PQ码表
	t.Log("Training PQ codebook...")
	vectorChan := make(chan []float64, numVectors)
	go func() {
		defer close(vectorChan)
		for _, vec := range testVectors {
			vec64 := make([]float64, len(vec))
			for i, v := range vec {
				vec64[i] = float64(v)
			}
			vectorChan <- vec64
		}
	}()

	codebook, err := pq.Train(vectorChan,
		pq.WithM(16),          // 16个子向量
		pq.WithK(256),         // 256个聚类中心
		pq.WithMaxIters(50),   // 最大50次迭代
		pq.WithRandomSeed(42), // 固定随机种子
	)
	if err != nil {
		t.Fatalf("Failed to train PQ codebook: %v", err)
	}
	t.Logf("PQ codebook trained: M=%d, K=%d, SubVectorDim=%d",
		codebook.M, codebook.K, codebook.SubVectorDim)

	// 创建启用PQ优化的HNSW图
	t.Log("Creating HNSW graph with PQ optimization...")
	graph := NewGraph[int](
		WithM[int](16),
		WithMl[int](0.25),
		WithEfSearch[int](20),
		WithDeterministicRng[int](42),
		WithCosineDistance[int](),
		WithPQCodebook[int](codebook), // 启用PQ优化
	)

	// 验证PQ优化已启用
	if !graph.IsPQEnabled() {
		t.Fatal("Expected PQ optimization to be enabled")
	}

	retrievedCodebook := graph.GetCodebook()
	if retrievedCodebook == nil {
		t.Fatal("Expected to get codebook from graph")
	}
	if retrievedCodebook.M != codebook.M || retrievedCodebook.K != codebook.K {
		t.Error("Retrieved codebook parameters don't match original")
	}

	// 向图中添加向量
	t.Log("Adding vectors to graph...")
	for i := 0; i < 100; i++ { // 只添加前100个向量用于测试
		node := MakeInputNode(i, testVectors[i])
		graph.Add(node)
	}

	t.Logf("Graph built with %d nodes", graph.Len())

	// 测试搜索功能
	t.Log("Testing search functionality...")
	queryVector := testVectors[0] // 使用第一个向量作为查询
	results := graph.SearchWithDistance(queryVector, 5)

	if len(results) == 0 {
		t.Fatal("No search results returned")
	}

	t.Logf("Search results for query vector 0:")
	for i, result := range results {
		t.Logf("  %d: Key=%d, Distance=%.6f", i+1, result.Key, result.Distance)

		// 验证可以获取PQ编码
		pqCodes, ok := graph.GetPQCodes(result.Key)
		if ok {
			t.Logf("    PQ codes length: %d bytes", len(pqCodes))
		} else {
			t.Logf("    No PQ codes available for key %d", result.Key)
		}
	}

	// 检查查询向量0是否在结果中
	foundQueryVector := false
	var queryDistance float64
	for _, result := range results {
		if result.Key == 0 {
			foundQueryVector = true
			queryDistance = result.Distance
			break
		}
	}

	if !foundQueryVector {
		t.Error("Query vector (key=0) not found in search results")
	} else {
		t.Logf("Query vector found with distance: %.6f", queryDistance)
		// 由于PQ量化的精度损失，距离不会完全为0，但应该相对较小
		if queryDistance > 0.5 {
			t.Errorf("Distance to self is unexpectedly large: %.6f", queryDistance)
		}
	}
}

func TestHNSWWithoutPQCodebook(t *testing.T) {
	// 创建不启用PQ优化的标准HNSW图
	graph := NewGraph[int](
		WithM[int](16),
		WithMl[int](0.25),
		WithEfSearch[int](20),
		WithDeterministicRng[int](42),
		WithCosineDistance[int](),
	)

	// 验证PQ优化未启用
	if graph.IsPQEnabled() {
		t.Error("Expected PQ optimization to be disabled")
	}

	if graph.GetCodebook() != nil {
		t.Error("Expected no codebook for standard graph")
	}

	// 添加一些向量
	testVectors := generateTestVectors(10, 128, 42)
	for i, vec := range testVectors {
		node := MakeInputNode(i, vec)
		graph.Add(node)
	}

	// 验证PQ编码不可用
	pqCodes, ok := graph.GetPQCodes(0)
	if ok {
		t.Errorf("Expected no PQ codes for standard graph, got %d bytes", len(pqCodes))
	}

	// 基本搜索仍然应该工作
	results := graph.Search(testVectors[0], 3)
	if len(results) == 0 {
		t.Error("Expected search results even without PQ optimization")
	}
}

func TestPQNodeCreation(t *testing.T) {
	// 测试PQ节点创建
	dimension := 128
	testVector := generateTestVectors(1, dimension, 42)[0]
	vector := func() []float32 { return testVector }

	// 创建一个简单的码表用于测试
	vectorChan := make(chan []float64, 100)
	go func() {
		defer close(vectorChan)
		for i := 0; i < 100; i++ {
			vec := generateTestVectors(1, dimension, int64(i))[0]
			vec64 := make([]float64, len(vec))
			for j, v := range vec {
				vec64[j] = float64(v)
			}
			vectorChan <- vec64
		}
	}()

	codebook, err := pq.Train(vectorChan,
		pq.WithM(8),
		pq.WithK(16),
		pq.WithRandomSeed(42),
	)
	if err != nil {
		t.Fatalf("Failed to train codebook: %v", err)
	}

	quantizer := pq.NewQuantizer(codebook)

	// 创建PQ节点
	pqNode, err := hnswspec.NewPQLayerNode[int](1, vector, quantizer)
	if err != nil {
		t.Fatalf("Failed to create PQ node: %v", err)
	}

	// 验证PQ节点属性
	if pqNode.GetKey() != 1 {
		t.Errorf("Expected key 1, got %d", pqNode.GetKey())
	}

	if !pqNode.IsPQEnabled() {
		t.Error("Expected PQ to be enabled for PQ node")
	}

	pqCodes, ok := pqNode.GetPQCodes()
	if !ok {
		t.Error("Expected to get PQ codes from PQ node")
	}

	if len(pqCodes) != codebook.M {
		t.Errorf("Expected PQ codes length %d, got %d", codebook.M, len(pqCodes))
	}

	// 注意：PQ节点不存储原始向量，所以GetVector()会panic
	// 这是预期的行为，因为PQ节点只存储编码数据

	// 创建标准节点进行比较
	standardNode := hnswspec.NewStandardLayerNode[int](2, vector)

	if standardNode.IsPQEnabled() {
		t.Error("Expected PQ to be disabled for standard node")
	}

	_, ok = standardNode.GetPQCodes()
	if ok {
		t.Error("Expected no PQ codes from standard node")
	}
}

// generateTestVectors 生成测试向量
func generateTestVectors(numVectors, dimension int, seed int64) [][]float32 {
	rng := rand.New(rand.NewSource(seed))
	vectors := make([][]float32, numVectors)

	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dimension)
		for j := 0; j < dimension; j++ {
			vec[j] = rng.Float32()
		}
		// 归一化向量（用于余弦距离）
		norm := float32(0)
		for _, v := range vec {
			norm += v * v
		}
		if norm > 0 {
			norm = float32(1.0) / float32(math.Sqrt(float64(norm)))
			for j := range vec {
				vec[j] *= norm
			}
		}
		vectors[i] = vec
	}

	return vectors
}
