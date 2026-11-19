package test

import "testing"

func TestLuaVariables(t *testing.T) {
	t.Run("local variables", func(t *testing.T) {
		checkValues(t, `
local a = 1
local b = "two"
println(a)
println(b)
`, "1", "\"two\"")
	})

	t.Run("assignment updates value", func(t *testing.T) {
		checkValues(t, `
local a = 1
a = 2
println(a)
`, "2")
	})

	t.Run("multiple assignment", func(t *testing.T) {
		checkValues(t, `
local a, b = 1, 2
println(a)
println(b)
`, "1", "2")
	})
}

func TestLuaBasicLiterals(t *testing.T) {
	t.Run("nil and booleans", func(t *testing.T) {
		checkPrintlnValue(t, `
println(nil)
println(true)
println(false)
`, "nil", "true", "false")
	})

	t.Run("table field assignment", func(t *testing.T) {
		checkPrintlnValue(t, `
local t = {x = 1}
t.x = 5
println(t.x)
`, "5")
	})

	t.Run("table index assignment", func(t *testing.T) {
		checkPrintlnValue(t, `
local t = {1, 2}
t[1] = 7
println(t[1])
`, "7")
	})

	t.Run("nested table", func(t *testing.T) {
		checkPrintlnValue(t, `
local t = {inner = {value = 3}}
println(t.inner.value)
`, "3")
	})
}
