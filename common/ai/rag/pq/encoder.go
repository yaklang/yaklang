package pq

import (
	"fmt"
	"math"

	"github.com/yaklang/yaklang/common/log"
)

// Quantizer PQ量化器，用于编码和解码向量
type Quantizer struct {
	codebook *Codebook
}

// NewQuantizer 创建新的量化器
func NewQuantizer(codebook *Codebook) *Quantizer {
	if codebook == nil {
		return nil
	}
	return &Quantizer{
		codebook: codebook,
	}
}

// Encode 将高维向量编码为PQ码
func (q *Quantizer) Encode(vector []float64) ([]byte, error) {
	if q.codebook == nil {
		return nil, fmt.Errorf("quantizer not initialized with codebook")
	}

	expectedDim := q.codebook.M * q.codebook.SubVectorDim
	if len(vector) != expectedDim {
		return nil, fmt.Errorf("vector dimension %d does not match expected dimension %d", len(vector), expectedDim)
	}

	codes := make([]byte, q.codebook.M)

	// 对每个子向量进行编码
	for m := 0; m < q.codebook.M; m++ {
		start := m * q.codebook.SubVectorDim
		end := start + q.codebook.SubVectorDim
		subVector := vector[start:end]

		// 找到最近的聚类中心
		bestCentroidIndex := 0
		minDistance := math.MaxFloat64

		for k, centroid := range q.codebook.Centroids[m] {
			distance := euclideanDistanceSquared(subVector, centroid)
			if distance < minDistance {
				minDistance = distance
				bestCentroidIndex = k
			}
		}

		codes[m] = byte(bestCentroidIndex)
	}

	return codes, nil
}

// Decode 将PQ码解码为近似的高维向量
func (q *Quantizer) Decode(codes []byte) ([]float64, error) {
	if q.codebook == nil {
		return nil, fmt.Errorf("quantizer not initialized with codebook")
	}

	if len(codes) != q.codebook.M {
		return nil, fmt.Errorf("code length %d does not match expected M %d", len(codes), q.codebook.M)
	}

	vector := make([]float64, q.codebook.M*q.codebook.SubVectorDim)

	// 对每个子向量进行解码
	for m := 0; m < q.codebook.M; m++ {
		centroidIndex := int(codes[m])

		// 检查索引有效性
		if centroidIndex < 0 || centroidIndex >= q.codebook.K {
			return nil, fmt.Errorf("invalid centroid index %d for sub-vector %d", centroidIndex, m)
		}

		centroid := q.codebook.Centroids[m][centroidIndex]
		start := m * q.codebook.SubVectorDim

		// 复制质心到输出向量
		copy(vector[start:start+q.codebook.SubVectorDim], centroid)
	}

	return vector, nil
}

// AsymmetricDistance 计算查询向量与PQ码之间的非对称距离
// 这是PQ算法的核心功能，用于在索引中快速计算距离
func (q *Quantizer) AsymmetricDistance(queryVector []float64, codes []byte) (float64, error) {
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

	totalDistance := 0.0

	// 计算每个子向量的距离并累加
	for m := 0; m < q.codebook.M; m++ {
		start := m * q.codebook.SubVectorDim
		end := start + q.codebook.SubVectorDim
		querySubVector := queryVector[start:end]

		centroidIndex := int(codes[m])
		if centroidIndex < 0 || centroidIndex >= q.codebook.K {
			return 0, fmt.Errorf("invalid centroid index %d for sub-vector %d", centroidIndex, m)
		}

		centroid := q.codebook.Centroids[m][centroidIndex]
		subDistance := euclideanDistanceSquared(querySubVector, centroid)
		totalDistance += subDistance
	}

	return math.Sqrt(totalDistance), nil
}

// ComputeDistanceTable 预计算查询向量与所有质心的距离表
// 这可以显著加速批量距离计算
func (q *Quantizer) ComputeDistanceTable(queryVector []float64) ([][]float64, error) {
	if q.codebook == nil {
		return nil, fmt.Errorf("quantizer not initialized with codebook")
	}

	expectedDim := q.codebook.M * q.codebook.SubVectorDim
	if len(queryVector) != expectedDim {
		return nil, fmt.Errorf("query vector dimension %d does not match expected dimension %d", len(queryVector), expectedDim)
	}

	distanceTable := make([][]float64, q.codebook.M)

	for m := 0; m < q.codebook.M; m++ {
		start := m * q.codebook.SubVectorDim
		end := start + q.codebook.SubVectorDim
		querySubVector := queryVector[start:end]

		distanceTable[m] = make([]float64, q.codebook.K)

		for k := 0; k < q.codebook.K; k++ {
			centroid := q.codebook.Centroids[m][k]
			distanceTable[m][k] = euclideanDistanceSquared(querySubVector, centroid)
		}
	}

	return distanceTable, nil
}

