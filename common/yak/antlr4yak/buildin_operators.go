package antlr4yak

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

func _eq(value *yakvm.Value, value2 *yakvm.Value) *yakvm.Value {
	// if value.IsInt() && value2.IsInt() {
	// 	return yakvm.NewBoolValue(value.Int() == value2.Int())
	// }

	// if value.IsFloat() && value2.IsFloat() {
	// 	return yakvm.NewBoolValue(value.Float64() == value2.Float64())
	// }

	// if value.IsFloat() && value2.IsInt() {
	// 	return yakvm.NewBoolValue(value.Float64() == value2.Float64())
	// }

	// if value2.IsFloat() && value.IsInt() {
	// 	return yakvm.NewBoolValue(value.Float64() == value2.Float64())
	// }

	// if value2.IsBool() && value2.IsBool() {
	// 	return yakvm.NewBoolValue(value.True() == value2.True())
	// }

	// if value2.IsBytes() || value.IsBytes() {
	// 	// 如果任意一个是 bytes 的话，都转为 string 进行比较
	// 	return yakvm.NewBoolValue(value.String() == value2.String())
	// }

	// // 如果任意又一个值为 undefined 的话
	// if value.IsUndefined() || value2.IsUndefined() {
	// 	return yakvm.NewBoolValue(value.False() == value2.False())
	// }

	// return yakvm.NewBoolValue(funk.Equal(value.Value, value2.Value))
	return yakvm.NewBoolValue(value.Equal(value2))
}
func _neq(value *yakvm.Value, value2 *yakvm.Value) *yakvm.Value {
	return yakvm.NewBoolValue(_eq(value, value2).False())
}

