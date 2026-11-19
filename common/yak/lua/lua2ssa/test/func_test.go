package test

import "testing"

func TestLuaFunctions(t *testing.T) {
	t.Run("function parameters", func(t *testing.T) {
		checkValues(t, `
local function add(a, b)
    println(a)
    println(b)
end
add(1, 2)
`, "Parameter-a", "Parameter-b")
	})

	t.Run("function definitions are exported", func(t *testing.T) {
		checkSymbols(t, `
function main()
end
function helper(a)
    return a
end
main()
helper(1)
`, "main", "helper")
	})

	t.Run("return value is used by caller", func(t *testing.T) {
		checkValuesContain(t, `
function identity(a)
    return a
end
println(identity(7))
`, "identity", "7")
	})

	t.Run("recursive function is called", func(t *testing.T) {
		checkSymbols(t, `
function fact(n)
    if n <= 1 then
        return 1
    end
    return n * fact(n - 1)
end
println(fact(3))
`, "fact")
	})

	t.Run("anonymous function", func(t *testing.T) {
		checkValues(t, `
local f = function(a)
    println(a)
end
f(9)
`, "Parameter-a")
	})

	t.Run("colon method receives self", func(t *testing.T) {
		checkValues(t, `
local t = {x = 1}
function t:get()
    println(self)
end
t:get()
`, "Parameter-self")
	})

	t.Run("varargs parameter", func(t *testing.T) {
		checkValuesContain(t, `
function collect(...)
    println(...)
end
collect(1, 2, 3)
`, "Parameter-...")
	})

	t.Run("nested function call target", func(t *testing.T) {
		checkValuesContain(t, `
local function inner(a)
    return a
end
local function outer(a)
    return inner(a)
end
println(outer(1))
`, "outer", "1")
	})
}

func TestLuaCallSiteOperand(t *testing.T) {
	checkPrintlnValueRegexp(t, `
function identity(a)
    return a
end
println(identity(7))
`, `identity\(7\)`)
}
