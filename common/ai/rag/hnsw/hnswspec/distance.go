package hnswspec

import (
	"cmp"
	"math"

	"github.com/yaklang/yaklang/common/ai/rag/pq"
	"github.com/yaklang/yaklang/common/log"
)

// CosineDistance 余弦距离计算函数（基于节点接口，不支持PQ）
func CosineDistance[K cmp.Ordered](a, b LayerNode[K]) float64 {
	return PQAwareCosineDistance(a, b, nil)
}

// PQAwareCosineDistance PQ感知的余弦距离计算函数
func PQAwareCosineDistance[K cmp.Ordered](a, b LayerNode[K], quantizer *pq.Quantizer) float64 {
	// 如果两个节点都启用PQ且有有效的PQ编码，使用PQ距离计算
	if a.IsPQEnabled() && b.IsPQEnabled() {
		codesA, okA := a.GetPQCodes()
		codesB, okB := b.GetPQCodes()
		if okA && okB {
			return PQCosineDistance(codesA, codesB, a, b)
		}
		// PQ节点但无法获取编码，返回最大距离
		log.Warnf("PQ nodes but failed to get PQ codes, returning max distance")
		return 1.0
	}

	// 如果只有一个节点启用PQ，使用非对称距离计算
	if a.IsPQEnabled() && !b.IsPQEnabled() {
		// a是PQ节点，b是标准节点（通常是查询向量）
		vecB := b.GetVector()()
		if len(vecB) == 0 {
			log.Warnf("node b has empty vector, returning max distance")
			return 1.0
		}
		return PQAsymmetricCosineDistance(vecB, a, quantizer)
	} else if !a.IsPQEnabled() && b.IsPQEnabled() {
		// a是标准节点（通常是查询向量），b是PQ节点
		vecA := a.GetVector()()
		if len(vecA) == 0 {
			log.Warnf("node a has empty vector, returning max distance")
			return 1.0
		}
		return PQAsymmetricCosineDistance(vecA, b, quantizer)
	}

	// 都是标准节点，使用原始向量进行精确计算
	vecA := a.GetVector()()
	vecB := b.GetVector()()
	if len(vecA) == 0 || len(vecB) == 0 {
		log.Warnf("one or both vectors are empty: len(a)=%d, len(b)=%d, returning max distance", len(vecA), len(vecB))
		return 1.0
	}
	if len(vecA) != len(vecB) {
		log.Warnf("vector dimension mismatch: len(a)=%d, len(b)=%d, returning max distance", len(vecA), len(vecB))
		return 1.0
	}
	return cosineDistanceRaw(vecA, vecB)
}

// EuclideanDistance 欧氏距离计算函数（基于节点接口，不支持PQ）
func EuclideanDistance[K cmp.Ordered](a, b LayerNode[K]) float64 {
	return PQAwareEuclideanDistance(a, b, nil)
}

// PQAwareEuclideanDistance PQ感知的欧氏距离计算函数
func PQAwareEuclideanDistance[K cmp.Ordered](a, b LayerNode[K], quantizer *pq.Quantizer) float64 {
	// 如果两个节点都启用PQ且有有效的PQ编码，使用PQ距离计算
	if a.IsPQEnabled() && b.IsPQEnabled() {
		codesA, okA := a.GetPQCodes()
		codesB, okB := b.GetPQCodes()
		if okA && okB {
			return PQEuclideanDistance(codesA, codesB, a, b)
		}
		// PQ节点但无法获取编码，返回最大距离
		log.Warnf("PQ nodes but failed to get PQ codes, returning max distance")
		return math.Inf(1)
	}

	// 如果只有一个节点启用PQ，使用非对称距离计算
	if a.IsPQEnabled() && !b.IsPQEnabled() {
		// a是PQ节点，b是标准节点（通常是查询向量）
		vecB := b.GetVector()()
		if len(vecB) == 0 {
			log.Warnf("node b has empty vector, returning max distance")
			return math.Inf(1)
		}
		return PQAsymmetricDistance(vecB, a, quantizer)
	} else if !a.IsPQEnabled() && b.IsPQEnabled() {
		// a是标准节点（通常是查询向量），b是PQ节点
		vecA := a.GetVector()()
		if len(vecA) == 0 {
			log.Warnf("node a has empty vector, returning max distance")
			return math.Inf(1)
		}
		return PQAsymmetricDistance(vecA, b, quantizer)
	}

	// 都是标准节点，使用原始向量进行精确计算
	vecA := a.GetVector()()
	vecB := b.GetVector()()
	if len(vecA) == 0 || len(vecB) == 0 {
		log.Warnf("one or both vectors are empty: len(a)=%d, len(b)=%d, returning max distance", len(vecA), len(vecB))
		return math.Inf(1)
	}
	if len(vecA) != len(vecB) {
		log.Warnf("vector dimension mismatch: len(a)=%d, len(b)=%d, returning max distance", len(vecA), len(vecB))
		return math.Inf(1)
	}
	return euclideanDistanceRaw(vecA, vecB)
}

