package yakvm

import (
	"fmt"
	"reflect"
	"sort"
	"yaklang/common/go-funk"
)

var buildMethodsArray = map[string]interface{}{}

func NewArrayMethodFactory(f func(*Frame, *Value, interface{}) interface{}) MethodFactory {
	return func(vm *Frame, value interface{}) interface{} {
		v := value.(*Value)
		return f(vm, v, v.Value)
	}
}

var arrayBuildinMethod map[string]*buildinMethod

func aliasArrayBuildinMethod(origin string, target string) {
	if i, ok := arrayBuildinMethod[origin]; ok {
		arrayBuildinMethod[target] = i
	}
}

func init() {
	arrayBuildinMethod = map[string]*buildinMethod{
		"Append": {
			Name:            "Append",
			ParamTable:      []string{"element"},
			IsVariadicParam: true,
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func(vi ...interface{}) {
					rv := reflect.ValueOf(i)

					sliceLen := rv.Len() + len(vi)
					vals := make([]interface{}, 0, sliceLen)
					for i := 0; i < rv.Len(); i++ {
						vals = append(vals, rv.Index(i).Interface())
					}
					for _, v := range vi {
						vals = append(vals, v)
					}
					elementType := GuessBasicType(vals...)
					sliceType := reflect.SliceOf(elementType)

					newSlice := reflect.MakeSlice(sliceType, sliceLen, sliceLen)
					for index, e := range vals {
						val := reflect.ValueOf(e)
						err := vm.AutoConvertReflectValueByType(&val, elementType)
						if err != nil {
							panic(fmt.Sprintf("cannot convert %v to %v", val.Type(), elementType))
						}
						newSlice.Index(index).Set(val)
					}
					ref.Assign(vm, NewAutoValue(newSlice.Interface()))
				}
			}),
			Description: "往数组/切片最后追加元素",
		},
		"Pop": {
			Name: "Pop",
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func(na ...int) interface{} {
					rv := reflect.ValueOf(i)
					vLen := rv.Len()
					n := vLen - 1
					if len(na) > 0 {
						n = na[0]
						if n < 0 {
							n = vLen - 1 + n - 1
						}
						if n > vLen-1 || n < 0 {
							n = vLen - 1
						}
					}
					ret := rv.Index(n).Interface()
					newSlice := reflect.AppendSlice(rv.Slice(0, n), rv.Slice(n+1, vLen))
					ref.Assign(vm, NewAutoValue(newSlice.Interface()))
					return ret
				}
			}),
			Description: "弹出数组/切片的一个元素,默认为最后一个",
		},
		"Extend": {
			Name:       "Extend",
			ParamTable: []string{"newSlice"},
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func(vi interface{}) {
					rv2 := reflect.ValueOf(vi)
					rt2 := rv2.Type().Kind()
					if rt2 != reflect.Array && rt2 != reflect.Slice {
						panic(fmt.Sprintf("extend argument[%v] is not iterable", rv2.Type()))
					}
					rv := reflect.ValueOf(i)

					sliceLen := rv.Len() + rv2.Len()
					vals := make([]interface{}, 0, sliceLen)
					for i := 0; i < rv.Len(); i++ {
						vals = append(vals, rv.Index(i).Interface())
					}
					for i := 0; i < rv2.Len(); i++ {
						vals = append(vals, rv2.Index(i).Interface())
					}
					elementType := GuessBasicType(vals...)
					sliceType := reflect.SliceOf(elementType)

					newSlice := reflect.MakeSlice(sliceType, sliceLen, sliceLen)
					for index, e := range vals {
						val := reflect.ValueOf(e)
						err := vm.AutoConvertReflectValueByType(&val, elementType)
						if err != nil {
							panic(fmt.Sprintf("cannot convert %v to %v", val.Type(), elementType))
						}
						newSlice.Index(index).Set(val)
					}
					ref.Assign(vm, NewAutoValue(newSlice.Interface()))
				}
			}),
			Description: "用一个新的数组/切片扩展原数组/切片",
		},
		"Length": {
			Name: "Length",
			HandlerFactory: NewArrayMethodFactory(func(frame *Frame, value *Value, i interface{}) interface{} {
				return func() int {
					r := reflect.ValueOf(i)
					_ = r.Len()
					switch r.Kind() {
					case reflect.Array:
					case reflect.Slice:
					default:
						panic(fmt.Sprintf("caller type: %v cannot call `length`", r.Type()))
					}
					return r.Len()
				}
			}),
			Description: "获取长度",
		},
		"Capability": {
			Name: "Capability",
			HandlerFactory: NewArrayMethodFactory(func(frame *Frame, value *Value, i interface{}) interface{} {
				return func() int {
					r := reflect.ValueOf(i)
					switch r.Kind() {
					case reflect.Array:
					case reflect.Slice:
					default:
						panic(fmt.Sprintf("caller type: %v cannot call `cap`", r.Type()))
					}
					return r.Cap()
				}
			}),
			Description: "获取容量",
		},
		"StringSlice": {
			Name:        "StringSlice",
			Description: "转换成 []string",
			HandlerFactory: NewArrayMethodFactory(func(frame *Frame, value *Value, i interface{}) interface{} {
				return func() []string {
					rv := reflect.ValueOf(i)
					if rv.Len() <= 0 {
						return nil
					}

					vLen := rv.Len()
					var result = make([]string, vLen)
					for i := 0; i < vLen; i++ {
						val := rv.Index(i)
						if a, ok := val.Interface().([]byte); ok {
							result[i] = string(a)
						} else if s, ok := val.Interface().(string); ok {
							result[i] = s
						} else if !val.IsValid() || val.IsZero() {
							result[i] = ""
						} else {
							result[i] = fmt.Sprint(val.Interface())
						}
					}
					return result
				}
			}),
		},
		"GeneralSlice": {
			Name:        "GeneralSlice",
			Description: "转换成最泛化的 Slice 类型 []any ([]interface{})",
			HandlerFactory: NewArrayMethodFactory(func(frame *Frame, value *Value, i interface{}) interface{} {
				return func() []interface{} {
					return funk.Map(i, func(i interface{}) interface{} {
						return i
					}).([]interface{})
				}
			}),
		},
		"Shift": {
			Name:        "Shift",
			Description: "从数据开头移除一个元素",
			HandlerFactory: NewArrayMethodFactory(func(frame *Frame, value *Value, i interface{}) interface{} {
				return func() interface{} {
					rv := reflect.ValueOf(i)
					originLen := rv.Len()
					if originLen <= 0 {
						return nil
					}

					target := reflect.MakeSlice(rv.Type(), originLen-1, originLen-1)
					for i := 0; i < originLen-1; i++ {
						target.Index(i).Set(rv.Index(i + 1))
					}
					value.Assign(frame, NewAutoValue(target.Interface()))
					return rv.Index(0).Interface()
				}
			}),
		},
		"Unshift": {
			Name:        "Unshift",
			Description: "从数据开头增加一个元素",
			ParamTable:  []string{"element"},
			HandlerFactory: NewArrayMethodFactory(func(frame *Frame, value *Value, caller interface{}) interface{} {
				return func(raw interface{}) {
					rv := reflect.ValueOf(caller)
					vLen := rv.Len()

					var vals = make([]interface{}, vLen+1)
					vals[0] = raw
					for i := 0; i < vLen; i++ {
						vals[i+1] = rv.Index(i).Interface()
					}
					target := reflect.MakeSlice(reflect.SliceOf(GuessBasicType(vals...)), vLen+1, vLen+1)
					for i := 0; i < vLen+1; i++ {
						target.Index(i).Set(reflect.ValueOf(vals[i]))
					}
					value.Assign(frame, NewAutoValue(target.Interface()))
				}
			}),
		},
		"Map": {
			Name:        "Map",
			Description: "数据/切片中元素经过运算返回结果",
			ParamTable:  []string{"mapFunc"},
			HandlerFactory: NewArrayMethodFactory(func(frame *Frame, value *Value, i interface{}) interface{} {
				return func(handler func(i interface{}) interface{}) interface{} {
					return funk.Map(i, handler)
				}
			}),
		},
		"Filter": {
			Name:        "Filter",
			Description: "数据/切片中元素经过运算返回结果",
			ParamTable:  []string{"filterFunc"},
			HandlerFactory: NewArrayMethodFactory(func(frame *Frame, value *Value, i interface{}) interface{} {
				return func(handler func(i interface{}) bool) interface{} {
					return funk.Filter(i, handler)
				}
			}),
		},
		"Insert": {
			Name:       "Insert",
			ParamTable: []string{"index", "element"},
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func(n int, vi interface{}) {
					rv := reflect.ValueOf(i)
					vLen := rv.Len()
					if n > vLen {
						n = vLen
					} else if n < 0 {
						n = vLen + n
						if n < 0 {
							n = 0
						}
					}

					sliceLen := rv.Len() + 1
					vals := make([]interface{}, sliceLen)
					for i := 0; i < n; i++ {
						vals[i] = rv.Index(i).Interface()
					}
					vals[n] = vi

					for i := n + 1; i < vLen+1; i++ {
						vals[i] = rv.Index(i - 1).Interface()
					}
					elementType := GuessBasicType(vals...)
					sliceType := reflect.SliceOf(elementType)

					newSlice := reflect.MakeSlice(sliceType, sliceLen, sliceLen)
					for index, e := range vals {
						val := reflect.ValueOf(e)
						err := vm.AutoConvertReflectValueByType(&val, elementType)
						if err != nil {
							panic(fmt.Sprintf("cannot convert %v to %v", val.Type(), elementType))
						}
						newSlice.Index(index).Set(val)
					}
					ref.Assign(vm, NewAutoValue(newSlice.Interface()))
				}
			}),
			Description: "在指定位置插入元素",
		},
		"Remove": {
			Name:       "Remove",
			ParamTable: []string{"element"},
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func(vi interface{}) {
					rv := reflect.ValueOf(i)
					vLen := rv.Len()
					n := -1
					for i := 0; i < vLen; i++ {
						if funk.Equal(rv.Index(i).Interface(), vi) {
							n = i
							break
						}
					}
					newSlice := reflect.AppendSlice(rv.Slice(0, n), rv.Slice(n+1, vLen))
					ref.Assign(vm, NewAutoValue(newSlice.Interface()))
				}
			}),
			Description: "移除数组/切片的第一次出现的元素",
		},
		"Reverse": {
			Name: "Reverse",
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func() {
					rv := reflect.ValueOf(i)
					vLen := rv.Len()
					for i := 0; i < vLen/2; i++ {
						temp := reflect.ValueOf(rv.Index(i).Interface())
						temp2 := reflect.ValueOf(rv.Index(vLen - 1 - i).Interface())
						rv.Index(i).Set(temp2)
						rv.Index(vLen - 1 - i).Set(temp)
					}
				}
			}),
			Description: "反转数组/切片",
		},
		"Sort": {
			Name:            "Sort",
			ParamTable:      []string{"reverse"},
			IsVariadicParam: true,
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func(reversea ...bool) {
					reverse := false
					if len(reversea) > 0 {
						reverse = reversea[0]
					}
					rv := reflect.ValueOf(i)

					sort.SliceStable(i, func(i, j int) bool {
						if reverse {
							return fmt.Sprint(rv.Index(i).Interface()) > fmt.Sprint(rv.Index(j).Interface())
						}
						return fmt.Sprint(rv.Index(i).Interface()) < fmt.Sprint(rv.Index(j).Interface())
					})
				}
			}),
			Description: "排序数组/切片",
		},
		"Clear": {
			Name: "Clear",
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func() {
					nv := reflect.MakeSlice(reflect.SliceOf(literalReflectType_Interface), 0, 0)
					ref.Assign(vm, NewAutoValue(nv.Interface()))
				}
			}),
			Description: "清空数组/切片",
		},
		"Count": {
			Name:       "Count",
			ParamTable: []string{"element"},
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func(vi interface{}) int {
					n := 0

					rv := reflect.ValueOf(i)
					vLen := rv.Len()
					for i := 0; i < vLen; i++ {
						if funk.Equal(rv.Index(i).Interface(), vi) {
							n++
						}
					}
					return n
				}
			}),
			Description: "计算数组/切片中元素数量",
		},
		"Index": {
			Name:       "Index",
			ParamTable: []string{"indexInt"},
			HandlerFactory: NewArrayMethodFactory(func(vm *Frame, ref *Value, i interface{}) interface{} {
				return func(n int) interface{} {
					rv := reflect.ValueOf(i)
					vLen := rv.Len()
					if n < 0 {
						n = vLen + n
					}
					if n > vLen-1 {
						n = vLen - 1
					} else if n < 0 {
						n = 0
					}

					return rv.Index(n).Interface()
				}
			}),
			Description: "返回数组/切片中第n个元素",
		},
	}

	// alias
	aliasArrayBuildinMethod("Append", "Push")
	aliasArrayBuildinMethod("Extend", "Merge")
	aliasArrayBuildinMethod("Capability", "Cap")
	aliasArrayBuildinMethod("Length", "Len")
}
