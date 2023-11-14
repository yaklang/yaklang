package yaklib

import (
	"fmt"
	"regexp"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// RegexpMatch 使用正则尝试匹配字符串，如果匹配成功返回 true，否则返回 false
// Example:
// ```
// str.RegexpMatch("^[a-z]+$", "abc") // true
// ```
func _strRegexpMatch(pattern string, s interface{}) bool {
	return _reMatch(pattern, s)
}

// Match 使用正则尝试匹配字符串，如果匹配成功返回 true，否则返回 false
// Example:
// ```
// re.Match("^[a-z]+$", "abc") // true
// ```
func _reMatch(pattern string, s interface{}) bool {
	r, err := regexp.Compile(pattern)
	if err != nil {
		_diewith(utils.Errorf("compile[%v] failed: %v", pattern, err))
		return false
	}

	switch ret := s.(type) {
	case []byte:
		return r.Match(ret)
	case string:
		return r.MatchString(ret)
	default:
		_diewith(utils.Errorf("target: %v should be []byte or string", spew.Sdump(s)))
	}
	return false
}

// Find 使用正则尝试匹配字符串，如果匹配成功返回第一个匹配的字符串，否则返回空字符串
// Example:
// ```
// re.Find("apple is an easy word", "^[a-z]+") // "apple"
// ```
func _find_extractByRegexp(origin interface{}, re string) string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return ""
	}
	return r.FindString(utils.InterfaceToString(origin))
}

// FindAll 使用正则尝试匹配字符串，如果匹配成功返回所有匹配的字符串，否则返回空字符串切片
// Example:
// ```
// re.FindAll("Well,yakit is GUI client for yaklang", "yak[a-z]+") // ["yakit", "yaklang"]
// ```
func _findAll_extractByRegexp(origin interface{}, re string) []string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllString(utils.InterfaceToString(origin), -1)
}

// FindAllIndex 使用正则尝试匹配字符串，如果匹配成功返回所有匹配的字符串的起始位置和结束位置，否则返回空整数的二维切片
// Example:
// ```
// re.FindAllIndex("Well,yakit is GUI client for yaklang", "yak[a-z]+") // [[5, 10], [29, 36]]
// ```
func _findAllIndex_extractByRegexp(origin interface{}, re string) [][]int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllStringIndex(utils.InterfaceToString(origin), -1)
}

// FindIndex 使用正则尝试匹配字符串，如果匹配成功返回一个长度为2的整数切片，第一个元素为起始位置，第二个元素为结束位置，否则返回空整数切片
// Example:
// ```
// re.FindIndex("Well,yakit is GUI client for yaklang", "yak[a-z]+") // [5, 10]
// ```
func _findIndex_extractByRegexp(origin interface{}, re string) []int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindStringIndex(utils.InterfaceToString(origin))
}

// FindSubmatch 使用正则尝试匹配字符串，如果匹配成功返回第一个匹配的字符串以及子匹配的字符串，否则返回空字符串切片
// Example:
// ```
// re.FindSubmatch("Well,yakit is GUI client for yaklang", "yak([a-z]+)") // ["yakit", "it"]
// ```
func _findSubmatch_extractByRegexp(origin interface{}, re string) []string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindStringSubmatch(utils.InterfaceToString(origin))
}

// FindSubmatchIndex 使用正则尝试匹配字符串，如果匹配成功返回第一个匹配的字符串以及子匹配的字符串的起始位置和结束位置，否则返回空整数切片
// Example:
// ```
// re.FindSubmatchIndex("Well,yakit is GUI client for yaklang", "yak([a-z]+)") // [5, 10, 8, 10]
// ```
func _findSubmatchIndex_extractByRegexp(origin interface{}, re string) []int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindStringSubmatchIndex(utils.InterfaceToString(origin))
}

// FindSubmatchAll 使用正则尝试匹配字符串，如果匹配成功返回所有匹配的字符串以及子匹配的字符串，否则返回空字符串切片的二维切片
// Example:
// ```
// // [["yakit", "it"], ["yaklang", "lang"]]
// re.FindSubmatchAll("Well,yakit is GUI client for yaklang", "yak([a-z]+)")
// ```
func _findSubmatchAll_extractByRegexp(origin interface{}, re string) [][]string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllStringSubmatch(utils.InterfaceToString(origin), -1)
}

// FindSubmatchAllIndex 使用正则尝试匹配字符串，如果匹配成功返回所有匹配的字符串以及子匹配的字符串的起始位置和结束位置，否则返回空整数切片的二维切片
// Example:
// ```
// // [[5, 10, 8, 10], [29, 36, 32, 36]]
// re.FindSubmatchAllIndex("Well,yakit is GUI client for yaklang", "yak([a-z]+)")
// ```
func _findSubmatchAllIndex_extractByRegexp(origin interface{}, re string) [][]int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllStringSubmatchIndex(utils.InterfaceToString(origin), -1)
}

// ReplaceAllWithFunc 使用正则表达式匹配并使用自定义的函数替换字符串，并返回替换后的字符串
// Example:
// ```
// // "yaklang is a programming language"
// re.ReplaceAllWithFunc("yakit is programming language", "yak([a-z]+)", func(s) {
// return "yaklang"
// })
// ```
func _replaceAllFunc_extractByRegexp(origin interface{}, re string, newStr func(string) string) string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return utils.InterfaceToString(origin)
	}
	return r.ReplaceAllStringFunc(utils.InterfaceToString(origin), newStr)
}

