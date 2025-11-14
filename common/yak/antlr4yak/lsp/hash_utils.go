package lsp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// CodeHashLevels 包含代码的多级哈希
type CodeHashLevels struct {
	Full      string // Level 1: 全文哈希（最快判断）
	Structure string // Level 2: 结构哈希（去除注释、空白、格式）
	Semantic  string // Level 3: 语义哈希（仅保留标识符、关键字、字面量、运算符）
}

// ComputeCodeHash 计算代码的多级哈希
func ComputeCodeHash(code string) CodeHashLevels {
	return CodeHashLevels{
		Full:      computeFullHash(code),
		Structure: computeStructureHash(code),
		Semantic:  computeSemanticHash(code),
	}
}

// computeFullHash 计算全文哈希（快速路径）
func computeFullHash(code string) string {
	h := sha256.New()
	h.Write([]byte(code))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// computeStructureHash 计算结构哈希（去除注释和多余空白）
func computeStructureHash(code string) string {
	// 去除单行注释
	code = removeLineComments(code)
	// 去除块注释
	code = removeBlockComments(code)
	// 规范化空白
	code = normalizeWhitespace(code)

	h := sha256.New()
	h.Write([]byte(code))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// computeSemanticHash 计算语义哈希（提取语义 token）
func computeSemanticHash(code string) string {
	// 去除单行注释
	code = removeLineComments(code)
	// 去除块注释
	code = removeBlockComments(code)
	// 规范化空白
	code = normalizeWhitespace(code)
	tokens := extractSemanticTokens(code)
	normalized := strings.Join(tokens, " ")

	h := sha256.New()
	h.Write([]byte(normalized))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// removeLineComments 移除单行注释（// 和 #）
func removeLineComments(code string) string {
	lines := strings.Split(code, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		// 查找注释起始位置（不在字符串内）
		inString := false
		stringChar := rune(0)
		for i, ch := range line {
			if !inString {
				if ch == '"' || ch == '\'' || ch == '`' {
					inString = true
					stringChar = ch
				} else if ch == '/' && i+1 < len(line) && line[i+1] == '/' {
					// 找到注释，截断
					line = line[:i]
					break
				} else if ch == '#' {
					// Yaklang 也支持 # 注释
					line = line[:i]
					break
				}
			} else if ch == stringChar && (i == 0 || line[i-1] != '\\') {
				inString = false
			}
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// removeBlockComments 移除块注释（/* ... */）
func removeBlockComments(code string) string {
	// 使用正则表达式移除块注释（简化版，不处理嵌套）
	blockCommentRe := regexp.MustCompile(`(?s)/\*.*?\*/`)
	return blockCommentRe.ReplaceAllString(code, "")
}

// normalizeWhitespace 规范化空白字符
func normalizeWhitespace(code string) string {
	// 将多个空白字符替换为单个空格
	spaceRe := regexp.MustCompile(`\s+`)
	code = spaceRe.ReplaceAllString(code, " ")
	// 去除行首行尾空白
	code = strings.TrimSpace(code)
	return code
}

// extractSemanticTokens 提取语义 token（关键字、标识符、字面量、运算符）
func extractSemanticTokens(code string) []string {
	var tokens []string
	var currentToken strings.Builder
	inString := false
	stringChar := rune(0)

	// Yaklang 关键字集合
	keywords := map[string]bool{
		"fn": true, "def": true, "var": true, "const": true,
		"if": true, "else": true, "elif": true, "for": true, "while": true,
		"break": true, "continue": true, "return": true,
		"try": true, "catch": true, "finally": true,
		"import": true, "as": true, "from": true,
		"go": true, "defer": true, "recover": true,
		"class": true, "struct": true, "interface": true,
		"true": true, "false": true, "nil": true, "undefined": true,
	}

	// 运算符集合
	operators := map[string]bool{
		"=": true, "+": true, "-": true, "*": true, "/": true, "%": true,
		"==": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
		"&&": true, "||": true, "!": true,
		"&": true, "|": true, "^": true, "<<": true, ">>": true,
		"+=": true, "-=": true, "*=": true, "/=": true,
		"++": true, "--": true,
		".": true, ",": true, ";": true, ":": true,
		"(": true, ")": true, "[": true, "]": true, "{": true, "}": true,
		"->": true, "=>": true, "...": true,
	}

	runes := []rune(code)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		// 处理字符串字面量
		if !inString && (ch == '"' || ch == '\'' || ch == '`') {
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
			inString = true
			stringChar = ch
			i++
			continue
		}

		if inString {
			if ch == stringChar && (i == 0 || runes[i-1] != '\\') {
				inString = false
				tokens = append(tokens, fmt.Sprintf("STRING:%v", currentToken.String()))
				currentToken.Reset()
			} else {
				currentToken.WriteRune(ch)
			}
			i++
			continue
		}

		// 处理标识符和关键字
		if unicode.IsLetter(ch) || ch == '_' {
			currentToken.WriteRune(ch)
			i++
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				currentToken.WriteRune(runes[i])
				i++
			}
			token := currentToken.String()
			if keywords[token] {
				tokens = append(tokens, "KW:"+token)
			} else {
				tokens = append(tokens, "ID")
			}
			currentToken.Reset()
			continue
		}

		// 处理数字字面量
		if unicode.IsDigit(ch) {
			currentToken.WriteRune(ch)
			i++
			for i < len(runes) && (unicode.IsDigit(runes[i]) || runes[i] == '.' || runes[i] == 'x' || runes[i] == 'e' || runes[i] == 'E') {
				currentToken.WriteRune(runes[i])
				i++
			}
			tokens = append(tokens, fmt.Sprintf("NUM:%v", currentToken.String()))
			currentToken.Reset()
			continue
		}

		// 处理运算符（多字符）
		if i+2 < len(runes) {
			op3 := string(runes[i : i+3])
			if operators[op3] {
				tokens = append(tokens, "OP:"+op3)
				i += 3
				continue
			}
		}
		if i+1 < len(runes) {
			op2 := string(runes[i : i+2])
			if operators[op2] {
				tokens = append(tokens, "OP:"+op2)
				i += 2
				continue
			}
		}
		op1 := string(ch)
		if operators[op1] {
			tokens = append(tokens, "OP:"+op1)
			i++
			continue
		}

		// 跳过其他字符（空白等）
		i++
	}

	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	return tokens
}

// HashesEqual 比较两个哈希级别是否相等
func HashesEqual(h1, h2 CodeHashLevels) (fullEqual, structEqual, semanticEqual bool) {
	fullEqual = h1.Full == h2.Full
	structEqual = h1.Structure == h2.Structure
	semanticEqual = h1.Semantic == h2.Semantic
	return
}

// ShouldRecompileSSA 判断是否需要重新编译 SSA
func ShouldRecompileSSA(oldHash, newHash CodeHashLevels) bool {
	// 如果语义哈希不同，需要重新编译
	if oldHash.Semantic != newHash.Semantic {
		return true
	}
	// 如果语义哈希相同，可以复用 SSA
	return false
}

// ShouldReparseAST 判断是否需要重新解析 AST
func ShouldReparseAST(oldHash, newHash CodeHashLevels) bool {
	// 如果结构哈希不同，需要重新解析
	if oldHash.Structure != newHash.Structure {
		return true
	}
	// 如果结构哈希相同，可以复用 AST
	return false
}

// GetCacheKey 生成缓存键
func GetCacheKey(scriptType string, hash string) string {
	return scriptType + ":" + hash
}
