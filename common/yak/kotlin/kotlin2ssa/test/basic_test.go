package test

import "testing"

func TestKotlinVariables(t *testing.T) {
	t.Run("val and var", func(t *testing.T) {
		checkValues(t, `
fun main() {
    val a = 1
    var b = "two"
    println(a)
    println(b)
}
`, "1", "\"two\"")
	})

	t.Run("assignment updates value", func(t *testing.T) {
		checkValues(t, `
fun main() {
    var a = 1
    a = 2
    println(a)
}
`, "2")
	})

	t.Run("local declaration", func(t *testing.T) {
		checkValues(t, `
fun main() {
    val a = 1
    val b = a + 1
    println(b)
}
`, "2")
	})
}
