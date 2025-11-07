package tests

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed testdata/replace_member_call_inf_loop.js
var inf_loop_js_file string

func TestSimplePrint(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
let a = 1
println(a)
`, []string{"1"}, t)
}

func TestBasicBlock(t *testing.T) {
	t.Parallel()
	// without "use strict" prologue directive
	ssatest.CheckPrintlnValue(`
{
	{
		a = 1
		println(a)
	}
	println(a)
}
println(a)
{
	println(a)
}
`, []string{"1", "1", "1", "1"}, t)

	// with "use strict" prologue directive
	ssatest.CheckPrintlnValue(`
"use strict";
{
	{
		a = 1
		println(a)
	}
	println(a)
}
println(a)
{
	println(a)
}
`, []string{"1", "Undefined-a", "Undefined-a", "Undefined-a"}, t)

	// with "use strict" prologue directive in labeled block
	ssatest.CheckPrintlnValue(`
"use strict";
test:{
		{
			a = 1
			println(a)
		}
		println(a)
}
println(a)
test1:{
	println(a)
}
`, []string{"1", "Undefined-a", "Undefined-a", "Undefined-a"}, t)
}

func TestBasicIfBlock(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
if(cond){
a=1
}
println(a)
`, []string{"phi(a)[1,Undefined-a]"}, t)
}

func TestBasicFunctionCall(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
function foo(){
println(1)
}
foo()
`, []string{"1"}, t)
}

func TestLabeledBlock(t *testing.T) {
	t.Parallel()

	code := `
gg:{
let b = 999
	{
		a = 1
		println(a)
	}
	println(a)
}
println(a)
println(b)
{
	println(a)
}
`
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage("ts"),
	)
	require.NoError(t, err)
	// 生成函数的控制流图
	dot := ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0])
	log.Infof("函数控制流图DOT: \n%s", dot)

	ssatest.CheckPrintlnValue(code, []string{"1", "1", "1", "Undefined-b", "1"}, t)
}

func TestBigInteger(t *testing.T) {
	t.Parallel()

	// 这里对待BigInt就是直接转成数字，不保留BigInt类型
	ssatest.CheckPrintlnValue(`
let a = 1234567n
println(a)
`, []string{"1234567"}, t)
}

func TestArithmeticBinaryExpressions(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
let a = 5 + 3
let b = a - 2
let c = b * 4
let d = c / 2
let e = d % 3
let f = 2 ** 3
println(a)
println(b)
println(c)
println(d)
println(e)
println(f)
`, []string{
		"8",  // 5 + 3
		"6",  // 8 - 2
		"24", // 6 * 4
		"12", // 24 / 2
		"0",  // 12 % 3
		"8",  // 2 ** 3
	}, t)
}

func TestComparisonBinaryExpressions(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
let a = 5
let b = 3
let c = 5
let d = a > b
let e = a < b
let f = a >= c
let g = a == c
let h = b != a
let i = a===c
let j = b !== a
let k = a <= c
println(d)
println(e)
println(f)
println(g)
println(h)
println(i)
println(j)
println(k)
`, []string{
		"true",  // 5 > 3
		"false", // 5 < 3
		"true",  // 5 >= 5
		"true",  // 5 == 5
		"true",  // 3 != 5
		"true",  // 5 === 5
		"true",  // 3 !== 5
		"true",  // 5 <= 5
	}, t)
}

func TestBitwiseBinaryExpressions(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
let a = 5 & 3
let b = 5 | 3
let c = 5 ^ 3
let d = 5 << 1
let e = 5 >> 1
let f = 5 >>> 1
println(a)
println(b)
println(c)
println(d)
println(e)
println(f)
`, []string{
		"1",  // 5 & 3 -> 0101 & 0011 = 0001
		"7",  // 5 | 3 -> 0101 | 0011 = 0111
		"6",  // 5 ^ 3 -> 0101 ^ 0011 = 0110
		"10", // 5 << 1 -> 0101 << 1 = 1010
		"2",  // 5 >> 1 -> 0101 >> 1 = 0010
		"2",  // 5 >>> 1 -> 0101 >>> 1 = 0010（但此处行为与 >> 相同）
	}, t)
}

func TestLogicalBinaryExpressions(t *testing.T) {
	t.Parallel()

	code := `
let a = true && false
let b = true || false
let d = false || (true && false)
let e = true && true
println(a)
println(b)
println(d)
println(e)
`
	parse, err := ssaapi.Parse(code, ssaapi.WithLanguage("ts"))
	require.NoError(t, err)
	parse.Show()
}

func TestLogicalBinaryExpressionWithVariables(t *testing.T) {
	t.Parallel()

	code := `
let a = 19
let b = 200
let c = a || b
let d = 0 || b
let e = null || b
let f = false || 123
println(c)
println(d)
println(e)
println(f)
`
	parse, err := ssaapi.Parse(code, ssaapi.WithLanguage("ts"))
	require.NoError(t, err)
	parse.Show()
}

func TestNullishCoalescingBinaryExpressions(t *testing.T) {
	t.Parallel()

	code := `
let e = (null ?? 5) ?? 10
`
	parse, err := ssaapi.Parse(code, ssaapi.WithLanguage("ts"))
	require.NoError(t, err)
	parse.Show()
}

