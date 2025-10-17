package main

import (
	"math/rand"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag/pq"
	"github.com/yaklang/yaklang/common/log"
)

func main() {
	log.Infof("Starting PQ algorithm demonstration...")

	// 设置随机种子以获得可重现的结果
	rand.Seed(42)

	// 配置参数
	const (
		vectorDim       = 1024  // 向量维度
		numTrainVectors = 10000 // 训练向量数量
		numTestVectors  = 100   // 测试向量数量
		M               = 16    // 子向量数量
		K               = 256   // 每个子空间的聚类中心数量
	)

	log.Infof("Configuration: vectorDim=%d, trainVectors=%d, testVectors=%d, M=%d, K=%d",
		vectorDim, numTrainVectors, numTestVectors, M, K)

	// 1. 生成训练数据
	log.Infof("Generating training data...")
	trainingVectors := generateRandomVectors(numTrainVectors, vectorDim)

	// 2. 创建训练数据通道
	trainingChannel := make(chan []float64, numTrainVectors)
	go func() {
		defer close(trainingChannel)
		for _, vector := range trainingVectors {
			trainingChannel <- vector
		}
	}()

	// 3. 训练PQ模型 (并行训练演示)
	log.Infof("Training PQ model with parallel K-Means...")
	startTime := time.Now()

	codebook, err := pq.Train(trainingChannel,
		pq.WithM(M),
		pq.WithK(K),
		pq.WithMaxIters(50),
		pq.WithTolerance(1e-6),
		pq.WithRandomSeed(42),
		pq.WithConcurrentKMeans(true), // 启用并行训练
		pq.WithMaxWorkers(4),          // 使用4个worker
		pq.WithParallelDistanceCalc(true)) // 启用并行距离计算

	if err != nil {
		log.Errorf("Training failed: %v", err)
		return
	}

	trainingTime := time.Since(startTime)
	log.Infof("Parallel training completed in %v", trainingTime)

	// 4. 创建量化器
	quantizer := pq.NewQuantizer(codebook)
	if quantizer == nil {
		log.Errorf("Failed to create quantizer")
		return
	}

	// 5. 显示码本信息
	info := quantizer.GetCodebookInfo()
	log.Infof("Codebook Information:")
	for key, value := range info {
		log.Infof("  %s: %v", key, value)
	}

	// 6. 生成测试数据并进行编码/解码测试
	log.Infof("Generating test data and performing encoding/decoding tests...")
	testVectors := generateRandomVectors(numTestVectors, vectorDim)

	var totalQuantizationError float64
	var encodingTimes []time.Duration
	var decodingTimes []time.Duration

	for i, testVector := range testVectors {
		// 编码测试
		encodeStart := time.Now()
		codes, err := quantizer.Encode(testVector)
		encodingTime := time.Since(encodeStart)
		encodingTimes = append(encodingTimes, encodingTime)

		if err != nil {
			log.Errorf("Encoding failed for vector %d: %v", i, err)
			continue
		}

		// 解码测试
		decodeStart := time.Now()
		decodedVector, err := quantizer.Decode(codes)
		decodingTime := time.Since(decodeStart)
		decodingTimes = append(decodingTimes, decodingTime)

		if err != nil {
			log.Errorf("Decoding failed for vector %d: %v", i, err)
			continue
		}

		// 计算量化误差
		quantError, err := quantizer.EstimateQuantizationError(testVector)
		if err != nil {
			log.Errorf("Error estimation failed for vector %d: %v", i, err)
			continue
		}

		totalQuantizationError += quantError

		// 测试非对称距离计算
		asymDist, err := quantizer.AsymmetricDistance(testVector, codes)
		if err != nil {
			log.Errorf("Asymmetric distance calculation failed for vector %d: %v", i, err)
			continue
		}

		// 验证解码向量的维度
		if len(decodedVector) != vectorDim {
			log.Errorf("Decoded vector dimension mismatch for vector %d: got %d, expected %d",
				i, len(decodedVector), vectorDim)
			continue
		}

		if i == 0 {
			// 为第一个向量显示详细信息
			log.Infof("Test Vector 1 Details:")
			log.Infof("  Original vector length: %d", len(testVector))
			log.Infof("  Encoded codes length: %d bytes", len(codes))
			log.Infof("  Decoded vector length: %d", len(decodedVector))
			log.Infof("  Quantization error: %.6f", quantError)
			log.Infof("  Asymmetric distance: %.6f", asymDist)
			log.Infof("  Encoding time: %v", encodingTime)
			log.Infof("  Decoding time: %v", decodingTime)
		}
	}

	// 7. 计算和显示统计信息
	avgQuantizationError := totalQuantizationError / float64(numTestVectors)
	compressionRatio := quantizer.GetCompressionRatio()

	// 计算平均编码/解码时间
	var totalEncodingTime, totalDecodingTime time.Duration
	for _, t := range encodingTimes {
		totalEncodingTime += t
	}
	for _, t := range decodingTimes {
		totalDecodingTime += t
	}
	avgEncodingTime := totalEncodingTime / time.Duration(len(encodingTimes))
	avgDecodingTime := totalDecodingTime / time.Duration(len(decodingTimes))

	log.Infof("\n=== PQ Algorithm Performance Summary ===")
	log.Infof("Training time: %v", trainingTime)
	log.Infof("Average quantization error: %.6f", avgQuantizationError)
	log.Infof("Compression ratio: %.2fx", compressionRatio)
	log.Infof("Average encoding time per vector: %v", avgEncodingTime)
	log.Infof("Average decoding time per vector: %v", avgDecodingTime)

	originalSize := float64(vectorDim * 8) // 8 bytes per float64
	compressedSize := float64(M * 1)       // 1 byte per code
	log.Infof("Memory usage: %.2f KB -> %.2f KB (%.2f%% reduction)",
		originalSize/1024, compressedSize/1024,
		(originalSize-compressedSize)/originalSize*100)

	// 8. 批量编码测试
	log.Infof("\n=== Batch Encoding Test ===")
	batchStart := time.Now()
	allCodes, err := quantizer.BatchEncode(testVectors)
	batchTime := time.Since(batchStart)

	if err != nil {
		log.Errorf("Batch encoding failed: %v", err)
	} else {
		log.Infof("Batch encoded %d vectors in %v", len(allCodes), batchTime)
		log.Infof("Average time per vector in batch: %v", batchTime/time.Duration(len(testVectors)))
	}

	// 9. 距离表测试
	log.Infof("\n=== Distance Table Test ===")
	if len(testVectors) > 0 {
		queryVector := testVectors[0]

		tableStart := time.Now()
		distanceTable, err := quantizer.ComputeDistanceTable(queryVector)
		tableTime := time.Since(tableStart)

		if err != nil {
			log.Errorf("Distance table computation failed: %v", err)
		} else {
			log.Infof("Distance table computed in %v", tableTime)
			log.Infof("Distance table size: %dx%d", len(distanceTable), len(distanceTable[0]))

			// 测试使用距离表的快速距离计算
			if len(allCodes) > 1 {
				codes := allCodes[1]
				fastDist, err := quantizer.AsymmetricDistanceWithTable(codes, distanceTable)
				if err != nil {
					log.Errorf("Fast distance calculation failed: %v", err)
				} else {
					// 比较与直接计算的结果
					directDist, err := quantizer.AsymmetricDistance(queryVector, codes)
					if err != nil {
						log.Errorf("Direct distance calculation failed: %v", err)
					} else {
						log.Infof("Distance calculation results: fast=%.6f, direct=%.6f, diff=%.8f",
							fastDist, directDist, fastDist-directDist)
					}
				}
			}
		}
	}

	log.Infof("PQ algorithm demonstration completed successfully!")

	// 10. 余弦相似度演示
	log.Infof("\n=== Cosine Similarity Demonstration ===")
	if err := pq.DemonstrateCosineSimiarity(); err != nil {
		log.Errorf("Cosine similarity demonstration failed: %v", err)
	}
}

// generateRandomVectors 生成指定数量和维度的随机向量
func generateRandomVectors(count, dimension int) [][]float64 {
	vectors := make([][]float64, count)
	for i := 0; i < count; i++ {
		vector := make([]float64, dimension)
		for j := 0; j < dimension; j++ {
			vector[j] = rand.Float64()*2 - 1 // 生成 [-1, 1] 范围内的随机数
		}
		vectors[i] = vector
	}
	return vectors
}
