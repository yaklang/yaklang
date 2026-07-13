package yaklib

import (
	"fmt"
	"regexp"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// RegexpMatch 使用正则尝试匹配字符串，如果匹配成功返回 true，否则返回 false
//
// 参数:
//   - pattern: 正则表达式
//   - s: 待匹配的对象，会被转换为字符串
//
// 返回值:
//   - 是否匹配成功
//
// Example:
// ```
// str.RegexpMatch("^[a-z]+$", "abc") // true
// ```
func _strRegexpMatch(pattern string, s interface{}) bool {
	return _reMatch(pattern, s)
}

// Match 使用正则尝试匹配字符串，如果匹配成功返回 true，否则返回 false
// 参数:
//   - pattern: 正则表达式
//   - s: 待匹配的字符串或字节切片
//
// 返回值:
//   - 是否匹配成功
//
// Example:
// ```
// ok = re.Match("^[a-z]+$", "abc")
// println(ok)   // OUT: true
// assert ok == true, "Match should match lowercase letters"
// assert re.Match("^[a-z]+$", "abc123") == false, "Match should fail when extra chars present"
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
// 参数:
//   - origin: 待匹配的输入（任意可转为字符串）
//   - re: 正则表达式
//
// 返回值:
//   - 第一个匹配的子串，未匹配返回空字符串
//
// Example:
// ```
// result = re.Find("apple is an easy word", "^[a-z]+")
// println(result)   // OUT: apple
// assert result == "apple", "Find should return first match"
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
// 参数:
//   - origin: 待匹配的输入（任意可转为字符串）
//   - re: 正则表达式
//
// 返回值:
//   - 所有匹配子串组成的切片，未匹配返回空切片
//
// Example:
// ```
// matches = re.FindAll("Well,yakit is GUI client for yaklang", "yak[a-z]+")
// println(matches)   // OUT: [yakit yaklang]
// assert len(matches) == 2, "FindAll should find two matches"
// assert matches[0] == "yakit" && matches[1] == "yaklang", "FindAll should return all matches in order"
// ```
func _findAll_extractByRegexp(origin interface{}, re string) []string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllString(utils.InterfaceToString(origin), -1)
}

// FindAllIndex 使用正则匹配字符串，返回所有匹配子串的起止位置（导出名为 re.FindAllIndex）
// 参数:
//   - origin: 待匹配的输入（任意可转为字符串）
//   - re: 正则表达式
//
// 返回值:
//   - 二维整数切片，每项为 [起始位置, 结束位置]，未匹配返回空切片
//
// Example:
// ```
// idx = re.FindAllIndex("Well,yakit is GUI client for yaklang", "yak[a-z]+")
// println(idx)   // OUT: [[5 10] [29 36]]
// assert len(idx) == 2, "FindAllIndex should locate two matches"
// assert idx[0][0] == 5 && idx[0][1] == 10, "first match index should be [5,10]"
// ```
func _findAllIndex_extractByRegexp(origin interface{}, re string) [][]int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllStringIndex(utils.InterfaceToString(origin), -1)
}

// FindIndex 使用正则匹配字符串，返回第一个匹配子串的起止位置（导出名为 re.FindIndex）
// 参数:
//   - origin: 待匹配的输入（任意可转为字符串）
//   - re: 正则表达式
//
// 返回值:
//   - 长度为 2 的整数切片 [起始位置, 结束位置]，未匹配返回空切片
//
// Example:
// ```
// idx = re.FindIndex("Well,yakit is GUI client for yaklang", "yak[a-z]+")
// println(idx)   // OUT: [5 10]
// assert idx[0] == 5 && idx[1] == 10, "FindIndex should return [5,10]"
// ```
func _findIndex_extractByRegexp(origin interface{}, re string) []int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindStringIndex(utils.InterfaceToString(origin))
}

// FindSubmatch 使用正则匹配字符串，返回第一个匹配及其子匹配（导出名为 re.FindSubmatch）
// 参数:
//   - origin: 待匹配的输入（任意可转为字符串）
//   - re: 含捕获组的正则表达式
//
// 返回值:
//   - 字符串切片，第 0 项为整体匹配，其余为各捕获组，未匹配返回空切片
//
// Example:
// ```
// m = re.FindSubmatch("Well,yakit is GUI client for yaklang", "yak([a-z]+)")
// println(m)   // OUT: [yakit it]
// assert m[0] == "yakit" && m[1] == "it", "FindSubmatch should return whole match and group"
// ```
func _findSubmatch_extractByRegexp(origin interface{}, re string) []string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindStringSubmatch(utils.InterfaceToString(origin))
}

