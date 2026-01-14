package test

import (
	"testing"
)

func TestFunction_Return(t *testing.T) {
	t.Run("simple return", func(t *testing.T) {
		code := `
def foo():
    return 42
`
		CheckPythonCode(code, t)
	})
}

func TestFunction_Call(t *testing.T) {
	t.Run("function call with args", func(t *testing.T) {
		code := `
def add(a, b):
    return a + b

result = add(10, 20)
`
		CheckPythonCode(code, t)
	})
}

func TestClass_Definition(t *testing.T) {
	t.Run("simple class", func(t *testing.T) {
		code := `
class MyClass:
    pass
`
		CheckPythonCode(code, t)
	})
}

func TestClass_Inheritance(t *testing.T) {
	t.Run("class with inheritance", func(t *testing.T) {
		code := `
class Parent:
    pass

class Child(Parent):
    pass
`
		CheckPythonCode(code, t)
	})
}

func TestClass_Method(t *testing.T) {
	t.Run("class method", func(t *testing.T) {
		code := `
class MyClass:
    def my_method(self):
        return 100

result = MyClass().my_method()
`
		CheckPythonCode(code, t)
	})
}

func TestClass_Constant(t *testing.T) {
	t.Run("class constant", func(t *testing.T) {
		code := `
class MyClass:
    CONST = 42

value = MyClass.CONST
`
		CheckPythonCode(code, t)
	})
}
