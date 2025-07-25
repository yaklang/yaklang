package utils

import (
	"fmt"
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	// 测试用例1: 两个相似的向量 (方向接近)
	t.Run("similar vectors", func(t *testing.T) {
		vec1 := []float32{1.0, 2.0, 3.0, 4.0}
		vec2 := []float32{2.0, 4.0, 6.0, 8.0} // vec2 是 vec1 的两倍

		similarity, err := CosineSimilarity(vec1, vec2)
		if err != nil {
			t.Fatalf("Error calculating similarity for vec1 and vec2: %v", err)
		}

		// 余弦相似度应该非常接近 1.0
		if math.Abs(float64(similarity-1.0)) > 1e-10 {
			t.Errorf("Expected similarity close to 1.0, got %f", similarity)
		}

		fmt.Printf("Similarity between vec1 and vec2: %f\n", similarity)
	})

	// 测试用例2: 两个正交的向量 (方向垂直)
	t.Run("orthogonal vectors", func(t *testing.T) {
		vec3 := []float32{1.0, 0.0}
		vec4 := []float32{0.0, 1.0}

		similarity, err := CosineSimilarity(vec3, vec4)
		if err != nil {
			t.Fatalf("Error calculating similarity for vec3 and vec4: %v", err)
		}

		// 正交向量的余弦相似度应该为 0.0
		if math.Abs(float64(similarity)) > 1e-10 {
			t.Errorf("Expected similarity to be 0.0, got %f", similarity)
		}

		fmt.Printf("Similarity between vec3 and vec4: %f\n", similarity)
	})

	// 测试用例3: 两个方向相反的向量
	t.Run("opposite vectors", func(t *testing.T) {
		vec5 := []float32{1.0, 1.0, 1.0}
		vec6 := []float32{-1.0, -1.0, -1.0}

		similarity, err := CosineSimilarity(vec5, vec6)
		if err != nil {
			t.Fatalf("Error calculating similarity for vec5 and vec6: %v", err)
		}

		// 方向相反的向量余弦相似度应该为 -1.0
		if math.Abs(float64(similarity+1.0)) > 1e-10 {
			t.Errorf("Expected similarity to be -1.0, got %f", similarity)
		}

		fmt.Printf("Similarity between vec5 and vec6: %f\n", similarity)
	})

	// 测试用例4: 错误处理 - 向量长度不同
	t.Run("different length vectors", func(t *testing.T) {
		vec7 := []float32{1.0, 2.0}
		vec8 := []float32{1.0, 2.0, 3.0}

		_, err := CosineSimilarity(vec7, vec8)
		if err == nil {
			t.Errorf("Expected error for different length vectors, but got nil")
		} else {
			fmt.Printf("Error (as expected) for different length vectors: %v\n", err)
		}
	})

	// 测试用例5: 包含零向量的情况
	t.Run("zero vector", func(t *testing.T) {
		vec9 := []float32{0.0, 0.0, 0.0}
		vec10 := []float32{5.0, 6.0, 7.0}

		similarity, err := CosineSimilarity(vec9, vec10)
		if err != nil {
			t.Fatalf("Error calculating similarity for vec9 and vec10: %v", err)
		}

		// 零向量与任何向量的余弦相似度都为 0.0
		if math.Abs(float64(similarity)) > 1e-10 {
			t.Errorf("Expected similarity to be 0.0, got %f", similarity)
		}

		fmt.Printf("Similarity between a zero vector and another vector: %f\n", similarity)
	})

	// 附加测试：相同向量
	t.Run("identical vectors", func(t *testing.T) {
		vec11 := []float32{3.0, 4.0, 5.0}

		similarity, err := CosineSimilarity(vec11, vec11)
		if err != nil {
			t.Fatalf("Error calculating similarity for identical vectors: %v", err)
		}

		// 相同向量的余弦相似度应为 1.0
		if math.Abs(float64(similarity-1.0)) > 1e-10 {
			t.Errorf("Expected similarity to be 1.0, got %f", similarity)
		}

		fmt.Printf("Similarity between identical vectors: %f\n", similarity)
	})
}

// 添加对 dotProduct 和 magnitude 函数的单元测试
func TestDotProduct(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{4, 5, 6}

	// 手动计算结果：1*4 + 2*5 + 3*6 = 4 + 10 + 18 = 32
	expected := float32(32.0)

	result, err := dotProduct(a, b)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result != expected {
		t.Errorf("Expected dot product to be %f, got %f", expected, result)
	}

	// 测试长度不同的情况
	c := []float32{1, 2}
	_, err = dotProduct(a, c)
	if err == nil {
		t.Errorf("Expected error for vectors with different lengths")
	}
}

func TestMagnitude(t *testing.T) {
	vec := []float32{3, 4}

	// 手动计算结果：sqrt(3^2 + 4^2) = sqrt(9 + 16) = sqrt(25) = 5
	expected := float32(5.0)

	result := magnitude(vec)
	if result != expected {
		t.Errorf("Expected magnitude to be %f, got %f", expected, result)
	}

	// 测试零向量
	zeroVec := []float32{0, 0, 0}
	result = magnitude(zeroVec)
	if result != 0 {
		t.Errorf("Expected magnitude of zero vector to be 0, got %f", result)
	}
}

// 基准测试，用于性能评估
func BenchmarkCosineSimilarity(b *testing.B) {
	vec1 := []float32{1.0, 2.0, 3.0, 4.0, 5.0}
	vec2 := []float32{5.0, 4.0, 3.0, 2.0, 1.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = CosineSimilarity(vec1, vec2)
	}
}
