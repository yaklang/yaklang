package pq

import (
	"fmt"
	"math"
)

/*
PQ环境下的余弦相似度计算方法:

1. 近似余弦相似度 (Approximate Cosine Similarity)
   - 直接解码PQ码，然后计算余弦相似度
   - 简单但需要解码开销

2. 非对称余弦相似度 (Asymmetric Cosine Similarity)
   - 查询向量保持原始精度，数据库向量使用PQ码
   - 高精度查询，适合搜索场景

3. 对称余弦相似度 (Symmetric Cosine Similarity)
   - 两个PQ码之间的余弦相似度估计
   - 快速但精度较低

4. 预计算余弦表 (Precomputed Cosine Table)
   - 预计算查询向量与所有质心的余弦值
   - 最快的批量计算方法
*/

// AsymmetricCosineSimilarity 计算查询向量与PQ码之间的非对称余弦相似度
// 这是最常用的方法，查询向量保持高精度，数据库向量使用PQ近似
func (q *Quantizer) AsymmetricCosineSimilarity(queryVector []float64, codes []byte) (float64, error) {
	if q.codebook == nil {
		return 0, fmt.Errorf("quantizer not initialized with codebook")
	}

	expectedDim := q.codebook.M * q.codebook.SubVectorDim
	if len(queryVector) != expectedDim {
		return 0, fmt.Errorf("query vector dimension %d does not match expected dimension %d", len(queryVector), expectedDim)
	}

	if len(codes) != q.codebook.M {
		return 0, fmt.Errorf("code length %d does not match expected M %d", len(codes), q.codebook.M)
	}

	var dotProduct, queryNormSq, dataBaseNormSq float64

	// 计算每个子向量的贡献
	for m := 0; m < q.codebook.M; m++ {
		start := m * q.codebook.SubVectorDim
		end := start + q.codebook.SubVectorDim
		querySubVector := queryVector[start:end]

		centroidIndex := int(codes[m])
		if centroidIndex < 0 || centroidIndex >= q.codebook.K {
			return 0, fmt.Errorf("invalid centroid index %d for sub-vector %d", centroidIndex, m)
		}

		centroid := q.codebook.Centroids[m][centroidIndex]

		// 计算子向量的点积和范数贡献
		for i := 0; i < q.codebook.SubVectorDim; i++ {
			dotProduct += querySubVector[i] * centroid[i]
			queryNormSq += querySubVector[i] * querySubVector[i]
			dataBaseNormSq += centroid[i] * centroid[i]
		}
	}

	// 计算余弦相似度
	queryNorm := math.Sqrt(queryNormSq)
	dataBaseNorm := math.Sqrt(dataBaseNormSq)

	if queryNorm == 0 || dataBaseNorm == 0 {
		return 0, nil // 零向量的余弦相似度定义为0
	}

	return dotProduct / (queryNorm * dataBaseNorm), nil
}

// SymmetricCosineSimilarity 计算两个PQ码之间的对称余弦相似度
// 用于快速比较两个压缩向量，但精度较低
func (q *Quantizer) SymmetricCosineSimilarity(codes1, codes2 []byte) (float64, error) {
	if q.codebook == nil {
		return 0, fmt.Errorf("quantizer not initialized with codebook")
	}

	if len(codes1) != q.codebook.M || len(codes2) != q.codebook.M {
		return 0, fmt.Errorf("codes length must be %d, got %d and %d",
			q.codebook.M, len(codes1), len(codes2))
	}

	var dotProduct, norm1Sq, norm2Sq float64

	// 计算每个子向量对应质心之间的点积和范数
	for m := 0; m < q.codebook.M; m++ {
		centroidIndex1 := int(codes1[m])
		centroidIndex2 := int(codes2[m])

		if centroidIndex1 < 0 || centroidIndex1 >= q.codebook.K ||
			centroidIndex2 < 0 || centroidIndex2 >= q.codebook.K {
			return 0, fmt.Errorf("invalid centroid indices: %d, %d", centroidIndex1, centroidIndex2)
		}

		centroid1 := q.codebook.Centroids[m][centroidIndex1]
		centroid2 := q.codebook.Centroids[m][centroidIndex2]

		// 计算子向量的点积和范数贡献
		for i := 0; i < q.codebook.SubVectorDim; i++ {
			dotProduct += centroid1[i] * centroid2[i]
			norm1Sq += centroid1[i] * centroid1[i]
			norm2Sq += centroid2[i] * centroid2[i]
		}
	}

	// 计算余弦相似度
	norm1 := math.Sqrt(norm1Sq)
	norm2 := math.Sqrt(norm2Sq)

	if norm1 == 0 || norm2 == 0 {
		return 0, nil
	}

	return dotProduct / (norm1 * norm2), nil
}

