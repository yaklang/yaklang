package tests

import (
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
	ssatest.CheckPrintlnValue(`
let a = true && false
let b = true || false
let d = false || (true && false)
let e = true && true
println(a)
println(b)
println(d)
println(e)
`, []string{
		"false", // a = true && false
		"true",  // b = true || false
		"false", // d = false || (true && false)
		"true",  // e = true && true
	}, t)
}

func TestNullishCoalescingBinaryExpressions(t *testing.T) {
	t.Parallel()
	ssatest.CheckPrintlnValue(`
let a = null ?? 5
let b = undefined ?? 10
let c = 0 ?? 20
let d = "" ?? 30
let e = (null ?? 5) ?? 10
println(a)
println(b)
println(c)
println(d)
println(e)
`, []string{
		"5",
		"10",
		"0",
		"",
		"5",
	}, t)
}

func TestLogicalAssignmentBinaryExpressions(t *testing.T) {
	t.Parallel()
	ssatest.CheckPrintlnValue(`
let a = false
a ||= true
let b = true
b &&= false
let c = false
c ??= 42
println(a, b, c)
`, []string{
		"true",  // a ||= true
		"false", // b &&= false
		"42",    // c ??= 42
	}, t)
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
