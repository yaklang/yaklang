package antlr4nasl

import (
	"fmt"
	"math"
	"yaklang/common/go-funk"
	"yaklang/common/yak/antlr4yak/yakvm"
	"reflect"
	"strconv"
	"strings"
)

func _eq(value *yakvm.Value, value2 *yakvm.Value) *yakvm.Value {
	if value.IsInt() && value2.IsInt() {
		return yakvm.NewBoolValue(value.Int() == value2.Int())
	}

	if value.IsFloat() && value2.IsFloat() {
		return yakvm.NewBoolValue(value.Float64() == value2.Float64())
	}

	if value.IsFloat() && value2.IsInt() {
		return yakvm.NewBoolValue(value.Float64() == value2.Float64())
	}

	if value2.IsFloat() && value.IsInt() {
		return yakvm.NewBoolValue(value.Float64() == value2.Float64())
	}

	if value2.IsBool() && value2.IsBool() {
		return yakvm.NewBoolValue(value.True() == value2.True())
	}

	return yakvm.NewBoolValue(funk.Equal(value.Value, value2.Value))
}
func _neq(value *yakvm.Value, value2 *yakvm.Value) *yakvm.Value {
	return yakvm.NewBoolValue(_eq(value, value2).False())
}

func init() {
	yakvm.ImportUnaryOperator(yakvm.OpNot, func(op *yakvm.Value) *yakvm.Value {
		b := op.True()
		return &yakvm.Value{
			TypeVerbose: "bool",
			Value:       !b,
			Literal:     fmt.Sprint(!b),
		}
	})

	yakvm.ImportUnaryOperator(yakvm.OpNeg, func(op *yakvm.Value) *yakvm.Value {
		if op.IsInt64() {
			v := op.Int64()
			return &yakvm.Value{
				TypeVerbose: "int64",
				Value:       -v,
				Literal:     fmt.Sprint(-v),
			}
		} else if op.IsFloat() {
			v := op.Float64()
			return &yakvm.Value{
				TypeVerbose: "float64",
				Value:       -v,
				Literal:     fmt.Sprint(-v),
			}
		}
		panic(fmt.Sprintf("cannot support - op1[%v]", op.TypeVerbose))
	})

	yakvm.ImportUnaryOperator(yakvm.OpPlus, func(op *yakvm.Value) *yakvm.Value {
		if op.IsInt64() {
			v := +op.Int64()
			return &yakvm.Value{
				TypeVerbose: "int64",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		} else if op.IsFloat() {
			v := op.Float64()
			return &yakvm.Value{
				TypeVerbose: "float64",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}
		panic(fmt.Sprintf("cannot support + op1[%v]", op.TypeVerbose))
	})

	// binary
	yakvm.ImportBinaryOperator(yakvm.OpEq, _eq)
	yakvm.ImportBinaryOperator(yakvm.OpNotEq, _neq)
	yakvm.ImportBinaryOperator(yakvm.OpGt, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() > op2.Int64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			v := op1.Float64() > op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() > op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] > op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpGtEq, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() >= op2.Int64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			ret := op1.Float64() >= op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       ret,
				Literal:     fmt.Sprint(ret),
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() >= op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] >= op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpLt, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() < op2.Int64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			v := op1.Float64() < op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() < op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] < op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpLtEq, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() <= op2.Int64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			v := op1.Float64() <= op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() <= op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] <= op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpAnd, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
			resultInt64 := op1.Int64() & op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
					Literal:     fmt.Sprint(resultInt64),
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
					Literal:     fmt.Sprint(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] & op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpAndNot, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
			resultInt64 := op1.Int64() &^ op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
					Literal:     fmt.Sprint(resultInt64),
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
					Literal:     fmt.Sprint(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] &^ op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpOr, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
			resultInt64 := op1.Int64() | op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
					Literal:     fmt.Sprint(resultInt64),
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
					Literal:     fmt.Sprint(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] | op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpXor, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
			resultInt64 := op1.Int64() ^ op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
					Literal:     fmt.Sprint(resultInt64),
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
					Literal:     fmt.Sprint(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] & op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpShl, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			resultInt64 := op1.Int64() << op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
					Literal:     fmt.Sprint(resultInt64),
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
					Literal:     fmt.Sprint(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] << op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpShr, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			resultInt64 := op1.Int64() >> op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
					Literal:     fmt.Sprint(resultInt64),
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
					Literal:     fmt.Sprint(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] >> op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(
		yakvm.OpAdd,
		func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
			if op1.IsUndefined() {
				return op2
			}
			if op2.IsUndefined() {
				return op1
			}
			if op1.IsInt64() && op2.IsInt64() {
				v := op1.Int64() + op2.Int64()
				if v > math.MaxInt {
					return &yakvm.Value{
						TypeVerbose: "int64",
						Value:       v,
						Literal:     fmt.Sprint(v),
					}
				} else {
					return &yakvm.Value{
						TypeVerbose: "int",
						Value:       int(v),
						Literal:     fmt.Sprint(v),
					}
				}
			} else if op1.IsUndefined() && op2.IsInt64() {
				v := 0 + op2.Int64()
				if v > math.MaxInt {
					return &yakvm.Value{
						TypeVerbose: "int64",
						Value:       v,
						Literal:     fmt.Sprint(v),
					}
				} else {
					return &yakvm.Value{
						TypeVerbose: "int",
						Value:       int(v),
						Literal:     fmt.Sprint(v),
					}
				}
			} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
				v := op1.Float64() + op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
				v := op1.Float64() + op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			} else if op1.IsString() && op2.IsString() {
				v := op1.AsString() + op2.AsString()
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			} else if op1.IsString() && op2.IsInt64() {
				v := op1.AsString() + strconv.Itoa(int(op2.Int64()))
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			} else if op1.IsInt64() && op2.IsString() {
				v := strconv.Itoa(int(op1.Int64())) + op2.AsString()
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			} else if op1.IsString() && op2.Value == nil {
				return op1
			}
			panic(fmt.Sprintf("cannot support op1[%v] + op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
		},
	)

	yakvm.ImportBinaryOperator(
		yakvm.OpSub,
		func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
			if op1.IsInt64() && op2.IsInt64() {
				v := op1.Int64() - op2.Int64()
				if v > math.MaxInt {
					return &yakvm.Value{
						TypeVerbose: "int64",
						Value:       v,
						Literal:     fmt.Sprint(v),
					}
				} else {
					return &yakvm.Value{
						TypeVerbose: "int",
						Value:       int(v),
						Literal:     fmt.Sprint(v),
					}
				}
			} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
				v := op1.Float64() - op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
				v := op1.Float64() - op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			}

			panic(fmt.Sprintf("cannot support op1[%v] - op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
		},
	)

	yakvm.ImportBinaryOperator(yakvm.OpMul, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		switch {
		case op1.IsInt64():
			switch {
			case op2.IsInt64():
				v := op1.Int64() * op2.Int64()
				if v > math.MaxInt {
					return &yakvm.Value{
						TypeVerbose: "int64",
						Value:       v,
						Literal:     fmt.Sprint(v),
					}
				} else {
					return &yakvm.Value{
						TypeVerbose: "int",
						Value:       int(v),
						Literal:     fmt.Sprint(v),
					}
				}
			case op2.IsFloat():
				v := op1.Float64() * op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			case op2.IsString():
				v := strings.Repeat(op2.AsString(), int(op1.Int64()))
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
					Literal:     strconv.Quote(v),
				}
			}
		case op1.IsFloat():
			switch {
			case op2.IsFloat(), op2.IsInt():
				v := op1.Float64() * op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			}
		case op1.IsString():
			switch {
			case op2.IsInt64():
				v := strings.Repeat(op1.AsString(), int(op2.Int64()))
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
					Literal:     strconv.Quote(v),
				}
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] * op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpDiv, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() / op2.Int64()
			if v > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(v),
					Literal:     fmt.Sprint(v),
				}
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			v := op1.Float64() / op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "float64",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() / op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "float64",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] / op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportBinaryOperator(yakvm.OpMod, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() % op2.Int64()
			if v > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       v,
					Literal:     fmt.Sprint(v),
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(v),
					Literal:     fmt.Sprint(v),
				}
			}
		} else if op1.IsString() {
			rv2 := reflect.ValueOf(op2.Value)
			switch rv2.Kind() {
			case reflect.Slice, reflect.Array:
				vals := make([]interface{}, rv2.Len())
				for i := 0; i < rv2.Len(); i++ {
					vals[i] = rv2.Index(i).Interface()
				}
				formatted := fmt.Sprintf(op1.AsString(), vals...)
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       formatted,
					Literal:     strconv.Quote(formatted),
				}
			default:
				formatted := fmt.Sprintf(op1.AsString(), op2.Value)
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       formatted,
					Literal:     strconv.Quote(formatted),
				}
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] %% op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})
}
