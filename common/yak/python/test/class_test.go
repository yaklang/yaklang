package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
)

// TestBasicClassDefinition tests basic class definition without inheritance
func TestBasicClassDefinition(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "simple empty class",
			code: `
class MyClass:
    pass
`,
		},
		{
			name: "class with empty parentheses",
			code: `
class MyClass():
    pass
`,
		},
		{
			name: "class with docstring",
			code: `
class MyClass:
    """A simple class"""
    pass
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := python2ssa.Frontend(tc.code)
			require.Nil(t, err, "parse AST FrontEnd error: %v", err)
		})
	}
}

// TestClassInheritance tests class with single and multiple inheritance
func TestClassInheritance(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "class with single parent",
			code: `
class Parent:
    pass

class Child(Parent):
    pass
`,
		},
		{
			name: "class with multiple inheritance",
			code: `
class Parent1:
    pass

class Parent2:
    pass

class Child(Parent1, Parent2):
    pass
`,
		},
		{
			name: "nested inheritance chain",
			code: `
class GrandParent:
    pass

class Parent(GrandParent):
    pass

class Child(Parent):
    pass
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := python2ssa.Frontend(tc.code)
			require.Nil(t, err, "parse AST FrontEnd error: %v", err)
		})
	}
}

// TestClassMethods tests class with methods
func TestClassMethods(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "class with simple method",
			code: `
class MyClass:
    def my_method(self):
        pass
`,
		},
		{
			name: "class with multiple methods",
			code: `
class MyClass:
    def method1(self):
        pass

    def method2(self):
        pass
`,
		},
		{
			name: "class with method parameters",
			code: `
class MyClass:
    def my_method(self, param1, param2):
        pass
`,
		},
		{
			name: "class with method return",
			code: `
class MyClass:
    def get_value(self):
        return 42
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := python2ssa.Frontend(tc.code)
			require.Nil(t, err, "parse AST FrontEnd error: %v", err)
		})
	}
}

// TestClassConstructor tests class with __init__ method
func TestClassConstructor(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "class with __init__",
			code: `
class MyClass:
    def __init__(self):
        pass
`,
		},
		{
			name: "class with __init__ and parameters",
			code: `
class MyClass:
    def __init__(self, x, y):
        pass
`,
		},
		{
			name: "class with __init__ and attribute initialization",
			code: `
class MyClass:
    def __init__(self, x):
        self.x = x
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := python2ssa.Frontend(tc.code)
			require.Nil(t, err, "parse AST FrontEnd error: %v", err)
		})
	}
}

// TestClassProperties tests class properties/attributes
func TestClassProperties(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "class with property assignment in __init__",
			code: `
class MyClass:
    def __init__(self):
        self.prop = 42
`,
		},
		{
			name: "class with multiple properties",
			code: `
class MyClass:
    def __init__(self):
        self.x = 1
        self.y = 2
        self.z = 3
`,
		},
		{
			name: "class with property in method",
			code: `
class MyClass:
    def set_prop(self, value):
        self.prop = value
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := python2ssa.Frontend(tc.code)
			require.Nil(t, err, "parse AST FrontEnd error: %v", err)
		})
	}
}

// TestClassConstants tests class constants (uppercase attributes)
func TestClassConstants(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "class with constant",
			code: `
class MyClass:
    CONST = 42
`,
		},
		{
			name: "class with multiple constants",
			code: `
class MyClass:
    CONST1 = 1
    CONST2 = "hello"
    CONST3 = [1, 2, 3]
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := python2ssa.Frontend(tc.code)
			require.Nil(t, err, "parse AST FrontEnd error: %v", err)
		})
	}
}

// TestCompleteClass tests a complete class with all features
func TestCompleteClass(t *testing.T) {
	code := `
class Animal:
    CLASS_NAME = "Animal"

    def __init__(self, name):
        self.name = name

    def speak(self):
        return f"{self.name} makes a sound"

class Dog(Animal):
    def speak(self):
        return f"{self.name} barks"
`
	_, err := python2ssa.Frontend(code)
	require.Nil(t, err, "parse AST FrontEnd error: %v", err)
}

// TestDecoratedClass tests class with decorators
func TestDecoratedClass(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "class with single decorator",
			code: `
@decorator
class MyClass:
    pass
`,
		},
		{
			name: "class with multiple decorators",
			code: `
@decorator1
@decorator2
class MyClass:
    pass
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := python2ssa.Frontend(tc.code)
			require.Nil(t, err, "parse AST FrontEnd error: %v", err)
		})
	}
}
