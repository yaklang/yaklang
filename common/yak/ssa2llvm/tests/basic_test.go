package tests

import (
	"testing"
)

func TestBasicArithmetic(t *testing.T) {
	// Add
	check(t, `check = () => { return 10 + 20 }`, 30)

	// Sub
	check(t, `check = () => { return 20 - 8 }`, 12)

	// Mul
	check(t, `check = () => { return 6 * 7 }`, 42)

	// Div
	check(t, `check = () => { return 100 / 5 }`, 20)

	// Mod (if supported) - assumes LLVM compiler handles OpMod
	// check(t, `check = () => { return 10 % 3 }`, 1)
}

func TestComplexExpressions(t *testing.T) {
	// (a + b) * c
	check(t, `check = () => { return (10 + 5) * 2 }`, 30)

	// Operator precedence
	check(t, `check = () => { return 10 + 5 * 2 }`, 20)

	// Negative numbers (if unary minus supported/parsed)
	// check(t, `check = () => { return -5 + 10 }`, 5)
}