func TestLogicalAssignmentBinaryExpressions(t *testing.T) {
	t.Parallel()

	code := `
let a = false
a ||= true
let b = true
b &&= false
let c = false
c ??= 42
println(a)
println(b)
println(c)
`
	parse, err := ssaapi.Parse(code, ssaapi.WithLanguage("ts"))
	require.NoError(t, err)
	parse.Show()
}

func TestArithmeticAssignmentBinaryExpressions(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
let a = 5
a += 3
let b = 10
b -= 2
let c = 4
c *= 2
let d = 8
d /= 2
let e = 15
e %= 4
let f = 2
f **= 3
println(a)
println(b)
println(c)
println(d)
println(e)
println(f)
`, []string{
		"8",
		"8",
		"8",
		"4",
		"3",
		"8",
	}, t)
}

func TestBitwiseAssignmentBinaryExpressions(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
let a = 5
a &= 3
let b = 5
b |= 3
let c = 5
c ^= 3
let d = 5
d <<= 1
let e = 5
e >>= 1
let f = 5
f >>>= 1
println(a)
println(b)
println(c)
println(d)
println(e)
println(f)
`, []string{
		"1",
		"7",
		"6",
		"10",
		"2",
		"2",
	}, t)
}

func TestAssignmentBinaryExpressions(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
let a = 5
let b = (a = 10)
println(a)
println(b)
`, []string{
		"10",
		"10",
	}, t)
}

func TestComplexBinaryExpressions(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
let a = 5 + 3 * 2
let b = (5 + 3) * 2
let c = (a + b) / 2
let d = (a + b) % 3
println(a)
println(b)
println(c)
println(d)
`, []string{
		"11",
		"16",
		"13",
		"0",
	}, t)
}

func TestPropertyAccessExpression(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
// 基本对象属性访问 - 右值
const obj = { name: "Alice", age: 30 }
println(obj.name)
println(obj.age)

// 嵌套对象属性访问
const nested = { user: { id: 123 } }
println(nested.user.id)

// 属性访问作为左值 - 简单赋值
let mutable = { counter: 0 }
mutable.counter = 10
println(mutable.counter)

// 属性访问作为左值 - 复合赋值
let numbers = { x: 5, y: 10 }
numbers.x += 3
numbers.y *= 2
println(numbers.x)
println(numbers.y)

// 函数返回对象的属性访问
function getObject() {
  return { data: "test" }
}
// println(getObject().data) 这个不应该通过println检测

// 多级属性赋值
let multi = { a: { b: 0 } }
multi.a.b = 100
println(multi.a.b)
`, []string{
		"\"Alice\"",
		"30",
		"123",
		"10",
		"8",
		"20",
		//"\"test\"",
		"100",
	}, t)
}

func TestElementAccessExpression(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
// 基本数组元素访问 - 右值
const arr = [10, 20, 30]
println(arr[0])
println(arr[2])

// 对象使用方括号访问 - 静态字符串键
const obj = { name: "Bob", age: 25 }
println(obj["name"])
println(obj["age"])

// 对象使用方括号访问 - 变量键
const key = "name"
println(obj[key])

// 元素访问作为左值 - 简单赋值
let mutableArr = [1, 2, 3]
mutableArr[1] = 200
println(mutableArr[1])

// 对象元素访问作为左值
let mutableObj = { x: 10, y: 20 }
mutableObj["x"] = 100
println(mutableObj["x"])

// 元素访问作为左值 - 复合赋值
let numbers = [5, 10, 15]
numbers[0] += 5
numbers[1] *= 3
println(numbers[0])
println(numbers[1])

// 常量表达式作为索引
println(arr[1+1])

// 多维数组访问
const matrix = [[1, 2], [3, 4]]
println(matrix[0][1])
println(matrix[1][0])

// 混合使用属性访问和元素访问
const mixed = { items: [10, 20] }
println(mixed.items[1])
mixed.items[0] = 30
println(mixed.items[0])

`, []string{
		"10",
		"30",
		"\"Bob\"",
		"25",
		"\"Bob\"",
		"200",
		"100",
		"10",
		"30",
		"30",
		"2",
		"3",
		"20",
		"30",
	}, t)
}

