package yakvm

import (
	"fmt"
	"reflect"
	"yaklang.io/yaklang/common/go-funk"
)

func aliasMapBuildinMethod(origin string, target string) {
	if i, ok := mapBuildinMethod[origin]; ok {
		mapBuildinMethod[target] = i
	}
}
func NewMapMethodFactory(f func(frame *Frame, v *Value) interface{}) MethodFactory {
	return func(vm *Frame, i interface{}) interface{} {
		mapV := i.(*Value)
		return f(vm, mapV)
	}
}

var mapBuildinMethod map[string]*buildinMethod

func init() {
	mapBuildinMethod = map[string]*buildinMethod{
		"Keys": {
			Name:       "Keys",
			ParamTable: nil,
			HandlerFactory: NewMapMethodFactory(func(frame *Frame, v *Value) interface{} {
				return func() interface{} {
					return funk.Keys(v.Value)
				}
			}),
			Description: "获取所有元素的key",
		},
		"Values": {
			Name:       "Values",
			ParamTable: nil,
			HandlerFactory: NewMapMethodFactory(func(frame *Frame, v *Value) interface{} {
				return func() interface{} {
					return funk.Values(v.Value)
				}
			}),
			Description: "获取所有元素的value",
		},
		"Entries": {
			Name:       "Entries",
			ParamTable: nil,
			HandlerFactory: NewMapMethodFactory(func(frame *Frame, v *Value) interface{} {
				return func() interface{} {
					ikeys := funk.Keys(v.Value)
					if ikeys == nil {
						return []interface{}{}
					}
					refV := reflect.ValueOf(v.Value)
					var result [][]interface{}
					if funk.IsIteratee(ikeys) {
						funk.ForEach(ikeys, func(key interface{}) {
							v := refV.MapIndex(reflect.ValueOf(key))
							result = append(result, []interface{}{key, v.Interface()})
						})
					}
					return result
				}
			}),
			Description: "获取所有元素的entity",
		},
		"ForEach": {
			Name:       "ForEach",
			ParamTable: []string{"handler"},
			HandlerFactory: NewMapMethodFactory(func(frame *Frame, v *Value) interface{} {
				return func(f func(k, v interface{})) interface{} {
					funk.ForEach(v.Value, f)
					return nil
				}
			}),
			Description: "遍历元素",
		},
		"Set": {
			Name:       "Set",
			ParamTable: []string{"key", "value"},
			HandlerFactory: NewMapMethodFactory(func(frame *Frame, caller *Value) interface{} {
				return func(k, v interface{}) interface{} {
					refV, err := frame.AutoConvertYakValueToNativeValue(NewAutoValue(v))
					if err != nil {
						panic(fmt.Sprintf("runtime error: cannot assign %v to map[index]", v))
					}
					reflect.ValueOf(caller.Value).SetMapIndex(reflect.ValueOf(k), refV)
					return true
				}
			}),
			Description: "设置元素的值，如果key不存在则添加",
		},
		"Remove": {
			Name:       "Remove",
			ParamTable: []string{"key"},
			HandlerFactory: NewMapMethodFactory(func(frame *Frame, val *Value) interface{} {
				return func(paramK interface{}) interface{} {
					refMap := reflect.ValueOf(val.Value)
					refMap.SetMapIndex(reflect.ValueOf(paramK), reflect.ValueOf(nil))
					return nil
				}
			}),
			Description: "移除一个值",
		},
		"Has": {
			Name:       "Has",
			ParamTable: []string{"key"},
			HandlerFactory: NewMapMethodFactory(func(frame *Frame, v *Value) interface{} {
				return func(k interface{}) interface{} {
					var ok bool
					funk.ForEach(v.Value, func(k_, v interface{}) {
						if funk.Equal(k, k_) {
							ok = true
						}
					})
					return ok
				}
			}),
			Description: "判断map元素中是否包含key",
		},
		"Length": {
			Name:       "Length",
			ParamTable: nil,
			HandlerFactory: NewMapMethodFactory(func(frame *Frame, v *Value) interface{} {
				return func() interface{} {
					return reflect.ValueOf(v.Value).Len()
				}
			}),
			Description: "获取元素长度",
		},
	}
	aliasMapBuildinMethod("Entries", "Items")
	aliasMapBuildinMethod("Remove", "Delete")
	aliasMapBuildinMethod("Has", "IsExisted")
	aliasMapBuildinMethod("Length", "Len")
}
