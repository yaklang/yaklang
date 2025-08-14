package embedding

import (
	"errors"
	"math"
	"math/rand"
)

// PCA2D 对一组向量执行 PCA，将维度降至 2。
// - inputs: 形状约为 [numSamples][numDims] 的向量集合，允许轻度非规则行（会截断为最短列数）
// - maxIter: 幂迭代最大轮数（建议 100~300）
// - tol: 幂迭代收敛阈值（如 1e-6）
// 返回形状为 [numSamples][2] 的二维坐标
func PCA2D(inputs [][]float32, maxIter int, tol float64) ([][]float32, error) {
	n := len(inputs)
	if n == 0 {
		return [][]float32{}, nil
	}
	d := minRowLen(inputs)
	if d == 0 {
		return nil, errors.New("empty dimension after alignment")
	}
	if n == 1 {
		// 单样本，直接返回 [0,0]
		return [][]float32{{0, 0}}, nil
	}
	if d == 1 {
		// 单维数据，第二主成分不存在，复制为 [x, 0]
		xs := make([][]float32, n)
		colMean := columnMean1D(inputs, 0, d)
		for i := 0; i < n; i++ {
			v := getPrefixAs64(inputs[i], d)
			xs[i] = []float32{float32(v[0] - colMean), 0}
		}
		return xs, nil
	}

	// 构造中心化后的矩阵 X (n x d)，使用 float64 做数值计算
	X := make([][]float64, n)
	means := make([]float64, d)
	for j := 0; j < d; j++ {
		means[j] = columnMean1D(inputs, j, d)
	}
	for i := 0; i < n; i++ {
		row := make([]float64, d)
		for j := 0; j < d; j++ {
			row[j] = float64Safe(inputs[i], j) - means[j]
		}
		X[i] = row
	}

	// 通过幂迭代在 A = X^T X 上求前两特征向量（即主轴）
	// 计算第一主成分
	v1 := powerIterationXT_X(X, nil, maxIter, tol)
	if l2 := vectorL2(v1); l2 < 1e-12 {
		// 数据可能全 0 或接近常量
		out := make([][]float32, n)
		for i := 0; i < n; i++ {
			out[i] = []float32{0, 0}
		}
		return out, nil
	}
	normalizeInPlace(v1)

	// 计算第二主成分，并对第一主成分做 Gram-Schmidt 正交
	v2 := powerIterationXT_X(X, v1, maxIter, tol)
	// 与 v1 正交化
	proj := dot64(v2, v1)
	for j := 0; j < len(v2); j++ {
		v2[j] -= proj * v1[j]
	}
	if l2 := vectorL2(v2); l2 < 1e-12 {
		// 数据基本在一条线上
		v2 = make([]float64, d)
	} else {
		normalizeInPlace(v2)
	}

	// 将样本投影到两个主成分上，得到二维坐标 Y = X * [v1 v2]
	out := make([][]float32, n)
	var minX, minY float64
	minX, minY = math.Inf(1), math.Inf(1)
	for i := 0; i < n; i++ {
		p1 := dotRowCol(X[i], v1)
		p2 := dotRowCol(X[i], v2)
		if p1 < minX {
			minX = p1
		}
		if p2 < minY {
			minY = p2
		}
		out[i] = []float32{float32(p1), float32(p2)}
	}

	// 平移到非负坐标（如果存在负值）
	shiftX, shiftY := 0.0, 0.0
	if minX < 0 {
		shiftX = -minX
	}
	if minY < 0 {
		shiftY = -minY
	}
	if shiftX != 0 || shiftY != 0 {
		for i := 0; i < n; i++ {
			out[i][0] = float32(float64(out[i][0]) + shiftX)
			out[i][1] = float32(float64(out[i][1]) + shiftY)
		}
	}
	return out, nil
}

// ReduceTo2D 是 PCA2D 的别名，提供更直观的名称。
func ReduceTo2D(inputs [][]float32) ([][]float32, error) {
	return PCA2D(inputs, 200, 1e-6)
}