// ComputeCosineTable 预计算查询向量与所有质心的余弦相似度表
// 这可以显著加速批量余弦相似度计算
func (q *Quantizer) ComputeCosineTable(queryVector []float64) ([][]float64, error) {
	if q.codebook == nil {
		return nil, fmt.Errorf("quantizer not initialized with codebook")
	}

	expectedDim := q.codebook.M * q.codebook.SubVectorDim
	if len(queryVector) != expectedDim {
		return nil, fmt.Errorf("query vector dimension %d does not match expected dimension %d", len(queryVector), expectedDim)
	}

	cosineTable := make([][]float64, q.codebook.M)

	for m := 0; m < q.codebook.M; m++ {
		start := m * q.codebook.SubVectorDim
		end := start + q.codebook.SubVectorDim
		querySubVector := queryVector[start:end]

		// 计算查询子向量的范数
		var querySubNormSq float64
		for _, val := range querySubVector {
			querySubNormSq += val * val
		}
		querySubNorm := math.Sqrt(querySubNormSq)

		cosineTable[m] = make([]float64, q.codebook.K)

		for k := 0; k < q.codebook.K; k++ {
			centroid := q.codebook.Centroids[m][k]

			// 计算质心的范数和点积
			var centroidNormSq, dotProduct float64
			for i := 0; i < q.codebook.SubVectorDim; i++ {
				dotProduct += querySubVector[i] * centroid[i]
				centroidNormSq += centroid[i] * centroid[i]
			}

			centroidNorm := math.Sqrt(centroidNormSq)

			// 计算子向量间的余弦相似度
			if querySubNorm == 0 || centroidNorm == 0 {
				cosineTable[m][k] = 0
			} else {
				cosineTable[m][k] = dotProduct / (querySubNorm * centroidNorm)
			}
		}
	}

	return cosineTable, nil
}

// AsymmetricCosineSimilarityWithTable 使用预计算的余弦表计算非对称余弦相似度
// 注意：这个方法已经废弃，请使用 AsymmetricCosineSimilarityWithDotProductTable
func (q *Quantizer) AsymmetricCosineSimilarityWithTable(codes []byte, cosineTable [][]float64) (float64, error) {
	return 0, fmt.Errorf("this method is deprecated, use AsymmetricCosineSimilarityWithDotProductTable instead")
}

// ComputeDotProductTable 预计算查询向量与所有质心的点积表（更适合余弦相似度计算）
func (q *Quantizer) ComputeDotProductTable(queryVector []float64) ([][]float64, []float64, error) {
	if q.codebook == nil {
		return nil, nil, fmt.Errorf("quantizer not initialized with codebook")
	}

	expectedDim := q.codebook.M * q.codebook.SubVectorDim
	if len(queryVector) != expectedDim {
		return nil, nil, fmt.Errorf("query vector dimension %d does not match expected dimension %d", len(queryVector), expectedDim)
	}

	dotProductTable := make([][]float64, q.codebook.M)
	querySubNorms := make([]float64, q.codebook.M)

	for m := 0; m < q.codebook.M; m++ {
		start := m * q.codebook.SubVectorDim
		end := start + q.codebook.SubVectorDim
		querySubVector := queryVector[start:end]

		// 计算查询子向量的范数平方
		var querySubNormSq float64
		for _, val := range querySubVector {
			querySubNormSq += val * val
		}
		querySubNorms[m] = math.Sqrt(querySubNormSq)

		dotProductTable[m] = make([]float64, q.codebook.K)

		for k := 0; k < q.codebook.K; k++ {
			centroid := q.codebook.Centroids[m][k]

			// 计算点积
			var dotProduct float64
			for i := 0; i < q.codebook.SubVectorDim; i++ {
				dotProduct += querySubVector[i] * centroid[i]
			}

			dotProductTable[m][k] = dotProduct
		}
	}

	return dotProductTable, querySubNorms, nil
}

// AsymmetricCosineSimilarityWithDotProductTable 使用预计算的点积表计算余弦相似度
func (q *Quantizer) AsymmetricCosineSimilarityWithDotProductTable(codes []byte, dotProductTable [][]float64, querySubNorms []float64) (float64, error) {
	if len(codes) != q.codebook.M {
		return 0, fmt.Errorf("code length %d does not match expected M %d", len(codes), q.codebook.M)
	}

	var totalDotProduct, totalDataBaseNormSq float64
	totalQueryNormSq := 0.0

	for m := 0; m < q.codebook.M; m++ {
		centroidIndex := int(codes[m])
		if centroidIndex < 0 || centroidIndex >= len(dotProductTable[m]) {
			return 0, fmt.Errorf("invalid centroid index %d for sub-vector %d", centroidIndex, m)
		}

		// 从预计算表获取点积
		dotProduct := dotProductTable[m][centroidIndex]
		totalDotProduct += dotProduct

		// 计算查询向量范数平方的贡献
		querySubNormSq := querySubNorms[m] * querySubNorms[m]
		totalQueryNormSq += querySubNormSq

		// 计算数据库向量（质心）范数平方的贡献
		centroid := q.codebook.Centroids[m][centroidIndex]
		var centroidNormSq float64
		for _, val := range centroid {
			centroidNormSq += val * val
		}
		totalDataBaseNormSq += centroidNormSq
	}

	// 计算最终的余弦相似度
	totalQueryNorm := math.Sqrt(totalQueryNormSq)
	totalDataBaseNorm := math.Sqrt(totalDataBaseNormSq)

	if totalQueryNorm == 0 || totalDataBaseNorm == 0 {
		return 0, nil
	}

	return totalDotProduct / (totalQueryNorm * totalDataBaseNorm), nil
}

