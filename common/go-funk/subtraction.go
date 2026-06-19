package funk

import (
	"reflect"
)

// Subtract 返回集合 x 中存在但集合 y 中不存在的元素
// 参数:
//   - x: 源集合
//   - y: 要排除的集合
//
// 返回值:
//   - 仅属于 x 而不属于 y 的元素集合
//
// Example:
// ```
// // VARS: 求差集
// result = x.Subtract([1, 2, 3, 4], [2, 4])
// // STDOUT: 打印结果
// println(result)   // OUT: [1 3]
// // assert: 锁定结论
// assert len(result) == 2, "Subtract should keep elements only in x"
// ```
func Subtract(x interface{}, y interface{}) interface{} {
	if !IsCollection(x) {
		panic("First parameter must be a collection")
	}
	if !IsCollection(y) {
		panic("Second parameter must be a collection")
	}

	hash := map[interface{}]struct{}{}

	xValue := reflect.ValueOf(x)
	xType := xValue.Type()

	yValue := reflect.ValueOf(y)
	yType := yValue.Type()

	if NotEqual(xType, yType) {
		panic("Parameters must have the same type")
	}

	zType := reflect.SliceOf(xType.Elem())
	zSlice := reflect.MakeSlice(zType, 0, 0)

	for i := 0; i < xValue.Len(); i++ {
		v := xValue.Index(i).Interface()
		hash[v] = struct{}{}
	}

	for i := 0; i < yValue.Len(); i++ {
		v := yValue.Index(i).Interface()
		_, ok := hash[v]
		if ok {
			delete(hash, v)
		}
	}

	for i := 0; i < xValue.Len(); i++ {
		v := xValue.Index(i).Interface()
		_, ok := hash[v]
		if ok {
			zSlice = reflect.Append(zSlice, xValue.Index(i))
		}
	}

	return zSlice.Interface()
}

// Subtract 返回两个字符串切片的差集
// Example:
// ```
// str.Subtract(["1", "2", "3"], ["3", "4", "5"]) // ["1", "2"]
// ```
func SubtractString(x []string, y []string) []string {
	if len(x) == 0 {
		return []string{}
	}

	if len(y) == 0 {
		return x
	}

	slice := []string{}
	hash := map[string]struct{}{}

	for _, v := range x {
		hash[v] = struct{}{}
	}

	for _, v := range y {
		_, ok := hash[v]
		if ok {
			delete(hash, v)
		}
	}

	for _, v := range x {
		_, ok := hash[v]
		if ok {
			slice = append(slice, v)
		}
	}

	return slice
}
