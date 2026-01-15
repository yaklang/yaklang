package python

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPython_FunctionParamFlow(t *testing.T) {
	t.Run("parameter flow", func(t *testing.T) {
		code := `def add(a, b):
    return a + b

result = add(10, 20)`
		ssatest.CheckSyntaxFlow(t, code,
			"add(* #-> * as $params)",
			map[string][]string{
				"params": {"10", "20"},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}

func TestPython_FunctionReturnFlow(t *testing.T) {
	t.Run("return value flow", func(t *testing.T) {
		code := `def get_value():
    return 42

x = get_value()`
		ssatest.CheckSyntaxFlow(t, code,
			"get_value() #-> * as $ret",
			map[string][]string{
				"ret": {"42"},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}

func TestPython_FunctionChainFlow(t *testing.T) {
	t.Run("function chaining", func(t *testing.T) {
		code := `def f1(x):
    return "A"

def f2(x):
    return "B"

y = f1(1)
z = f2(y)`
		ssatest.CheckSyntaxFlow(t, code,
			"f1(*) as $y\nf2(*) as $z",
			map[string][]string{
				"y": {"Function-f1(1)"},
				"z": {"Function-f2(Function-f1(1))"},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}

func TestPython_ScopeFlow(t *testing.T) {
	t.Run("scope flow", func(t *testing.T) {
		code := `
def sink(x):
    pass

x = 1
def inner():
    x = 2

inner()
sink(x)`
		ssatest.CheckSyntaxFlow(t, code,
			"sink(* as $param)",
			map[string][]string{
				"param": {"1"},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}

func TestPython_ClassConstructorFlow(t *testing.T) {
	t.Run("class constructor flow", func(t *testing.T) {
		code := `
def sink(x):
    pass

class Point:
    def __init__(self, x, y):
        self.x = x
        self.y = y

p = Point(3, 4)
sink(p.x)
sink(p.y)`
		ssatest.CheckSyntaxFlow(t, code,
			"sink(* as $param)",
			map[string][]string{
				"param": {"Undefined-p.x", "Undefined-p.y"},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}

func TestPython_ClassMethodFlow(t *testing.T) {
	t.Run("class method flow", func(t *testing.T) {
		code := `class Counter:
    def __init__(self):
        self.count = 0
    
    def get_val(self):
        return 100

c = Counter()
x = c.get_val()`
		ssatest.CheckSyntaxFlow(t, code,
			"get_val() as $ret",
			map[string][]string{
				"ret": {"Undefined-c.get_val(Counter())"},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}

func TestPython_ClassStaticFlow(t *testing.T) {
	t.Run("class static flow", func(t *testing.T) {
		code := `class Config:
    MAX = 100

value = Config.MAX`
		ssatest.CheckSyntaxFlow(t, code,
			"Config.MAX #-> * as $value",
			map[string][]string{
				"value": {"Config"},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}

func TestPython_DefaultParamFlow(t *testing.T) {
	t.Run("default parameter flow", func(t *testing.T) {
		code := `def greet(name="World"):
    return "Hello " + name

x = greet()
y = greet("Alice")`
		ssatest.CheckSyntaxFlow(t, code,
			"greet(* #-> * as $args)",
			map[string][]string{
				"args": {"\"Alice\""},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}

func TestPython_NestedFunctionFlow(t *testing.T) {
	t.Run("nested function flow", func(t *testing.T) {
		code := `def outer(x):
    def inner(y):
        return x
    
    return inner(x)

result = outer(5)`
		ssatest.CheckSyntaxFlow(t, code,
			"inner(* #-> * as $inner_ret)\nouter(* #-> * as $outer_ret)",
			map[string][]string{
				"inner_ret": {"5"},
				"outer_ret": {"5"},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}

func TestPython_MultiParamFlow(t *testing.T) {
	t.Run("multi param flow", func(t *testing.T) {
		code := `def combine(a, b, c):
    return a

result = combine(1, 2, 3)`
		ssatest.CheckSyntaxFlow(t, code,
			"combine(* #-> * as $params)",
			map[string][]string{
				"params": {"1", "2", "3"},
			},
			ssaapi.WithLanguage(ssaconfig.PYTHON))
	})
}
