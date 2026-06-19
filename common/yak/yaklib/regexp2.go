package yaklib

import (
	"fmt"

	"github.com/dlclark/regexp2"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Find 使用 .NET 风格(regexp2)正则在输入中查找第一个匹配的子串
// 参数:
//   - data: 待匹配的输入数据，会被转换为字符串
//   - pattern: regexp2 正则表达式
//
// 返回值:
//   - 第一个匹配到的子串，未匹配或编译失败时返回空字符串
//
// Example:
// ```
// // VARS: 提取第一段连续数字
// result = re2.Find("abc123def", `\d+`)
// // STDOUT: 打印匹配结果
// println(result)   // OUT: 123
// // assert: 锁定结论
// assert result == "123", "Find should return the first digit run"
// ```
func re2Find(data interface{}, pattern string) string {
	re, err := re2Compile(pattern)
	if err != nil {
		return ""
	}
	match, err := re.FindStringMatch(utils.InterfaceToString(data))
	if err != nil {
		return ""
	}
	return match.String()
}

// FindAll 使用 .NET 风格(regexp2)正则查找输入中所有匹配的子串
// 参数:
//   - data: 待匹配的输入数据，会被转换为字符串
//   - pattern: regexp2 正则表达式
//
// 返回值:
//   - 所有匹配到的子串组成的切片，未匹配或编译失败时返回空
//
// Example:
// ```
// // VARS: 取出全部单个数字
// result = re2.FindAll("a1b2c3", `\d`)
// // STDOUT: 打印切片
// println(result)   // OUT: [1 2 3]
// // assert: 锁定数量
// assert len(result) == 3, "FindAll should find three digits"
// ```
func re2FindAll(data interface{}, pattern string) []string {
	re, err := re2Compile(pattern)
	if err != nil {
		return nil
	}
	match, err := re.FindStringMatch(utils.InterfaceToString(data))
	if err != nil {
		return nil
	}
	var results []string
	for {
		results = append(results, match.String())
		if nextMatch, err := re.FindNextMatch(match); err == nil && nextMatch != nil {
			match = nextMatch
		} else {
			break
		}
	}
	return results
}

// FindSubmatch 查找第一个匹配并返回其完整匹配与各捕获分组
// 参数:
//   - i: 待匹配的输入数据，会被转换为字符串
//   - pattern: 含捕获分组的 regexp2 正则表达式
//
// 返回值:
//   - 切片，第 0 项为完整匹配，其后依次为各分组内容
//
// Example:
// ```
// // VARS: 解析 年-月 并取分组
// result = re2.FindSubmatch("2023-01", `(\d+)-(\d+)`)
// // STDOUT: 打印分组切片
// println(result)   // OUT: [2023-01 2023 01]
// // assert: 第一个分组为年份
// assert result[1] == "2023", "first group should be the year"
// ```
func re2FindSubmatch(i interface{}, pattern string) []string {
	re, err := re2Compile(pattern)
	if err != nil {
		log.Error(err)
		return nil
	}
	match, err := re.FindStringMatch(utils.InterfaceToString(i))
	if err != nil {
		log.Error(err)
		return nil
	}
	result := make([]string, match.GroupCount())
	for index, g := range match.Groups() {
		result[index] = g.String()
	}
	return result
}

// FindSubmatchAll 查找所有匹配，每个匹配返回其完整匹配与各捕获分组
// 参数:
//   - i: 待匹配的输入数据，会被转换为字符串
//   - pattern: 含捕获分组的 regexp2 正则表达式
//
// 返回值:
//   - 二维切片，每个元素为一次匹配的[完整匹配, 分组1, 分组2, ...]
//
// Example:
// ```
// // VARS: 批量解析 字母+数字 组合
// result = re2.FindSubmatchAll("a1-b2", `(\w)(\d)`)
// // assert: 命中两组，且第二组的首分组为 b
// assert len(result) == 2, "should match twice"
// assert result[1][1] == "b", "second match first group should be b"
// ```
func re2FindSubmatchAll(i interface{}, pattern string) [][]string {
	re, err := re2Compile(pattern)
	if err != nil {
		log.Errorf("re2 compile failed: %s", err)
		return nil
	}
	var results [][]string
	match, err := re.FindStringMatch(utils.InterfaceToString(i))
	if err != nil {
		log.Error(err)
		return nil
	}
	for {
		results = append(results, lo.Map(match.Groups(), func(item regexp2.Group, index int) string {
			return item.String()
		}))
		if nextMatch, err := re.FindNextMatch(match); err == nil && nextMatch != nil {
			match = nextMatch
		} else {
			break
		}
	}
	return results
}

// Compile 编译一个 .NET 风格(regexp2)正则表达式，返回可复用的正则对象
// 参数:
//   - pattern: regexp2 正则表达式字符串
//
// 返回值:
//   - 编译后的正则对象，可调用 MatchString 等方法
//   - 编译失败时返回的错误
//
// Example:
// ```
// // VARS: 编译正则并复用匹配
// re = re2.Compile(`\d+`)~
// matched = re.MatchString("abc123")~
// // STDOUT: 打印是否匹配
// println(matched)   // OUT: true
// // assert: 锁定结论
// assert matched, "compiled pattern should match digits"
// ```
func re2Compile(pattern string) (*regexp2.Regexp, error) {
	_, _, re, err := utils.Regexp2Compile(pattern)
	return re, err
}

// ReplaceAll 将输入中所有匹配正则的部分替换为目标字符串，支持 $1 分组引用
// 参数:
//   - i: 待处理的输入数据，会被转换为字符串
//   - pattern: regexp2 正则表达式
//   - target: 替换目标，可用 $1、$2 引用捕获分组
//
// 返回值:
//   - 替换后的字符串，编译失败时返回原始输入
//
// Example:
// ```
// // VARS: 把所有数字替换为 X
// result = re2.ReplaceAll("a1b2c3", `\d`, "X")
// // STDOUT: 打印替换结果
// println(result)   // OUT: aXbXcX
// // assert: 使用分组引用交换 年/月
// assert re2.ReplaceAll("2023-01", `(\d+)-(\d+)`, "$2/$1") == "01/2023", "group reference should swap parts"
// ```
func re2ReplaceAll(i interface{}, pattern string, target string) string {
	re, err := re2Compile(pattern)
	if err != nil {
		log.Error(err)
		return utils.InterfaceToString(i)
	}
	raw := utils.InterfaceToString(i)
	m, err := re.Replace(raw, target, 0, -1)
	if err != nil {
		return raw
	}
	return m
}

// ReplaceAllWithFunc 将输入中所有匹配交给回调函数处理，用其返回值替换
// 参数:
//   - i: 待处理的输入数据，会被转换为字符串
//   - pattern: regexp2 正则表达式
//   - target: 回调函数，入参为单次匹配的子串，返回替换后的字符串
//
// 返回值:
//   - 替换后的字符串，编译失败时返回原始输入
//
// Example:
// ```
// // VARS: 给每个数字加上方括号
// result = re2.ReplaceAllWithFunc("a1b2", `\d`, func(s) { return "[" + s + "]" })
// // STDOUT: 打印替换结果
// println(result)   // OUT: a[1]b[2]
// // assert: 锁定结论
// assert result == "a[1]b[2]", "callback should wrap each digit"
// ```
func re2ReplaceAllFunc(i interface{}, pattern string, target func(string) string) string {
	re, err := re2Compile(pattern)
	if err != nil {
		log.Error(err)
		return utils.InterfaceToString(i)
	}
	raw := utils.InterfaceToString(i)
	m, err := re.ReplaceFunc(raw, regexp2.MatchEvaluator(func(match regexp2.Match) string {
		return target(match.String())
	}), 0, -1)
	if err != nil {
		return raw
	}
	return m
}

// FindGroup 查找第一个匹配并以 map 返回命名/编号分组，键 __all__ 为完整匹配
// 参数:
//   - i: 待匹配的输入数据，会被转换为字符串
//   - pattern: 含命名分组(?<name>...)或编号分组的 regexp2 正则
//
// 返回值:
//   - map，命名分组以名字为键，匿名分组以序号为键，__all__ 为完整匹配
//
// Example:
// ```
// // VARS: 用命名分组解析 年-月
// result = re2.FindGroup("2023-01", `(?<year>\d+)-(?<month>\d+)`)
// // STDOUT: 打印 year 分组
// println(result["year"])   // OUT: 2023
// // assert: month 分组正确
// assert result["month"] == "01", "named group month should be 01"
// ```
func re2ExtractGroups(i interface{}, pattern string) map[string]string {
	re, err := re2Compile(pattern)
	if err != nil {
		log.Error(err)
		return make(map[string]string)
	}

	result := make(map[string]string)
	match, err := re.FindStringMatch(utils.InterfaceToString(i))
	if err != nil {
		return make(map[string]string)
	}
	result["__all__"] = match.String()
	for _, value := range match.Groups() {
		if value.Name == "" {
			result[fmt.Sprint(value.Index)] = value.String()
		} else {
			result[value.Name] = value.String()
		}
	}
	return result
}

// FindGroupAll 查找所有匹配并为每个匹配返回命名/编号分组 map
// 参数:
//   - i: 待匹配的输入数据，会被转换为字符串
//   - pattern: 含命名分组(?<name>...)或编号分组的 regexp2 正则
//
// 返回值:
//   - map 切片，每个元素含该次匹配的命名/编号分组与 __all__ 完整匹配
//
// Example:
// ```
// // VARS: 批量解析 字母+数字
// result = re2.FindGroupAll("a1 b2", `(?<ch>\w)(?<num>\d)`)
// // assert: 命中两次，第二次的 ch 分组为 b
// assert len(result) == 2, "should match twice"
// assert result[1]["ch"] == "b", "second match ch group should be b"
// ```
func re2ExtractGroupsAll(i interface{}, pattern string) []map[string]string {
	re, err := re2Compile(pattern)
	if err != nil {
		log.Error(err)
		return nil
	}

	var results []map[string]string
	match, err := re.FindStringMatch(utils.InterfaceToString(i))
	if err != nil {
		return nil
	}

	for {
		result := make(map[string]string)
		result["__all__"] = match.String()
		for _, value := range match.Groups() {
			if value.Name == "" {
				result[fmt.Sprint(value.Index)] = value.String()
			} else {
				result[value.Name] = value.String()
			}
		}
		results = append(results, result)

		if nextMatch, err := re.FindNextMatch(match); err == nil && nextMatch != nil {
			match = nextMatch
		} else {
			break
		}
	}
	return results
}

// CompileWithOption 使用指定的选项编译 .NET 风格(regexp2)正则，返回编译后的正则对象
// 选项可使用 re2.OPT_IgnoreCase、re2.OPT_Multiline 等常量，多个选项可用按位或组合
// 参数:
//   - rule: regexp2 正则表达式
//   - opt: 编译选项，如 re2.OPT_IgnoreCase
//
// 返回值:
//   - 编译后的正则对象，可调用 MatchString 等方法
//   - 编译失败时返回的错误
//
// Example:
// ```
// // VARS: 以忽略大小写的方式编译并匹配
// re = re2.CompileWithOption(`abc`, re2.OPT_IgnoreCase)~
// result = re.MatchString("ABC")~
// // STDOUT: 打印匹配结果
// println(result)   // OUT: true
// // assert: 忽略大小写后能匹配大写串
// assert result == true, "case-insensitive compile should match uppercase"
// ```
func re2CompileWithOption(rule string, opt int) (*regexp2.Regexp, error) {
	_, _, pattern, err := utils.Regexp2Compile(rule, opt)
	return pattern, err
}

var Regexp2Export = map[string]interface{}{
	"QuoteMeta":                   regexp2.Escape,
	"Compile":                     re2Compile,
	"CompileWithOption":           re2CompileWithOption,
	"OPT_None":                    regexp2.None,
	"OPT_IgnoreCase":              regexp2.IgnoreCase,
	"OPT_Multiline":               regexp2.Multiline,
	"OPT_ExplicitCapture":         regexp2.ExplicitCapture,
	"OPT_Compiled":                regexp2.Compiled,
	"OPT_Singleline":              regexp2.Singleline,
	"OPT_IgnorePatternWhitespace": regexp2.IgnorePatternWhitespace,
	"OPT_RightToLeft":             regexp2.RightToLeft,
	"OPT_Debug":                   regexp2.Debug,
	"OPT_ECMAScript":              regexp2.ECMAScript,
	"OPT_RE2":                     regexp2.RE2,

	"Find":               re2Find,
	"FindAll":            re2FindAll,
	"FindSubmatch":       re2FindSubmatch,
	"FindSubmatchAll":    re2FindSubmatchAll,
	"FindGroup":          re2ExtractGroups,
	"FindGroupAll":       re2ExtractGroupsAll,
	"ReplaceAll":         re2ReplaceAll,
	"ReplaceAllWithFunc": re2ReplaceAllFunc,
}
