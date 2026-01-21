package main

func fib(n int) int {
	if n <= 2 {
		return 1
	}
	return fib(n-1) + fib(n-2)
}

func factorial(n int) int {
	result := 1
	for i := 1; i <= n; i++ {
		result = result * i
	}
	return result
}

func sumRange(start, end int) int {
	sum := 0
	for i := start; i <= end; i++ {
		sum = sum + i
	}
	return sum
}

func check() int {
	a := fib(7)
	b := factorial(5)
	c := sumRange(1, 10)
	return a + b + c
}
