package test

import "testing"

func TestLuaExpressions(t *testing.T) {
	t.Run("integer arithmetic", func(t *testing.T) {
		checkValues(t, `
println(1 + 2)
println(7 - 3)
println(2 * 3)
println(8 / 2)
println(7 % 3)
`, "3", "4", "6", "4", "1")
	})

	t.Run("unary operators", func(t *testing.T) {
		checkValues(t, `
println(-1)
println(not false)
`, "-1", "true")
	})

	t.Run("comparison operators", func(t *testing.T) {
		checkValues(t, `
println(1 < 2)
println(2 <= 2)
println(3 == 3)
println(3 ~= 4)
println(4 > 3)
`, "true", "true", "true", "true", "true")
	})

	t.Run("logical operators", func(t *testing.T) {
		checkValues(t, `
println(true and false)
println(false or true)
`, "false", "true")
	})

	t.Run("bitwise operators", func(t *testing.T) {
		checkValues(t, `
println(1 | 2)
println(7 & 3)
println(1 << 2)
println(8 >> 1)
`, "3", "3", "4", "4")
	})

	t.Run("string concatenation", func(t *testing.T) {
		checkValuesContain(t, `
println("a" .. "b" .. "c")
`, "abc")
	})

	t.Run("string length", func(t *testing.T) {
		checkValues(t, `
println(#"abcd")
`, "4")
	})

	t.Run("boolean and nil literals", func(t *testing.T) {
		checkValuesContain(t, `
println(true)
println(false)
println(nil)
`, "true", "false", "nil")
	})

	t.Run("table field access", func(t *testing.T) {
		checkValues(t, `
local t = {x = 42, y = 7}
println(t.x)
`, "42")
	})

	t.Run("table array access", func(t *testing.T) {
		checkValues(t, `
local t = {10, 20, 30}
println(t[2])
`, "20")
	})

	t.Run("hexadecimal literal", func(t *testing.T) {
		checkValues(t, `
println(0xFF)
`, "255")
	})

	t.Run("long string literal", func(t *testing.T) {
		checkValuesContain(t, `
println([[long
string]])
`, "long")
	})
}
