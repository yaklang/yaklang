package tests

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestSimplePrint(t *testing.T) {
	t.Parallel()
	ssatest.CheckPrintlnValue(`
let a = 1
println(a)
`, []string{"1"}, t)
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
	parse, err := ssaapi.Parse(code, ssaapi.WithLanguage("new-js"))
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
	parse, err := ssaapi.Parse(code, ssaapi.WithLanguage("new-js"))
	require.NoError(t, err)
	parse.Show()
}

func TestNullishCoalescingBinaryExpressions(t *testing.T) {
	t.Parallel()
	code := `
let e = (null ?? 5) ?? 10
`
	parse, err := ssaapi.Parse(code, ssaapi.WithLanguage("new-js"))
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
	parse, err := ssaapi.Parse(code, ssaapi.WithLanguage("new-js"))
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