// cosineDistanceRaw 原始余弦距离计算
func cosineDistanceRaw(a, b []float32) float64 {
	if len(a) != len(b) {
		log.Errorf("cosineDistanceRaw: vectors must have the same length, but len(a)=%d, len(b)=%d", len(a), len(b))
		return 1.0 // Maximum cosine distance
	}

	if len(a) == 0 {
		log.Warnf("cosineDistanceRaw: empty vectors")
		return 1.0
	}

	var (
		dotProduct = float64(0)
		normA      = float64(0)
		normB      = float64(0)
	)

	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 1 // Maximum distance for zero vectors
	}

	similarity := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))

	// Clamp to [-1, 1] to handle floating point precision issues
	if similarity > 1 {
		similarity = 1
	} else if similarity < -1 {
		similarity = -1
	}

	// Convert similarity to distance (0 = identical, 2 = opposite)
	return 1 - similarity
}

// euclideanDistanceRaw 原始欧氏距离计算
func euclideanDistanceRaw(a, b []float32) float64 {
	if len(a) != len(b) {
		log.Errorf("euclideanDistanceRaw: vectors must have the same length, but len(a)=%d, len(b)=%d", len(a), len(b))
		return math.Inf(1) // Maximum distance
	}

	if len(a) == 0 {
		log.Warnf("euclideanDistanceRaw: empty vectors")
		return math.Inf(1)
	}

	var sum float64
	for i := range a {
		diff := float64(a[i]) - float64(b[i])
		sum += diff * diff
	}

	return math.Sqrt(sum)
}

// PQCosineDistance 基于PQ编码的余弦距离计算
func PQCosineDistance[K cmp.Ordered](codesA, codesB []byte, nodeA, nodeB LayerNode[K]) float64 {
	if len(codesA) != len(codesB) {
		return 1.0 // 最大距离
	}

	// 简单的码表差异计算（临时实现）
	// 这里可以进一步优化，使用codebook解码后再计算
	distance := float64(0)
	for i := range codesA {
		if codesA[i] != codesB[i] {
			distance += 1.0
		}
	}

	// 归一化到[0,1]范围
	return distance / float64(len(codesA))
}

// PQEuclideanDistance 基于PQ编码的欧氏距离计算
func PQEuclideanDistance[K cmp.Ordered](codesA, codesB []byte, nodeA, nodeB LayerNode[K]) float64 {
	// PQ编码之间的距离计算
	if len(codesA) != len(codesB) {
		return math.Inf(1) // 最大距离
	}

	// 简单的码表差异计算（临时实现）
	distance := float64(0)
	for i := range codesA {
		if codesA[i] != codesB[i] {
			distance += 1.0
		}
	}

	return distance
}

// PQAsymmetricDistance 非对称PQ距离（查询向量使用原始向量，数据向量使用PQ编码）
func PQAsymmetricDistance[K cmp.Ordered](queryVec []float32, dataNode LayerNode[K], quantizer *pq.Quantizer) float64 {
	if !dataNode.IsPQEnabled() {
		// 如果数据节点未启用PQ，直接使用原始向量计算（仅对StandardNode）
		dataVec := dataNode.GetVector()()
		return euclideanDistanceRaw(queryVec, dataVec)
	}

	dataCodes, ok := dataNode.GetPQCodes()
	if !ok {
		// 如果无法获取PQ编码，返回最大距离（PQ节点不应该到这里）
		return math.Inf(1)
	}

	// 使用PQ解码功能：将PQ编码解码为近似向量，然后使用传统算法
	if quantizer == nil {
		return math.Inf(1)
	}

	// 解码PQ编码为近似向量
	decodedVec64, err := quantizer.Decode(dataCodes)
	if err != nil {
		// 解码失败，返回默认距离
		return math.Inf(1)
	}

	// 转换为float32
	decodedVec32 := make([]float32, len(decodedVec64))
	for i, v := range decodedVec64 {
		decodedVec32[i] = float32(v)
	}

	// 使用传统的欧氏距离算法
	return euclideanDistanceRaw(queryVec, decodedVec32)
}