// AsymmetricDistanceWithTable 使用预计算的距离表计算非对称距离
func (q *Quantizer) AsymmetricDistanceWithTable(codes []byte, distanceTable [][]float64) (float64, error) {
	if len(codes) != q.codebook.M {
		return 0, fmt.Errorf("code length %d does not match expected M %d", len(codes), q.codebook.M)
	}

	if len(distanceTable) != q.codebook.M {
		return 0, fmt.Errorf("distance table size %d does not match expected M %d", len(distanceTable), q.codebook.M)
	}

	totalDistance := 0.0

	for m := 0; m < q.codebook.M; m++ {
		centroidIndex := int(codes[m])
		if centroidIndex < 0 || centroidIndex >= len(distanceTable[m]) {
			return 0, fmt.Errorf("invalid centroid index %d for sub-vector %d", centroidIndex, m)
		}

		totalDistance += distanceTable[m][centroidIndex]
	}

	return math.Sqrt(totalDistance), nil
}

// GetCompressionRatio 计算压缩比
func (q *Quantizer) GetCompressionRatio() float64 {
	if q.codebook == nil {
		return 0
	}

	originalSize := q.codebook.M * q.codebook.SubVectorDim * 8 // 8 bytes per float64
	compressedSize := q.codebook.M * 1                         // 1 byte per code

	return float64(originalSize) / float64(compressedSize)
}

// M 返回子向量数量
func (q *Quantizer) M() int {
	if q.codebook == nil {
		return 0
	}
	return q.codebook.M
}

// K 返回每个子空间的聚类中心数量
func (q *Quantizer) K() int {
	if q.codebook == nil {
		return 0
	}
	return q.codebook.K
}

// SubVectorDim 返回每个子向量的维度
func (q *Quantizer) SubVectorDim() int {
	if q.codebook == nil {
		return 0
	}
	return q.codebook.SubVectorDim
}

// GetCodebookInfo 获取码本信息
func (q *Quantizer) GetCodebookInfo() map[string]interface{} {
	if q.codebook == nil {
		return nil
	}

	return map[string]interface{}{
		"M":                q.codebook.M,
		"K":                q.codebook.K,
		"SubVectorDim":     q.codebook.SubVectorDim,
		"TotalDimension":   q.codebook.M * q.codebook.SubVectorDim,
		"CompressionRatio": q.GetCompressionRatio(),
		"CodebookSizeKB":   float64(q.codebook.M*q.codebook.K*q.codebook.SubVectorDim*8) / 1024.0,
	}
}

// EstimateQuantizationError 估计给定向量的量化误差
func (q *Quantizer) EstimateQuantizationError(vector []float64) (float64, error) {
	// 编码向量
	codes, err := q.Encode(vector)
	if err != nil {
		return 0, fmt.Errorf("failed to encode vector: %v", err)
	}

	// 解码向量
	decodedVector, err := q.Decode(codes)
	if err != nil {
		return 0, fmt.Errorf("failed to decode vector: %v", err)
	}

	// 计算原始向量与解码向量之间的距离
	return math.Sqrt(euclideanDistanceSquared(vector, decodedVector)), nil
}

// BatchEncode 批量编码向量
func (q *Quantizer) BatchEncode(vectors [][]float64) ([][]byte, error) {
	if q.codebook == nil {
		return nil, fmt.Errorf("quantizer not initialized with codebook")
	}

	if len(vectors) == 0 {
		return [][]byte{}, nil
	}

	log.Infof("Starting batch encoding of %d vectors", len(vectors))

	codes := make([][]byte, len(vectors))
	for i, vector := range vectors {
		var err error
		codes[i], err = q.Encode(vector)
		if err != nil {
			return nil, fmt.Errorf("failed to encode vector %d: %v", i, err)
		}

		if (i+1)%1000 == 0 {
			log.Infof("Encoded %d/%d vectors", i+1, len(vectors))
		}
	}

	log.Infof("Completed batch encoding of %d vectors", len(vectors))
	return codes, nil
}

