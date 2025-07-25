package utils

import (
	"fmt"
	"math"
)

// dotProduct 计算两个向量的点积
// A · B = Σ(Ai * Bi)
func dotProduct(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector lengths are not equal: %d != %d", len(a), len(b))
	}
	var sum float32
	for i := 0; i < len(a); i++ {
		sum += a[i] * b[i]
	}
	return sum, nil
}

// magnitude 计算向量的欧几里得范数（模长）
// ||A|| = sqrt(Σ(Ai^2))
func magnitude(vec []float32) float32 {
	var sumOfSquares float32
	for _, val := range vec {
		sumOfSquares += val * val
	}
	return float32(math.Sqrt(float64(sumOfSquares)))
}

// CosineSimilarity 计算两个向量的余弦相似度
// 返回值在 [-1, 1] 之间。越接近1，表示越相似。
func CosineSimilarity(a, b []float32) (float32, error) {
	// 1. 计算点积
	dot, err := dotProduct(a, b)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate dot product: %w", err)
	}

	// 2. 计算各自的模长
	magA := magnitude(a)
	magB := magnitude(b)

	// 3. 处理边界情况：如果任一向量的模长为0，则相似度为0
	//    这是为了避免除以零的错误。一个零向量与任何向量都不相似。
	if magA == 0 || magB == 0 {
		return 0, nil
	}

	// 4. 计算余弦相似度
	similarity := dot / (magA * magB)
	return similarity, nil
}
