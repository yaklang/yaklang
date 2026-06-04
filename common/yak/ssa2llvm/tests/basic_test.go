package tests

import (
	"testing"
)

func TestArithmeticExpressions(t *testing.T) {
	checkBinaryEx(t, `
check = () => {
	if (10 + 20) != 30 { return 1 }
	if (20 - 8) != 12 { return 2 }
	if (6 * 7) != 42 { return 3 }
	if (100 / 5) != 20 { return 4 }
	if ((10 + 5) * 2) != 30 { return 5 }
	if (10 + 5 * 2) != 20 { return 6 }
	return 0
}
`, "check", "yak", 0)
}