func TestNestedAccessPatterns(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
// 测试嵌套的属性和元素访问组合
const data = {
  users: [
    { id: 1, name: "Alice" },
    { id: 2, name: "Bob" }
  ],
  settings: {
    theme: "dark"
  }
}

// 读取嵌套值
println(data.users[0].name)
println(data.users[1].id)
println(data["users"][0]["name"])
println(data.settings.theme)

// 修改嵌套值
data.users[0].name = "Alicia"
data["settings"]["theme"] = "light"
println(data.users[0].name)
println(data.settings.theme)

// 复合赋值
let counter = { values: [10, 20] }
counter.values[0] += 5
counter["values"][1] *= 2
println(counter.values[0])
println(counter.values[1])

// 同一个对象的不同属性访问方式
const mixed = { 
  a: 1, 
  b: 2,
  c: [3, 4],
  d: { e: 5 }
}

println(mixed.a)
println(mixed["b"])
println(mixed.c[0])
println(mixed["c"][1])
println(mixed.d.e)
println(mixed["d"]["e"])

// 使用同一个索引/键访问不同对象
const index = 0;
const key = "name";
const collection = [
  { name: "First" },
  { name: "Second" }
];

println(collection[index][key])
println(collection[index+1][key])
`, []string{
		"\"Alice\"",
		"2",
		"\"Alice\"",
		"\"dark\"",
		"\"Alicia\"",
		"\"light\"",
		"15",
		"40",
		"1",
		"2",
		"3",
		"4",
		"5",
		"5",
		"\"First\"",
		"\"Second\"",
	}, t)
}

func TestAssignmentVariations(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
// 测试各种赋值变体与属性/元素访问

// 1. 链式赋值
let obj1 = { a: 0 }
let obj2 = { b: 0 }
let obj3 = { c: 0 }

obj1.a = obj2.b = obj3.c = 100
println(obj1.a)
println(obj2.b)
println(obj3.c)

// 2. 所有可能的复合赋值操作符
let target = { 
  num: 10,
  arr: [5, 10]
}

// 算术复合赋值
target.num += 5
println(target.num)
target.num -= 3
println(target.num)
target.num *= 2
println(target.num)
target.num /= 4
println(target.num)
target.num %= 2
println(target.num)
target.num = 2
target.num **= 3
println(target.num)

// 位运算复合赋值
target.num = 5
target.num &= 3
println(target.num)
target.num = 5
target.num |= 3
println(target.num)
target.num = 5
target.num ^= 3
println(target.num)
target.num = 5
target.num <<= 1
println(target.num)
target.num = 5
target.num >>= 1
println(target.num)
target.num = 5
target.num >>>= 1
println(target.num)

// 逻辑赋值操作符
target.num = 0
target.num ||= 42

target.num &&= 10

target.num = null
target.num ??= 99


// 数组元素的复合赋值
target.arr[0] += 10
target.arr[1] *= 3
println(target.arr[0])
println(target.arr[1])

// 3. 连续访问并赋值
const deep = { a: { b: { c: { value: 1 } } } }
deep.a.b.c.value += 10
println(deep.a.b.c.value)

// 4. 使用属性访问结果进行计算并赋值回去
const compute = { x: 5, y: 10 }
compute.x = compute.x + compute.y
println(compute.x)

// 5. 交换两个属性值
const swap = { first: "A", second: "B" }
const temp = swap.first
swap.first = swap.second
swap.second = temp
println(swap.first)
println(swap.second)
`, []string{
		"100",
		"100",
		"100",
		"15",
		"12",
		"24",
		"6",
		"0",
		"8",
		"1",
		"7",
		"6",
		"10",
		"2",
		"2",
		"15",
		"30",
		"11",
		"15",
		"\"B\"",
		"\"A\"",
	}, t)
}

func TestPropertyAndElementCombinations(t *testing.T) {
	t.Parallel()

	ssatest.CheckPrintlnValue(`
// 测试属性访问和元素访问的各种组合

// 1. 属性访问 -> 元素访问 -> 属性访问
const data = {
  items: [
    { id: 1, name: "Item 1" },
    { id: 2, name: "Item 2" }
  ]
}

println(data.items[0].name)
println(data.items[1].id)

// 2. 元素访问 -> 属性访问 -> 元素访问
const collections = [
  { keys: ["a", "b", "c"] },
  { keys: ["x", "y", "z"] }
]

println(collections[0].keys[1])
println(collections[1].keys[2])

// 3. 方法返回值上的属性/元素访问
function getData() {
  return {
    records: [
      { value: 100 },
      { value: 200 }
    ]
  }
}

// println(getData().records[0].value) 这个不应该通过println检测
// println(getData().records[1].value) 这个不应该通过println检测

// 4. 使用属性访问结果作为元素索引
const lookup = {
  index: 1,
  values: [10, 20, 30]
}

println(lookup.values[lookup.index])

// 5. 使用元素访问结果作为属性名
const dynamic = [
  { prop: "x" },
  { prop: "y" }
]

const target = { x: "X value", y: "Y value" }

println(target[dynamic[0].prop])
println(target[dynamic[1].prop])

// 6. 复杂的读写组合
const complex = {
  levels: [
    { points: [10, 20, 30] },
    { points: [40, 50, 60] }
  ]
}

// 读取和修改深层嵌套值
println(complex.levels[0].points[1])
complex.levels[1].points[2] = 99
println(complex.levels[1].points[2])

// 复合赋值
complex.levels[0].points[0] += 5
println(complex.levels[0].points[0])

// 使用一个位置的值设置另一个位置
complex.levels[0].points[2] = complex.levels[1].points[0]
println(complex.levels[0].points[2])
`, []string{
		"\"Item 1\"",
		"2",
		"\"b\"",
		"\"z\"",
		//		"100",
		//		"200",
		"20",
		"\"X value\"",
		"\"Y value\"",
		"20",
		"99",
		"15",
		"40",
	}, t)
}

