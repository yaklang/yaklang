package yaklib

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// parseInt 尝试将传入的字符串转换为对应进制的整数，默认为十进制，如果失败则返回0
// Example:
// ```
// parseInt("123") // 123
// parseInt("10", 16) // 16
// parseInt("abc") // 0
// ```
func parseInt(s string, bases ...int) int {
	base := 10
	if len(bases) > 0 {
		base = bases[0]
	}
	i, err := strconv.ParseInt(s, base, 64)
	if err != nil {
		// 尝试处理科学计数法表示的数字
		if err.Error() == "strconv.ParseInt: parsing \""+s+"\": invalid syntax" {
			f, err := strconv.ParseFloat(s, 64)
			if err == nil {
				return int(f)
			}
		}
		// [ERRO] 2025-05-08 12:08:17 [cli:168] parse int[2e+09] failed: strconv.ParseInt: parsing "2e+09": invalid syntax
		log.Errorf("parse int[%s] failed: %s", s, err)
		return 0
	}
	return int(i)
}

// parseFloat 尝试将传入的字符串转换为浮点数，如果失败则返回0
// Example:
// ```
// parseFloat("123.456") // 123.456
// parseFloat("abc") // 0
// ```
func parseFloat(s string) float64 {
	i, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Errorf("parse float[%s] failed: %s", s, err)
		return 0
	}
	return float64(i)
}

// parseString 尝试将传入的值转换为字符串，其实际上相当于 `sprintf("%v", i)“
// Example:
// ```
// parseString(123) // "123"
// parseString(["1", "2", "3"]) // "[1 2 3]"
// ```
func parseString(i interface{}) string {
	return fmt.Sprintf("%v", i)
}

// parseBool 尝试将传入的值转换为布尔值，如果失败则返回false
// Example:
// ```
// parseBool("true") // true
// parseBool("false") // false
// parseBool("abc") // false
// ```
func parseBool(i interface{}) bool {
	r, _ := strconv.ParseBool(fmt.Sprint(i))
	return r
}

// atoi 尝试将传入的字符串转换为整数，返回转换后的整数和错误信息
// Example:
// ```
// atoi("123") // 123, nil
// atoi("abc") // 0, error
// ```
func atoi(s string) (int, error) {
	return strconv.Atoi(s)
}

func _input(s ...string) string {
	var input string
	if len(s) > 0 {
		fmt.Print(s[0])
	}
	n, err := fmt.Scanln(&input)
	if err != nil && n != 0 {
		panic(err)
	}
	return input
}

func IsYakFunction(i interface{}) bool {
	return IsNewYakFunction(i)
}

func IsNewYakFunction(i interface{}) bool {
	_, ok := i.(*yakvm.Function)
	if ok {
		return true
	}

	return reflect.TypeOf(i).Kind() == reflect.Func
}
