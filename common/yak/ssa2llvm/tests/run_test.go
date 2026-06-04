package tests

import "testing"

func TestRun_Smoke(t *testing.T) {
	checkBinaryEx(t, `
add = (a, b) => { return a + b }
check = () => {
	if 42 != 42 { return 1 }
	if add(10, 20) != 30 { return 2 }
	a = 10
	if !(a > 5) { return 3 }
	return 0
}
`, "check", "yak", 0)
}