func TestBasic_Variable_InBlock(t *testing.T) {
	t.Parallel()

	t.Run("test simple assign", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a)
		a = 2
		println(a)
`, []string{
			"1",
			"2",
		}, t)
	})

	t.Run("simple test", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		println(a)
		`, []string{"Undefined-a"}, t)
	})

	t.Run("test sub-scope capture parent scope in basic block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a)
		{
			a = 2
			println(a)
		}
		println(a)
		`, []string{
			"1",
			"2",
			"2",
		}, t)
	})

	t.Run("test sub-scope let local variable in basic block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		{
			let a = 2
			println(a) // 2
		}
		println(a) // 1
		`, []string{
			"1",
			"2",
			"1",
		}, t)
	})

	t.Run("test sub-scope var local variable in basic block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		{
			var a = 2
			println(a) // 2
		}
		println(a) // 2
		`, []string{
			"1",
			"2",
			"2",
		}, t)
	})

	t.Run("test sub-scope let local variable without assign in basic block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		{
			let a
			println(a) // any
		}
		println(a) // 1
		`, []string{
			"1",
			"Undefined-a",
			"1",
		}, t)
	})

	t.Run("test sub-scope function level variable in basic block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		{
			a = 2
			println(a) // 2
		}
		println(a) // 2
		`, []string{
			"1",
			"2",
			"2",
		}, t)
	})

	t.Run("test sub-scope and return", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		{
			let a  = 2 
			println(a) // 2
			return 
		}
		println(a) // unreachable
		`,
			[]string{
				"1", "2",
			}, t)
	})

	t.Run("undefine variable in sub-scope", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		{
			let a = 2
			println(a) // 2
		}
		println(a) // undefine-a
		`, []string{
			"2",
			"Undefined-a",
		}, t)
	})

	t.Run("test ++ expression", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		{
			a ++
			println(a) // 2
		}
		`,
			[]string{
				"2",
			},
			t)
	})

	t.Run("test syntax block lose capture variable", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1 
		{
			a = 2  // capture [a: 2]
			{
				println(a) // 2
			} 
			// end-scope capture is []
		}
		println(a) // 2
		
		`, []string{
			"2", "2",
		}, t)
	})
}

func TestBasic_Variable_InIf(t *testing.T) {
	t.Parallel()

	t.Run("test simple if", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a)
		if(c) {
			a = 2
			println(a)
		}
		println(a)
		`, []string{
			"1",
			"2",
			"phi(a)[2,1]",
		}, t)
	})
	t.Run("test simple if with local variable", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a)
		if(c) {
			let a = 2
			println(a)
		}
		println(a) // 1
		`, []string{
			"1",
			"2",
			"1",
		}, t)
	})

	t.Run("test multiple phi if", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		if(c) {
			a = 2
		}
		println(a)
		println(a)
		println(a)
		`, []string{
			"phi(a)[2,1]",
			"phi(a)[2,1]",
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("test multiple if ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	a = 1
	if(1) {
		if(2) {
			a = 2
		}
	}
	println(a)
	`,
			[]string{
				"phi(a)[phi(a)[2,1],1]",
			},
			t)
	})

	t.Run("test simple if else", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a)
		if(c) {
			a = 2
			println(a)
		} else {
			a = 3
			println(a)
		}
		println(a)
		`, []string{
			"1",
			"2",
			"3",
			"phi(a)[2,3]",
		}, t)
	})

	t.Run("test simple if else with origin branch", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a)
		if(c) {
			// a = 1
		} else {
			a = 3
			println(a)
		}
		println(a) // phi(a)[1, 3]
		`, []string{
			"1",
			"3",
			"phi(a)[1,3]",
		}, t)
	})

	t.Run("test if-elseif", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a)
		if(c) {
			a = 2
			println(a)
		}else if(c == 2){
			a = 3
			println(a)
		}
		println(a)
		`,
			[]string{
				"1",
				"2",
				"3",
				"phi(a)[2,3,1]",
			}, t)
	})
	t.Run("test with return, no DoneBlock", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		if(c) {
			return 
		}
		println(a) // phi(a)[Undefined-a,1]
		`, []string{
			"1",
			"phi(a)[Undefined-a,1]",
		}, t)
	})
	t.Run("test with return in branch, no DoneBlock", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		if(c) {
			if(b) {
				a = 2
				println(a) // 2
				return 
			}else {
				a = 3
				println(a) // 3
				return 
			}
			println(a) // unreachable // phi[2, 3]
		}
		println(a) // phi(a)[Undefined-a,1]
		`, []string{
			"1",
			"2",
			"3",
			"phi(a)[Undefined-a,1]",
		}, t)
	})

	t.Run("in if sub-scope", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		if(c) {
			let a = 2
		}
		println(a)
		`, []string{"Undefined-a"}, t)
	})
}

func TestBasic_Variable_If_Logical(t *testing.T) {
	t.Parallel()

	t.Run("test simple", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		if(c || b) {
			a = 2
		}
		println(a)
		`, []string{"phi(a)[2,1]"}, t)
	})

	t.Run("test multiple logical", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		if(c || b && d) {
			a = 2
		}
		println(a)
		`, []string{"phi(a)[2,1]"}, t)
	})
}

func TestBasic_variable_logical(t *testing.T) {
	t.Parallel()

	t.Run("simple", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1 || 2 
		println(a)`,
			[]string{
				"phi(a)[1,2]",
			}, t)
	})

	t.Run("test syntax block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		{
			t = 1 || 2
			println(t)
		}
		println(t)
		`, []string{
			"phi(t)[1,2]", "Undefined-t",
		}, t)
	})

	t.Run("test syntax block local", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		{
			let t = 1 || 2
			println(t)
		}
		println(t)
		`, []string{
			"phi(t)[1,2]", "Undefined-t",
		}, t)
	})

	t.Run("test closure not leak", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = () => {
			let t = 1 || 2
			println(t)
		}
		println(t)
		a()
		println(t)
		`, []string{
			"Undefined-t", "phi(t)[1,2]", "Undefined-t",
		}, t)
	})

	// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Strict_mode
	t.Run("test closure side-effect which will create global variable implicitly", func(t *testing.T) {
		t.Skip("目前还不能处理这种函数内部隐式创建全局变量，当前开启TryBuildValue选项后也只能创建函数级全局变量")
		ssatest.CheckPrintlnValue(`
		a = () => {
			t = 1
			println(t)
		}
		println(t)
		a()
		println(t)
		`, []string{
			"Undefined-t", "1", "side-effect(phi(t)[1,2], t)",
		}, t)
	})

	t.Run("test function side-effect which will create global variable implicitly", func(t *testing.T) {
		t.Skip("目前还不能处理这种函数内部隐式创建全局变量，当前开启TryBuildValue选项后也只能创建函数级全局变量")
		ssatest.CheckPrintlnValue(`
		function a(){
			t = 1
			println(t)
		}
		println(t)
		a()
		println(t)
		`, []string{
			"Undefined-t", "1", "side-effect(phi(t)[1,2], t)",
		}, t)
	})

	t.Run("test closure side-effect case 1", func(t *testing.T) {
		t.Skip("目前还不能处理这种函数内部隐式创建全局变量，当前开启TryBuildValue选项后也只能创建函数级全局变量")
		ssatest.CheckPrintlnValue(`
		a = () => {
			t = 1 || 2
			println(t)
		}
		println(t)
		a()
		println(t)
		`, []string{
			"Undefined-t", "phi(t)[1,2]", "side-effect(phi(t)[1,2], t)",
		}, t)
	})

	t.Run("test closure side-effect case 2", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		var t;
		a = () => {
			t = 1 || 2
			println(t)
		}
		println(t)
		a()
		println(t)
		`, []string{
			"Undefined-t", "phi(t)[1,2]", "side-effect(phi(t)[1,2], t)",
		}, t)
	})

	t.Run("test closure side-effect case 3", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		let t;
		a = () => {
			t = 1 || 2
			println(t)
		}
		println(t)
		a()
		println(t)
		`, []string{
			"Undefined-t", "phi(t)[1,2]", "side-effect(phi(t)[1,2], t)",
		}, t)
	})
}

func TestBasic_For_Loop(t *testing.T) {
	t.Parallel()

	t.Run("simple loop not change", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		for(i=0; i < 10 ; i++) {
			println(a) // 1
		}
		println(a) //1 
		`,
			[]string{
				"1",
				"1",
			},
			t)
	})

	t.Run("simple loop only condition", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		i = 1
		for(i < 10;;) { 
			println(i) // phi
			i = 2 
			println(i) // 2
		}
		println(i) // phi
		`, []string{
			"phi(i)[1,2]",
			"2",
			"phi(i)[1,2]",
		}, t)
	})

	t.Run("simple loop", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		i=0
		for(i=0; i<10; i++) {
			println(i) // phi[0, i+1]
		}
		println(i)
		`,
			[]string{
				"phi(i)[0,add(i, 1)]",
				"phi(i)[0,add(i, 1)]",
			}, t)
	})

	t.Run("loop with spin, signal phi", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		for(let i = 0; i < 10; i ++) { // i=0; i=phi[0,1]; i=0+1=1
			println(a) // phi[0, $+1]
			a = 0
			println(a) // 0 
		}
		println(a)  // phi[0, 1]
		`,
			[]string{
				"phi(a)[1,0]",
				"0",
				"phi(a)[1,0]",
			},
			t)
	})

	t.Run("loop with spin, double phi", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		for(let i = 0; i < 10; i++) {
			a += 1
			println(a) // add(phi, 1)
		}
		println(a)  // phi[1, add(phi, 1)]
		`,
			[]string{
				"add(phi(a)[1,add(a, 1)], 1)",
				"phi(a)[1,add(a, 1)]",
			},
			t)
	})
}

