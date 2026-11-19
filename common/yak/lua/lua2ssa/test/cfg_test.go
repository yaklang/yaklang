package test

import "testing"

func TestLuaCFGIfPhi(t *testing.T) {
	t.Run("if true branch", func(t *testing.T) {
		checkPhi(t, `
local a = 1
if c then
    a = 2
end
println(a)
`, "a", "1", "2")
	})

	t.Run("if else branch", func(t *testing.T) {
		checkPhi(t, `
local a = 1
if c then
    a = 2
else
    a = 3
end
println(a)
`, "a", "2", "3")
	})

	t.Run("elseif chain", func(t *testing.T) {
		checkValuesContain(t, `
local a = 1
if c then
    a = 2
elseif d then
    a = 3
else
    a = 4
end
println(a)
`, "phi(a)", "2", "3", "4")
	})

	t.Run("nested if", func(t *testing.T) {
		checkValuesContain(t, `
local a = 1
if c then
    if d then
        a = 3
    else
        a = 2
    end
end
println(a)
`, "phi(a)", "1", "2", "3")
	})

	t.Run("if without else keeps old value", func(t *testing.T) {
		checkValues(t, `
local a = 1
if c then
    a = 2
end
println(a)
`, "phi(a)[2,1]")
	})

	t.Run("if condition false branch value", func(t *testing.T) {
		checkValues(t, `
local a = 1
if c then
    a = 2
else
    a = 3
end
println(a)
`, "phi(a)[2,3]")
	})
}

func TestLuaCFGLoops(t *testing.T) {
	t.Run("while creates loop phi", func(t *testing.T) {
		checkValuesContain(t, `
local a = 1
while c do
    a = a + 1
end
println(a)
`, "phi(a)", "add(a, 1)")
	})

	t.Run("while break", func(t *testing.T) {
		checkValuesContain(t, `
local a = 1
while c do
    a = 2
    break
end
println(a)
`, "phi(a)", "2", "1")
	})

	t.Run("repeat until", func(t *testing.T) {
		checkValuesContain(t, `
local a = 1
repeat
    a = a + 1
until c
println(a)
`, "phi(a)")
	})

	t.Run("numeric for", func(t *testing.T) {
		checkValuesContain(t, `
local a = 1
for i = 1, 3 do
    a = a + i
end
println(a)
`, "phi(a)")
	})

	t.Run("generic for", func(t *testing.T) {
		checkValuesContain(t, `
local a = 1
for k, v in pairs(t) do
    a = a + v
end
println(a)
`, "phi(a)")
	})

	t.Run("break and continue in loop", func(t *testing.T) {
		checkValuesContain(t, `
local a = 1
for i = 1, 3 do
    if i == 2 then
        continue
    end
    if i == 3 then
        break
    end
    a = a + i
end
println(a)
`, "phi(a)")
	})

	t.Run("do block keeps outer variable", func(t *testing.T) {
		checkValues(t, `
local a = 1
do
    a = 2
end
println(a)
`, "2")
	})
}
