package minirehs

import (
	"regexp/syntax"
	"strings"
)

// 本文件实现 IMPL 第 3.4 节的"路线 B (近似字面量提取)": 对仅 regexp2 可编译 (含 lookaround /
// backreference / \uXXXX 等 RE2 不可解析构造) 的 pattern, 在不触碰 regexp2 内部 AST 的前提下,
// 把它改写为一个 *语言只增不减* 的 RE2 骨架 (over-approximation), 再用现成 RE2 字面量提取算法
// (extractRequiredLiterals) 取"必需字面量". 最终匹配仍由 regexp2 verifier 兜底, 故语义绝对正确;
// route-B 只决定能否把这些 pattern 纳入字面量预过滤, 从而免去 always-on 全量扫描.
//
// 健全性 (绝不漏报) 的关键: 改写只做"放大语言"的等价或超集变换 ——
//   - 移除零宽断言 (?=...) (?!...) (?<=...) (?<!...): 去掉约束 => 语言变大 (超集).
//   - backreference \1..\9 / \k<name> -> [\s\S]* (匹配任意串, 含空): 放宽 => 超集.
//   - 原子组 (?>...) -> (?:...): 去掉占有式回溯限制 => 超集.
//   - 命名捕获 (?<name>...) / (?P<name>...) / (?'name'...) -> (?:...): 同语言.
//   - \uXXXX -> \x{XXXX}: 同一码点, 同语言.
// 任何无法确定为"超集/等价"的构造一律 bail (返回 ok=false), 该 pattern 保持 always-on.
// 因 R_super ⊇ R_orig, "R_super 的任一匹配必含字面量 L" 蕴含 "R_orig 的任一匹配必含 L", 故 L
// 用作 R_orig 的预过滤必需字面量是安全的 (只可能多验证, 绝不漏).
//
// 关键词: route-B, 近似字面量, regexp2 over-approximation, 必需字面量, prefilter, 不漏报

// extractRequiredLiteralsApprox 对 regexp2-only 的 expr 走 route-B 提取必需字面量;
// 无法安全提取时返回 nil (该 pattern 归 always-on).
func extractRequiredLiteralsApprox(expr string, minLen int) []string {
	super, ok := re2Superset(expr)
	if !ok {
		return nil
	}
	parsed, err := syntax.Parse(super, syntax.Perl)
	if err != nil {
		return nil
	}
	return extractRequiredLiterals(parsed.Simplify(), minLen)
}

// re2Superset 把 regexp2 pattern 文本改写为语言只增不减的 RE2 骨架. ok=false 表示遇到无法
// 安全处理 (无法判定为超集/等价) 的构造, 调用方应放弃 route-B.
func re2Superset(expr string) (string, bool) {
	s, _, ok := re2SupersetEx(expr)
	return s, ok
}

