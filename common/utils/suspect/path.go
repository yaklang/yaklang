package suspect

import (
	"fmt"
	"strings"
)

// IsFullURL 根据 value 猜测是否是一个完整 url，目前只关心 http 和 https
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

// 根据 value 猜测是否是一个 url path
func IsURLPath(v interface{}) bool {
	var value = fmt.Sprint(v)
	return strings.Contains(value, "/") || commonURLPathExtRegex.MatchString(value)
}
