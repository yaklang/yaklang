package test

import (
	"testing"
)

func TestPython_Basic_Assignment(t *testing.T) {
	t.Run("test simple variable assign", func(t *testing.T) {
		CheckPythonPrintlnValue(`
x = 1
println(x)
y = 2
println(y)
z = True
println(z)
w = 3.14
println(w)
s = "hello"
println(s)
`, []string{"1", "2", "true", "3.14", "\"hello\""}, t)
	})

	t.Run("test multiple assignment", func(t *testing.T) {
		CheckPythonPrintlnValue(`
a, b = 1, 2
println(a)
println(b)
x = y = 10
println(x)
println(y)
`, []string{"1", "2", "10", "10"}, t)
	})

	t.Run("test augmented assignment", func(t *testing.T) {
		CheckPythonPrintlnValue(`
x = 5
x += 3
println(x)
x -= 2
println(x)
x *= 2
println(x)
`, []string{"8", "6", "12"}, t)
	})
}

func TestPython_Basic_Expression(t *testing.T) {
	t.Run("test arithmetic expressions", func(t *testing.T) {
		CheckPythonPrintlnValue(`
a = 1 + 2
println(a)
b = 5 - 3
println(b)
c = 2 * 3
println(c)
d = 8 / 2
println(d)
e = 7 % 3
println(e)
`, []string{"3", "2", "6", "4", "1"}, t)
	})

	t.Run("test comparison expressions", func(t *testing.T) {
		CheckPythonPrintlnValue(`
a = 1 < 2
println(a)
b = 2 > 1
println(b)
c = 1 == 1
println(c)
d = 1 != 2
println(d)
`, []string{"true", "true", "true", "true"}, t)
	})

	t.Run("test logical expressions", func(t *testing.T) {
		CheckPythonPrintlnValue(`
a = True and False
println(a)
b = True or False
println(b)
c = not False
println(c)
`, []string{"false", "true", "true"}, t)
	})
}

func TestPython_Basic_ControlFlow(t *testing.T) {
	t.Run("test if statement", func(t *testing.T) {
		CheckPythonPrintlnValue(`
x = 10
if x > 5:
    println("big")
else:
    println("small")
`, []string{"\"big\""}, t)
	})

	t.Run("test for loop", func(t *testing.T) {
		CheckPythonPrintlnValue(`
for i in range(3):
    println(i)
`, []string{"0", "1", "2"}, t)
	})

	t.Run("test while loop", func(t *testing.T) {
		CheckPythonPrintlnValue(`
i = 0
while i < 3:
    println(i)
    i += 1
`, []string{"0", "1", "2"}, t)
	})
}

func TestPython_Basic_DataStructures(t *testing.T) {
	t.Run("test list", func(t *testing.T) {
		CheckPythonPrintlnValue(`
lst = [1, 2, 3]
println(lst)
println(lst[0])
println(lst[1])
`, []string{"make([]number)", "1", "2"}, t)
	})

	t.Run("test dict", func(t *testing.T) {
		CheckPythonPrintlnValue(`
d = {"a": 1, "b": 2}
println(d)
println(d["a"])
`, []string{"make(map[string]any)", "1"}, t)
	})

	t.Run("test tuple", func(t *testing.T) {
		CheckPythonPrintlnValue(`
t = (1, 2, 3)
println(t)
println(t[0])
`, []string{"make([]number)", "1"}, t)
	})
}
