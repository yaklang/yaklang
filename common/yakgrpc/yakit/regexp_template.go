package yakit

import (
	"regexp"
	"strconv"
	"strings"
)

// FormatRegexpGroups 根据模板渲染正则捕获组，支持 $1、\1、{1} 三种语法。
// 从高索引到低索引替换，避免 $1 误替换 $10 中的部分。
// groupByNumber(n) 返回第 n 个捕获组的值，不存在时返回空字符串。
func FormatRegexpGroups(template string, groupByNumber func(int) string) string {
	if template == "" {
		return ""
	}
	result := template
	// 从高到低替换，避免 $1 误替换 $10
	const maxGroup = 99
	for n := maxGroup; n >= 1; n-- {
		val := groupByNumber(n)
		// {N} 语法
		bracePattern := `\{` + strconv.Itoa(n) + `\}`
		if re, err := regexp.Compile(bracePattern); err == nil {
			result = re.ReplaceAllString(result, val)
		}
		// $N 语法：$N 后跟非数字字母或结尾，避免 $2_v2 中 $2 误伤
		dollarPattern := `\$` + strconv.Itoa(n) + `(?:[^0-9a-zA-Z]|$)`
		if re, err := regexp.Compile(dollarPattern); err == nil {
			result = re.ReplaceAllStringFunc(result, func(match string) string {
				if len(match) > len("$")+len(strconv.Itoa(n)) {
					suffix := match[len("$")+len(strconv.Itoa(n)):]
					return val + suffix
				}
				return val
			})
		}
		// \N 语法：反斜杠后跟数字
		backslashPattern := `\\` + strconv.Itoa(n) + `(?:[^0-9]|$)`
		if re, err := regexp.Compile(backslashPattern); err == nil {
			result = re.ReplaceAllStringFunc(result, func(match string) string {
				nStr := strconv.Itoa(n)
				if strings.HasPrefix(match, "\\"+nStr) {
					if len(match) > len("\\")+len(nStr) {
						return val + match[len("\\")+len(nStr):]
					}
					return val
				}
				return match
			})
		}
	}
	return result
}
