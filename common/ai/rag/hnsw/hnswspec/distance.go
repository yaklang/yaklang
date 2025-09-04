package hnswspec

import (
	"cmp"
	"math"

	"github.com/yaklang/yaklang/common/ai/rag/pq"
)

// CosineDistance 余弦距离计算函数（基于节点接口）
func CosineDistance[K cmp.Ordered](a, b LayerNode[K]) float64 {
	// 如果两个节点都启用PQ且有有效的PQ编码，使用PQ距离计算
	if a.IsPQEnabled() && b.IsPQEnabled() {
		codesA, okA := a.GetPQCodes()
		codesB, okB := b.GetPQCodes()
		if okA && okB {
			return PQCosineDistance(codesA, codesB, a, b)
		}
	}

	// 如果任一节点启用了PQ，但无法获取到PQ编码，则无法计算距离
	if a.IsPQEnabled() || b.IsPQEnabled() {
		// 对于PQ节点，我们不能调用GetVector()，因为它会panic
		// 这种情况说明PQ配置有问题，返回最大距离
		panic("不支持PQ模式和非PQ模式混合计算距离")
	}

	// 否则使用原始向量进行精确计算
	vecA := a.GetVector()()
	vecB := b.GetVector()()
	return cosineDistanceRaw(vecA, vecB)
}

// EuclideanDistance 欧氏距离计算函数（基于节点接口）
func EuclideanDistance[K cmp.Ordered](a, b LayerNode[K]) float64 {
	// 如果两个节点都启用PQ且有有效的PQ编码，使用PQ距离计算
	if a.IsPQEnabled() && b.IsPQEnabled() {
		codesA, okA := a.GetPQCodes()
		codesB, okB := b.GetPQCodes()
		if okA && okB {
			return PQEuclideanDistance(codesA, codesB, a, b)
		}
	}

	// 如果任一节点启用了PQ，但无法获取到PQ编码，则无法计算距离
	if a.IsPQEnabled() || b.IsPQEnabled() {
		// 对于PQ节点，我们不能调用GetVector()，因为它会panic
		// 这种情况说明PQ配置有问题，返回最大距离
		panic("不支持PQ模式和非PQ模式混合计算距离")
	}

	// 否则使用原始向量进行精确计算
	vecA := a.GetVector()()
	vecB := b.GetVector()()
	return euclideanDistanceRaw(vecA, vecB)
}

// cosineDistanceRaw 原始余弦距离计算
func cosineDistanceRaw(a, b []float32) float64 {
	if len(a) != len(b) {
		panic("vectors must have the same length")
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
		panic("vectors must have the same length")
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
	// PQ编码之间的距离计算
	// 这里应该使用PQ量化器进行距离计算，但需要从节点获取量化器
	// 作为临时实现，我们使用简单的汉明距离或欧氏距离
	if len(codesA) != len(codesB) {
		return 1.0 // 最大距离
	}

	// 简单的码表差异计算（临时实现）
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
func PQAsymmetricDistance[K cmp.Ordered](queryVec []float32, dataNode LayerNode[K], codebook *pq.Codebook) float64 {
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

	// 使用PQ码表计算非对称距离
	return pqAsymmetricDistanceImpl(queryVec, dataCodes, codebook)
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
