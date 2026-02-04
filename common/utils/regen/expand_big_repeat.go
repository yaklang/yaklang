package regen

import (
	"bytes"
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
		// 转义的 '{'，按普通字符处理
		if isEscaped(pattern, i) {
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
		end := i + 1
		n64, err := strconv.ParseInt(pattern[numStart:i], 10, 0)
		if err != nil {
			// 溢出或其他解析错误：回退为原始 {n} 输出
			out = append(out, pattern[start:end]...)
			i = end
			continue
		}
		n := int(n64)
		i = end // skip '}'
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
		if len(out) < len(atom) || !bytes.Equal(out[len(out)-len(atom):], []byte(atom)) {
			// 无法在 out 末尾找到 atom，无法安全展开，保持原样
			out = append(out, pattern[start:i]...)
			continue
		}
		out = out[:len(out)-len(atom)]
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
	if lastStart > 0 && pattern[lastStart-1] == '\\' && !isEscaped(pattern, lastStart-1) {
		// 转义：原子是 \ 及其后一个字符（可能多字节）
		return lastStart - 1
	}
	switch pattern[lastStart] {
	case ']':
		return findMatchingBracket(pattern, end-1, '[', ']')
	case ')':
		return findMatchingParen(pattern, end-1)
	default:
		return lastStart
	}
}

func findMatchingBracket(pattern string, from int, open, close byte) int {
	if from < 0 || pattern[from] != close || isEscaped(pattern, from) {
		return -1
	}
	for i := from - 1; i >= 0; i-- {
		if pattern[i] == open && !isEscaped(pattern, i) {
			return i
		}
	}
	return -1
}

func findMatchingParen(pattern string, from int) int {
	if from < 0 || pattern[from] != ')' || isEscaped(pattern, from) {
		return -1
	}
	count := 1
	for i := from - 1; i >= 0; i-- {
		if pattern[i] == ')' && !isEscaped(pattern, i) {
			count++
		} else if pattern[i] == '(' && !isEscaped(pattern, i) {
			count--
			if count == 0 {
				return i
			}
		}
	}
	return -1
}

// isEscaped 判断 pattern[idx] 是否被反斜杠转义（前缀连续反斜杠数量为奇数）
func isEscaped(pattern string, idx int) bool {
	if idx <= 0 {
		return false
	}
	count := 0
	for i := idx - 1; i >= 0 && pattern[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}
