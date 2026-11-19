package test

import "testing"

func TestLuaTypeCast(t *testing.T) {
	t.Run("tonumber uses typecast", func(t *testing.T) {
		checkTypeCast(t, `
local a = "42"
local target = tonumber(a)
println(target)
`, "target")
	})

	t.Run("tostring uses typecast", func(t *testing.T) {
		checkTypeCast(t, `
local a = 42
local target = tostring(a)
println(target)
`, "target")
	})

	t.Run("typecast keeps source operand", func(t *testing.T) {
		checkPrintlnValueContain(t, `
local a = "42"
local target = tonumber(a)
println(target)
`, "castType(number", "42")
	})
}
