package yaklib

import "math"

var MathExport = map[string]interface{}{
	"Round":       math.Round,
	"Sqrt":        math.Sqrt,
	"Pow":         math.Pow,
	"Pow10":       math.Pow10,
	"Floor":       math.Floor,
	"Ceil":        math.Ceil,
	"RoundToEven": math.RoundToEven,
	"Abs":         math.Abs,
	"NaN":         math.NaN,
	"IsNaN":       math.IsNaN,
	"Pi":          math.Pi,
	"Ln10":        math.Ln10,
	"Ln2":         math.Ln2,
	"E":           math.E,
	"Sqrt2":       math.Sqrt2,
	"SqrtPi":      math.SqrtPi,
	"SqrtE":       math.SqrtE,
}
