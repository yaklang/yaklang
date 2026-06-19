package funk

import (
	"reflect"
)

// IsSubset 判断集合 x 是否为集合 y 的子集
// 参数:
//   - x: 待判断的子集
//   - y: 父集合
//
// 返回值:
//   - x 是否为 y 的子集
//
// Example:
// ```
// // VARS: 判断子集关系
// result = x.IsSubset([1, 2], [1, 2, 3])
// // STDOUT: 打印结果
// println(result)   // OUT: true
// // assert: 锁定结论
// assert result == true, "[1,2] is a subset of [1,2,3]"
// ```
func Subset(x interface{}, y interface{}) bool {
	if !IsCollection(x) {
		panic("First parameter must be a collection")
	}
	if !IsCollection(y) {
		panic("Second parameter must be a collection")
	}

	xValue := reflect.ValueOf(x)
	xType := xValue.Type()

	yValue := reflect.ValueOf(y)
	yType := yValue.Type()

	if NotEqual(xType, yType) {
		panic("Parameters must have the same type")
	}

	if xValue.Len() == 0 {
		return true
	}

	if yValue.Len() == 0 || yValue.Len() < xValue.Len() {
		return false
	}

	for i := 0; i < xValue.Len(); i++ {
		if !Contains(yValue.Interface(), xValue.Index(i).Interface()) {
			return false
		}
	}

	return true
}
