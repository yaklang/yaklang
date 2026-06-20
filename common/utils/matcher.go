package utils

import (
	"regexp"

	"github.com/gobwas/glob"
)

func interfaceToStr(i interface{}) string {
	return InterfaceToString(i)
}

// MatchAnyOfSubString 尝试将 i 转换为字符串，然后判断是否有任意子串 subStr 存在于 i 中，如果有其中一个子串存在于 i 中则返回 true，否则返回 false，此函数忽略大小写
//
// 参数:
//   - i: 待匹配的对象，会被转换为字符串
//   - subStr: 一个或多个子串
//
// 返回值:
//   - 是否存在任意子串于 i 中
//
// Example:
// ```
// str.MatchAnyOfSubString("abc", "a", "z", "x") // true
// ```
func MatchAnyOfSubString(i interface{}, subStr ...string) bool {
	raw := interfaceToStr(i)
	for _, subStr := range subStr {
		if IContains(raw, subStr) {
			return true
		}
	}
	return false
}

// MatchAllOfSubString 尝试将 i 转换为字符串，然后判断所有子串 subStr 是否都存在于 i 中，如果都存在则返回 true，否则返回 false，此函数忽略大小写
//
// 参数:
//   - i: 待匹配的对象，会被转换为字符串
//   - subStr: 一个或多个子串
//
// 返回值:
//   - 是否所有子串都存在于 i 中
//
// Example:
// ```
// str.MatchAllOfSubString("abc", "a", "b", "c") // true
// ```
func MatchAllOfSubString(i interface{}, subStr ...string) bool {
	if len(subStr) <= 0 {
		return false
	}

	raw := interfaceToStr(i)
	for _, subStr := range subStr {
		if !IContains(raw, subStr) {
			return false
		}
	}
	return true
}

// MatchAnyOfGlob 尝试将 i 转换为字符串，然后使用 glob 匹配模式匹配，如果任意一个glob模式匹配成功，则返回 true，否则返回 false
//
// 参数:
//   - i: 待匹配的对象，会被转换为字符串
//   - re: 一个或多个 glob 模式
//
// 返回值:
//   - 是否有任意 glob 模式匹配成功
//
// Example:
// ```
// str.MatchAnyOfGlob("abc", "a*", "??b", "[^a-z]?c") // true
// ```
func MatchAnyOfGlob(
	i interface{}, re ...string) bool {
	raw := interfaceToStr(i)
	for _, r := range re {
		if glob.MustCompile(r).Match(raw) {
			return true
		}
	}
	return false
}

// MatchAllOfGlob 尝试将 i 转换为字符串，然后使用 glob 匹配模式匹配，如果所有的glob模式都匹配成功，则返回 true，否则返回 false
//
// 参数:
//   - i: 待匹配的对象，会被转换为字符串
//   - re: 一个或多个 glob 模式
//
// 返回值:
//   - 是否所有 glob 模式都匹配成功
//
// Example:
// ```
// str.MatchAllOfGlob("abc", "a*", "?b?", "[a-z]?c") // true
// ```
func MatchAllOfGlob(
	i interface{}, re ...string) bool {
	if len(re) <= 0 {
		return false
	}

	raw := interfaceToStr(i)
	for _, r := range re {
		if !glob.MustCompile(r).Match(raw) {
			return false
		}
	}
	return true
}

// MatchAnyOfRegexp 尝试将 i 转换为字符串，然后使用正则表达式匹配，如果任意一个正则表达式匹配成功，则返回 true，否则返回 false
//
// 参数:
//   - i: 待匹配的对象，会被转换为字符串
//   - re: 一个或多个正则表达式
//
// 返回值:
//   - 是否有任意正则表达式匹配成功
//
// Example:
// ```
// str.MatchAnyOfRegexp("abc", "a.+", "Ab.?", ".?bC") // true
// ```
func MatchAnyOfRegexp(
	i interface{},
	re ...string) bool {
	raw := interfaceToStr(i)
	for _, r := range re {
		result, err := regexp.MatchString(r, raw)
		if err != nil {
			continue
		}
		if result {
			return true
		}
	}
	return false
}

// MatchAllOfRegexp 尝试将 i 转换为字符串，然后使用正则表达式匹配，如果所有的正则表达式都匹配成功，则返回 true，否则返回 false
//
// 参数:
//   - i: 待匹配的对象，会被转换为字符串
//   - re: 一个或多个正则表达式
//
// 返回值:
//   - 是否所有正则表达式都匹配成功
//
// Example:
// ```
// str.MatchAllOfRegexp("abc", "a.+", ".?b.?", "\\w{2}c") // true
// ```
func MatchAllOfRegexp(
	i interface{},
	re ...string) bool {
	if len(re) <= 0 {
		return false
	}

	raw := interfaceToStr(i)
	for _, r := range re {
		result, err := regexp.MatchString(r, raw)
		if err != nil {
			return false
		}
		if !result {
			return false
		}
	}
	return true
}
