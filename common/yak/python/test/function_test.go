package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
)

// TestFunction_Definition tests Python function definitions
func TestFunction_Definition(t *testing.T) {
	t.Run("simple function", func(t *testing.T) {
		code := `
def foo():
    return 42
`
		_, err := python2ssa.Frontend(code)
		require.Nil(t, err, "parse AST FrontEnd error: %v", err)
	})

	t.Run("function with parameters", func(t *testing.T) {
		code := `
def add(a, b):
    return a + b
`
		_, err := python2ssa.Frontend(code)
		require.Nil(t, err, "parse AST FrontEnd error: %v", err)
	})

	t.Run("function with default parameter", func(t *testing.T) {
		code := `
def greet(name="World"):
    return "Hello, " + name
`
		_, err := python2ssa.Frontend(code)
		require.Nil(t, err, "parse AST FrontEnd error: %v", err)
	})

	t.Run("async function", func(t *testing.T) {
		code := `
async def fetch():
    pass
`
		_, err := python2ssa.Frontend(code)
		require.Nil(t, err, "parse AST FrontEnd error: %v", err)
	})
}

// TestFunction_Call tests Python function calls
func TestFunction_Call(t *testing.T) {
	t.Run("simple function call", func(t *testing.T) {
		test.CheckPrintlnValue(`
def foo():
    return 42
result = foo()
print(result)
`, []string{"42"}, t)
	})

	t.Run("function call with arguments", func(t *testing.T) {
		test.CheckPrintlnValue(`
def add(a, b):
    return a + b
result = add(3, 5)
print(result)
`, []string{"8"}, t)
	})

	t.Run("nested function calls", func(t *testing.T) {
		test.CheckPrintlnValue(`
def outer():
    def inner():
        return 10
    return inner() + 5
result = outer()
print(result)
`, []string{"15"}, t)
	})
}

// TestFunction_Scope tests function variable scope
func TestFunction_Scope(t *testing.T) {
	t.Run("local variables", func(t *testing.T) {
		test.CheckPrintlnValue(`
x = 10

def foo():
    x = 20
    print(x)

foo()
print(x)
`, []string{"20", "10"}, t)
	})

	t.Run("parameter shadows outer variable", func(t *testing.T) {
		test.CheckPrintlnValue(`
x = 10

def foo(x):
    print(x)

foo(5)
print(x)
`, []string{"5", "10"}, t)
	})
}

// TestFunction_Return tests return statements
func TestFunction_Return(t *testing.T) {
	t.Run("simple return", func(t *testing.T) {
		test.CheckPrintlnValue(`
def foo():
    return 42

result = foo()
print(result)
`, []string{"42"}, t)
	})

	t.Run("return expression", func(t *testing.T) {
		test.CheckPrintlnValue(`
def add(a, b):
    return a + b

result = add(3, 7)
print(result)
`, []string{"10"}, t)
	})

	t.Run("early return", func(t *testing.T) {
		test.CheckPrintlnValue(`
def foo(x):
    if x > 0:
        return "positive"
    return "negative"

result1 = foo(5)
result2 = foo(-3)
print(result1)
print(result2)
`, []string{"positive", "negative"}, t)
	})

	t.Run("implicit return (no return statement)", func(t *testing.T) {
		test.CheckPrintlnValue(`
def foo():
    x = 42

result = foo()
print(result)
`, []string{"Undefined-result"}, t)
	})
}

// TestFunction_Recursion tests recursive functions
func TestFunction_Recursion(t *testing.T) {
	t.Run("simple recursion", func(t *testing.T) {
		test.CheckPrintlnValue(`
def factorial(n):
    if n <= 1:
        return 1
    return n * factorial(n - 1)

result = factorial(5)
print(result)
`, []string{"120"}, t)
	})

	t.Run("mutual recursion", func(t *testing.T) {
		test.CheckPrintlnValue(`
def is_even(n):
    if n == 0:
        return True
    return is_odd(n - 1)

def is_odd(n):
    if n == 1:
        return False
    return is_even(n - 1)

result1 = is_even(4)
result2 = is_odd(5)
print(result1)
print(result2)
`, []string{"true", "false"}, t)
	})
}

// TestFunction_MultipleParams tests functions with many parameters
func TestFunction_MultipleParams(t *testing.T) {
	t.Run("four parameters", func(t *testing.T) {
		code := `
def combine(a, b, c, d):
    return a + b + c + d

result = combine(1, 2, 3, 4)
print(result)
`
		_, err := python2ssa.Frontend(code)
		require.Nil(t, err, "parse AST FrontEnd error: %v", err)
	})

	t.Run("five parameters", func(t *testing.T) {
		code := `
def sum_all(a, b, c, d, e):
    return a + b + c + d + e

result = sum_all(1, 2, 3, 4, 5)
print(result)
`
		_, err := python2ssa.Frontend(code)
		require.Nil(t, err, "parse AST FrontEnd error: %v", err)
	})
}

// TestFunction_DefaultParameters tests default parameter values
func TestFunction_DefaultParameters(t *testing.T) {
	t.Run("use default value", func(t *testing.T) {
		test.CheckPrintlnValue(`
def greet(name="World"):
    return "Hello, " + name

result = greet()
print(result)
`, []string{"Hello, World"}, t)
	})

	t.Run("override default value", func(t *testing.T) {
		test.CheckPrintlnValue(`
def greet(name="World"):
    return "Hello, " + name

result = greet("Alice")
print(result)
`, []string{"Hello, Alice"}, t)
	})

	t.Run("partial defaults", func(t *testing.T) {
		test.CheckPrintlnValue(`
def foo(a, b=10, c=20):
    return a + b + c

result1 = foo(1)
result2 = foo(1, 2)
result3 = foo(1, 2, 3)
print(result1)
print(result2)
print(result3)
`, []string{"31", "23", "6"}, t)
	})
}

// TestFunction_Closure tests closure and lambda functions
func TestFunction_Closure(t *testing.T) {
	t.Run("lambda function", func(t *testing.T) {
		code := `
f = lambda x: x * 2
result = f(5)
print(result)
`
		_, err := python2ssa.Frontend(code)
		require.Nil(t, err, "parse AST FrontEnd error: %v", err)
	})

	t.Run("closure capturing variables", func(t *testing.T) {
		test.CheckPrintlnValue(`
def outer():
    x = 10
    def inner():
        return x + 5
    return inner()

result = outer()
print(result)
`, []string{"15"}, t)
	})

	t.Run("closure with parameter", func(t *testing.T) {
		test.CheckPrintlnValue(`
def outer():
    x = 10
    def inner(y):
        return x + y
    return inner(5)

result = outer()
print(result)
`, []string{"15"}, t)
	})
}