// ReplaceAll 使用正则表达式匹配并替换字符串，并返回替换后的字符串
// Example:
// ```
// // "yaklang is a programming language"
// re.ReplaceAll("yakit is programming language", "yak([a-z]+)", "yaklang")
// ```
func _replaceAll_extractByRegexp(origin interface{}, re string, newStr interface{}) string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return utils.InterfaceToString(origin)
	}
	return r.ReplaceAllString(utils.InterfaceToString(origin), utils.InterfaceToString(newStr))
}

// FindGroup 使用正则表达式匹配字符串，如果匹配成功返回一个映射，其键名为正则表达式中的命名捕获组，键值为匹配到的字符串，否则返回空映射
// Example:
// ```
// // {"0": "yakit", "other": "it"}
// re.FindGroup("Well,yakit is GUI client for yaklang", "yak(?P<other>[a-z]+)")
// ```
func reExtractGroups(i interface{}, re string) map[string]string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Error(err)
		return make(map[string]string)
	}
	matchIndex := map[int]string{}
	for _, name := range r.SubexpNames() {
		matchIndex[r.SubexpIndex(name)] = name
	}

	result := make(map[string]string)
	for index, value := range r.FindStringSubmatch(utils.InterfaceToString(i)) {
		name, ok := matchIndex[index]
		if !ok {
			name = fmt.Sprint(index)
		}
		result[name] = value
	}
	return result
}

// FindGroupAll 使用正则表达式匹配字符串，如果匹配成功返回一个映射切片，其键名为正则表达式中的命名捕获组，键值为匹配到的字符串，否则返回空映射切片
// Example:
// ```
// // [{"0": "yakit", "other": "it"}, {"0": "yaklang", "other": "lang"}]
// re.FindGroupAll("Well,yakit is GUI client for yaklang", "yak(?P<other>[a-z]+)")
// ```
func reExtractGroupsAll(i interface{}, raw string) []map[string]string {
	re, err := regexp.Compile(raw)
	if err != nil {
		log.Error(err)
		return nil
	}
	matchIndex := map[int]string{}
	for _, name := range re.SubexpNames() {
		matchIndex[re.SubexpIndex(name)] = name
	}

	var results []map[string]string
	for _, matches := range re.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		result := make(map[string]string)
		for index, value := range matches {
			name, ok := matchIndex[index]
			if !ok {
				name = fmt.Sprint(index)
			}
			result[name] = value
		}
		results = append(results, result)
	}
	return results
}

// QuoteMeta 返回一个字符串，该字符串是将 s 中所有正则表达式元字符进行转义后的结果
// Example:
// ```
// str.QuoteMeta("^[a-z]+$") // "\^\\[a-z\]\\+$"
// ```
func _quoteMeta(s string) string {
	return regexp.QuoteMeta(s)
}

// Compile 将正则表达式解析为一个正则表达式结构体引用
// Example:
// ```
// re.Compile("^[a-z]+$")
// ```
func _compile(expr string) (*regexp.Regexp, error) {
	return regexp.Compile(expr)
}

// CompilePOSIX 将正则表达式解析为一个符合 POSIX ERE(egrep) 语法的正则表达式结构体引用，并且匹配语义改为左最长匹配
// Example:
// ```
// re.CompilePOSIX("^[a-z]+$")
// ```
func _compilePOSIX(expr string) (*regexp.Regexp, error) {
	return regexp.CompilePOSIX(expr)
}

// MustCompile 将正则表达式解析为一个正则表达式对象结构体引用，如果解析失败则会引发崩溃
// Example:
// ```
// re.MustCompile("^[a-z]+$")
// ```
func _mustCompile(str string) *regexp.Regexp {
	return regexp.MustCompile(str)
}

// MustCompilePOSIX 将正则表达式解析为一个POSIX正则表达式结构体引用，如果解析失败则会引发崩溃
// Example:
// ```
// re.MustCompilePOSIX("^[a-z]+$")
// ```
func _mustCompilePOSIX(str string) *regexp.Regexp {
	return regexp.MustCompilePOSIX(str)
}

var RegexpExport = map[string]interface{}{
	"QuoteMeta":        _quoteMeta,
	"Compile":          _compile,
	"CompilePOSIX":     _compilePOSIX,
	"MustCompile":      _mustCompile,
	"MustCompilePOSIX": _mustCompilePOSIX,

	"Match":                _reMatch,
	"Grok":                 Grok,
	"ExtractIPv4":          RegexpMatchIPv4,
	"ExtractIPv6":          RegexpMatchIPv6,
	"ExtractIP":            RegexpMatchIP,
	"ExtractEmail":         RegexpMatchEmail,
	"ExtractPath":          RegexpMatchPathParam,
	"ExtractTTY":           RegexpMatchTTY,
	"ExtractURL":           RegexpMatchURL,
	"ExtractHostPort":      RegexpMatchHostPort,
	"ExtractMac":           RegexpMatchMac,
	"Find":                 _find_extractByRegexp,
	"FindIndex":            _findIndex_extractByRegexp,
	"FindAll":              _findAll_extractByRegexp,
	"FindAllIndex":         _findAllIndex_extractByRegexp,
	"FindSubmatch":         _findSubmatch_extractByRegexp,
	"FindSubmatchIndex":    _findSubmatchIndex_extractByRegexp,
	"FindSubmatchAll":      _findSubmatchAll_extractByRegexp,
	"FindSubmatchAllIndex": _findSubmatchAllIndex_extractByRegexp,
	"FindGroup":            reExtractGroups,
	"FindGroupAll":         reExtractGroupsAll,
	"ReplaceAll":           _replaceAll_extractByRegexp,
	"ReplaceAllWithFunc":   _replaceAllFunc_extractByRegexp,
}
