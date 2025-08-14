package embedding

import (
	"math"
	"testing"
)

func nearlyEqual(a, b []float32, tol float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > tol {
			return false
		}
	}
	return true
}

func TestNormalizeVectorL2(t *testing.T) {
	vec := []float32{3, 4}
	out := NormalizeVector(vec, 2, 1e-12)
	expect := []float32{0.6, 0.8}
	if !nearlyEqual(out, expect, 1e-6) {
		t.Fatalf("L2 normalize failed: got %v expect %v", out, expect)
	}
}

func TestNormalizeVectorL1(t *testing.T) {
	vec := []float32{1, 2, 3}
	out := NormalizeVector(vec, 1, 1e-12)
	expect := []float32{1.0 / 6.0, 2.0 / 6.0, 3.0 / 6.0}
	if !nearlyEqual(out, expect, 1e-6) {
		t.Fatalf("L1 normalize failed: got %v expect %v", out, expect)
	}
}

func TestNormalizeVectorInf(t *testing.T) {
	vec := []float32{1, -3, 2}
	out := NormalizeVector(vec, math.Inf(1), 1e-12)
	expect := []float32{1.0 / 3.0, -1.0, 2.0 / 3.0}
	if !nearlyEqual(out, expect, 1e-6) {
		t.Fatalf("Inf-norm normalize failed: got %v expect %v", out, expect)
	}
}

func TestNormalizeVectorZeroWithEps(t *testing.T) {
	vec := []float32{0, 0}
	out := NormalizeVector(vec, 2, 1e-6)
	expect := []float32{0, 0}
	if !nearlyEqual(out, expect, 1e-12) {
		t.Fatalf("zero vector normalize failed: got %v expect %v", out, expect)
	}
}

func TestNormalizeMatrixDim1_RowWise(t *testing.T) {
	m := [][]float32{{3, 4}, {0, 0}}
	out := NormalizeMatrix(m, 2, 1)
	expect := [][]float32{{0.6, 0.8}, {0, 0}}
	for i := range expect {
		if !nearlyEqual(out[i], expect[i], 1e-6) {
			t.Fatalf("row %d failed: got %v expect %v", i, out[i], expect[i])
		}
	}
}

func TestNormalizeMatrixDim0_ColWise(t *testing.T) {
	m := [][]float32{{3, 0}, {4, 0}}
	out := NormalizeMatrix(m, 2, 0)
	expect := [][]float32{{0.6, 0}, {0.8, 0}}
	for i := range expect {
		if !nearlyEqual(out[i], expect[i], 1e-6) {
			t.Fatalf("row %d failed: got %v expect %v", i, out[i], expect[i])
		}
	}
}

func TestNormalizeMatrix_NonRectangular(t *testing.T) {
	m := [][]float32{{1, 2, 2}, {3}}
	out := NormalizeMatrix(m, 2, 1) // row-wise
	// row0 norm = 3, row1 norm = 3
	expect := [][]float32{{1.0 / 3.0, 2.0 / 3.0, 2.0 / 3.0}, {1.0, 0, 0}}
	// align lengths for compare
	for len(out[1]) < len(expect[1]) {
		out[1] = append(out[1], 0)
	}
	for i := range expect {
		if !nearlyEqual(out[i], expect[i], 1e-6) {
			t.Fatalf("row %d failed: got %v expect %v", i, out[i], expect[i])
		}
	}
}
