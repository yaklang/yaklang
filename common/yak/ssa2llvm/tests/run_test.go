package tests

import "testing"

func TestRun_Smoke(t *testing.T) {
	checkBinaryEx(t, `check = () => { return 42 }`, "check", "yak", 42)
	checkBinaryEx(t, `
add = (a, b) => { return a + b }
check = () => { return add(10, 20) }
`, "check", "yak", 30)
	checkBinaryEx(t, `
check = () => {
	a = 10
	if a > 5 {
		return 1
	}
	return 0
}
`, "check", "yak", 1)
}
