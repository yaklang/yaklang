package test

import "testing"

func TestKotlinFunctions(t *testing.T) {
	t.Run("parameters", func(t *testing.T) {
		checkValues(t, `
fun add(a: Int, b: Int) {
    println(a)
    println(b)
}
fun main() {
    add(1, 2)
}
`, "Parameter-a", "Parameter-b")
	})

	t.Run("function symbols", func(t *testing.T) {
		checkSymbols(t, `
fun helper(a: Int): Int {
    return a
}
fun main() {
    helper(1)
}
`, "helper", "main")
	})

	t.Run("return value", func(t *testing.T) {
		checkValuesContain(t, `
fun identity(a: Int): Int {
    return a
}
fun main() {
    println(identity(7))
}
`, "identity", "7")
	})

	t.Run("expression body", func(t *testing.T) {
		checkValuesContain(t, `
fun add(a: Int, b: Int): Int = a + b
fun main() {
    println(add(1, 2))
}
`, "add", "1", "2")
	})

	t.Run("recursive function", func(t *testing.T) {
		checkSymbols(t, `
fun fact(n: Int): Int {
    if (n <= 1) {
        return 1
    }
    return n * fact(n - 1)
}
fun main() {
    println(fact(3))
}
`, "fact", "main")
	})

	t.Run("local function", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    fun local(a: Int): Int {
        return a
    }
    println(local(1))
}
`, "local")
	})

	t.Run("lambda expression", func(t *testing.T) {
		checkSymbols(t, `
fun main() {
    val f = { x: Int -> x + 1 }
    println(f(1))
}
`, "main", "f")
	})
}