// PQAsymmetricCosineDistance 非对称PQ余弦距离（查询向量使用原始向量，数据向量使用PQ编码）
func PQAsymmetricCosineDistance[K cmp.Ordered](queryVec []float32, dataNode LayerNode[K], quantizer *pq.Quantizer) float64 {
	if !dataNode.IsPQEnabled() {
		// 如果数据节点未启用PQ，直接使用原始向量计算（仅对StandardNode）
		dataVec := dataNode.GetVector()()
		return cosineDistanceRaw(queryVec, dataVec)
	}

	dataCodes, ok := dataNode.GetPQCodes()
	if !ok {
		// 如果无法获取PQ编码，返回最大距离（PQ节点不应该到这里）
		return 1.0
	}

	// 如果没有quantizer，回退到简单距离计算
	if quantizer == nil {
		return 0.5
	}

	// 解码PQ编码为近似向量
	decodedVec64, err := quantizer.Decode(dataCodes)
	if err != nil {
		// 解码失败，返回默认距离
		return 1.0
	}

	// 转换为float32
	decodedVec32 := make([]float32, len(decodedVec64))
	for i, v := range decodedVec64 {
		decodedVec32[i] = float32(v)
	}

	// 使用传统的余弦距离算法
	return cosineDistanceRaw(queryVec, decodedVec32)
}

// pqAsymmetricDistanceImpl PQ非对称距离的具体实现
func pqAsymmetricDistanceImpl(queryVec []float32, dataCodes []byte, codebook *pq.Codebook) float64 {
	if codebook == nil || len(queryVec) != codebook.M*codebook.SubVectorDim {
		// 参数不匹配时回退到简单计算
		return 1.0
	}

	distance := float64(0)

	// 对每个子向量计算距离
	for m := 0; m < codebook.M; m++ {
		if m >= len(dataCodes) {
			break
		}

		// 获取查询向量的子向量
		start := m * codebook.SubVectorDim
		end := start + codebook.SubVectorDim
		if end > len(queryVec) {
			break
		}
		querySubVec := queryVec[start:end]

		// 获取数据向量的码表索引
		codeIndex := int(dataCodes[m])
		if codeIndex >= len(codebook.Centroids[m]) {
			continue
		}

		// 获取对应的聚类中心
		centroid := codebook.Centroids[m][codeIndex]

		// 计算子向量距离（这里使用欧氏距离）
		subDist := float64(0)
		for i := 0; i < len(querySubVec) && i < len(centroid); i++ {
			diff := float64(querySubVec[i]) - centroid[i]
			subDist += diff * diff
		}

		distance += subDist
	}

	return math.Sqrt(distance)
}

// pqAsymmetricCosineDistanceImpl PQ非对称余弦距离的具体实现
func pqAsymmetricCosineDistanceImpl(queryVec []float32, dataCodes []byte, codebook *pq.Codebook) float64 {
	if codebook == nil || len(queryVec) != codebook.M*codebook.SubVectorDim {
		// 参数不匹配时回退到简单计算
		return 1.0
	}

	// 转换float32向量为float64向量以便与PQ包兼容
	queryVec64 := make([]float64, len(queryVec))
	for i, v := range queryVec {
		queryVec64[i] = float64(v)
	}

	// 创建量化器（这里需要从codebook创建）
	quantizer := pq.NewQuantizer(codebook)
	if quantizer == nil {
		return 1.0
	}

	// 使用PQ包中的非对称余弦相似度计算
	similarity, err := quantizer.AsymmetricCosineSimilarity(queryVec64, dataCodes)
	if err != nil {
		// 发生错误时返回最大距离
		return 1.0
	}

	// 将相似度转换为距离（0 = 相同, 2 = 完全相反）
	return 1.0 - similarity
}
