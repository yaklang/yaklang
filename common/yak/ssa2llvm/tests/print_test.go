package tests

import "testing"

func TestPrint_Simple(t *testing.T) {
	checkPrint(t, `
	check = () => {
		println(12345)
	}
	`, 12345)
}

func TestPrint_Calculated(t *testing.T) {
	checkPrint(t, `
		println(10 + 20 * 3)
	`, 70)
}

func TestPrint_Variables(t *testing.T) {
	checkPrint(t, `
		a = 100
		b = 55
		println(a + b)
	`, 155)
}

func TestPrint_Multiple(t *testing.T) {
	checkPrint(t, `
	check = () => {
		println(10)
		println(20)
		println(30)
		return 0
	}
	`, 10, 20, 30)
}
