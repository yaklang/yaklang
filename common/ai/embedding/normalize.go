package embedding

import (
	"math"
)

// 公式：out = x / max(||x||_p, eps)
// - p: 范数阶，常用 2（L2）、1（L1）；支持 +Inf（按最大绝对值）
// - eps: 为避免除零引入的微小常数
// 返回新切片，不修改入参
func NormalizeVector(input []float32, p float64, eps float64) []float32 {
	if len(input) == 0 {
		return []float32{}
	}

	norm := computePNorm1D(input, p)
	if norm < eps {
		norm = eps
	}

	out := make([]float32, len(input))
	inv := 1.0 / norm
	for i, v := range input {
		out[i] = float32(float64(v) * inv)
	}
	return out
}

// 当 dim == 0 时，按列归一化；当 dim == 1 时，按行归一化（与 Torch 默认使用最常见用法一致）。
// - p: 范数阶，常用 2（L2）、1（L1）；支持 +Inf（按最大绝对值）
// - eps: 为避免除零引入的微小常数
// 返回新矩阵，不修改入参
func NormalizeMatrix(input [][]float32, p int, dim int) [][]float32 {
	// 兼容：允许 p 作为 int 传入（更贴合 Go 使用习惯），内部转为 float64
	return normalizeMatrixInternal(input, float64(p), dim, 1e-6)
}

func normalizeMatrixInternal(input [][]float32, p float64, dim int, eps float64) [][]float32 {
	rows := len(input)
	if rows == 0 {
		return [][]float32{}
	}
	// 复制形状
	out := make([][]float32, rows)
	for i := range input {
		if input[i] == nil {
			out[i] = nil
			continue
		}
		out[i] = make([]float32, len(input[i]))
		copy(out[i], input[i])
	}

	if dim == 0 { // 按列规范化
		// 找到最大列数，允许轻度非规则矩阵
		maxCols := 0
		for i := 0; i < rows; i++ {
			if l := len(input[i]); l > maxCols {
				maxCols = l
			}
		}
		if maxCols == 0 {
			return out
		}

		// 为每一列计算范数（跳过短行缺失的列）
		norms := make([]float64, maxCols)
		for c := 0; c < maxCols; c++ {
			var colVals []float32
			for r := 0; r < rows; r++ {
				if c < len(input[r]) {
					colVals = append(colVals, input[r][c])
				}
			}
			n := computePNorm1D(colVals, p)
			if n < eps {
				n = eps
			}
			norms[c] = n
		}

		// 归一化
		for r := 0; r < rows; r++ {
			for c := 0; c < len(out[r]); c++ {
				out[r][c] = float32(float64(out[r][c]) / norms[c])
			}
		}
		return out
	}

	// 按行规范化（dim == 1）
	for r := 0; r < rows; r++ {
		n := computePNorm1D(input[r], p)
		if n < eps {
			n = eps
		}
		inv := 1.0 / n
		for c := 0; c < len(out[r]); c++ {
			out[r][c] = float32(float64(out[r][c]) * inv)
		}
	}
	return out
}

func computePNorm1D(vec []float32, p float64) float64 {
	if len(vec) == 0 {
		return 0
	}
	if math.IsInf(p, 1) {
		var maxAbs float64
		for _, v := range vec {
			av := math.Abs(float64(v))
			if av > maxAbs {
				maxAbs = av
			}
		}
		return maxAbs
	}
	if p <= 0 {
		// 不合法 p，回退为 2 范数
		p = 2
	}
	var sum float64
	for _, v := range vec {
		sum += math.Pow(math.Abs(float64(v)), p)
	}
	return math.Pow(sum, 1.0/p)
}
