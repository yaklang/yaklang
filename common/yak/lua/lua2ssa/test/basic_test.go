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
