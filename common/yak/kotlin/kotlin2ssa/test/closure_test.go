package test

import "testing"

func TestKotlinClosures(t *testing.T) {
	t.Run("captured local is a free value", func(t *testing.T) {
		checkFreeValues(t, `
fun main() {
    val n = 10
    val target = { x: Int -> x + n }
    println(target(1))
}
`, "n")
	})

	t.Run("nested closure captures outer parameter", func(t *testing.T) {
		checkFreeValues(t, `
fun outer(a: Int): () -> Int {
    val target = { a }
    return target
}
fun main() {
    val f = outer(3)
    println(f())
}
`, "a")
	})

	t.Run("anonymous function closes over value", func(t *testing.T) {
		checkFreeValues(t, `
fun main() {
    val n = 2
    val target = fun(x: Int): Int { return x * n }
    println(target(3))
}
`, "n")
	})
}