// BatchDecode 批量解码向量
func (q *Quantizer) BatchDecode(allCodes [][]byte) ([][]float64, error) {
	if q.codebook == nil {
		return nil, fmt.Errorf("quantizer not initialized with codebook")
	}

	if len(allCodes) == 0 {
		return [][]float64{}, nil
	}

	log.Infof("Starting batch decoding of %d vectors", len(allCodes))

	vectors := make([][]float64, len(allCodes))
	for i, codes := range allCodes {
		var err error
		vectors[i], err = q.Decode(codes)
		if err != nil {
			return nil, fmt.Errorf("failed to decode vector %d: %v", i, err)
		}

		if (i+1)%1000 == 0 {
			log.Infof("Decoded %d/%d vectors", i+1, len(allCodes))
		}
	}

	log.Infof("Completed batch decoding of %d vectors", len(allCodes))
	return vectors, nil
}

// ApproximateDistance 计算两个PQ码之间的近似距离
// 这个函数用于快速计算两个压缩向量之间的距离，而不需要完全解码
func (q *Quantizer) ApproximateDistance(codes1, codes2 []byte) (float64, error) {
	if q.codebook == nil {
		return 0, fmt.Errorf("quantizer not initialized with codebook")
	}

	if len(codes1) != q.codebook.M || len(codes2) != q.codebook.M {
		return 0, fmt.Errorf("codes length must be %d, got %d and %d",
			q.codebook.M, len(codes1), len(codes2))
	}

	totalDistance := 0.0

	// 计算每个子向量对应质心之间的距离
	for m := 0; m < q.codebook.M; m++ {
		centroidIndex1 := int(codes1[m])
		centroidIndex2 := int(codes2[m])

		if centroidIndex1 < 0 || centroidIndex1 >= q.codebook.K ||
			centroidIndex2 < 0 || centroidIndex2 >= q.codebook.K {
			return 0, fmt.Errorf("invalid centroid indices: %d, %d", centroidIndex1, centroidIndex2)
		}

		centroid1 := q.codebook.Centroids[m][centroidIndex1]
		centroid2 := q.codebook.Centroids[m][centroidIndex2]

		subDistance := euclideanDistanceSquared(centroid1, centroid2)
		totalDistance += subDistance
	}

	return math.Sqrt(totalDistance), nil
}

// GetApproximateVector 获取指定PQ码对应的近似向量（解码的别名，更语义化）
func (q *Quantizer) GetApproximateVector(codes []byte) ([]float64, error) {
	return q.Decode(codes)
}

// ComputeSimilarity 计算查询向量与PQ码的相似度（基于余弦相似度）
func (q *Quantizer) ComputeSimilarity(queryVector []float64, codes []byte) (float64, error) {
	if q.codebook == nil {
		return 0, fmt.Errorf("quantizer not initialized with codebook")
	}

	// 解码PQ码
	decodedVector, err := q.Decode(codes)
	if err != nil {
		return 0, fmt.Errorf("failed to decode vector: %v", err)
	}

	// 计算余弦相似度
	return cosineSimilarity(queryVector, decodedVector)
}

// BatchComputeSimilarity 批量计算相似度
func (q *Quantizer) BatchComputeSimilarity(queryVector []float64, allCodes [][]byte) ([]float64, error) {
	if len(allCodes) == 0 {
		return []float64{}, nil
	}

	similarities := make([]float64, len(allCodes))
	for i, codes := range allCodes {
		var err error
		similarities[i], err = q.ComputeSimilarity(queryVector, codes)
		if err != nil {
			return nil, fmt.Errorf("failed to compute similarity for vector %d: %v", i, err)
		}
	}

	return similarities, nil
}

