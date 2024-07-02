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
// Example:
// ```
// math.Round(1.5) // 2
// math.Round(1.4) // 1
// ```
func Round(x float64) float64 {
	return math.Round(x)
}

// Sqrt 返回一个数的平方根
// 如果x < 0，返回NaN
// Example:
// ```
// math.Sqrt(4) // 2
// math.Sqrt(-1) // NaN
// ```
func Sqrt(x float64) float64 {
	return math.Sqrt(x)
}

// Pow 返回x的y次方
// Example:
// ```
// math.Pow(2, 3) // 8
// math.Pow(2, -1) // 0.5
// ```
func Pow(x, y float64) float64 {
	return math.Pow(x, y)
}

// Pow10 返回10的n次方
// Example:
// ```
// math.Pow10(2) // 100
// math.Pow10(-1) // 0.1
// ```
func Pow10(n int) float64 {
	return math.Pow10(n)
}

// Floor 返回不大于x的最大整数
// Example:
// ```
// math.Floor(1.5) // 1
// math.Floor(-1.5) // -2
// ```
func Floor(x float64) float64 {
	return math.Floor(x)
}

// Ceil 返回不小于x的最小整数
// Example:
// ```
// math.Ceil(1.5) // 2
// math.Ceil(-1.5) // -1
// ```
func Ceil(x float64) float64 {
	return math.Ceil(x)
}

// RoundToEven 返回四舍五入到最近的偶整数
// Example:
// ```
// math.RoundToEven(1.5) // 2
// math.RoundToEven(2.5) // 2
// math.RoundToEven(3.5) // 4
// math.RoundToEven(4.5) // 4
// ```
func RoundToEven(x float64) float64 {
	return math.RoundToEven(x)
}

// Abs 返回x的绝对值
// Example:
// ```
// math.Abs(-1) // 1
// math.Abs(1) // 1
// ```
func Abs(x float64) float64 {
	return math.Abs(x)
}

// NaN 返回一个IEEE-574 “非数字”的值
// Example:
// ```
// math.NaN()
// ```
func NaN() float64 {
	return math.NaN()
}

// IsNaN 判断一个数是否是NaN
// Example:
// ```
// math.IsNaN(1) // false
// math.IsNaN(math.NaN()) // true
// ```
func IsNaN(x float64) bool {
	return math.IsNaN(x)
}

// Sinh 双曲正弦函数
func Sinh(x float64) float64 {
	return math.Sinh(x)
}

//trigonometric functions

// Sin 三角函数 sin
func Sin(x float64) float64 {
	return math.Sin(x)
}

// Cos 三角函数 Cos
func Cos(x float64) float64 {
	return math.Cos(x)
}

// Tan 三角函数 Tan
func Tan(x float64) float64 {
	return math.Tan(x)
}

// Asin 反三角函数 Asin
func Asin(x float64) float64 {
	return math.Asin(x)
}

// Acos 反三角函数 Acos
func Acos(x float64) float64 {
	return math.Acos(x)
}

// Atan 反三角函数 Atan
func Atan(x float64) float64 {
	return math.Atan(x)
}
