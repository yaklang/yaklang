package embedding

import (
	"math"
	"math/rand"
	"testing"
)

// 生成低维潜在变量经线性变换后的高维样本，并验证 PCA2D 能保留主要方差。
func TestPCA2D_ExplainedVariance(t *testing.T) {
	seed := int64(12345)
	rnd := rand.New(rand.NewSource(seed))

	numSamples := 600
	dim := 8

	// 构造线性映射 A: dim x 2，将 2 维潜在变量映射到高维
	A := make([][]float64, dim)
	for i := 0; i < dim; i++ {
		A[i] = make([]float64, 2)
		for j := 0; j < 2; j++ {
			A[i][j] = rnd.NormFloat64()
		}
	}

	// 合成数据 X = A * U + 噪声，U ~ N(0, I2)
	inputs := make([][]float32, numSamples)
	for n := 0; n < numSamples; n++ {
		u1 := rnd.NormFloat64()
		u2 := rnd.NormFloat64()
		row := make([]float32, dim)
		for i := 0; i < dim; i++ {
			val := A[i][0]*u1 + A[i][1]*u2 + 0.05*rnd.NormFloat64()
			row[i] = float32(val)
		}
		inputs[n] = row
	}

	Y, err := ReduceTo2D(inputs)
	if err != nil {
		t.Fatalf("ReduceTo2D error: %v", err)
	}
	if len(Y) != numSamples || len(Y[0]) != 2 {
		t.Fatalf("unexpected output shape: got %dx%d", len(Y), len(Y[0]))
	}

	// 计算输入的中心化总方差 Var(X) = (1/N) * sum ||x_i - mean||^2
	means := make([]float64, dim)
	for i := 0; i < dim; i++ {
		var s float64
		for n := 0; n < numSamples; n++ {
			s += float64(inputs[n][i])
		}
		means[i] = s / float64(numSamples)
	}
	var totalVar float64
	for n := 0; n < numSamples; n++ {
		var s float64
		for i := 0; i < dim; i++ {
			d := float64(inputs[n][i]) - means[i]
			s += d * d
		}
		totalVar += s
	}
	totalVar /= float64(numSamples)

	// 计算输出二维的中心化方差 Var(Y)
	yMean := []float64{0, 0}
	for n := 0; n < numSamples; n++ {
		yMean[0] += float64(Y[n][0])
		yMean[1] += float64(Y[n][1])
	}
	yMean[0] /= float64(numSamples)
	yMean[1] /= float64(numSamples)
	var varY float64
	for n := 0; n < numSamples; n++ {
		d0 := float64(Y[n][0]) - yMean[0]
		d1 := float64(Y[n][1]) - yMean[1]
		varY += d0*d0 + d1*d1
	}
	varY /= float64(numSamples)

	// 由于数据本质上 2 维，PCA2D 应保留绝大部分方差
	// 在噪声较小情况下，比例应显著大于 0.7
	ratio := varY / math.Max(totalVar, 1e-12)
	if ratio < 0.7 {
		t.Fatalf("explained variance too low: %.3f (total=%.3f, twoPC=%.3f)", ratio, totalVar, varY)
	}
}
