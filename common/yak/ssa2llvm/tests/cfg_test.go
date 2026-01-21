package tests

import (
	"testing"
)

func TestCFG_IfElse(t *testing.T) {
	// Simple If
	check(t, `
	check = () => {
		a = 10
		if a > 5 {
			return 1
		}
		return 0
	}`, 1)

	// If-Else with Phi
	check(t, `
	check = () => {
		a = 10
		b = 20
		if a > b {
			return a
		} else {
			return b
		}
	}`, 20)
}

func TestCFG_NestedIf(t *testing.T) {
	check(t, `
	check = () => {
		a = 10
		b = 5
		if a > 0 {
			if b > 0 {
				return 100
			}
		}
		return 0
	}`, 100)
}

func TestCFG_Loop(t *testing.T) {
	// Simple Loop Sum
	check(t, `
	check = () => {
		sum = 0
		for i = 0; i < 5; i++ {
			sum = sum + i // 0+1+2+3+4 = 10 (if i starts at 0 and increments after)
			             // Wait, standard for loop: init; cond; post.
			             // i=0; sum=0
			             // i=1; sum=1
			             // i=2; sum=3
			             // i=3; sum=6
			             // i=4; sum=10
		}
		return sum
	}`, 10)
}

func TestCFG_LoopWithBreak(t *testing.T) {
	// Loop with early break
	check(t, `
	check = () => {
		sum = 0
		for i = 0; i < 10; i++ {
			if i == 5 {
				break
			}
			sum = sum + 1
		}
		return sum // 5 times (0,1,2,3,4) -> 5
	}`, 5)
}

// Factorial Loop
func TestCFG_FactorialLoop(t *testing.T) {
	check(t, `
	check = () => {
		res = 1
		for i = 1; i <= 5; i++ {
			res = res * i
		}
		return res // 120
	}`, 120)
}
