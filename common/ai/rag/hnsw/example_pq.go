package hnsw

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag/pq"
	"github.com/yaklang/yaklang/common/log"
)

// ExampleWithPQOptimization 展示HNSW+PQ优化的使用方法
func ExampleWithPQOptimization() {
	log.Info("Starting HNSW with PQ optimization example...")

	// 第一步：生成训练数据并训练PQ码表
	log.Info("Generating training data for PQ codebook...")
	numTrainingVectors := 5000
	dimension := 256
	trainingVectors := generateRandomVectors(numTrainingVectors, dimension, 12345)

	// 将训练向量转换为PQ训练所需的通道
	vectorChan := make(chan []float64, numTrainingVectors)
	go func() {
		defer close(vectorChan)
		for _, vec := range trainingVectors {
			vec64 := make([]float64, len(vec))
			for i, v := range vec {
				vec64[i] = float64(v)
			}
			vectorChan <- vec64
		}
	}()

	// 训练PQ码表
	log.Info("Training PQ codebook...")
	startTime := time.Now()
	codebook, err := pq.Train(vectorChan,
		pq.WithM(32),                  // 32个子向量，每个子向量8维
		pq.WithK(256),                 // 每个子空间256个聚类中心
		pq.WithMaxIters(100),          // 最大100次迭代
		pq.WithTolerance(1e-6),        // 收敛阈值
		pq.WithRandomSeed(42),         // 固定随机种子保证可重现
		pq.WithConcurrentKMeans(true), // 启用并行K-means
	)
	if err != nil {
		log.Errorf("Failed to train PQ codebook: %v", err)
		return
	}
	trainTime := time.Since(startTime)
	log.Infof("PQ codebook trained in %v: M=%d, K=%d, SubVectorDim=%d",
		trainTime, codebook.M, codebook.K, codebook.SubVectorDim)

	// 第二步：创建启用PQ优化的HNSW图
	log.Info("Creating HNSW graph with PQ optimization...")
	graph := NewGraph[string](
		WithM[string](32),                // 每个节点最多32个邻居
		WithMl[string](0.25),             // 层级生成因子
		WithEfSearch[string](64),         // 搜索时考虑64个候选
		WithDeterministicRng[string](42), // 确定性随机数生成器
		WithCosineDistance[string](),     // 使用余弦距离
		WithPQCodebook[string](codebook), // 启用PQ优化！！！
	)

	// 验证PQ优化已启用
	if !graph.IsPQEnabled() {
		log.Error("PQ optimization should be enabled")
		return
	}
	log.Info("PQ optimization successfully enabled")

	// 第三步：向图中添加数据向量
	log.Info("Adding vectors to HNSW graph...")
	numDataVectors := 2000
	dataVectors := generateRandomVectors(numDataVectors, dimension, 54321)

	startTime = time.Now()
	for i, vec := range dataVectors {
		nodeKey := fmt.Sprintf("doc_%d", i)
		node := MakeInputNode(nodeKey, vec)
		graph.Add(node)

		if (i+1)%500 == 0 {
			log.Infof("Added %d vectors to graph", i+1)
		}
	}
	indexTime := time.Since(startTime)
	log.Infof("Graph built with %d nodes in %v", graph.Len(), indexTime)

	// 第四步：测试搜索性能
	log.Info("Testing search performance...")
	queryVector := generateRandomVectors(1, dimension, 98765)[0]

	// 执行多次搜索测试
	numQueries := 100
	k := 10

	startTime = time.Now()
	for i := 0; i < numQueries; i++ {
		results := graph.SearchWithDistance(queryVector, k)
		if i == 0 {
			// 打印第一次搜索的结果
			log.Infof("Top %d search results:", len(results))
			for j, result := range results {
				pqCodes, ok := graph.GetPQCodes(result.Key)
				if ok {
					log.Infof("  %d: Key=%s, Distance=%.6f, PQ_size=%d bytes",
						j+1, result.Key, result.Distance, len(pqCodes))
				} else {
					log.Infof("  %d: Key=%s, Distance=%.6f, No PQ codes",
						j+1, result.Key, result.Distance)
				}
			}
		}
	}
	searchTime := time.Since(startTime)
	avgSearchTime := searchTime / time.Duration(numQueries)
	log.Infof("Search performance: %d queries in %v, average %.2fms per query",
		numQueries, searchTime, float64(avgSearchTime.Nanoseconds())/1e6)

	// 第五步：展示存储优化效果
	log.Info("Analyzing storage optimization...")
	originalSize := dimension * 4 // float32 = 4 bytes per element

	// 计算平均PQ编码大小
	totalPQSize := 0
	sampleSize := 100
	validSamples := 0
	for i := 0; i < sampleSize && i < numDataVectors; i++ {
		nodeKey := fmt.Sprintf("doc_%d", i)
		pqCodes, ok := graph.GetPQCodes(nodeKey)
		if ok {
			totalPQSize += len(pqCodes)
			validSamples++
		}
	}

	if validSamples > 0 {
		avgPQSize := totalPQSize / validSamples
		compressionRatio := float64(originalSize) / float64(avgPQSize)

		log.Infof("Storage optimization results:")
		log.Infof("  Original vector size: %d bytes", originalSize)
		log.Infof("  Average PQ encoding size: %d bytes", avgPQSize)
		log.Infof("  Compression ratio: %.2fx", compressionRatio)
		log.Infof("  Space saved: %.1f%%", (1.0-float64(avgPQSize)/float64(originalSize))*100)
	}

	// 第六步：测试过滤搜索
	log.Info("Testing filtered search...")
	filter := func(key string, vector Vector) bool {
		// 只返回键名包含偶数的节点
		if len(key) < 5 {
			return false
		}
		// 提取数字部分并检查是否为偶数
		for i := 4; i < len(key); i++ {
			if key[i] >= '0' && key[i] <= '9' {
				return (key[i]-'0')%2 == 0
			}
		}
		return false
	}

	filteredResults := graph.SearchWithDistanceAndFilter(queryVector, k, filter)
	log.Infof("Filtered search results (%d results):", len(filteredResults))
	for i, result := range filteredResults {
		log.Infof("  %d: Key=%s, Distance=%.6f", i+1, result.Key, result.Distance)
	}

	// 第七步：对比非PQ版本
	log.Info("Creating standard HNSW graph for comparison...")
	standardGraph := NewGraph[string](
		WithM[string](32),
		WithMl[string](0.25),
		WithEfSearch[string](64),
		WithDeterministicRng[string](42),
		WithCosineDistance[string](),
		// 注意：没有WithPQCodebook，所以是标准HNSW
	)

	// 验证标准图没有PQ优化
	if standardGraph.IsPQEnabled() {
		log.Error("Standard graph should not have PQ optimization")
		return
	}

	// 添加少量数据到标准图进行对比
	testVectors := dataVectors[:100] // 只取前100个
	for i, vec := range testVectors {
		nodeKey := fmt.Sprintf("std_doc_%d", i)
		node := MakeInputNode(nodeKey, vec)
		standardGraph.Add(node)
	}

	// 比较搜索结果
	pqResults := graph.Search(queryVector, 5)
	stdResults := standardGraph.Search(queryVector, 5)

	log.Infof("Search result comparison:")
	log.Infof("  PQ-optimized graph: %d results", len(pqResults))
	log.Infof("  Standard graph: %d results", len(stdResults))

	log.Info("HNSW with PQ optimization example completed successfully!")
}

// generateRandomVectors 生成随机向量用于测试
func generateRandomVectors(numVectors, dimension int, seed int64) [][]float32 {
	rng := rand.New(rand.NewSource(seed))
	vectors := make([][]float32, numVectors)

	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dimension)
		for j := 0; j < dimension; j++ {
			vec[j] = rng.Float32()*2 - 1 // 生成[-1, 1]范围的随机数
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

// RunPQExample 运行PQ优化示例
func RunPQExample() {
	ExampleWithPQOptimization()
}