// re2SupersetEx 同 re2Superset, 但额外报告 widened: true 表示改写过程中做过"严格放大语言"的变换
// (移除 lookaround / backref -> [\s\S]*), 即 R_super 严格 ⊋ R_orig; false 表示只做了"语言等价"
// 变换 (\uXXXX->\x{}, 原子组/命名捕获->非捕获), 即 R_super == R_orig.
//
// 用途 (Phase 1, NFA 门控): widened=false 时, 由该骨架编出的 NFA 与原 regexp2 语言完全相同,
// 可作"权威存在性"(无需 regexp2 复核); widened=true 时只能作"超集存在性门", 命中后须 regexp2
// 复核以滤除假阳. 二者都绝不漏报 (R_super ⊇ R_orig).
func re2SupersetEx(expr string) (string, bool, bool) {
	var b strings.Builder
	widened := false
	s := expr
	n := len(s)
	i := 0
	for i < n {
		c := s[i]
		switch c {
		case '\\':
			j, ok := rewriteEscape(s, i, false, &b, &widened)
			if !ok {
				return "", false, false
			}
			i = j
		case '[':
			j, ok := copyClass(s, i, &b, &widened)
			if !ok {
				return "", false, false
			}
			i = j
		case '(':
			j, ok := rewriteGroupOpen(s, i, &b, &widened)
			if !ok {
				return "", false, false
			}
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), widened, true
}

// rewriteEscape 处理一个 \X 转义 (inClass 标记是否位于字符类内部). 返回消费到的下一个下标.
// widened 在做严格放大变换 (backref->[\s\S]*) 时置位.
func rewriteEscape(s string, i int, inClass bool, b *strings.Builder, widened *bool) (int, bool) {
	n := len(s)
	if i+1 >= n {
		return 0, false // 悬空反斜杠
	}
	nc := s[i+1]
	switch {
	case nc == 'u':
		// \uXXXX -> \x{XXXX} (同一码点). 必须恰好 4 个十六进制位.
		if i+6 > n || !isHex(s[i+2:i+6]) {
			return 0, false
		}
		b.WriteString("\\x{")
		b.WriteString(s[i+2 : i+6])
		b.WriteByte('}')
		return i + 6, true
	case !inClass && nc >= '1' && nc <= '9':
		// 反向引用 \1..\9 -> [\s\S]* (任意串, 超集). 多位反向引用 (\12) 与八进制歧义, 保守 bail.
		if i+2 < n && s[i+2] >= '0' && s[i+2] <= '9' {
			return 0, false
		}
		*widened = true
		b.WriteString("[\\s\\S]*")
		return i + 2, true
	case !inClass && nc == 'k':
		// 命名反向引用 \k<name> / \k'name' -> [\s\S]*.
		j := i + 2
		if j >= n || (s[j] != '<' && s[j] != '\'') {
			return 0, false
		}
		closer := byte('>')
		if s[j] == '\'' {
			closer = '\''
		}
		j++
		for j < n && s[j] != closer {
			j++
		}
		if j >= n {
			return 0, false
		}
		j++ // 消费 closer
		*widened = true
		b.WriteString("[\\s\\S]*")
		return j, true
	default:
		// 其它转义 (\d \w \s \. \b \\ 等) 原样拷贝; 若 RE2 不认 (如 \Z \Q) 后续 parse 失败即 bail.
		b.WriteByte('\\')
		b.WriteByte(nc)
		return i + 2, true
	}
}

// copyClass 拷贝一个字符类 [..](含起始 '['), 处理内部转义 (尤其 \uXXXX -> \x{XXXX}).
// 返回消费到的下一个下标 (即匹配的 ']' 之后).
func copyClass(s string, i int, b *strings.Builder, widened *bool) (int, bool) {
	n := len(s)
	b.WriteByte('[')
	j := i + 1
	// 起始的 ^ 与紧随的 ] 都是普通字符.
	if j < n && s[j] == '^' {
		b.WriteByte('^')
		j++
	}
	if j < n && s[j] == ']' {
		b.WriteByte(']')
		j++
	}
	for j < n {
		c := s[j]
		if c == '\\' {
			nj, ok := rewriteEscape(s, j, true, b, widened)
			if !ok {
				return 0, false
			}
			j = nj
			continue
		}
		if c == ']' {
			b.WriteByte(']')
			return j + 1, true
		}
		b.WriteByte(c)
		j++
	}
	return 0, false // 未闭合的字符类
}

// rewriteGroupOpen 处理一个 '(' 起始的分组开头. 对零宽断言整体替换为 (?:); 对原子组/命名捕获
// 转为非捕获 (?:); 对 (?flags) / (?flags:) / (?:) / 普通捕获原样保留. 返回消费到的下一个下标.
// widened 在移除 lookaround (严格放大语言) 时置位; 原子组/命名捕获 -> 非捕获是语言等价, 不置位.
func rewriteGroupOpen(s string, i int, b *strings.Builder, widened *bool) (int, bool) {
	n := len(s)
	// 非 (? 开头: 普通捕获组, 原样保留 '(' .
	if i+1 >= n || s[i+1] != '?' {
		b.WriteByte('(')
		return i + 1, true
	}
	if i+2 >= n {
		return 0, false
	}
	c2 := s[i+2]
	switch c2 {
	case ':':
		b.WriteString("(?:")
		return i + 3, true
	case '>':
		// 原子组 (?>...) -> (?:...) (超集).
		*widened = true
		b.WriteString("(?:")
		return i + 3, true
	case '=', '!':
		// 前视断言 (?=...) (?!...): 整组移除为 (?:).
		closeIdx, ok := findGroupClose(s, i)
		if !ok {
			return 0, false
		}
		*widened = true
		b.WriteString("(?:)")
		return closeIdx + 1, true
	case '<':
		if i+3 >= n {
			return 0, false
		}
		c3 := s[i+3]
		if c3 == '=' || c3 == '!' {
			// 后视断言 (?<=...) (?<!...): 整组移除为 (?:).
			closeIdx, ok := findGroupClose(s, i)
			if !ok {
				return 0, false
			}
			*widened = true
			b.WriteString("(?:)")
			return closeIdx + 1, true
		}
		// 命名捕获 (?<name>...) -> (?:...).
		j := i + 3
		for j < n && s[j] != '>' {
			j++
		}
		if j >= n {
			return 0, false
		}
		b.WriteString("(?:")
		return j + 1, true
	case '\'':
		// 命名捕获 (?'name'...) -> (?:...).
		j := i + 3
		for j < n && s[j] != '\'' {
			j++
		}
		if j >= n {
			return 0, false
		}
		b.WriteString("(?:")
		return j + 1, true
	case 'P':
		// (?P<name>...) -> (?:...); (?P=name) 命名反向引用 -> bail.
		if i+3 < n && s[i+3] == '<' {
			j := i + 4
			for j < n && s[j] != '>' {
				j++
			}
			if j >= n {
				return 0, false
			}
			b.WriteString("(?:")
			return j + 1, true
		}
		return 0, false
	default:
		// (?flags) 或 (?flags:...): flags 取自 [imsxU-]. 原样保留交 RE2 解析 (不认则 parse 失败 bail).
		j := i + 2
		for j < n && isFlagChar(s[j]) {
			j++
		}
		if j < n && s[j] == ')' {
			b.WriteString(s[i : j+1])
			return j + 1, true
		}
		if j < n && s[j] == ':' {
			b.WriteString(s[i : j+1])
			return j + 1, true
		}
		return 0, false
	}
}

// findGroupClose 返回与 s[openIdx]=='(' 匹配的 ')' 的下标, 过程中正确跳过转义与字符类,
// 并对内部嵌套分组计数. 找不到返回 false.
func findGroupClose(s string, openIdx int) (int, bool) {
	n := len(s)
	depth := 0
	i := openIdx
	for i < n {
		c := s[i]
		switch c {
		case '\\':
			i += 2
			continue
		case '[':
			// 跳过字符类 (内部的 ( ) 不计).
			j := i + 1
			if j < n && s[j] == '^' {
				j++
			}
			if j < n && s[j] == ']' {
				j++
			}
			for j < n {
				if s[j] == '\\' {
					j += 2
					continue
				}
				if s[j] == ']' {
					break
				}
				j++
			}
			if j >= n {
				return 0, false
			}
			i = j + 1
			continue
		case '(':
			depth++
			i++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
			i++
		default:
			i++
		}
	}
	return 0, false
}

func isHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func isFlagChar(c byte) bool {
	switch c {
	case 'i', 'm', 's', 'x', 'U', '-':
		return true
	}
	return false
}
