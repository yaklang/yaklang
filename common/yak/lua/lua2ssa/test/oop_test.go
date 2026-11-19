package test

import "testing"

func TestLuaTablesAndMethods(t *testing.T) {
	t.Run("table constructor field", func(t *testing.T) {
		checkValues(t, `
local point = {x = 1, y = 2}
println(point.x)
println(point.y)
`, "1", "2")
	})

	t.Run("table function field", func(t *testing.T) {
		checkValuesContain(t, `
local obj = {value = 10}
function obj:get()
    return self.value
end
println(obj:get())
`, "obj.get")
	})

	t.Run("table assignment through field", func(t *testing.T) {
		checkValues(t, `
local box = {value = 1}
box.value = 7
println(box.value)
`, "7")
	})

	t.Run("metatable call is visible", func(t *testing.T) {
		checkSymbols(t, `
local base = {x = 1}
local child = setmetatable({}, {__index = base})
println(child.x)
`, "setmetatable", "base", "child")
	})
}
