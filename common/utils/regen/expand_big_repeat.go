package regen

import (
	"strconv"
	"unicode/utf8"
)

const maxRepeat = 1000

// expandBigRepeat 将模式中 {n}（n>1000）展开为 (atom{1000})*k + atom{rem}，使 Go regexp/syntax 可解析
func expandBigRepeat(pattern string) string {
	var out []byte
	i := 0
	for i < len(pattern) {
		if pattern[i] != '{' {
			out = append(out, pattern[i])
			i++
			continue
		}
		// 尝试解析 {n}（不含逗号，精确重复）
		start := i
		i++
		numStart := i
		for i < len(pattern) && pattern[i] >= '0' && pattern[i] <= '9' {
			i++
		}
		if i == numStart || i >= len(pattern) || pattern[i] != '}' {
			// 不是 {n}，原样输出
			out = append(out, pattern[start])
			i = start + 1
			continue
		}
		n, _ := strconv.Atoi(pattern[numStart:i])
		i++ // skip '}'
		if n <= maxRepeat {
			out = append(out, pattern[start:i]...)
			continue
		}
		// n > 1000：找前面的 atom，再展开
		atomStart := findAtomStart(pattern, start)
		if atomStart < 0 {
			out = append(out, pattern[start:i]...)
			continue
		}
		atom := pattern[atomStart:start]
		// 主循环已把 atom 逐个字符追加到 out，需先撤掉，再写展开
		if len(out) >= len(atom) && string(out[len(out)-len(atom):]) == atom {
			out = out[:len(out)-len(atom)]
		}
		// 展开为 (atom{1000})*q + atom{rem}，其中 n = 1000*q + rem, 0 <= rem < 1000
		q := n / maxRepeat
		rem := n % maxRepeat
		for j := 0; j < q; j++ {
			out = append(out, atom...)
			out = append(out, '{')
			out = strconv.AppendInt(out, int64(maxRepeat), 10)
			out = append(out, '}')
		}
		if rem > 0 {
			out = append(out, atom...)
			out = append(out, '{')
			out = strconv.AppendInt(out, int64(rem), 10)
			out = append(out, '}')
		}
	}
	return string(out)
}

// findAtomStart 返回 pattern 中紧挨在 braceStart（即 '{' 的位置）之前的“原子”的起始下标
// 原子可以是：字符类 [...]、分组 (...)、或单个字符/转义（如 \d）。
func findAtomStart(pattern string, braceStart int) int {
	if braceStart <= 0 {
		return -1
	}
	end := braceStart
	// 最后一个“单元”的起始（可能是一个 rune 或 \x）
	_, runeSize := utf8.DecodeLastRuneInString(pattern[:end])
	lastStart := end - runeSize
	if lastStart < 0 {
		return -1
	}
	switch pattern[lastStart] {
	case ']':
		return findMatchingBracket(pattern, end-1, '[', ']')
	case ')':
		return findMatchingParen(pattern, end-1)
	case '\\':
		// 转义：原子是 \ 及其后一个字符（可能多字节）
		if lastStart > 0 {
			return lastStart
		}
		return -1
	default:
		return lastStart
	}
}

func findMatchingBracket(pattern string, from int, open, close byte) int {
	if from < 0 || pattern[from] != close {
		return -1
	}
	count := 1
	for i := from - 1; i >= 0; i-- {
		if pattern[i] == '\\' {
			i--
			continue
		}
		if pattern[i] == close {
			count++
		} else if pattern[i] == open {
			count--
			if count == 0 {
				return i
			}
		}
	}
	return -1
}

func findMatchingParen(pattern string, from int) int {
	if from < 0 || pattern[from] != ')' {
		return -1
	}
	count := 1
	for i := from - 1; i >= 0; i-- {
		if pattern[i] == '\\' {
			i--
			continue
		}
		if pattern[i] == ')' {
			count++
		} else if pattern[i] == '(' {
			count--
			if count == 0 {
				return i
			}
		}
	}
	return -1
}
