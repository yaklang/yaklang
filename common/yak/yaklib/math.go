package yaklib

import "math"

var MathExport = map[string]interface{}{
	"Round":       Round,
	"Sqrt":        Sqrt,
	"Pow":         Pow,
	"Pow10":       Pow10,
	"Floor":       Floor,
	"Ceil":        Ceil,
	"RoundToEven": RoundToEven,
	"Abs":         Abs,
	"NaN":         NaN,
	"IsNaN":       IsNaN,
	"Sinh":        Sinh,
	"Sin":         Sin,
	"Cos":         Cos,
	"Tan":         Tan,
	"Asin":        Asin,
	"Acos":        Acos,
	"Atan":        Atan,
	"Pi":          math.Pi,
	"Ln10":        math.Ln10,
	"Ln2":         math.Ln2,
	"E":           math.E,
	"Sqrt2":       math.Sqrt2,
	"SqrtPi":      math.SqrtPi,
	"SqrtE":       math.SqrtE,
}

// Round 返回四舍五入到最近的整数
// 存在一些特殊情况：Round(±0) = ±0，Round(±Inf) = ±Inf，Round(NaN) = NaN
// 参数:
//   - x: 输入数值
//
// 返回值:
//   - 四舍五入到最近整数的结果
//
// Example:
// ```
// result = math.Round(1.5)
// println(result)   // OUT: 2
// assert result == 2.0, "Round should round half up"
// assert math.Round(1.4) == 1.0, "Round should round down below half"
// ```
func Round(x float64) float64 {
	return math.Round(x)
}

// Sqrt 返回一个数的平方根
// 如果x < 0，返回NaN
// 参数:
//   - x: 输入数值
//
// 返回值:
//   - x 的平方根；x<0 时为 NaN
//
// Example:
// ```
// result = math.Sqrt(4)
// println(result)   // OUT: 2
// assert result == 2.0, "Sqrt of 4 should be 2"
// assert math.IsNaN(math.Sqrt(-1)) == true, "Sqrt of negative should be NaN"
// ```
func Sqrt(x float64) float64 {
	return math.Sqrt(x)
}

// Pow 返回x的y次方
// 参数:
//   - x: 底数
//   - y: 指数
//
// 返回值:
//   - x 的 y 次幂
//
// Example:
// ```
// result = math.Pow(2, 3)
// println(result)   // OUT: 8
// assert result == 8.0, "2 to the power 3 should be 8"
// assert math.Pow(2, -1) == 0.5, "2 to the power -1 should be 0.5"
// ```
func Pow(x, y float64) float64 {
	return math.Pow(x, y)
}

// Pow10 返回10的n次方
// 参数:
//   - n: 整数指数
//
// 返回值:
//   - 10 的 n 次幂
//
// Example:
// ```
// result = math.Pow10(2)
// println(result)   // OUT: 100
// assert result == 100.0, "10 to the power 2 should be 100"
// assert math.Pow10(3) == 1000.0, "10 to the power 3 should be 1000"
// ```
func Pow10(n int) float64 {
	return math.Pow10(n)
}

// Floor 返回不大于x的最大整数
// 参数:
//   - x: 输入数值
//
// 返回值:
//   - 向下取整（朝 -Inf）后的结果
//
// Example:
// ```
// result = math.Floor(1.5)
// println(result)   // OUT: 1
// assert result == 1.0, "Floor should round down"
// assert math.Floor(-1.5) == -2.0, "Floor of negative rounds toward -Inf"
// ```
func Floor(x float64) float64 {
	return math.Floor(x)
}

// Ceil 返回不小于x的最小整数
// 参数:
//   - x: 输入数值
//
// 返回值:
//   - 向上取整（朝 +Inf）后的结果
//
// Example:
// ```
// result = math.Ceil(1.5)
// println(result)   // OUT: 2
// assert result == 2.0, "Ceil should round up"
// assert math.Ceil(-1.5) == -1.0, "Ceil of negative rounds toward +Inf"
// ```
func Ceil(x float64) float64 {
	return math.Ceil(x)
}