// FindNearestCodes 在给定的PQ码集合中找到与查询向量最近的N个
func (q *Quantizer) FindNearestCodes(queryVector []float64, allCodes [][]byte, topN int) ([]int, []float64, error) {
	if len(allCodes) == 0 {
		return []int{}, []float64{}, nil
	}

	if topN <= 0 || topN > len(allCodes) {
		topN = len(allCodes)
	}

	// 计算所有距离
	type DistanceEntry struct {
		index    int
		distance float64
	}

	distances := make([]DistanceEntry, len(allCodes))
	for i, codes := range allCodes {
		dist, err := q.AsymmetricDistance(queryVector, codes)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to compute distance for vector %d: %v", i, err)
		}
		distances[i] = DistanceEntry{index: i, distance: dist}
	}

	// 选择前topN个最小距离
	for i := 0; i < topN; i++ {
		minIdx := i
		for j := i + 1; j < len(distances); j++ {
			if distances[j].distance < distances[minIdx].distance {
				minIdx = j
			}
		}
		if minIdx != i {
			distances[i], distances[minIdx] = distances[minIdx], distances[i]
		}
	}

	// 提取结果
	indices := make([]int, topN)
	resultDistances := make([]float64, topN)
	for i := 0; i < topN; i++ {
		indices[i] = distances[i].index
		resultDistances[i] = distances[i].distance
	}

	return indices, resultDistances, nil
}

// CompareVectors 比较原始向量和其PQ近似向量的差异
func (q *Quantizer) CompareVectors(originalVector []float64) (map[string]interface{}, error) {
	// 编码
	codes, err := q.Encode(originalVector)
	if err != nil {
		return nil, fmt.Errorf("failed to encode vector: %v", err)
	}

	// 解码
	approximateVector, err := q.Decode(codes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode vector: %v", err)
	}

	// 计算各种误差指标
	mse := meanSquaredError(originalVector, approximateVector)
	mae := meanAbsoluteError(originalVector, approximateVector)
	maxError := maxAbsoluteError(originalVector, approximateVector)
	cosingSim, err := cosineSimilarity(originalVector, approximateVector)
	if err != nil {
		cosingSim = 0 // 如果计算失败，设为0
	}

	// 计算压缩比
	originalSize := len(originalVector) * 8 // 8 bytes per float64
	compressedSize := len(codes)            // 1 byte per code
	compressionRatio := float64(originalSize) / float64(compressedSize)

	return map[string]interface{}{
		"original_dimension":    len(originalVector),
		"compressed_size_bytes": len(codes),
		"compression_ratio":     compressionRatio,
		"mean_squared_error":    mse,
		"mean_absolute_error":   mae,
		"max_absolute_error":    maxError,
		"cosine_similarity":     cosingSim,
		"codes":                 codes,
	}, nil
}

// 辅助函数：计算余弦相似度
func cosineSimilarity(v1, v2 []float64) (float64, error) {
	if len(v1) != len(v2) {
		return 0, fmt.Errorf("vectors must have same dimension")
	}

	var dotProduct, norm1, norm2 float64

	for i := 0; i < len(v1); i++ {
		dotProduct += v1[i] * v2[i]
		norm1 += v1[i] * v1[i]
		norm2 += v2[i] * v2[i]
	}

	norm1 = math.Sqrt(norm1)
	norm2 = math.Sqrt(norm2)

	if norm1 == 0 || norm2 == 0 {
		return 0, nil // 零向量的余弦相似度定义为0
	}

	return dotProduct / (norm1 * norm2), nil
}

// 辅助函数：计算均方误差
func meanSquaredError(v1, v2 []float64) float64 {
	if len(v1) != len(v2) {
		return math.MaxFloat64
	}

	var sum float64
	for i := 0; i < len(v1); i++ {
		diff := v1[i] - v2[i]
		sum += diff * diff
	}
	return sum / float64(len(v1))
}

// 辅助函数：计算平均绝对误差
func meanAbsoluteError(v1, v2 []float64) float64 {
	if len(v1) != len(v2) {
		return math.MaxFloat64
	}

	var sum float64
	for i := 0; i < len(v1); i++ {
		sum += math.Abs(v1[i] - v2[i])
	}
	return sum / float64(len(v1))
}

// 辅助函数：计算最大绝对误差
func maxAbsoluteError(v1, v2 []float64) float64 {
	if len(v1) != len(v2) {
		return math.MaxFloat64
	}

	var maxErr float64
	for i := 0; i < len(v1); i++ {
		err := math.Abs(v1[i] - v2[i])
		if err > maxErr {
			maxErr = err
		}
	}
	return maxErr
}
