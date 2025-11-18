package aidmock

import (
	"regexp"
	"strings"
)

// 常用的匹配器构建函数

// MatcherContains 创建一个包含指定子字符串的匹配器（不区分大小写）
func MatcherContains(substring string) PromptMatcherFunc {
	substringLower := strings.ToLower(substring)
	return func(prompt string) bool {
		return strings.Contains(strings.ToLower(prompt), substringLower)
	}
}

// MatcherContainsCaseSensitive 创建一个包含指定子字符串的匹配器（区分大小写）
func MatcherContainsCaseSensitive(substring string) PromptMatcherFunc {
	return func(prompt string) bool {
		return strings.Contains(prompt, substring)
	}
}

// MatcherPrefix 创建一个匹配指定前缀的匹配器（不区分大小写）
func MatcherPrefix(prefix string) PromptMatcherFunc {
	prefixLower := strings.ToLower(prefix)
	return func(prompt string) bool {
		return strings.HasPrefix(strings.ToLower(prompt), prefixLower)
	}
}

// MatcherSuffix 创建一个匹配指定后缀的匹配器（不区分大小写）
func MatcherSuffix(suffix string) PromptMatcherFunc {
	suffixLower := strings.ToLower(suffix)
	return func(prompt string) bool {
		return strings.HasSuffix(strings.ToLower(prompt), suffixLower)
	}
}

// MatcherRegex 创建一个使用正则表达式匹配的匹配器
func MatcherRegex(pattern string) PromptMatcherFunc {
	re := regexp.MustCompile(pattern)
	return func(prompt string) bool {
		return re.MatchString(prompt)
	}
}

// MatcherExact 创建一个精确匹配的匹配器（不区分大小写）
func MatcherExact(target string) PromptMatcherFunc {
	targetLower := strings.ToLower(target)
	return func(prompt string) bool {
		return strings.ToLower(strings.TrimSpace(prompt)) == targetLower
	}
}

// MatcherAny 创建一个匹配任意prompt的匹配器
func MatcherAny() PromptMatcherFunc {
	return func(prompt string) bool {
		return true
	}
}

// MatcherNone 创建一个不匹配任何prompt的匹配器
func MatcherNone() PromptMatcherFunc {
	return func(prompt string) bool {
		return false
	}
}

// MatcherAnd 创建一个组合多个匹配器的AND匹配器（所有匹配器都要返回true）
func MatcherAnd(matchers ...PromptMatcherFunc) PromptMatcherFunc {
	return func(prompt string) bool {
		for _, matcher := range matchers {
			if !matcher(prompt) {
				return false
			}
		}
		return true
	}
}

// MatcherOr 创建一个组合多个匹配器的OR匹配器（任一匹配器返回true即可）
func MatcherOr(matchers ...PromptMatcherFunc) PromptMatcherFunc {
	return func(prompt string) bool {
		for _, matcher := range matchers {
			if matcher(prompt) {
				return true
			}
		}
		return false
	}
}

// MatcherNot 创建一个反转匹配器结果的NOT匹配器
func MatcherNot(matcher PromptMatcherFunc) PromptMatcherFunc {
	return func(prompt string) bool {
		return !matcher(prompt)
	}
}

// MatcherLength 创建一个根据prompt长度匹配的匹配器
func MatcherLength(min, max int) PromptMatcherFunc {
	return func(prompt string) bool {
		length := len(prompt)
		return length >= min && length <= max
	}
}

// MatcherLengthMin 创建一个匹配最小长度的匹配器
func MatcherLengthMin(min int) PromptMatcherFunc {
	return func(prompt string) bool {
		return len(prompt) >= min
	}
}

// MatcherLengthMax 创建一个匹配最大长度的匹配器
func MatcherLengthMax(max int) PromptMatcherFunc {
	return func(prompt string) bool {
		return len(prompt) <= max
	}
}

// MatcherContainsAny 创建一个匹配包含任一子字符串的匹配器
func MatcherContainsAny(substrings ...string) PromptMatcherFunc {
	return func(prompt string) bool {
		promptLower := strings.ToLower(prompt)
		for _, substring := range substrings {
			if strings.Contains(promptLower, strings.ToLower(substring)) {
				return true
			}
		}
		return false
	}
}

// MatcherContainsAll 创建一个匹配包含所有子字符串的匹配器
func MatcherContainsAll(substrings ...string) PromptMatcherFunc {
	return func(prompt string) bool {
		promptLower := strings.ToLower(prompt)
		for _, substring := range substrings {
			if !strings.Contains(promptLower, strings.ToLower(substring)) {
				return false
			}
		}
		return true
	}
}

// MatcherEmpty 创建一个匹配空prompt的匹配器
func MatcherEmpty() PromptMatcherFunc {
	return func(prompt string) bool {
		return strings.TrimSpace(prompt) == ""
	}
}

// MatcherNotEmpty 创建一个匹配非空prompt的匹配器
func MatcherNotEmpty() PromptMatcherFunc {
	return func(prompt string) bool {
		return strings.TrimSpace(prompt) != ""
	}
}

// MatcherFunc 创建一个自定义函数匹配器（包装，提供更明确的语义）
func MatcherFunc(fn func(string) bool) PromptMatcherFunc {
	return fn
}