func TestBasic_DoWhile_Loop(t *testing.T) {
	t.Parallel()

	t.Run("simple do-while not change", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		do {
			println(a) // 1
		} while(i < 10)
		println(a) // 1
		`,
			[]string{
				"1",
				"1",
			},
			t)
	})

	t.Run("simple do-while with counter", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		i = 0
		do {
			println(i) // phi[0, i+1]
			i++
		} while(i < 3)
		println(i) // phi[i+1]
		`,
			[]string{
				"phi(i)[0,add(i, 1)]",
				"phi(i)[0,add(i, 1)]",
			},
			t)
	})
}

func TestBasic_While_Loop(t *testing.T) {
	t.Parallel()

	t.Run("simple while not change", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		while(i < 10) {
			println(a) // 1
		}
		println(a) // 1
		`,
			[]string{
				"1",
				"1",
			},
			t)
	})

	t.Run("while with counter", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		i = 0
		while(i < 3) {
			println(i) // phi[0, i+1]
			i++
		}
		println(i) // phi[0, i+1]
		`,
			[]string{
				"phi(i)[0,add(i, 1)]",
				"phi(i)[0,add(i, 1)]",
			},
			t)
	})

	t.Run("while with variable modification", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		i = 0
		while(i < 3) {
			a += 1
			println(a) // add(phi, 1)
			i++
		}
		println(a) // phi[1, add(phi,1)]
		`,
			[]string{
				"add(phi(a)[1,add(a, 1)], 1)",
				"phi(a)[1,add(a, 1)]",
			},
			t)
	})
}

func TestForInOf_SSA(t *testing.T) {
	t.Parallel()

	t.Run("for-in with phi", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		const obj = {a: 1, b: 2, c: 3}
		let sum = 0
		for(const key in obj) {
			sum += obj[key]
		}
		println(sum)
		`, []string{
			"phi(sum)[0,add(sum, Undefined-obj.key(valid))]", // for-in循环中的phi
		}, t)
	})

	t.Run("for-of with phi", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		const arr = [10, 20, 30]
		let sum = 0
		for(const val of arr) {
			sum += val
		}
		println(sum)
		`, []string{
			"phi(sum)[0,add(sum, Undefined-val(valid))]", // for-of循环中的phi
		}, t)
	})

}

func TestBasic_CFG_Break(t *testing.T) {
	t.Parallel()

	t.Run("simple break in loop", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		for(let i = 0; i < 10; i++) {
			if(i == 5) {
				a = 2
				break
			}
		}
		println(a) // phi[1, 2]
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("simple continue in loop", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		for(let i = 0; i < 10; i++) {
			if(i == 5) {
				a = 2
				continue
			}
		}
		println(a) // phi[1, 2]
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("simple break in switch-1", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		switch(a) {
		case 1:
			if(c) {
				a = 2
				break
			}
			a = 4
			break
		case 2:
			a = 3
		}
		println(a) // phi[1, 2, 3, 4]
		`, []string{
			"phi(a)[2,4,phi(a)[3,1]]",
		}, t)
	})

	t.Run("simple break in switch-2", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1;
		switch (a) {
		case 1:
			if (c) {
				 a = 2;
				 break;
			}
			a = 4;
		case 2:
			a = 3;
		}
		println(a) ;
		`, []string{
			"phi(a)[2,phi(a)[3,1]]",
		}, t)
	})

	t.Run("simple continue in loop", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
 		a = 1;
		for (let i = 0; i < 10; i++) {
			if (i == 5) {
				a = 2;
				continue;
			}
		}
		println(a); // phi[1, 2]
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("simple fallthrough in switch", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		switch(a) {
		case 1:
			a = 2
		case 2:
			println(a) // 1 2
			a = 3
		default: 
			a = 4
		}
		println(a) // 3 4
		`, []string{
			"phi(a)[2,1]",
			"4",
		}, t)
	})
}

