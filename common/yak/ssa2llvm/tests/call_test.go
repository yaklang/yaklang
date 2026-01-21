package tests

import (
	"testing"
)

func TestCall_Simple(t *testing.T) {
	check(t, `
	add = (a, b) => { return a + b }
	check = () => { return add(100, 200) }
	`, 300)
}

func TestCall_Nested(t *testing.T) {
	check(t, `
	sq = (x) => { return x * x }
	check = () => { return sq(5) + sq(4) } // 25 + 16 = 41
	`, 41)
}

func TestCall_Recursive_Fib(t *testing.T) {
	// Fibonacci: 1, 1, 2, 3, 5, 8, 13, 21, 34, 55
	// fib(10) = 55
	check(t, `
	fib = (n) => {
		if n <= 2 {
			return 1
		}
		return fib(n-1) + fib(n-2)
	}
	check = () => { return fib(10) }
	`, 55)
}

func TestCall_MultipleArgs(t *testing.T) {
	check(t, `
	sum3 = (a, b, c) => { return a + b + c }
	check = () => { return sum3(10, 20, 30) }
	`, 60)
}
