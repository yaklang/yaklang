package funk

import (
	"reflect"
)

// Tuple is the return type of Zip
type Tuple struct {
	Element1 interface{}
	Element2 interface{}
}

// Zip 将两个切片按下标两两组合成元组列表，长度取较短切片
// 参数:
//   - slice1: 第一个切片
//   - slice2: 第二个切片
//
// 返回值:
//   - 元组列表，每个元组包含两个切片中同下标的元素
//
// Example:
// ```
// // VARS: 按下标配对
// result = x.Zip([1, 2], ["a", "b"])
// // assert: 配对数量为较短切片长度
// assert len(result) == 2, "zip should pair elements by index"
// ```
func Zip(slice1 interface{}, slice2 interface{}) []Tuple {
	if !IsCollection(slice1) || !IsCollection(slice2) {
		panic("First parameter must be a collection")
	}

	var (
		minLength int
		inValue1  = reflect.ValueOf(slice1)
		inValue2  = reflect.ValueOf(slice2)
		result    = []Tuple{}
		length1   = inValue1.Len()
		length2   = inValue2.Len()
	)

	if length1 <= length2 {
		minLength = length1
	} else {
		minLength = length2
	}

	for i := 0; i < minLength; i++ {
		newTuple := Tuple{
			Element1: inValue1.Index(i).Interface(),
			Element2: inValue2.Index(i).Interface(),
		}
		result = append(result, newTuple)
	}
	return result
}