func init() {

	// unary
	yakvm.ImportYakUnaryOperator(yakvm.OpNot, func(op *yakvm.Value) *yakvm.Value {
		b := op.True()
		return &yakvm.Value{
			TypeVerbose: "bool",
			Value:       !b,
			Literal:     fmt.Sprint(!b),
		}
	})

	yakvm.ImportYakUnaryOperator(yakvm.OpBitwiseNot, func(op *yakvm.Value) *yakvm.Value {
		if op.IsInt64() {
			resultInt64 := ^op.Int64()
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
		panic(fmt.Sprintf("cannot support ^op[%v]", op.TypeVerbose))
	})

	yakvm.ImportYakUnaryOperator(yakvm.OpNeg, func(op *yakvm.Value) *yakvm.Value {
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

	yakvm.ImportYakUnaryOperator(yakvm.OpPlus, func(op *yakvm.Value) *yakvm.Value {
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
	yakvm.ImportYakBinaryOperator(yakvm.OpEq, _eq)
	yakvm.ImportYakBinaryOperator(yakvm.OpNotEq, _neq)
	yakvm.ImportYakBinaryOperator(yakvm.OpGt, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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
		} else if op2.IsString() && op1.IsString() {
			v := op1.String() > op2.String()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] > op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportYakBinaryOperator(yakvm.OpGtEq, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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
		} else if op2.IsString() && op1.IsString() {
			v := op1.String() >= op2.String()
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] >= op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportYakBinaryOperator(yakvm.OpLt, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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
		} else if op2.IsString() && op1.IsString() {
			v := strings.Compare(op1.String(), op2.String()) < 0
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}
		panic(fmt.Sprintf("cannot support op1[%v] < op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportYakBinaryOperator(yakvm.OpLtEq, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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
		} else if op2.IsString() && op1.IsString() {
			v := strings.Compare(op1.String(), op2.String()) <= 0
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       v,
				Literal:     fmt.Sprint(v),
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] <= op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportYakBinaryOperator(yakvm.OpAnd, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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

	yakvm.ImportYakBinaryOperator(yakvm.OpAndNot, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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

	yakvm.ImportYakBinaryOperator(yakvm.OpOr, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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

	yakvm.ImportYakBinaryOperator(yakvm.OpXor, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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

	yakvm.ImportYakBinaryOperator(yakvm.OpShl, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
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

	yakvm.ImportYakBinaryOperator(yakvm.OpShr, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsInt64() && op2.IsInt64() {
			// 都是整数相加相减
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

	yakvm.ImportYakBinaryOperator(
		yakvm.OpAdd,
		func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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
			} else if op1.IsStringOrBytes() && op2.IsStringOrBytes() {
				v := op1.AsString() + op2.AsString()
				if op1.IsBytes() && op2.IsBytes() {
					return &yakvm.Value{
						TypeVerbose: "bytes",
						Value:       []byte(v),
						Literal:     fmt.Sprint(v),
					}
				} else if op1.IsString() && op2.IsString() {
					return &yakvm.Value{
						TypeVerbose: "string",
						Value:       v,
						Literal:     fmt.Sprint(v),
					}
				}
				panic(fmt.Sprintf("cannot support op1[%v] + op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
			} else if op1.IsString() && op2.IsInt64() {
				str := op1.Value.(string)
				char, ok := op2.IsInt64EX()
				if !ok {
					panic(fmt.Sprintf("cannot support convert %v to char", op2.Value))
				}
				ret := str + string(rune(char))
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       ret,
					Literal:     fmt.Sprint(ret),
				}
			} else if op1.IsInt64() && op2.IsString() {
				str := op2.Value.(string)
				char, ok := op1.Value.(rune)
				if !ok {
					panic("cannot support string + int64")
				}
				ret := string(char) + str
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       ret,
					Literal:     fmt.Sprint(ret),
				}
			} else if op1.IsBytes() && op2.IsInt64() {
				b := op1.Value.([]byte)
				char, ok := op2.IsInt64EX()
				if !ok {
					panic(fmt.Sprintf("cannot support convert %v to char", op2.Value))
				}
				ret := append(b, byte(char))
				return &yakvm.Value{
					TypeVerbose: "bytes",
					Value:       ret,
					Literal:     fmt.Sprint(ret),
				}
			} else if op1.IsInt64() && op2.IsBytes() {
				b := op2.Value.([]byte)
				char, ok := op1.Value.(rune)
				if !ok {
					panic("cannot support string + int64")
				}
				ret := append([]byte{byte(char)}, b...)
				return &yakvm.Value{
					TypeVerbose: "bytes",
					Value:       ret,
					Literal:     fmt.Sprint(ret),
				}
			}

			// slice/array merge
			var reversed bool
			if op2.IsIterable() && !op1.IsIterable() {
				// make sure op1 is iterable if op1/op2 is iterable
				op1, op2 = op2, op1
				reversed = true
			}

			if op1.IsIterable() {
				rv, rv2 := reflect.ValueOf(op1.Value), reflect.ValueOf(op2.Value)
				sliceLen := rv.Len()
				if op2.IsIterable() {
					sliceLen += rv2.Len()
				} else {
					sliceLen += 1
				}

				vals := make([]interface{}, 0, sliceLen)

				if reversed {
					if op2.IsIterable() {
						for i := 0; i < rv2.Len(); i++ {
							vals = append(vals, rv2.Index(i).Interface())
						}
					} else {
						vals = append(vals, op2.Value)
					}
					for i := 0; i < rv.Len(); i++ {
						vals = append(vals, rv.Index(i).Interface())
					}
				} else {
					for i := 0; i < rv.Len(); i++ {
						vals = append(vals, rv.Index(i).Interface())
					}
					if op2.IsIterable() {
						for i := 0; i < rv2.Len(); i++ {
							vals = append(vals, rv2.Index(i).Interface())
						}
					} else {
						vals = append(vals, op2.Value)
					}
				}

				elementType := yakvm.GuessBasicType(vals...)
				sliceType := reflect.SliceOf(elementType)

				newSlice := reflect.MakeSlice(sliceType, sliceLen, sliceLen)
				for index, e := range vals {
					val := reflect.ValueOf(e)
					err := (*yakvm.Frame)(nil).AutoConvertReflectValueByType(&val, elementType)
					if err != nil {
						panic(fmt.Sprintf("cannot convert %v to %v", val.Type(), elementType))
					}
					newSlice.Index(index).Set(val)
				}
				return yakvm.NewValue(sliceType.String(), newSlice.Interface(), "")
			}

			panic(fmt.Sprintf("cannot support op1[%v] + op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
		},
	)

	yakvm.ImportYakBinaryOperator(
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

	yakvm.ImportYakBinaryOperator(yakvm.OpMul, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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
			case op2.IsStringOrBytes():
				v := strings.Repeat(op2.AsString(), int(op1.Int64()))
				if op2.IsBytes() {
					return &yakvm.Value{
						TypeVerbose: "bytes",
						Value:       []byte(v),
						Literal:     v,
					}
				}
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
		case op1.IsStringOrBytes():
			switch {
			case op2.IsInt64():
				v := strings.Repeat(op1.AsString(), int(op2.Int64()))
				if op1.IsBytes() {
					return &yakvm.Value{
						TypeVerbose: "bytes",
						Value:       []byte(v),
						Literal:     v,
					}
				}
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       v,
					Literal:     strconv.Quote(v),
				}
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] * op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportYakBinaryOperator(yakvm.OpDiv, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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

	yakvm.ImportYakBinaryOperator(yakvm.OpMod, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
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
		} else if op1.IsStringOrBytes() {
			rv2 := reflect.ValueOf(op2.Value)
			switch rv2.Kind() {
			case reflect.Slice, reflect.Array:
				vals := make([]interface{}, rv2.Len())
				for i := 0; i < rv2.Len(); i++ {
					vals[i] = rv2.Index(i).Interface()
				}
				var formatted string
				if op2.IsStringOrBytes() {
					formatted = fmt.Sprintf(op1.AsString(), op2.String())
				} else {
					formatted = fmt.Sprintf(op1.AsString(), vals...)
				}
				if op1.IsBytes() {
					return &yakvm.Value{
						TypeVerbose: "bytes",
						Value:       []byte(formatted),
						Literal:     formatted,
					}
				}
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       formatted,
					Literal:     strconv.Quote(formatted),
				}
			default:
				formatted := fmt.Sprintf(op1.AsString(), op2.Value)
				if op1.IsBytes() {
					return &yakvm.Value{
						TypeVerbose: "bytes",
						Value:       formatted,
						Literal:     formatted,
					}
				}
				return &yakvm.Value{
					TypeVerbose: "string",
					Value:       formatted,
					Literal:     strconv.Quote(formatted),
				}
			}
		}

		panic(fmt.Sprintf("cannot support op1[%v] %% op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
	})

	yakvm.ImportYakBinaryOperator(yakvm.OpIn, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		var result, valid bool
		typA, typB := reflect.TypeOf(op1.Value), reflect.TypeOf(op2.Value)
		a, b := op1.Value, op2.Value

		switch typA.Kind() {
		case reflect.String:
			a1 := a.(string)
			switch typB.Kind() {
			case reflect.String:
				valid = true
				result = strings.Contains(b.(string), a1)
			case reflect.Slice:
				if b1, ok := b.([]byte); ok {
					valid = true
					result = bytes.Contains(b1, []byte(a1))
				}
				fallthrough
			case reflect.Array:
				valid = true
				valB := reflect.ValueOf(b)
				for i := 0; i < valB.Len(); i++ {
					if reflect.DeepEqual(a1, valB.Index(i).Interface()) {
						result = true
						break
					}
				}
			case reflect.Map:
				valid = true
				v := reflect.ValueOf(b).MapIndex(reflect.ValueOf(a))
				result = v.IsValid()
			case reflect.Ptr:
				valid = true
				vb := reflect.ValueOf(b)
				// safe check
				if vb.IsNil() || vb.IsZero() || !vb.IsValid() {
					result = false
					break
				}
				// exclude members that are not exported
				if !unicode.IsUpper(rune(a1[0])) {
					result = false
					break
				}
				typVB := reflect.TypeOf(vb.Elem().Interface())
				// only support *struct
				if typVB.Kind() != reflect.Struct {
					break
				}
				// field
				for i := 0; i < typVB.NumField(); i++ {
					field := typVB.Field(i)
					if field.Name == a1 {
						result = true
						goto END
					}
				}
				// method
				for i := 0; i < typB.NumMethod(); i++ {
					method := typB.Method(i)
					if method.Name == a1 {
						result = true
						goto END
					}
				}
			case reflect.Struct:
				// exclude members that are not exported
				if !unicode.IsUpper(rune(a1[0])) {
					result = false
					break
				}
				// field
				for i := 0; i < typB.NumField(); i++ {
					field := typB.Field(i)
					if field.Name == a1 {
						result = true
						goto END
					}
				}
				// method
				for i := 0; i < typB.NumMethod(); i++ {
					method := typB.Method(i)
					if method.Name == a1 {
						result = true
						goto END
					}
				}
			}
		case reflect.Slice:
			a1, ok := a.([]byte)
			if !ok {
				break
			}
			switch typB.Kind() {
			case reflect.String:
				valid = true
				result = bytes.Contains([]byte(b.(string)), a1)
			case reflect.Slice:
				if b1, ok := b.([]byte); ok {
					valid = true
					result = bytes.Contains(b1, a1)
				}
			case reflect.Array:
				valid = true
				valB := reflect.ValueOf(b)
				sliceB := valB.Slice(0, valB.Len())
				result = bytes.Contains(sliceB.Bytes(), a1)
			}
		}

		// default
		switch typB.Kind() {
		case reflect.Array, reflect.Slice:
			valid = true
			valB := reflect.ValueOf(b)
			for i := 0; i < valB.Len(); i++ {
				if reflect.DeepEqual(a, valB.Index(i).Interface()) {
					result = true
					goto END
				}
			}
		}
	END:
		if valid {
			return &yakvm.Value{
				TypeVerbose: "bool",
				Value:       result,
				Literal:     fmt.Sprint(result),
			}
		} else {
			panic(fmt.Sprintf("cannot support op1[%v] in op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
		}
	})

	yakvm.ImportYakBinaryOperator(yakvm.OpSendChan, func(op1 *yakvm.Value, op2 *yakvm.Value) *yakvm.Value {
		if op1.IsChannel() {
			rv := reflect.ValueOf(op1.Value)
			rv.Send(reflect.ValueOf(op2.Value))
			return yakvm.GetUndefined()
		} else {
			panic(fmt.Sprintf("cannot support op1[%v] <- op2[%v]", op1.TypeVerbose, op2.TypeVerbose))
		}
	})
}
