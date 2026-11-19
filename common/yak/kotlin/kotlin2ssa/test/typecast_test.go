package test

import "testing"

func TestKotlinTypeCast(t *testing.T) {
	t.Run("as uses typecast", func(t *testing.T) {
		checkTypeCast(t, `
fun main() {
    val a = 1
    val target = a as Long
    println(target)
}
`, "target")
	})

	t.Run("safe as uses typecast", func(t *testing.T) {
		checkTypeCast(t, `
fun main() {
    val a = 1
    val target = a as? Long
    println(target)
}
`, "target")
	})

	t.Run("typecast keeps source operand", func(t *testing.T) {
		checkPrintlnValueContain(t, `
fun main() {
    val a = 1
    val target = a as Long
    println(target)
}
`, "castType(number", "1")
	})
}
