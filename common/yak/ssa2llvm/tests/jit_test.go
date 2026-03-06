package tests

import "testing"

func TestJIT_Smoke(t *testing.T) {
	checkJIT(t, `check = () => { return 42 }`, 42)
	checkJIT(t, `
add = (a, b) => { return a + b }
check = () => { return add(10, 20) }
`, 30)
	checkJIT(t, `
check = () => {
	a = 10
	if a > 5 {
		return 1
	}
	return 0
}
`, 1)
}
