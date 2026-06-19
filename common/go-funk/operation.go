package funk

import (
	"fmt"
	"reflect"
)

func calculate(arr interface{}, name string, operation rune) float64 {
	value := redirectValue(reflect.ValueOf(arr))
	valueType := value.Type()

	kind := value.Kind()

	if kind == reflect.Array || kind == reflect.Slice {
		length := value.Len()

		if length == 0 {
			return 0
		}

		result := map[rune]float64{
			'+': 0.0,
			'*': 1,
		}[operation]

		for i := 0; i < length; i++ {
			elem := redirectValue(value.Index(i)).Interface()

			var value float64
			switch e := elem.(type) {
			case int:
				value = float64(e)
			case int8:
				value = float64(e)
			case int16:
				value = float64(e)
			case int32:
				value = float64(e)
			case int64:
				value = float64(e)
			case float32:
				value = float64(e)
			case float64:
				value = e
			}

			switch operation {
			case '+':
				result += value
			case '*':
				result *= value
			}
		}

		return result
	}

	panic(fmt.Sprintf("Type %s is not supported by %s", valueType.String(), name))
}

// Sum 计算切片中所有数值元素之和
// 参数:
//   - arr: 数值切片
//
// 返回值:
//   - 所有元素之和
//
// Example:
// ```
// // VARS: 求和
// result = x.Sum([1, 2, 3])
// // STDOUT: 打印结果
// println(result)   // OUT: 6
// // assert: 锁定结论
// assert result == 6, "sum of 1,2,3 should be 6"
// ```
func Sum(arr interface{}) float64 {
	return calculate(arr, "Sum", '+')
}

// Product computes the product of the values in array.
func Product(arr interface{}) float64 {
	return calculate(arr, "Product", '*')
}