func TestBasic_Variable_Switch(t *testing.T) {
	t.Parallel()

	t.Run("simple switch, no default", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		switch(a) {
		case 2: 
			a = 22
			println(a)
			break
		case 3:
			a = 33
			println(a)
			break
		}
		println(a) // phi[1, 22, 33]
		`, []string{
			"22", "33", "phi(a)[22,33,1]",
		}, t)
	})

	t.Run("simple switch, no default, without break", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		switch(a) {
		case 2: 
			a = 22
			println(a)
		case 3:
			a = 33
			println(a)
		}
		println(a) // phi[1, 22, 33]
		`, []string{
			"22", "33", "phi(a)[33,1]",
		}, t)
	})

	t.Run("simple switch, has default but nothing", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		switch(a) {
		case 2: 
			a = 22
			println(a)
			break
		case 3:
			a = 33
			println(a)
			break
		default: 
		}
		println(a) // phi[1, 22, 33]
		`, []string{
			"22", "33", "phi(a)[22,33,1]",
		}, t)
	})

	t.Run("simple switch, has default", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		a = 1
		switch(a) {
		case 2: 
			a = 22
			println(a)
			break
		case 3:
			a = 33
			println(a)
			break
		default: 
			a = 44
			println(a)
		}
		println(a) // phi[22, 33, 44]
		`, []string{
			"22", "33", "44", "phi(a)[22,33,44]",
		}, t)
	})

	t.Run("simple switch, has default branch and label block without break label", func(t *testing.T) {
		code := `
a = 2
outer: {
    switch (a) {
        case 2:
            break ;
            a = 3
        case 3:
            break ;
            a = 4
        default:
            a = 1
            println(a)
            break ;
    }
    a = 100
}
println(a)
			`
		prog, err := ssaapi.Parse(code,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		// 生成函数的控制流图
		dot := ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0])
		log.Infof("函数控制流图DOT: \n%s", dot)
		ssatest.CheckPrintlnValue(code, []string{
			"1", "100",
		}, t)
	})

	t.Run("simple switch, has default branch and label block without break label", func(t *testing.T) {
		code := `
a = 2
outer: {
    switch (a) {
        case 2:
            break outer; // ✅ 跳出 outer 标签块
            a = 3
        case 3:
            break outer; // ✅ 跳出 outer 标签块
            a = 4
        default:
            a = 1
            println(a)
            break outer; // ✅ 跳出 outer 标签块

    }
    a = 100
}
println(a)
			`
		prog, err := ssaapi.Parse(code,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		// 生成函数的控制流图
		dot := ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0])
		log.Infof("函数控制流图DOT: \n%s", dot)
		ssatest.CheckPrintlnValue(code, []string{
			"1", "phi(a)[2,2,1]", // not 100
		}, t)
	})
}

func TestFunctionCFG(t *testing.T) {
	t.Parallel()

	code := `
				a = 1
		println(a) // 1
		if(c) {
			if(b) {
				a = 2
				println(a) // 2
				return 
			}else {
				a = 3
				println(a) // 3
				return 
			}
			println(a) // unreachable // phi[2, 3]
		}
		println(a) // phi(a)[Undefined-a,1]
	`

	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage("ts"),
	)
	require.NoError(t, err)

	// 生成函数的控制流图
	subLib, ok := prog.Program.Application.UpStream.Get("/")
	assert.True(t, ok)
	dot := ssaapi.FunctionDotGraph(subLib.Funcs.Values()[0])
	log.Infof("函数控制流图DOT: \n%s", dot)

	// 验证控制流图包含必要的元素
	require.True(t, strings.Contains(dot, "digraph"), "控制流图应该包含digraph定义")
	require.True(t, strings.Contains(dot, "->"), "控制流图应该包含边")

	// 验证分支信息
	require.True(t, strings.Contains(dot, "true") || strings.Contains(dot, "false"), "控制流图应该包含条件分支标签")
}

