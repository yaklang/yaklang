package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestBasicClassDefinition tests basic class definition without inheritance
func TestBasicClassDefinition(t *testing.T) {
	testCases := []struct {
		name   string
		code   string
		expect []string
	}{
		{
			name: "simple empty class",
			code: `
class MyClass:
    pass
`,
			expect: []string{"MyClass"},
		},
		{
			name: "class with empty parentheses",
			code: `
class MyClass():
    pass
`,
			expect: []string{"MyClass"},
		},
		{
			name: "class with docstring",
			code: `
class MyClass:
    """A simple class"""
    pass
`,
			expect: []string{"MyClass"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify class definition can be found
			ssatest.CheckSyntaxFlow(t, tc.code, `MyClass as $class`, map[string][]string{
				"class": tc.expect,
			}, ssaapi.WithLanguage(ssaconfig.PYTHON))
		})
	}
}

// TestClassInheritance tests class with single and multiple inheritance
func TestClassInheritance(t *testing.T) {
	testCases := []struct {
		name   string
		code   string
		rule   string
		expect map[string][]string
	}{
		{
			name: "class with single parent",
			code: `
class Parent:
    pass

class Child(Parent):
    pass
`,
			rule: `Child as $child`,
			expect: map[string][]string{
				"child": {"Child"},
			},
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
			rule: `Child as $child`,
			expect: map[string][]string{
				"child": {"Child"},
			},
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
			rule: `Child as $child`,
			expect: map[string][]string{
				"child": {"Child"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify child class can be found
			ssatest.CheckSyntaxFlow(t, tc.code, tc.rule, tc.expect, ssaapi.WithLanguage(ssaconfig.PYTHON))
		})
	}
}

// TestClassMethods tests class with methods
func TestClassMethods(t *testing.T) {
	testCases := []struct {
		name   string
		code   string
		rule   string
		expect map[string][]string
	}{
		{
			name: "class with simple method",
			code: `
class MyClass:
    def my_method(self):
        pass
`,
			rule: `my_method as $method`,
			expect: map[string][]string{
				"method": {"Function-MyClass.my_method"},
			},
		},
		{
			name: "class with method parameters",
			code: `
class MyClass:
    def my_method(self, param1, param2):
        pass
`,
			rule: `my_method as $method`,
			expect: map[string][]string{
				"method": {"Function-MyClass.my_method"},
			},
		},
		{
			name: "class with method return",
			code: `
class MyClass:
    def get_value(self):
        return 42
`,
			rule: `get_value as $method`,
			expect: map[string][]string{
				"method": {"Function-MyClass.get_value"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckSyntaxFlow(t, tc.code, tc.rule, tc.expect, ssaapi.WithLanguage(ssaconfig.PYTHON))
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
			// Verify SSA builds without errors
			ssatest.NonStrictMockSSA(t, tc.code)
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
			// Verify SSA builds without errors
			ssatest.NonStrictMockSSA(t, tc.code)
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
			// Verify SSA builds without errors
			ssatest.NonStrictMockSSA(t, tc.code)
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
	// Use NonStrictMockSSA since the complete class has multiple nodes
	// named "Dog" (class, method references, etc.) making exact matching complex
	ssatest.NonStrictMockSSA(t, code)
}

// TestDecoratedClass tests class with decorators
func TestDecoratedClass(t *testing.T) {
	testCases := []struct {
		name   string
		code   string
		rule   string
		expect map[string][]string
	}{
		{
			name: "class with single decorator",
			code: `
def decorator(cls):
    return cls

@decorator
class MyClass:
    pass
`,
			rule: `MyClass as $class`,
			expect: map[string][]string{
				"class": {"MyClass"},
			},
		},
		{
			name: "class with multiple decorators",
			code: `
def decorator1(cls):
    return cls

def decorator2(cls):
    return cls

@decorator1
@decorator2
class MyClass:
    pass
`,
			rule: `MyClass as $class`,
			expect: map[string][]string{
				"class": {"MyClass"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckSyntaxFlow(t, tc.code, tc.rule, tc.expect, ssaapi.WithLanguage(ssaconfig.PYTHON))
		})
	}
}