// powerIterationXT_X 在不显式构造 X^T X 的情况下，对称幂迭代求解 A = X^T X 的主特征向量。
// 如果 ref 不为空，则在每次迭代后对向量做一次与 ref 的正交化（用于求解第二主成分）。
func powerIterationXT_X(X [][]float64, ref []float64, maxIter int, tol float64) []float64 {
	d := 0
	if len(X) > 0 {
		d = len(X[0])
	}
	if d == 0 {
		return []float64{}
	}
	v := make([]float64, d)
	// 可重复的随机初始化，避免退化
	rnd := rand.New(rand.NewSource(42))
	for j := 0; j < d; j++ {
		v[j] = rnd.NormFloat64()
	}
	normalizeInPlace(v)

	if maxIter <= 0 {
		maxIter = 200
	}
	if tol <= 0 {
		tol = 1e-6
	}

	prev := make([]float64, d)
	for it := 0; it < maxIter; it++ {
		copy(prev, v)
		// w = (X^T X) v = X^T (X v)
		w := matVecXT_X(X, v)
		if ref != nil && len(ref) == len(w) {
			// 与参考向量正交化
			proj := dot64(w, ref)
			for j := 0; j < len(w); j++ {
				w[j] -= proj * ref[j]
			}
		}
		normalizeInPlace(w)
		v = w
		// 检查收敛
		if vecDiffL2(prev, v) < tol {
			break
		}
	}
	return v
}

// matVecXT_X 计算 (X^T X) v，但不显式构造 A = X^T X。
// 分两步：t = X v (O(n*d))，然后 u = X^T t (O(n*d))，返回 u。
func matVecXT_X(X [][]float64, v []float64) []float64 {
	n := len(X)
	if n == 0 {
		return []float64{}
	}
	d := len(X[0])
	t := make([]float64, n)
	for i := 0; i < n; i++ {
		t[i] = dotRowCol(X[i], v)
	}
	u := make([]float64, d)
	for j := 0; j < d; j++ {
		var s float64
		for i := 0; i < n; i++ {
			s += X[i][j] * t[i]
		}
		u[j] = s
	}
	return u
}

func dotRowCol(row []float64, col []float64) float64 {
	var s float64
	m := minInt(len(row), len(col))
	for i := 0; i < m; i++ {
		s += row[i] * col[i]
	}
	return s
}

func dot64(a, b []float64) float64 {
	var s float64
	m := minInt(len(a), len(b))
	for i := 0; i < m; i++ {
		s += a[i] * b[i]
	}
	return s
}

func normalizeInPlace(v []float64) {
	l2 := vectorL2(v)
	if l2 == 0 {
		return
	}
	inv := 1.0 / l2
	for i := 0; i < len(v); i++ {
		v[i] *= inv
	}
}

func vectorL2(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x * x
	}
	return math.Sqrt(s)
}

func vecDiffL2(a, b []float64) float64 {
	m := minInt(len(a), len(b))
	var s float64
	for i := 0; i < m; i++ {
		d := a[i] - b[i]
		s += d * d
	}
	return math.Sqrt(s)
}

func minRowLen(inputs [][]float32) int {
	if len(inputs) == 0 {
		return 0
	}
	minL := len(inputs[0])
	for i := 1; i < len(inputs); i++ {
		if l := len(inputs[i]); l < minL {
			minL = l
		}
	}
	return minL
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func float64Safe(row []float32, j int) float64 {
	if j < len(row) {
		return float64(row[j])
	}
	return 0
}

func getPrefixAs64(row []float32, d int) []float64 {
	out := make([]float64, d)
	m := minInt(len(row), d)
	for j := 0; j < m; j++ {
		out[j] = float64(row[j])
	}
	for j := m; j < d; j++ {
		out[j] = 0
	}
	return out
}

func columnMean1D(inputs [][]float32, col int, d int) float64 {
	var s float64
	var cnt float64
	for i := 0; i < len(inputs); i++ {
		if col < len(inputs[i]) && col < d {
			s += float64(inputs[i][col])
			cnt += 1
		}
	}
	if cnt == 0 {
		return 0
	}
	return s / cnt
}
