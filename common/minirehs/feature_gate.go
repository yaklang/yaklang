package minirehs

import "regexp"

// IsRE2Expressible 报告一条正则是否能被本引擎 (RE2 自动机方法) 表达.
// 与 stdlib regexp 一致: backreference (\1)、任意 lookaround ((?=...) 等) 均不支持.
// 这是数学本质 (自动机方法无法记忆已匹配文本/变宽回看), 不是实现缺陷.
//
// 返回 (true, "") 表示可表达; (false, reason) 给出英文原因.
//
// 关键词: feature gate, RE2, backreference, lookaround, 特性兼容
func IsRE2Expressible(expr string) (bool, string) {
	if _, err := regexp.Compile(expr); err != nil {
		return false, err.Error()
	}
	return true, ""
}
