package yaklib

import (
	"fmt"
	"strconv"

	"github.com/yaklang/yaklang/common/log"
)

// parseInt 尝试将传入的字符串转换为整数，如果失败则返回0
// Example:
// ```
// parseInt("123") // 123
// parseInt("abc") // 0
// ```
func parseInt(s string) int {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
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
