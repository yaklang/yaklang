package funk

import (
	"fmt"
	"reflect"
)

// Keys 返回 map 的所有键或结构体的所有字段名组成的切片
// 参数:
//   - out: map 或结构体
//
// 返回值:
//   - 键名/字段名切片
//
// Example:
// ```
// // VARS: 取出 map 的键
// result = x.Keys({"a": 1})
// // STDOUT: 打印键
// println(result)   // OUT: [a]
// // assert: 单键 map 只有一个键
// assert len(result) == 1, "single-key map should have one key"
// ```
func Keys(out interface{}) interface{} {
	value := redirectValue(reflect.ValueOf(out))
	valueType := value.Type()

	if value.Kind() == reflect.Map {
		keys := value.MapKeys()

		length := len(keys)

		resultSlice := reflect.MakeSlice(reflect.SliceOf(valueType.Key()), length, length)

		for i, key := range keys {
			resultSlice.Index(i).Set(key)
		}

		return resultSlice.Interface()
	}

	if value.Kind() == reflect.Struct {
		length := value.NumField()

		resultSlice := make([]string, length)

		for i := 0; i < length; i++ {
			resultSlice[i] = valueType.Field(i).Name
		}

		return resultSlice
	}

	panic(fmt.Sprintf("Type %s is not supported by Keys", valueType.String()))
}

// Values 返回 map 的所有值或结构体的所有字段值组成的切片
// 参数:
//   - out: map 或结构体
//
// 返回值:
//   - 值切片
//
// Example:
// ```
// // VARS: 取出 map 的值
// result = x.Values({"a": 1})
// // STDOUT: 打印值
// println(result)   // OUT: [1]
// // assert: 单键 map 只有一个值
// assert len(result) == 1, "single-key map should have one value"
// ```
func Values(out interface{}) interface{} {
	value := redirectValue(reflect.ValueOf(out))
	valueType := value.Type()

	if value.Kind() == reflect.Map {
		keys := value.MapKeys()

		length := len(keys)

		resultSlice := reflect.MakeSlice(reflect.SliceOf(valueType.Elem()), length, length)

		for i, key := range keys {
			resultSlice.Index(i).Set(value.MapIndex(key))
		}

		return resultSlice.Interface()
	}

	if value.Kind() == reflect.Struct {
		length := value.NumField()

		resultSlice := make([]interface{}, length)

		for i := 0; i < length; i++ {
			resultSlice[i] = value.Field(i).Interface()
		}

		return resultSlice
	}

	panic(fmt.Sprintf("Type %s is not supported by Keys", valueType.String()))
}
