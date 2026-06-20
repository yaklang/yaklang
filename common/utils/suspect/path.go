package suspect

import (
	"fmt"
	"strings"
)

// IsHttpURL 根据 value 猜测是否是一个完整 url，目前只关心 http 和 https
//
// 参数:
//   - v: 待判断的对象，会被转换为字符串
//
// 返回值:
//   - 是否为 http(s) 协议的完整 URL
//
// Example:
// ```
// str.IsHttpURL("http://www.yaklang.com") // true
// str.IsHttpURL("www.yaklang.com") // false
// ```
func IsFullURL(v interface{}) bool {
	var value = fmt.Sprint(v)
	prefix := []string{"http://", "https://"}
	value = strings.ToLower(value)
	for _, p := range prefix {
		if strings.HasPrefix(value, p) && len(value) > len(p) {
			return true
		}
	}
	return false
}

// IsUrlPath 根据 value 猜测是否是一个 url path
//
// 参数:
//   - v: 待判断的对象，会被转换为字符串
//
// 返回值:
//   - 是否为 URL 路径
//
// Example:
// ```
// str.IsUrlPath("/index.php") // true
// str.IsUrlPath("index.php") // false
// ```
func IsURLPath(v interface{}) bool {
	var value = fmt.Sprint(v)
	return strings.Contains(value, "/") || commonURLPathExtRegex.MatchString(value)
}