// FindSubmatchIndex 使用正则匹配字符串，返回第一个匹配及子匹配的位置（导出名为 re.FindSubmatchIndex）
// 参数:
//   - origin: 待匹配的输入（任意可转为字符串）
//   - re: 含捕获组的正则表达式
//
// 返回值:
//   - 整数切片，每两个一组依次为整体匹配与各捕获组的 [起始, 结束]，未匹配返回空切片
//
// Example:
// ```
// idx = re.FindSubmatchIndex("Well,yakit is GUI client for yaklang", "yak([a-z]+)")
// println(idx)   // OUT: [5 10 8 10]
// assert idx[0] == 5 && idx[1] == 10, "FindSubmatchIndex should locate whole match at [5,10]"
// ```
func _findSubmatchIndex_extractByRegexp(origin interface{}, re string) []int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindStringSubmatchIndex(utils.InterfaceToString(origin))
}

// FindSubmatchAll 使用正则匹配字符串，返回所有匹配及其子匹配（导出名为 re.FindSubmatchAll）
// 参数:
//   - origin: 待匹配的输入（任意可转为字符串）
//   - re: 含捕获组的正则表达式
//
// 返回值:
//   - 二维字符串切片，每项为 [整体匹配, 捕获组...]，未匹配返回空切片
//
// Example:
// ```
// all = re.FindSubmatchAll("Well,yakit is GUI client for yaklang", "yak([a-z]+)")
// println(all)   // OUT: [[yakit it] [yaklang lang]]
// assert len(all) == 2, "FindSubmatchAll should find two matches"
// assert all[1][1] == "lang", "second group should be lang"
// ```
func _findSubmatchAll_extractByRegexp(origin interface{}, re string) [][]string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllStringSubmatch(utils.InterfaceToString(origin), -1)
}

// FindSubmatchAllIndex 使用正则匹配字符串，返回所有匹配及子匹配的位置（导出名为 re.FindSubmatchAllIndex）
// 参数:
//   - origin: 待匹配的输入（任意可转为字符串）
//   - re: 含捕获组的正则表达式
//
// 返回值:
//   - 二维整数切片，每项每两个一组依次为整体匹配与各捕获组的 [起始, 结束]，未匹配返回空切片
//
// Example:
// ```
// idx = re.FindSubmatchAllIndex("Well,yakit is GUI client for yaklang", "yak([a-z]+)")
// println(idx)   // OUT: [[5 10 8 10] [29 36 32 36]]
// assert len(idx) == 2, "FindSubmatchAllIndex should find two matches"
// ```
func _findSubmatchAllIndex_extractByRegexp(origin interface{}, re string) [][]int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllStringSubmatchIndex(utils.InterfaceToString(origin), -1)
}

// ReplaceAllWithFunc 使用正则匹配并用回调函数生成替换内容（导出名为 re.ReplaceAllWithFunc）
// 参数:
//   - origin: 原始输入（任意可转为字符串）
//   - re: 正则表达式
//   - newStr: 回调函数 func(matched string) string，入参为每个匹配，返回替换后的内容
//
// 返回值:
//   - 替换完成后的字符串
//
// Example:
// ```
//
//	result = re.ReplaceAllWithFunc("yakit is programming language", "yak([a-z]+)", func(s) {
//	    return "yaklang"
//	})
//
// println(result)   // OUT: yaklang is programming language
// assert result == "yaklang is programming language", "ReplaceAllWithFunc should replace matched token"
// ```
func _replaceAllFunc_extractByRegexp(origin interface{}, re string, newStr func(string) string) string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return utils.InterfaceToString(origin)
	}
	return r.ReplaceAllStringFunc(utils.InterfaceToString(origin), newStr)
}

// ReplaceAll 使用正则匹配并替换为指定字符串（导出名为 re.ReplaceAll）
// 替换字符串支持 $1、${name} 等引用捕获组
// 参数:
//   - origin: 原始输入（任意可转为字符串）
//   - re: 正则表达式
//   - newStr: 替换字符串（支持 $1 等捕获组引用）
//
// 返回值:
//   - 替换完成后的字符串
//
// Example:
// ```
// result = re.ReplaceAll("yakit is programming language", "yak([a-z]+)", "yaklang")
// println(result)   // OUT: yaklang is programming language
// assert result == "yaklang is programming language", "ReplaceAll should replace matched token"
// ```
func _replaceAll_extractByRegexp(origin interface{}, re string, newStr interface{}) string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return utils.InterfaceToString(origin)
	}
	return r.ReplaceAllString(utils.InterfaceToString(origin), utils.InterfaceToString(newStr))
}