func TestTemplateString(t *testing.T) {
	t.Parallel()

	t.Run("NoSubstitutionTemplateLiteral test", func(t *testing.T) {
		ssatest.CheckPrintlnValue("var a = `hello world`; println(a)", []string{`"hello world"`}, t)
	})

	t.Run("TemplateExpression with variable", func(t *testing.T) {
		ssatest.CheckPrintlnValue("var b = 123; var a = `b = ${b}`; println(a)", []string{`add("b = ", castType(string, 123))`}, t)
	})

	t.Run("TemplateExpression with expr", func(t *testing.T) {
		ssatest.CheckPrintlnValue("var a = `hello ${5 + 10} world`; println(a)", []string{`add(add("hello ", castType(string, 15)), " world")`}, t)
	})
}

func TestClass(t *testing.T) {
	t.Parallel()

	t.Run("simple class with constructor and field access", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			a = 0;
		}
		let a = new A();
		println(a.a);
		`, []string{"0"}, t)
	})

	t.Run("method with side effect (setA)", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			a = 0 
			setA(par) {
				this.a = par;
			}
		}
		let a = new A();
		println(a.a);
		a.setA(1);
		println(a.a);
		`, []string{
			"0",
			"side-effect(Parameter-par, this.a)",
		}, t)
	})

	t.Run("method returning member value", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			a = 0
			getA() {
				return this.a;
			}
		}
		let a = new A();
		println(a.getA());
		a.a = 1;
		println(a.getA());
		`, []string{
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[1]",
		}, t)
	})

	t.Run("just use method set/get", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			a = 0;
			getA() {
				return this.a;
			}
			setA(par) {
				this.a = par;
			}
		}
		let a = new A();
		println(a.getA());
		a.setA(1);
		println(a.getA());
		`, []string{
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[side-effect(Parameter-par, this.a)]",
		}, t)
	})

	t.Run("static member access and mutation", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			static a = 0;
			static getA() {
				return A.a;
			}
			static setA(par) {
				A.a = par;
			}
		}
		println(A.a)
		println(A.getA());
		A.setA(1);
		println(A.getA());
		`, []string{
			"0",
			"Function-A.getA() binding[A] member[0]",
			"Function-A.getA() binding[A] member[side-effect(Parameter-par, A.a)]",
		}, t)
	})

	t.Run("static method call", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class Calc {
	static add(a: number) { return a; }
}
println(Calc.add(1))
`, []string{"Function-Calc.add(1)"}, t)
	})

	t.Run("simple class with constructor and method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class Person {
			constructor(name) {
				this.name = name;
			}
			greet() {
				println("Hello, " + this.name);
			}
		}
		let p = new Person("Alice");
		p.greet();
		`,
			[]string{
				`add("Hello, ", ParameterMember-parameter[0].name)`,
			},
			t)
	})
}

func TestLabel(t *testing.T) {
	t.Parallel()

	t.Run("simple label jump outer loop", func(t *testing.T) {
		codeWithLabel := `
outer: for (let i = 0; i < 3; i++) {
	for (let j = 0; j < 3; j++) {
		if (foo == 2) {
			break outer; // 跳出外层循环
		}
		a = 0
	}
}
println(a);
`

		codeWithLabelNoBreakLabel := `
outer: for (let i = 0; i < 3; i++) {
	for (let j = 0; j < 3; j++) {
		if (foo = 2) {
			break;
		}
		a = 0
	}
}
println(a);
`

		codeWithoutLabel := `
for (let i = 0; i < 3; i++) {
	for (let j = 0; j < 3; j++) {
		if (foo == 2) {
			break;
		}
		a = 0
	}
}
println(a);
`

		prog, err := ssaapi.Parse(codeWithoutLabel,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
		ssatest.CheckPrintlnValue(codeWithoutLabel, []string{"phi(a)[Undefined-a,0]"}, t)

		prog, err = ssaapi.Parse(codeWithLabelNoBreakLabel,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
		ssatest.CheckPrintlnValue(codeWithLabelNoBreakLabel, []string{"phi(a)[Undefined-a,0]"}, t)

		prog, err = ssaapi.Parse(codeWithLabel,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
		ssatest.CheckPrintlnValue(codeWithLabel, []string{"Undefined-a"}, t)

	})

	t.Run("simple label jump outer block", func(t *testing.T) {
		code := `
		process: {
			break process; // 跳出 block，阻止继续执行后续语句
			a = 0
			println(a);
			println(a);
			a = 2
			println(a);
			println(a);
			a = 3
		}
println(a);
		`
		prog, err := ssaapi.Parse(code,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
		ssatest.CheckPrintlnValue(code, []string{"Undefined-a"}, t)
	})

	t.Run("simple label jump outer block", func(t *testing.T) {
		code := `
