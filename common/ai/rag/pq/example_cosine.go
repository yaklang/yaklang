package pq

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// DemonstrateCosineSimiarity 演示PQ算法在余弦相似度计算中的应用
func DemonstrateCosineSimiarity() error {
	log.Infof("=== PQ算法余弦相似度计算演示 ===")

	// 配置参数
	const (
		vectorDim       = 512  // 向量维度（模拟词嵌入）
		numTrainVectors = 5000 // 训练向量数量
		numTestVectors  = 20   // 测试向量数量
		M               = 8    // 子向量数量
		K               = 256  // 每个子空间的聚类中心数量
	)

	log.Infof("配置: 向量维度=%d, 训练向量=%d, 测试向量=%d, M=%d, K=%d",
		vectorDim, numTrainVectors, numTestVectors, M, K)

	// 1. 生成模拟的向量数据（模拟词嵌入或文档向量）
	log.Infof("生成训练数据...")
	trainingData := make(chan []float64, numTrainVectors)
	go func() {
		defer close(trainingData)
		rand.Seed(42) // 固定种子以获得可重现结果

		for i := 0; i < numTrainVectors; i++ {
			vector := generateNormalizedVector(vectorDim)
			trainingData <- vector
		}
	}()

	// 2. 训练PQ模型
	log.Infof("训练PQ模型...")
	startTime := time.Now()

	codebook, err := Train(trainingData,
		WithM(M),
		WithK(K),
		WithMaxIters(30),
		WithTolerance(1e-6),
		WithRandomSeed(42))

	if err != nil {
		return fmt.Errorf("训练失败: %v", err)
	}

	trainingTime := time.Since(startTime)
	log.Infof("训练完成，耗时: %v", trainingTime)

	// 3. 创建量化器
	quantizer := NewQuantizer(codebook)
	if quantizer == nil {
		return fmt.Errorf("创建量化器失败")
	}

	// 显示压缩信息
	info := quantizer.GetCodebookInfo()
	log.Infof("压缩比: %.2fx", info["CompressionRatio"])
	log.Infof("内存节省: %.2f%%", (1.0-1.0/info["CompressionRatio"].(float64))*100)

	// 4. 生成测试数据
	log.Infof("生成测试向量...")
	testVectors := make([][]float64, numTestVectors)
	for i := 0; i < numTestVectors; i++ {
		testVectors[i] = generateNormalizedVector(vectorDim)
	}

	// 编码测试向量
	log.Infof("编码测试向量...")
	testCodes := make([][]byte, numTestVectors)
	for i, vector := range testVectors {
		codes, err := quantizer.Encode(vector)
		if err != nil {
			return fmt.Errorf("编码向量 %d 失败: %v", i, err)
		}
		testCodes[i] = codes
	}

	// 5. 演示不同的余弦相似度计算方法
	queryVector := generateNormalizedVector(vectorDim)
	log.Infof("使用查询向量演示不同的余弦相似度计算方法...")

	// 方法1: 标准余弦相似度（解码后计算）
	log.Infof("--- 方法1: 标准余弦相似度 ---")
	standardSimilarities := make([]float64, numTestVectors)
	standardStart := time.Now()

	for i, codes := range testCodes {
		decodedVector, err := quantizer.Decode(codes)
		if err != nil {
			return fmt.Errorf("解码失败: %v", err)
		}

		similarity, err := cosineSimilarity(queryVector, decodedVector)
		if err != nil {
			return fmt.Errorf("余弦相似度计算失败: %v", err)
		}
		standardSimilarities[i] = similarity
	}
	standardTime := time.Since(standardStart)
	log.Infof("标准方法耗时: %v", standardTime)

	// 方法2: 非对称余弦相似度（PQ优化）
	log.Infof("--- 方法2: 非对称余弦相似度 ---")
	asymmetricSimilarities := make([]float64, numTestVectors)
	asymmetricStart := time.Now()

	for i, codes := range testCodes {
		similarity, err := quantizer.AsymmetricCosineSimilarity(queryVector, codes)
		if err != nil {
			return fmt.Errorf("非对称余弦相似度计算失败: %v", err)
		}
		asymmetricSimilarities[i] = similarity
	}
	asymmetricTime := time.Since(asymmetricStart)
	log.Infof("非对称方法耗时: %v", asymmetricTime)

	// 方法3: 批量非对称余弦相似度（最优化）
	log.Infof("--- 方法3: 批量非对称余弦相似度 ---")
	batchStart := time.Now()
	batchSimilarities, err := quantizer.BatchAsymmetricCosineSimilarity(queryVector, testCodes)
	if err != nil {
		return fmt.Errorf("批量余弦相似度计算失败: %v", err)
	}
	batchTime := time.Since(batchStart)
	log.Infof("批量方法耗时: %v", batchTime)

	// 6. 比较不同方法的结果和性能
	log.Infof("=== 性能比较 ===")
	log.Infof("标准方法: %v (每个向量 %.2fμs)", standardTime, float64(standardTime.Nanoseconds())/float64(numTestVectors)/1000.0)
	log.Infof("非对称方法: %v (每个向量 %.2fμs)", asymmetricTime, float64(asymmetricTime.Nanoseconds())/float64(numTestVectors)/1000.0)
	log.Infof("批量方法: %v (每个向量 %.2fμs)", batchTime, float64(batchTime.Nanoseconds())/float64(numTestVectors)/1000.0)

	speedupAsymmetric := float64(standardTime) / float64(asymmetricTime)
	speedupBatch := float64(standardTime) / float64(batchTime)
	log.Infof("非对称方法加速比: %.2fx", speedupAsymmetric)
	log.Infof("批量方法加速比: %.2fx", speedupBatch)

	// 7. 精度分析
	log.Infof("=== 精度分析 ===")
	var totalDiffStandardAsymmetric, totalDiffStandardBatch float64
	maxDiffStandardAsymmetric, maxDiffStandardBatch := 0.0, 0.0

	for i := 0; i < numTestVectors; i++ {
		diffSA := abs(standardSimilarities[i] - asymmetricSimilarities[i])
		diffSB := abs(standardSimilarities[i] - batchSimilarities[i])

		totalDiffStandardAsymmetric += diffSA
		totalDiffStandardBatch += diffSB

		if diffSA > maxDiffStandardAsymmetric {
			maxDiffStandardAsymmetric = diffSA
		}
		if diffSB > maxDiffStandardBatch {
			maxDiffStandardBatch = diffSB
		}
	}

	avgDiffSA := totalDiffStandardAsymmetric / float64(numTestVectors)
	avgDiffSB := totalDiffStandardBatch / float64(numTestVectors)

	log.Infof("标准vs非对称 - 平均差异: %.6f, 最大差异: %.6f", avgDiffSA, maxDiffStandardAsymmetric)
	log.Infof("标准vs批量 - 平均差异: %.6f, 最大差异: %.6f", avgDiffSB, maxDiffStandardBatch)

	// 8. 相似性搜索演示
	log.Infof("=== 相似性搜索演示 ===")
	topK := 5
	log.Infof("查找与查询向量最相似的 %d 个向量...", topK)

	searchStart := time.Now()
	indices, similarities, err := quantizer.FindMostSimilarCodes(queryVector, testCodes, topK)
	if err != nil {
		return fmt.Errorf("相似性搜索失败: %v", err)
	}
	searchTime := time.Since(searchStart)

	log.Infof("搜索耗时: %v", searchTime)
	log.Infof("最相似的 %d 个向量:", topK)
	for i, idx := range indices {
		log.Infof("  排名%d: 向量%d, 相似度=%.6f", i+1, idx, similarities[i])
	}

	// 9. 对称相似度演示（两个PQ码之间）
	log.Infof("=== 对称相似度演示 ===")
	if len(testCodes) >= 2 {
		symSim, err := quantizer.SymmetricCosineSimilarity(testCodes[0], testCodes[1])
		if err != nil {
			return fmt.Errorf("对称相似度计算失败: %v", err)
		}
		log.Infof("向量0和向量1之间的对称相似度: %.6f", symSim)

		// 与解码后的标准计算比较
		decoded0, _ := quantizer.Decode(testCodes[0])
		decoded1, _ := quantizer.Decode(testCodes[1])
		standardSymSim, _ := cosineSimilarity(decoded0, decoded1)
		log.Infof("解码后的标准相似度: %.6f", standardSymSim)
		log.Infof("两种方法的差异: %.6f", abs(symSim-standardSymSim))
	}

	log.Infof("=== 演示完成 ===")
	return nil
}

// generateNormalizedVector 生成归一化的随机向量（模拟真实的词向量）
func generateNormalizedVector(dim int) []float64 {
	vector := make([]float64, dim)
	var norm float64

	// 生成随机向量
	for i := 0; i < dim; i++ {
		vector[i] = rand.NormFloat64() // 标准正态分布
		norm += vector[i] * vector[i]
	}

	// 归一化
	norm = 1.0 / (math.Sqrt(norm) + 1e-12) // 避免除零
	for i := 0; i < dim; i++ {
		vector[i] *= norm
	}

	return vector
}

// abs 计算绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
