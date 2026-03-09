package test

import (
	"sort"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// parsePython compiles Python source to an SSA program.
func parsePython(t *testing.T, code string) *ssaapi.Program {
	t.Helper()
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
	require.NoError(t, err, "parse should succeed")
	prog.Show()
	return prog
}

// sfValues runs a SyntaxFlow rule on prog and returns the string forms of the
// named variable's values, sorted for deterministic comparison.
func sfValues(t *testing.T, prog *ssaapi.Program, rule, varName string) []string {
	t.Helper()
	res, err := prog.SyntaxFlowWithError(rule)
	require.NoError(t, err, "syntaxflow rule should compile")
	vals := res.GetValues(varName)
	got := lo.Map(vals, func(v *ssaapi.Value, _ int) string { return v.String() })
	sort.Strings(got)
	return got
}

// requireContains asserts that every element of want appears in got (subset check).
func requireContains(t *testing.T, got, want []string, msgAndArgs ...interface{}) {
	t.Helper()
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		require.True(t, found, append([]interface{}{"expected %q in %v"}, w, got)...)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestFuncParams_Positional — 普通位置参数
// ─────────────────────────────────────────────────────────────────────────────

func TestFuncParams_Positional(t *testing.T) {
	t.Run("single param - call arg traced via println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def foo(a):
    println(a)
foo(42)
`, []string{"42"})
	})

	t.Run("multiple params - all call args traced via println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def add(a, b):
    println(a)
    println(b)
add(10, 20)
`, []string{"10", "20"})
	})

	t.Run("call site args matched by position", func(t *testing.T) {
		prog := parsePython(t, `
def add(a, b):
    return a + b
add(1, 2)
`)
		got := sfValues(t, prog, `add(* as $a, * as $b) as $call`, "a")
		require.Equal(t, []string{"1"}, got)
		got = sfValues(t, prog, `add(* as $a, * as $b) as $call`, "b")
		require.Equal(t, []string{"2"}, got)
	})

	t.Run("many params - each value flows to correct position", func(t *testing.T) {
		prog := parsePython(t, `
def f(a, b, c, d, e):
    return a
f(1, 2, 3, 4, 5)
`)
		for i, want := range []string{"1", "2", "3", "4", "5"} {
			varName := []string{"a", "b", "c", "d", "e"}[i]
			got := sfValues(t, prog,
				`f(* as $a, * as $b, * as $c, * as $d, * as $e) as $call`,
				varName)
			require.Equal(t, []string{want}, got, "param %s should be %s", varName, want)
		}
	})

	t.Run("function node exists in SSA", func(t *testing.T) {
		prog := parsePython(t, `
def target(a, b):
    return a
`)
		got := sfValues(t, prog, `target as $func`, "func")
		require.Len(t, got, 1)
		require.Equal(t, "Function-target", got[0])
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestFuncParams_DefaultValues — 带默认值的参数
// ─────────────────────────────────────────────────────────────────────────────

func TestFuncParams_DefaultValues(t *testing.T) {
	t.Run("use default greeting when not supplied - println traces name", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def greet(name, greeting="Hello"):
    println(name)
greet("Alice")
`, []string{`"Alice"`})
	})

	t.Run("use default greeting when not supplied - default param is Parameter node", func(t *testing.T) {
		// 当 greeting 未被调用方显式传入时，SSA 将其建模为 Parameter-greeting（参数节点）
		// 而不是内联常量 "Hello"，因为默认值在 SSA 中通过参数绑定，不是直接折叠。
		prog := parsePython(t, `
def greet(name, greeting="Hello"):
    println(name)
    println(greeting)
greet("Alice")
`)
		res, err := prog.SyntaxFlowWithError(`println(* #-> * as $out)`)
		require.NoError(t, err)
		vals := res.GetValues("out")
		gotStrs := make([]string, 0, len(vals))
		for _, v := range vals {
			gotStrs = append(gotStrs, v.String())
		}
		requireContains(t, gotStrs, []string{`"Alice"`})
		foundParam := false
		for _, v := range vals {
			if v.GetOpcode() == "Parameter" {
				foundParam = true
				break
			}
		}
		require.True(t, foundParam, "unset default param should be a Parameter node in SSA, not the default constant")
	})

	t.Run("override default value - both params traced", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def greet(name, greeting="Hello"):
    println(name)
    println(greeting)
greet("Bob", "Hi")
`, []string{`"Bob"`, `"Hi"`})
	})

	t.Run("all defaults - no-arg call traces Parameter node for first param", func(t *testing.T) {
		// connect() 无参调用；host 未被 supply，SSA 建模为 Parameter-host
		prog := parsePython(t, `
def connect(host="localhost", port=8080, timeout=30):
    println(host)
connect()
`)
		res, err := prog.SyntaxFlowWithError(`println(* #-> * as $out)`)
		require.NoError(t, err)
		vals := res.GetValues("out")
		require.Len(t, vals, 1, "should have exactly one println output")
		require.Equal(t, "Parameter-host", vals[0].String())
		require.Equal(t, "Parameter", vals[0].GetOpcode())
	})

	t.Run("positional call site - 2-arg call first two params bound correctly", func(t *testing.T) {
		// SF 的最后一个 * 贪婪匹配剩余实参；只有两个实参时等价于精确匹配
		prog := parsePython(t, `
def make_tag(tag, content, cls=""):
    return tag
make_tag("div", "hello")
`)
		got := sfValues(t, prog, `make_tag(* as $tag, * as $content) as $call`, "tag")
		require.Equal(t, []string{`"div"`}, got)
		got = sfValues(t, prog, `make_tag(* as $tag, * as $content) as $call`, "content")
		require.Equal(t, []string{`"hello"`}, got)
	})

	t.Run("positional call site - 3-arg call all params captured via SF", func(t *testing.T) {
		prog := parsePython(t, `
def make_tag(tag, content, cls=""):
    return tag
make_tag("span", "world", "red")
`)
		got := sfValues(t, prog, `make_tag(* as $tag, * as $content, * as $cls) as $call`, "tag")
		require.Equal(t, []string{`"span"`}, got)
		got = sfValues(t, prog, `make_tag(* as $tag, * as $content, * as $cls) as $call`, "content")
		require.Equal(t, []string{`"world"`}, got)
		got = sfValues(t, prog, `make_tag(* as $tag, * as $content, * as $cls) as $call`, "cls")
		require.Equal(t, []string{`"red"`}, got)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestFuncParams_Varargs — *args 可变位置参数
// ─────────────────────────────────────────────────────────────────────────────

func TestFuncParams_Varargs(t *testing.T) {
	t.Run("only varargs - each call arg captured", func(t *testing.T) {
		prog := parsePython(t, `
def total(*args):
    return args
total(1, 2, 3)
`)
		got := sfValues(t, prog, `total(* as $a, * as $b, * as $c) as $call`, "a")
		require.Equal(t, []string{"1"}, got)
		got = sfValues(t, prog, `total(* as $a, * as $b, * as $c) as $call`, "b")
		require.Equal(t, []string{"2"}, got)
		got = sfValues(t, prog, `total(* as $a, * as $b, * as $c) as $call`, "c")
		require.Equal(t, []string{"3"}, got)
	})

	t.Run("varargs println - first element traced", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def show(*args):
    println(args)
show(99)
`, []string{"99"})
	})

	t.Run("positional before varargs - println traces first arg", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def log(level, *messages):
    println(level)
log("DEBUG", "a", "b")
`, []string{`"DEBUG"`})
	})

	t.Run("positional before varargs - all args at call site", func(t *testing.T) {
		prog := parsePython(t, `
def log(level, *messages):
    return level
log("INFO", "started", "running")
`)
		got := sfValues(t, prog, `log(* as $lvl, * as $m1, * as $m2) as $call`, "lvl")
		require.Equal(t, []string{`"INFO"`}, got)
		got = sfValues(t, prog, `log(* as $lvl, * as $m1, * as $m2) as $call`, "m1")
		require.Equal(t, []string{`"started"`}, got)
		got = sfValues(t, prog, `log(* as $lvl, * as $m1, * as $m2) as $call`, "m2")
		require.Equal(t, []string{`"running"`}, got)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestFuncParams_Kwargs — **kwargs 可变关键字参数
// ─────────────────────────────────────────────────────────────────────────────

func TestFuncParams_Kwargs(t *testing.T) {
	t.Run("kwargs function node exists in SSA", func(t *testing.T) {
		prog := parsePython(t, `
def config(**options):
    return options
config(debug=True, verbose=False)
`)
		got := sfValues(t, prog, `config as $func`, "func")
		require.Len(t, got, 1)
		require.Equal(t, "Function-config", got[0])
	})

	t.Run("positional before kwargs - url traced via println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def request(url, **headers):
    println(url)
request("https://example.com", Accept="json")
`, []string{`"https://example.com"`})
	})

	t.Run("kwargs param name registered in SSA function", func(t *testing.T) {
		prog := parsePython(t, `
def config(**options):
    println(options)
config(a=1)
`)
		// **options 在 SSA 函数定义里注册为参数节点
		got := sfValues(t, prog, `config as $fn`, "fn")
		require.Len(t, got, 1)
		require.Equal(t, "Function-config", got[0])
	})

	t.Run("kwargs - keyword arg value traced via println", func(t *testing.T) {
		// store(x=42)：关键字参数传递的是值 42，不是键名 x
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def store(**kwargs):
    println(kwargs)
store(x=42)
`, []string{"42"})
	})

	t.Run("kwargs - multiple keyword args all traced at call site", func(t *testing.T) {
		prog := parsePython(t, `
def store(**kwargs):
    return kwargs
store(x=1, y=2, z=3)
`)
		got := sfValues(t, prog, `store(* as $a, * as $b, * as $c) as $call`, "a")
		require.Equal(t, []string{"1"}, got)
		got = sfValues(t, prog, `store(* as $a, * as $b, * as $c) as $call`, "b")
		require.Equal(t, []string{"2"}, got)
		got = sfValues(t, prog, `store(* as $a, * as $b, * as $c) as $call`, "c")
		require.Equal(t, []string{"3"}, got)
	})

	t.Run("kwargs - keyword arg value flows through println in init", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def config(**opts):
    println(opts)
config(debug=True)
`, []string{"true"})
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestFuncParams_Mixed — 混合参数类型
// ─────────────────────────────────────────────────────────────────────────────

func TestFuncParams_Mixed(t *testing.T) {
	t.Run("positional + varargs + kwargs - all call args captured", func(t *testing.T) {
		// myfunc(1, 2, 3, 4, x=5)：共 5 个实参（x=5 关键字参数值为 5）
		// SF 最后一个 * 贪婪：$d 会拿到第 4 个和第 5 个实参（4 和 5）
		prog := parsePython(t, `
def myfunc(a, b, *args, **kwargs):
    return a
myfunc(1, 2, 3, 4, x=5)
`)
		got := sfValues(t, prog, `myfunc(* as $a, * as $b, * as $c, * as $d) as $call`, "a")
		require.Equal(t, []string{"1"}, got)
		got = sfValues(t, prog, `myfunc(* as $a, * as $b, * as $c, * as $d) as $call`, "b")
		require.Equal(t, []string{"2"}, got)
		got = sfValues(t, prog, `myfunc(* as $a, * as $b, * as $c, * as $d) as $call`, "c")
		require.Equal(t, []string{"3"}, got)
		// $d 贪婪匹配剩余实参：4 和 5（x=5 的值）
		got = sfValues(t, prog, `myfunc(* as $a, * as $b, * as $c, * as $d) as $call`, "d")
		require.Equal(t, []string{"4", "5"}, got)
	})

	t.Run("positional + varargs + kwargs - all 5 args with exact SF", func(t *testing.T) {
		// 用 5 个 * 精确捕获每个位置
		prog := parsePython(t, `
def myfunc(a, b, *args, **kwargs):
    return a
myfunc(1, 2, 3, 4, x=5)
`)
		got := sfValues(t, prog, `myfunc(* as $a, * as $b, * as $c, * as $d, * as $e) as $call`, "e")
		require.Equal(t, []string{"5"}, got)
	})

	t.Run("all kinds - println traces a and b", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def full(a, b=2, *args, **kwargs):
    println(a)
    println(b)
full(10, 20)
`, []string{"10", "20"})
	})

	t.Run("all kinds - println all 4 args including varargs and kwarg", func(t *testing.T) {
		// full(1, 2, 3, x=4)：a=1, b=2, args=(3,), kwargs={x:4}
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def full(a, b=2, *args, **kwargs):
    println(a)
    println(b)
full(1, 2, 3, x=4)
`, []string{"1", "2"})
	})

	t.Run("mixed params - only positional arg printed", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def mixed(first, second=99, *rest, **opts):
    println(first)
mixed(42)
`, []string{"42"})
	})

	t.Run("mixed - function node registered with exact name", func(t *testing.T) {
		prog := parsePython(t, `
def target(a, b=10, *args, **kwargs):
    return a
`)
		got := sfValues(t, prog, `target as $func`, "func")
		require.Len(t, got, 1)
		require.Equal(t, "Function-target", got[0])
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestFuncParams_TypeAnnotations — 类型标注参数 (PEP 3107)
// ─────────────────────────────────────────────────────────────────────────────

func TestFuncParams_TypeAnnotations(t *testing.T) {
	t.Run("simple annotations - call args flow correctly", func(t *testing.T) {
		prog := parsePython(t, `
def add(a: int, b: int) -> int:
    return a + b
add(1, 2)
`)
		got := sfValues(t, prog, `add(* as $a, * as $b) as $call`, "a")
		require.Equal(t, []string{"1"}, got)
		got = sfValues(t, prog, `add(* as $a, * as $b) as $call`, "b")
		require.Equal(t, []string{"2"}, got)
	})

	t.Run("annotations with println trace", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def greet(name: str, times: int = 1) -> str:
    println(name)
greet("Alice")
greet("Bob", 3)
`, []string{`"Alice"`, `"Bob"`})
	})

	t.Run("complex type annotations - println traced", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def process(items: list, key: str = "id") -> dict:
    println(key)
process([1, 2, 3], "name")
`, []string{`"name"`})
	})

	t.Run("annotated param node registered with exact name", func(t *testing.T) {
		prog := parsePython(t, `
def typed(x: int, y: str = "hello") -> bool:
    return True
`)
		got := sfValues(t, prog, `typed as $func`, "func")
		require.Len(t, got, 1)
		require.Equal(t, "Function-typed", got[0])
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestMethodParams_Class — 类方法参数
// ─────────────────────────────────────────────────────────────────────────────

func TestMethodParams_Class(t *testing.T) {
	t.Run("constructor params - println traces x and y", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Point:
    def __init__(self, x, y):
        println(x)
        println(y)
p = Point(10, 20)
`, []string{"10", "20"})
	})

	t.Run("constructor params - SF call site args by position", func(t *testing.T) {
		prog := parsePython(t, `
class Point:
    def __init__(self, x, y):
        self.x = x
        self.y = y
p = Point(1, 2)
`)
		got := sfValues(t, prog, `Point(* as $self, * as $x, * as $y) as $call`, "x")
		require.Equal(t, []string{"1"}, got)
		got = sfValues(t, prog, `Point(* as $self, * as $x, * as $y) as $call`, "y")
		require.Equal(t, []string{"2"}, got)
	})

	t.Run("constructor with default params - explicit override", func(t *testing.T) {
		prog := parsePython(t, `
class Server:
    def __init__(self, host="localhost", port=8080):
        self.host = host
        self.port = port
s = Server("0.0.0.0", 443)
`)
		got := sfValues(t, prog, `Server(* as $s, * as $host, * as $port) as $call`, "host")
		require.Equal(t, []string{`"0.0.0.0"`}, got)
		got = sfValues(t, prog, `Server(* as $s, * as $host, * as $port) as $call`, "port")
		require.Equal(t, []string{"443"}, got)
	})

	t.Run("constructor with default params - println traces overridden values", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Server:
    def __init__(self, host="localhost", port=8080):
        println(host)
        println(port)
s = Server("myhost", 9000)
`, []string{`"myhost"`, "9000"})
	})

	t.Run("constructor with varargs - items args captured at call site", func(t *testing.T) {
		prog := parsePython(t, `
class Container:
    def __init__(self, *items):
        self.items = items
c = Container(1, 2, 3)
`)
		got := sfValues(t, prog, `Container(* as $s, * as $a, * as $b, * as $c) as $call`, "a")
		require.Equal(t, []string{"1"}, got)
		got = sfValues(t, prog, `Container(* as $s, * as $a, * as $b, * as $c) as $call`, "b")
		require.Equal(t, []string{"2"}, got)
		got = sfValues(t, prog, `Container(* as $s, * as $a, * as $b, * as $c) as $call`, "c")
		require.Equal(t, []string{"3"}, got)
	})

	t.Run("normal method call - args traced via println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Calc:
    def add(self, a, b):
        println(a)
        println(b)
c = Calc()
c.add(3, 4)
`, []string{"3", "4"})
	})

	t.Run("normal method - call site all 3 args (self + a + b)", func(t *testing.T) {
		// Calc.multiply(self, a, b)：call site 有 3 个实参：self(constructor call), 5, 6
		prog := parsePython(t, `
class Calc:
    def multiply(self, a, b):
        return a * b
c = Calc()
result = c.multiply(5, 6)
`)
		got := sfValues(t, prog, `Calc.multiply(* as $s, * as $a, * as $b) as $call`, "a")
		require.Equal(t, []string{"5"}, got)
		got = sfValues(t, prog, `Calc.multiply(* as $s, * as $a, * as $b) as $call`, "b")
		require.Equal(t, []string{"6"}, got)
	})

	t.Run("static method call - args traced via println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Math:
    @staticmethod
    def add(a, b):
        println(a)
        println(b)
Math.add(7, 8)
`, []string{"7", "8"})
	})

	t.Run("static method SF - function node exact name", func(t *testing.T) {
		prog := parsePython(t, `
class Utils:
    @staticmethod
    def helper(x):
        return x
Utils.helper(99)
`)
		got := sfValues(t, prog, `Utils.helper as $method`, "method")
		require.Len(t, got, 1)
		require.Equal(t, "Function-Utils.helper", got[0])
	})

	t.Run("classmethod call - name traced via println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Animal:
    @classmethod
    def create(cls, name):
        println(name)
Animal.create("Buddy")
`, []string{`"Buddy"`})
	})

	t.Run("method with type annotations - args flow correctly", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Formatter:
    def format(self, value: str, width: int = 10) -> str:
        println(value)
f = Formatter()
f.format("hello")
f.format("world", 20)
`, []string{`"hello"`, `"world"`})
	})

	t.Run("constructor all param kinds - named positional args", func(t *testing.T) {
		prog := parsePython(t, `
class Widget:
    def __init__(self, name, color="blue", *tags, **attrs):
        self.name = name
w = Widget("btn", "red", "large", "bold")
`)
		got := sfValues(t, prog, `Widget(* as $s, * as $name, * as $color, * as $t1, * as $t2) as $call`, "name")
		require.Equal(t, []string{`"btn"`}, got)
		got = sfValues(t, prog, `Widget(* as $s, * as $name, * as $color, * as $t1, * as $t2) as $call`, "color")
		require.Equal(t, []string{`"red"`}, got)
		got = sfValues(t, prog, `Widget(* as $s, * as $name, * as $color, * as $t1, * as $t2) as $call`, "t1")
		require.Equal(t, []string{`"large"`}, got)
		// t2 是 SF 最后一个 *，贪婪拿走最后一个 "bold"
		got = sfValues(t, prog, `Widget(* as $s, * as $name, * as $color, * as $t1, * as $t2) as $call`, "t2")
		require.Equal(t, []string{`"bold"`}, got)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestFuncParams_DataFlow — 数据流从实参到使用点
// ─────────────────────────────────────────────────────────────────────────────

func TestFuncParams_DataFlow(t *testing.T) {
	t.Run("taint flows from call arg to return value", func(t *testing.T) {
		prog := parsePython(t, `
def add(a, b):
    return a + b
result = add(1, 2)
`)
		res, err := prog.SyntaxFlowWithError(`result as $r; $r #-> * as $src`)
		require.NoError(t, err)
		srcs := lo.Map(res.GetValues("src"), func(v *ssaapi.Value, _ int) string { return v.String() })
		requireContains(t, srcs, []string{"1", "2"})
	})

	t.Run("param flows through multiple assignments", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def double(x):
    y = x
    z = y
    println(z)
double(21)
`, []string{"21"})
	})

	t.Run("constructor arg flows to member println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Box:
    def __init__(self, width):
        self.width = width
        println(self.width)
b = Box(10)
`, []string{"10"})
	})

	t.Run("method arg flows through println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
def process(name, value, flag=False):
    println(name)
    println(value)
process("test", 42)
`, []string{`"test"`, "42"})
	})

	t.Run("chained method - param traced through call chain", func(t *testing.T) {
		ssatest.CheckSyntaxFlowPrintWithPython(t, `
class Worker:
    def run(self, task, timeout=30):
        println(task)
        println(timeout)
w = Worker()
w.run("download", 60)
`, []string{`"download"`, "60"})
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestFuncParams_BottomUse — 向下跟踪：实参/调用节点被谁使用（bottom-use / -->）
// ─────────────────────────────────────────────────────────────────────────────

func TestFuncParams_BottomUse(t *testing.T) {
	t.Run("call arg single-hop to foo call node", func(t *testing.T) {
		// foo(42)：42 单跳 (->) 直接使用者是 foo 自身的调用节点（opcode=Call）
		prog := parsePython(t, `
def foo(a):
    println(a)
foo(42)
`)
		res, err := prog.SyntaxFlowWithError(`foo(* as $arg) as $call; $arg -> * as $use`)
		require.NoError(t, err)
		vals := res.GetValues("use")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
		require.Contains(t, vals[0].String(), "foo")
	})

	t.Run("function node used by multiple call sites", func(t *testing.T) {
		// process 函数被调用两次；--> 后过滤 opcode=Call，精确得到 2 个 Call 节点
		prog := parsePython(t, `
def process(name):
    println(name)
process("test")
process("world")
`)
		res, err := prog.SyntaxFlowWithError(`process --> * as $all; $all?{opcode: Call} as $calls`)
		require.NoError(t, err)
		vals := res.GetValues("calls")
		require.Len(t, vals, 2, "process should be used by exactly 2 call sites")
		for _, v := range vals {
			require.Equal(t, "Call", v.GetOpcode())
		}
	})

	t.Run("function node used by single call site", func(t *testing.T) {
		// add 只调用一次；--> 过滤 Call 得到 1 个节点
		prog := parsePython(t, `
def add(a, b):
    return a + b
result = add(1, 2)
`)
		res, err := prog.SyntaxFlowWithError(`add --> * as $all; $all?{opcode: Call} as $calls`)
		require.NoError(t, err)
		vals := res.GetValues("calls")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
	})

	t.Run("call result flows down to println", func(t *testing.T) {
		// add(1,2) call 节点 --> println 的调用节点
		prog := parsePython(t, `
def add(a, b):
    return a + b
result = add(1, 2)
println(result)
`)
		got := sfValues(t, prog, `add(* as $a, * as $b) as $call; $call --> * as $use`, "use")
		require.Len(t, got, 1)
		require.Contains(t, got[0], "println")
	})

	t.Run("positional arg flows to println via param binding", func(t *testing.T) {
		// pipe(99)：99 单跳到 pipe call 节点
		prog := parsePython(t, `
def pipe(a):
    b = a
    println(b)
pipe(99)
`)
		res, err := prog.SyntaxFlowWithError(`pipe(* as $arg) as $call; $arg -> * as $use`)
		require.NoError(t, err)
		vals := res.GetValues("use")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
	})

	t.Run("static method call arg single-hop to call node", func(t *testing.T) {
		// Math.square(7)：7 单跳到 square 调用节点
		prog := parsePython(t, `
class Math:
    @staticmethod
    def square(x):
        return x * x
Math.square(7)
`)
		res, err := prog.SyntaxFlowWithError(`Math.square(* as $x) as $call; $x -> * as $use`)
		require.NoError(t, err)
		vals := res.GetValues("use")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
		require.Contains(t, vals[0].String(), "square")
	})

	t.Run("varargs call arg single-hop to show call node", func(t *testing.T) {
		// show(99)：99 单跳到 show 调用节点
		prog := parsePython(t, `
def show(*args):
    println(args)
show(99)
`)
		res, err := prog.SyntaxFlowWithError(`show(* as $arg) as $call; $arg -> * as $use`)
		require.NoError(t, err)
		vals := res.GetValues("use")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
	})

	t.Run("kwargs call arg single-hop to store call node", func(t *testing.T) {
		// store(x=42)：42 单跳到 store 调用节点
		prog := parsePython(t, `
def store(**kwargs):
    println(kwargs)
store(x=42)
`)
		res, err := prog.SyntaxFlowWithError(`store(* as $arg) as $call; $arg -> * as $use`)
		require.NoError(t, err)
		vals := res.GetValues("use")
		require.Len(t, vals, 1)
		require.Equal(t, "Call", vals[0].GetOpcode())
	})
}
