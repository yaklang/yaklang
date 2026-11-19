package test

import "testing"

func TestLuaClosures(t *testing.T) {
	t.Run("captured local is a free value", func(t *testing.T) {
		checkFreeValues(t, `
local target
function make()
    local n = 10
    target = function(x)
        return x + n
    end
end
make()
println(target(1))
`, "n")
	})

	t.Run("nested closure captures outer parameter", func(t *testing.T) {
		checkFreeValues(t, `
local target
function outer(a)
    target = function()
        return a
    end
end
outer(3)
println(target())
`, "a")
	})

	t.Run("closure updates captured variable", func(t *testing.T) {
		checkFreeValues(t, `
local target
local function counter()
    local n = 0
    target = function()
        n = n + 1
        return n
    end
end
counter()
println(target())
`, "n")
	})
}