// BatchAsymmetricCosineSimilarity 批量计算非对称余弦相似度
func (q *Quantizer) BatchAsymmetricCosineSimilarity(queryVector []float64, allCodes [][]byte) ([]float64, error) {
	if len(allCodes) == 0 {
		return []float64{}, nil
	}

	// 预计算点积表以加速批量计算
	dotProductTable, querySubNorms, err := q.ComputeDotProductTable(queryVector)
	if err != nil {
		return nil, fmt.Errorf("failed to compute dot product table: %v", err)
	}

	similarities := make([]float64, len(allCodes))
	for i, codes := range allCodes {
		similarity, err := q.AsymmetricCosineSimilarityWithDotProductTable(codes, dotProductTable, querySubNorms)
		if err != nil {
			return nil, fmt.Errorf("failed to compute similarity for vector %d: %v", i, err)
		}
		similarities[i] = similarity
	}

	return similarities, nil
}

// FindMostSimilarCodes 在给定的PQ码集合中找到与查询向量最相似的N个（基于余弦相似度）
func (q *Quantizer) FindMostSimilarCodes(queryVector []float64, allCodes [][]byte, topN int) ([]int, []float64, error) {
	if len(allCodes) == 0 {
		return []int{}, []float64{}, nil
	}

	if topN <= 0 || topN > len(allCodes) {
		topN = len(allCodes)
	}

	// 计算所有相似度
	type SimilarityEntry struct {
		index      int
		similarity float64
	}

	similarities := make([]SimilarityEntry, len(allCodes))
	for i, codes := range allCodes {
		sim, err := q.AsymmetricCosineSimilarity(queryVector, codes)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to compute similarity for vector %d: %v", i, err)
		}
		similarities[i] = SimilarityEntry{index: i, similarity: sim}
	}

	// 选择前topN个最高相似度
	for i := 0; i < topN; i++ {
		maxIdx := i
		for j := i + 1; j < len(similarities); j++ {
			if similarities[j].similarity > similarities[maxIdx].similarity {
				maxIdx = j
			}
		}
		if maxIdx != i {
			similarities[i], similarities[maxIdx] = similarities[maxIdx], similarities[i]
		}
	}

	// 提取结果
	indices := make([]int, topN)
	resultSimilarities := make([]float64, topN)
	for i := 0; i < topN; i++ {
		indices[i] = similarities[i].index
		resultSimilarities[i] = similarities[i].similarity
	}

	return indices, resultSimilarities, nil
}

// CompareCosineMethods 比较不同余弦相似度计算方法的结果
func (q *Quantizer) CompareCosineMethods(queryVector []float64, codes []byte) (map[string]interface{}, error) {
	// 方法1: 非对称余弦相似度
	asymmetricSim, err := q.AsymmetricCosineSimilarity(queryVector, codes)
	if err != nil {
		return nil, fmt.Errorf("asymmetric cosine similarity failed: %v", err)
	}

	// 方法2: 解码后计算标准余弦相似度
	decodedVector, err := q.Decode(codes)
	if err != nil {
		return nil, fmt.Errorf("decode failed: %v", err)
	}

	standardSim, err := cosineSimilarity(queryVector, decodedVector)
	if err != nil {
		return nil, fmt.Errorf("standard cosine similarity failed: %v", err)
	}

	// 方法3: 使用点积表的快速计算
	dotProductTable, querySubNorms, err := q.ComputeDotProductTable(queryVector)
	if err != nil {
		return nil, fmt.Errorf("dot product table computation failed: %v", err)
	}

	fastSim, err := q.AsymmetricCosineSimilarityWithDotProductTable(codes, dotProductTable, querySubNorms)
	if err != nil {
		return nil, fmt.Errorf("fast cosine similarity failed: %v", err)
	}

	return map[string]interface{}{
		"asymmetric_cosine_similarity": asymmetricSim,
		"standard_cosine_similarity":   standardSim,
		"fast_cosine_similarity":       fastSim,
		"asymmetric_vs_standard_diff":  math.Abs(asymmetricSim - standardSim),
		"asymmetric_vs_fast_diff":      math.Abs(asymmetricSim - fastSim),
		"standard_vs_fast_diff":        math.Abs(standardSim - fastSim),
	}, nil
}