// RoundToEven 返回四舍五入到最近的偶整数
// 参数:
//   - x: 输入数值
//
// 返回值:
//   - 银行家舍入到最近偶整数的结果
//
// Example:
// ```
// // 银行家舍入：恰好 .5 时向最近的偶数取整
// result = math.RoundToEven(2.5)
// println(result)   // OUT: 2
// assert result == 2.0, "2.5 rounds to even 2"
// assert math.RoundToEven(1.5) == 2.0, "1.5 rounds to even 2"
// assert math.RoundToEven(3.5) == 4.0, "3.5 rounds to even 4"
// ```
func RoundToEven(x float64) float64 {
	return math.RoundToEven(x)
}

// Abs 返回x的绝对值
// 参数:
//   - x: 输入数值
//
// 返回值:
//   - x 的绝对值
//
// Example:
// ```
// result = math.Abs(-1)
// println(result)   // OUT: 1
// assert result == 1.0, "Abs of -1 should be 1"
// assert math.Abs(1) == 1.0, "Abs of 1 should be 1"
// ```
func Abs(x float64) float64 {
	return math.Abs(x)
}

// NaN 返回一个IEEE-574 “非数字”的值
// 返回值:
//   - 一个 NaN 浮点值
//
// Example:
// ```
// result = math.IsNaN(math.NaN())
// println(result)   // OUT: true
// assert result == true, "NaN should produce a NaN value"
// ```
func NaN() float64 {
	return math.NaN()
}

// IsNaN 判断一个数是否是NaN
// 参数:
//   - x: 输入数值
//
// 返回值:
//   - 是否为 NaN
//
// Example:
// ```
// result = math.IsNaN(math.NaN())
// println(result)   // OUT: true
// assert result == true, "NaN should be detected"
// assert math.IsNaN(1) == false, "1 is a number"
// ```
func IsNaN(x float64) bool {
	return math.IsNaN(x)
}

// Sinh 双曲正弦函数
// 参数:
//   - x: 输入数值（弧度）
//
// 返回值:
//   - x 的双曲正弦值
//
// Example:
// ```
// result = math.Sinh(0)
// println(result)   // OUT: 0
// assert result == 0.0, "Sinh of 0 should be 0"
// ```
func Sinh(x float64) float64 {
	return math.Sinh(x)
}

//trigonometric functions

// Sin 三角函数 sin
// 参数:
//   - x: 输入角度（弧度）
//
// 返回值:
//   - x 的正弦值
//
// Example:
// ```
// result = math.Sin(0)
// println(result)   // OUT: 0
// assert result == 0.0, "Sin of 0 should be 0"
// ```
func Sin(x float64) float64 {
	return math.Sin(x)
}

// Cos 三角函数 Cos
// 参数:
//   - x: 输入角度（弧度）
//
// 返回值:
//   - x 的余弦值
//
// Example:
// ```
// result = math.Cos(0)
// println(result)   // OUT: 1
// assert result == 1.0, "Cos of 0 should be 1"
// ```
func Cos(x float64) float64 {
	return math.Cos(x)
}

// Tan 三角函数 Tan
// 参数:
//   - x: 输入角度（弧度）
//
// 返回值:
//   - x 的正切值
//
// Example:
// ```
// result = math.Tan(0)
// println(result)   // OUT: 0
// assert result == 0.0, "Tan of 0 should be 0"
// ```
func Tan(x float64) float64 {
	return math.Tan(x)
}

// Asin 反三角函数 Asin
// 参数:
//   - x: 输入数值（区间 [-1, 1]）
//
// 返回值:
//   - x 的反正弦值（弧度）
//
// Example:
// ```
// result = math.Asin(0)
// println(result)   // OUT: 0
// assert result == 0.0, "Asin of 0 should be 0"
// ```
func Asin(x float64) float64 {
	return math.Asin(x)
}

// Acos 反三角函数 Acos
// 参数:
//   - x: 输入数值（区间 [-1, 1]）
//
// 返回值:
//   - x 的反余弦值（弧度）
//
// Example:
// ```
// result = math.Acos(1)
// println(result)   // OUT: 0
// assert result == 0.0, "Acos of 1 should be 0"
// ```
func Acos(x float64) float64 {
	return math.Acos(x)
}

// Atan 反三角函数 Atan
// 参数:
//   - x: 输入数值
//
// 返回值:
//   - x 的反正切值（弧度）
//
// Example:
// ```
// result = math.Atan(0)
// println(result)   // OUT: 0
// assert result == 0.0, "Atan of 0 should be 0"
// ```
func Atan(x float64) float64 {
	return math.Atan(x)
}
