package test

import "testing"

func TestKotlinExpressions(t *testing.T) {
	t.Run("integer arithmetic", func(t *testing.T) {
		checkValues(t, `
fun main() {
    println(1 + 2)
    println(7 - 3)
    println(2 * 3)
    println(8 / 2)
    println(7 % 3)
}
`, "3", "4", "6", "4", "1")
	})

	t.Run("unary operators", func(t *testing.T) {
		checkValues(t, `
fun main() {
    println(-1)
    println(!false)
}
`, "-1", "true")
	})

	t.Run("comparison operators", func(t *testing.T) {
		checkValues(t, `
fun main() {
    println(1 < 2)
    println(2 <= 2)
    println(3 == 3)
    println(3 != 4)
    println(4 > 3)
}
`, "true", "true", "true", "true", "true")
	})

	t.Run("logical operators", func(t *testing.T) {
		checkValues(t, `
fun main() {
    println(true && false)
    println(false || true)
}
`, "false", "true")
	})

	t.Run("string literal", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    println("hello")
}
`, "hello")
	})

	t.Run("string concatenation", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    println("a" + "b")
}
`, "ab")
	})

	t.Run("boolean and null literals", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    println(true)
    println(false)
    println(null)
}
`, "true", "false", "nil")
	})

	t.Run("hex literal", func(t *testing.T) {
		checkValues(t, `
fun main() {
    println(0xFF)
}
`, "255")
	})

	t.Run("list index", func(t *testing.T) {
		checkValuesContain(t, `
fun main() {
    val list = listOf(10, 20)
    println(list[0])
}
`, "list", "0")
	})

	t.Run("when statement branches", func(t *testing.T) {
		// TODO: value-producing when expressions are not lowered yet; the
		// branch assignment form is covered by TestKotlinCFGLoops/when_branch.
		checkSymbols(t, `
fun main() {
    when (c) {
        1 -> println(10)
        else -> println(20)
    }
}
`, "main", "println")
	})
}

func TestKotlinShortCircuit(t *testing.T) {
	t.Run("false and short circuits", func(t *testing.T) {
		checkPrintlnValueContain(t, `
fun side(): Boolean { return true }
fun main() {
    println(false && side())
}
`, "false")
	})

	t.Run("true or short circuits", func(t *testing.T) {
		checkPrintlnValueContain(t, `
fun side(): Boolean { return false }
fun main() {
    println(true || side())
}
`, "true")
	})
}