var k = 1
		outer1: for (let i = 0; i < 3; i++) {
  console.log("outer1:", i);
  outer2: for (let j = 0; j < 3; j++) {
    console.log("  outer2:", j);
    outer3: for (let k = 0; k < 3; k++) {
      console.log("outer3:", k);
      if (k == 1) {
        break outer1; // 直接跳出outer1, outer2，outer3 也随之结束
		k = 2
      }
    }
  }
}
println(k)
		`
		prog, err := ssaapi.Parse(code,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})

	t.Run("do-while", func(t *testing.T) {
		code := `
a = 1;

function b(){
	a += 1;
	return true;
}


do {
  break;
  a = 3;
} while (b());
println(a);`
		prog, err := ssaapi.Parse(code,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})

	t.Run("simple label jump outer block with do-while", func(t *testing.T) {
		code := `outer:
a = 1;

function b(){
	a += 1;
	return true;
}

outer:
do {
  console.log('开始执行任务');
  break outer;
  a = 3;
  

  console.log('执行核心任务...');
  // ...a
} while (b());

console.log('任务流程结束');
println(a);`
		prog, err := ssaapi.Parse(code,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		log.Info(ssaapi.FunctionDotGraph(prog.Program.Funcs.Values()[0]))
		ssatest.CheckPrintlnValue(code, []string{"1"}, t)
	})

}

func TestPanicWhenBuilt(t *testing.T) {
	t.Run("panic when switch built", func(t *testing.T) {
		code := `switch (i.shape) {
                            case "circle":
                            default:
                                i.shape = "circle";
                                break;
                            case "cardioid":
                                i.shape = function(t) {
                                    return 1 - Math.sin(t)
                                };
                                break;
                            case "diamond":
                                i.shape = function(t) {
                                    var e = t % (2 * Math.PI / 4);
                                    return 1 / (Math.cos(e) + Math.sin(e))
                                };
                                break;
                            case "square":
                                i.shape = function(t) {
                                    return Math.min(1 / Math.abs(Math.cos(t)), 1 / Math.abs(Math.sin(t)))
                                };
                                break;
                            case "triangle-forward":
                                i.shape = function(t) {
                                    var e = t % (2 * Math.PI / 3);
                                    return 1 / (Math.cos(e) + Math.sqrt(3) * Math.sin(e))
                                };
                                break;
                            case "triangle":
                            case "triangle-upright":
                                i.shape = function(t) {
                                    var e = (t + 3 * Math.PI / 2) % (2 * Math.PI / 3);
                                    return 1 / (Math.cos(e) + Math.sqrt(3) * Math.sin(e))
                                };
                                break;
                            case "pentagon":
                                i.shape = function(t) {
                                    var e = (t + .955) % (2 * Math.PI / 5);
                                    return 1 / (Math.cos(e) + .726543 * Math.sin(e))
                                };
                                break;
                            case "star":
                                i.shape = function(t) {
                                    var e = (t + .955) % (2 * Math.PI / 10);
                                    return (t + .955) % (2 * Math.PI / 5) - 2 * Math.PI / 10 >= 0 ? 1 / (Math.cos(2 * Math.PI / 10 - e) + 3.07768 * Math.sin(2 * Math.PI / 10 - e)) : 1 / (Math.cos(e) + 3.07768 * Math.sin(e))
                                }
                        }`
		prog, err := ssaapi.Parse(code,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		_ = prog
	})
	//t.Run("stuck", func(t *testing.T) {
	//	code := inf_loop_js_file
	//	prog, err := ssaapi.Parse(code,
	//		ssaapi.WithLanguage("ts"),
	//	)
	//	require.NoError(t, err)
	//	_ = prog
	//
	//})
	t.Run("stuck1", func(t *testing.T) {
		prog, err := ssaapi.Parse(`
  const r = function(n) {
                return function(e, t, r) {
                    for (var o = -1, i = Object(e), u = r(e), c = u.length; c--;) {
                        var a = u[n ? c : ++o];
                        if (!1 === t(i[a], a, i)) break
                    }
                    return e
                }
            }()
`,
			ssaapi.WithLanguage("ts"),
		)
		require.NoError(t, err)
		_ = prog

	})

}

//func TestDestructuring(t *testing.T) {
//	ssatest.CheckPrintlnValue(`
//const array = [1, 2];
//const obj = { a: 10, b: 20, c: 30 };
//const key = 'a';
//
//let a, b, a1, b1, c, d, rest;
//
//// Example 1
//let [a, b] = array;
//println(a, b);
//
//// Example 2
//let [a, , b] = array;
//println(a, b);
//
//// Example 3
//const aDefault = 100;
//[a = aDefault, b] = array;
//println(a, b);
//
//// Example 4
//[a, b, ...rest] = array;
//println(a, b, rest);
//
//// Example 5
//[a, , b, ...rest] = array;
//println(a, b, rest);
//
//// Example 7
//[a, b, ...[c, d]] = array;
//println(a, b, c, d);
//
//// Object Destructuring
//({ a, b } = obj);
//println(a, b);
//
//({ a: a1, b: b1 } = obj);
//println(a1, b1);
//
//({ a: a1 = aDefault, b = 200 } = obj);
//println(a1, b);
//
//({ a, b, ...rest } = obj);
//println(a, b, rest);
//
//({ a: a1, b: b1, ...rest } = obj);
//println(a1, b1, rest);
//
//({ [key]: a } = obj);
//println(a);
//`, []string{
//		"1 2",
//	}, t)
//}