// FindGroup 使用正则匹配并按命名捕获组返回结果映射（导出名为 re.FindGroup）
// 键为捕获组名（未命名组用其序号字符串），值为匹配内容；键 "0" 表示整体匹配
// 参数:
//   - i: 待匹配的输入（任意可转为字符串）
//   - re: 含命名捕获组的正则表达式
//
// 返回值:
//   - 命名捕获组到匹配内容的映射，未匹配返回空映射
//
// Example:
// ```
// g = re.FindGroup("Well,yakit is GUI client for yaklang", "yak(?P<other>[a-z]+)")
// println(g["other"])   // OUT: it
// assert g["0"] == "yakit", "group 0 should be whole match"
// assert g["other"] == "it", "named group other should be it"
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

// FindGroupAll 使用正则匹配并按命名捕获组返回所有结果映射（导出名为 re.FindGroupAll）
// 每个匹配对应一个映射，键为捕获组名（未命名组用序号），键 "0" 为整体匹配
// 参数:
//   - i: 待匹配的输入（任意可转为字符串）
//   - raw: 含命名捕获组的正则表达式
//
// 返回值:
//   - 映射切片，每项对应一个匹配，未匹配返回空切片
//
// Example:
// ```
// gs = re.FindGroupAll("Well,yakit is GUI client for yaklang", "yak(?P<other>[a-z]+)")
// println(len(gs))   // OUT: 2
// assert gs[0]["other"] == "it" && gs[1]["other"] == "lang", "FindGroupAll should capture both named groups"
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

// QuoteMeta 转义字符串中所有正则元字符，使其可作为普通文本参与匹配（导出名为 re.QuoteMeta）
// 参数:
//   - s: 原始字符串
//
// 返回值:
//   - 转义后的字符串
//
// Example:
// ```
// q = re.QuoteMeta("a.b+c")
// println(q)   // OUT: a\.b\+c
// assert q == "a\\.b\\+c", "QuoteMeta should escape . and +"
// ```
func _quoteMeta(s string) string {
	return regexp.QuoteMeta(s)
}

// Compile 将正则表达式编译为正则对象（导出名为 re.Compile）
// 参数:
//   - expr: 正则表达式字符串
//
// 返回值:
//   - 编译得到的正则对象，可调用 MatchString/FindString 等方法
//   - 错误信息（正则语法非法时返回）
//
// Example:
// ```
// r = re.Compile("^[a-z]+$")~
// println(r.MatchString("abc"))   // OUT: true
// assert r.MatchString("abc") == true, "Compile result should match lowercase letters"
// ```
func _compile(expr string) (*regexp.Regexp, error) {
	return regexp.Compile(expr)
}

// CompilePOSIX 按 POSIX ERE(egrep) 语法编译正则，匹配语义为左最长匹配（导出名为 re.CompilePOSIX）
// 参数:
//   - expr: 正则表达式字符串
//
// 返回值:
//   - 编译得到的 POSIX 正则对象
//   - 错误信息（正则语法非法时返回）
//
// Example:
// ```
// r = re.CompilePOSIX("^[a-z]+$")~
// println(r.MatchString("abc"))   // OUT: true
// assert r.MatchString("abc") == true, "CompilePOSIX result should match lowercase letters"
// ```
func _compilePOSIX(expr string) (*regexp.Regexp, error) {
	return regexp.CompilePOSIX(expr)
}

// MustCompile 编译正则表达式，语法非法时直接 panic（导出名为 re.MustCompile）
// 适用于编译期可确定合法的常量正则
// 参数:
//   - str: 正则表达式字符串
//
// 返回值:
//   - 编译得到的正则对象
//
// Example:
// ```
// r = re.MustCompile("^[a-z]+$")
// println(r.MatchString("abc"))   // OUT: true
// assert r.MatchString("abc") == true, "MustCompile result should match lowercase letters"
// ```
func _mustCompile(str string) *regexp.Regexp {
	return regexp.MustCompile(str)
}

// MustCompilePOSIX 按 POSIX 语法编译正则，语法非法时直接 panic（导出名为 re.MustCompilePOSIX）
// 参数:
//   - str: 正则表达式字符串
//
// 返回值:
//   - 编译得到的 POSIX 正则对象
//
// Example:
// ```
// r = re.MustCompilePOSIX("^[a-z]+$")
// println(r.MatchString("abc"))   // OUT: true
// assert r.MatchString("abc") == true, "MustCompilePOSIX result should match lowercase letters"
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
