package ssdeep

import "testing"

var h1 = "192:MUPMinqP6+wNQ7Q40L/iB3n2rIBrP0GZKF4jsef+0FVQLSwbLbj41iH8nFVYv980:x0CllivQiFmt"

var h2 = "192:JkjRcePWsNVQza3ntZStn5VfsoXMhRD9+xJMinqF6+wNQ7Q40L/i737rPVt:JkjlQyIrx+kll2"

var h3 = "196608:pDSC8olnoL1v/uawvbQD7XlZUFYzYyMb615NktYHF7dREN/JNnQrmhnUPI+/n2Yr:5DHoJXv7XOq7Mb2TwYHXREN/3QrmktPd"

var h4 = "196608:7DSC8olnoL1v/uawvbQD7XlZUFYzYyMb615NktYHF7dREN/JNnQrmhnUPI+/n2Y7:3DHoJXv7XOq7Mb2TwYHXREN/3QrmktPt"

func assertDistanceEqual(t *testing.T, expected, actual int) {
	if expected != actual {
		t.Fatalf("Distance mismatch: %d (expected)\n"+
			"                != %d (actual)", expected, actual)
	}
}

func TestHashDistanceSame(t *testing.T) {
	d, err := Distance(h1, h1)
	assertNoError(t, err)
	assertDistanceEqual(t, 100, d)
}

func TestHashDistance1(t *testing.T) {
	d, err := Distance(h1, h2)
	assertNoError(t, err)
	assertDistanceEqual(t, 35, d)
}

func TestHashDistance2(t *testing.T) {
	d, err := Distance(h3, h4)
	assertNoError(t, err)
	assertDistanceEqual(t, 97, d)
}

func TestEmptyHash1(t *testing.T) {
	d, err := Distance("", h2)
	assertError(t, err)
	assertDistanceEqual(t, 0, d)
}

func TestEmptyHash2(t *testing.T) {
	d, err := Distance(h1, "")
	assertError(t, err)
	if d != 0 {
		t.Errorf("hash2 is nil: %d", d)
	}
}

func TestInvalidHash1(t *testing.T) {
	d, err := Distance("192:asdasd", h1)
	assertError(t, err)

	if d != 0 {
		t.Errorf("hash1 and hash2 are nil: %d", d)
	}
}

func TestInvalidHash2(t *testing.T) {
	d, err := Distance(h1, "asd:asdasd:aaaa")
	assertError(t, err)
	if d != 0 {
		t.Errorf("hash1 and hash2 are nil: %d", d)
	}
}

func BenchmarkDistance(b *testing.B) {
	var h1 = `7DSC8olnoL1v/uawvbQD7XlZUFYzYyMb615NktYHF7dREN/JNnQrmhnUPI+/n2Y7`
	var h2 = `7DSC8olnoL1v/uawvbQD7XlZUFYzYyMb615NktYHF7dREN/JNnQrmhnUPI+/ngrr`
	for i := 0; i < b.N; i++ {
		distance(h1, h2)
	}
}
