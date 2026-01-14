package python

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestPython_FunctionDefinition tests basic Python function definition
func TestPython_FunctionDefinition(t *testing.T) {
	t.Run("simple function", func(t *testing.T) {
		code := `
def foo():
    return 42

result = foo()
`
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
		require.NoError(t, err)
		require.NotNil(t, prog)
		prog.Show()
	})
}

// TestPython_FunctionWithParams tests Python function with parameters
func TestPython_FunctionWithParams(t *testing.T) {
	t.Run("function with params", func(t *testing.T) {
		code := `
def add(a, b):
    return a + b

result = add(10, 20)
`
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
		require.NoError(t, err)
		require.NotNil(t, prog)
		prog.Show()
	})
}

// TestPython_ClassDefinition tests basic Python class definition
func TestPython_ClassDefinition(t *testing.T) {
	t.Run("simple class", func(t *testing.T) {
		code := `
class MyClass:
    pass

obj = MyClass()
`
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
		require.NoError(t, err)
		require.NotNil(t, prog)
		prog.Show()
	})
}

// TestPython_ClassInheritance tests Python class inheritance
func TestPython_ClassInheritance(t *testing.T) {
	t.Run("class with inheritance", func(t *testing.T) {
		code := `
class Parent:
    pass

class Child(Parent):
    pass
`
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
		require.NoError(t, err)
		require.NotNil(t, prog)
		prog.Show()
	})
}

// TestPython_ClassMethod tests Python class methods
func TestPython_ClassMethod(t *testing.T) {
	t.Run("class method", func(t *testing.T) {
		code := `
class MyClass:
    def my_method(self):
        return 100

result = MyClass().my_method()
`
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
		require.NoError(t, err)
		require.NotNil(t, prog)
		prog.Show()
	})
}

// TestPython_ClassConstant tests Python class constants
func TestPython_ClassConstant(t *testing.T) {
	t.Run("class constant", func(t *testing.T) {
		code := `
class MyClass:
    CONST = 42

value = MyClass.CONST
`
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
		require.NoError(t, err)
		require.NotNil(t, prog)
		prog.Show()
	})
}
