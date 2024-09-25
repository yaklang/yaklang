package executor

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/executor/nasl_type"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func convertBoolToInt(f func(value *yakvm.Value, value2 *yakvm.Value) *yakvm.Value) func(value *yakvm.Value, value2 *yakvm.Value) *yakvm.Value {
	return func(value *yakvm.Value, value2 *yakvm.Value) *yakvm.Value {
		res := f(value, value2)
		if res.Bool() {
			return yakvm.NewIntValue(1)
		} else {
			return yakvm.NewIntValue(0)
		}
	}
}

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

func convertBoolValueToInt(b bool) int {
	if b {
		return 1
	} else {
		return 0
	}
}

func init() {
	yakvm.ImportNaslUnaryOperator(yakvm.OpNot, func(op *yakvm.Value) *yakvm.Value {
		if v, ok := op.Value.(*nasl_type.NaslArray); ok {
			b := len(v.Num_elt) != 0 || len(v.Hash_elt) != 0
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       convertBoolValueToInt(!b),
			}
		}
		var b bool
		if op.IsString() {
			b = op.String() != ""
		} else if op.IsInt() {
			b = op.Int() != 0
		} else {
			b = op.Value != nil
		}
		return &yakvm.Value{
			TypeVerbose: "bool",
			Value:       convertBoolValueToInt(!b),
		}
	})

	yakvm.ImportNaslUnaryOperator(yakvm.OpNeg, func(op *yakvm.Value) *yakvm.Value {
		if op.IsInt64() {
			v := op.Int64()
			return &yakvm.Value{
				TypeVerbose: "int64",
				Value:       -v,
			}
		} else if op.IsFloat() {
			v := op.Float64()
			return &yakvm.Value{
				TypeVerbose: "float64",
				Value:       -v,
			}
		}
		panic(fmt.Sprintf("cannot support - op1[%v]", op.TypeVerbose))
	})

	yakvm.ImportNaslUnaryOperator(yakvm.OpPlus, func(op *yakvm.Value) *yakvm.Value {
		if op.IsInt64() {
			v := +op.Int64()
			return &yakvm.Value{
				TypeVerbose: "int64",
				Value:       v,
			}
		} else if op.IsFloat() {
			v := op.Float64()
			return &yakvm.Value{
				TypeVerbose: "float64",
				Value:       v,
			}
		}
		panic(fmt.Sprintf("cannot support + op1[%v]", op.TypeVerbose))
	})

	// binary
	yakvm.ImportNaslBinaryOperator(yakvm.OpEq, convertBoolToInt(_eq))
	yakvm.ImportNaslBinaryOperator(yakvm.OpNotEq, convertBoolToInt(_neq))
	yakvm.ImportNaslBinaryOperator(yakvm.OpGt, convertBoolToInt(func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsUndefined() {
			op1 = yakvm.NewAutoValue(0)
		}
		if op2.IsUndefined() {
			op2 = yakvm.NewAutoValue(0)
		}
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() > op2.Int64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			v := op1.Float64() > op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() > op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] > op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	}))

	yakvm.ImportNaslBinaryOperator(yakvm.OpGtEq, convertBoolToInt(func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsUndefined() {
			op1 = yakvm.NewAutoValue(0)
		}
		if op2.IsUndefined() {
			op2 = yakvm.NewAutoValue(0)
		}
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() >= op2.Int64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			ret := op1.Float64() >= op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       ret,
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() >= op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] >= op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	}))

	yakvm.ImportNaslBinaryOperator(yakvm.OpLt, convertBoolToInt(func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsUndefined() {
			op1 = yakvm.NewAutoValue(0)
		}
		if op2.IsUndefined() {
			op2 = yakvm.NewAutoValue(0)
		}
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() < op2.Int64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			v := op1.Float64() < op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() < op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] < op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	}))

	yakvm.ImportNaslBinaryOperator(yakvm.OpLtEq, convertBoolToInt(func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsUndefined() {
			op1 = yakvm.NewAutoValue(0)
		}
		if op2.IsUndefined() {
			op2 = yakvm.NewAutoValue(0)
		}
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() <= op2.Int64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			v := op1.Float64() <= op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() <= op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] <= op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	}))

	yakvm.ImportNaslBinaryOperator(yakvm.OpAnd, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
			resultInt64 := op1.Int64() & op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] & op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportNaslBinaryOperator(yakvm.OpAndNot, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
			resultInt64 := op1.Int64() &^ op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] &^ op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportNaslBinaryOperator(yakvm.OpOr, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
			resultInt64 := op1.Int64() | op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] | op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportNaslBinaryOperator(yakvm.OpXor, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
			resultInt64 := op1.Int64() ^ op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] & op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportNaslBinaryOperator(yakvm.OpShl, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			resultInt64 := op1.Int64() << op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] << op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportNaslBinaryOperator(yakvm.OpShr, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			resultInt64 := op1.Int64() >> op2.Int64()
			if resultInt64 > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       resultInt64,
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(resultInt64),
				}
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] >> op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportNaslBinaryOperator(
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
					}
				} else {
					return &yakvm.Value{
						TypeVerbose: "int",
						Value:       int(v),
					}
				}
			} else if op1.IsUndefined() && op2.IsInt64() {
				v := 0 + op2.Int64()
				if v > math.MaxInt {
					return &yakvm.Value{
						TypeVerbose: "int64",
						Value:       v,
					}
				} else {
					return &yakvm.Value{
						TypeVerbose: "int",
						Value:       int(v),
					}
				}
			} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
				v := op1.Float64() + op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
				}
			} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
				v := op1.Float64() + op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
				}
			} else if op1.IsString() && op2.IsString() {
				v := op1.AsString() + op2.AsString()
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
				}
			} else if op1.IsString() && op2.IsInt64() {
				v := op1.AsString() + strconv.Itoa(int(op2.Int64()))
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
				}
			} else if op1.IsInt64() && op2.IsString() {
				v := strconv.Itoa(int(op1.Int64())) + op2.AsString()
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
				}
			} else if op1.IsString() && op2.Value == nil {
				return op1
			} else if op1.IsString() {
				v := op1.AsString() + utils.InterfaceToString(op2.Value)
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
				}
			}
			panic(fmt.Sprintf("cannot support op1[%v] + op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
		},
	)

	yakvm.ImportNaslBinaryOperator(
		yakvm.OpSub,
		func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
			if op1.IsInt64() && op2.IsInt64() {
				v := op1.Int64() - op2.Int64()
				if v > math.MaxInt {
					return &yakvm.Value{
						TypeVerbose: "int64",
						Value:       v,
					}
				} else {
					return &yakvm.Value{
						TypeVerbose: "int",
						Value:       int(v),
					}
				}
			} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
				v := op1.Float64() - op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
				}
			} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
				v := op1.Float64() - op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
				}
			} else if op2.IsString() && op1.IsString() {
				index := strings.Index(op1.String(), op2.String())
				v := op1.String()[:index] + op1.String()[index+len(op2.String()):]
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
				}
			}

			panic(fmt.Sprintf("cannot support op1[%v] - op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
		},
	)

	yakvm.ImportNaslBinaryOperator(yakvm.OpMul, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		switch {
		case op1.IsInt64():
			switch {
			case op2.IsInt64():
				v := op1.Int64() * op2.Int64()
				if v > math.MaxInt {
					return &yakvm.Value{
						TypeVerbose: "int64",
						Value:       v,
					}
				} else {
					return &yakvm.Value{
						TypeVerbose: "int",
						Value:       int(v),
					}
				}
			case op2.IsFloat():
				v := op1.Float64() * op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
				}
			case op2.IsString():
				v := strings.Repeat(op2.AsString(), int(op1.Int64()))
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
				}
			}
		case op1.IsFloat():
			switch {
			case op2.IsFloat(), op2.IsInt():
				v := op1.Float64() * op2.Float64()
				return &yakvm.Value{
					TypeVerbose: "float64",
					Value:       v,
				}
			}
		case op1.IsString():
			switch {
			case op2.IsInt64():
				v := strings.Repeat(op1.AsString(), int(op2.Int64()))
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
				}
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] * op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportNaslBinaryOperator(yakvm.OpDiv, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() / op2.Int64()
			if v > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       v,
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(v),
				}
			}
		} else if op1.IsFloat() && (op2.IsInt64() || op2.IsFloat()) {
			v := op1.Float64() / op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "float64",
				Value:       v,
			}
		} else if op2.IsFloat() && (op1.IsInt64() || op1.IsFloat()) {
			v := op1.Float64() / op2.Float64()
			return &yakvm.Value{
				TypeVerbose: "float64",
				Value:       v,
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] / op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportNaslBinaryOperator(yakvm.OpMod, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			v := op1.Int64() % op2.Int64()
			if v > math.MaxInt {
				return &yakvm.Value{
					TypeVerbose: "int64",
					Value:       v,
				}
			} else {
				return &yakvm.Value{
					TypeVerbose: "int",
					Value:       int(v),
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
				}
			default:
				formatted := fmt.Sprintf(op1.AsString(), op2.Value)
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       formatted,
				}
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] %% op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})
}
