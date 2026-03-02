package sfverify

import (
	"fmt"
	"regexp"
	"strings"
)

// ruleVarAnalysis 从规则文本中解析变量定义、使用及依赖关系，用于诊断
type ruleVarAnalysis struct {
	Defined      map[string]bool   // 已定义的变量（as $x 或 include...as $x）
	Used         map[string]bool   // 被使用的变量（$x. 或出现在表达式中）
	Undefined    []string          // 被使用但未定义的变量
	Dependencies map[string][]string // var -> 其依赖的变量列表，用于从下往上追溯
}

var (
	reVarDefAs       = regexp.MustCompile(`as\s+\$([a-zA-Z0-9_]+)`)
	reIncludeAs      = regexp.MustCompile(`<include\s*\([^)]+\)\s*>\s*as\s+\$([a-zA-Z0-9_]+)`)
	reVarUsedDot     = regexp.MustCompile(`\$([a-zA-Z0-9_]+)\.`)       // $gin.Context
	reVarUsedSpace   = regexp.MustCompile(`\$([a-zA-Z0-9_]+)(?:\s|\))`) // $gin 或 $gin)
	reDataflowTo     = regexp.MustCompile(`#->\s*\$([a-zA-Z0-9_]+)`)   // #-> $sink (部分)
	reVarInPattern   = regexp.MustCompile(`\$([a-zA-Z0-9_]+)`)          // 所有变量引用
)

// parseRuleVarAnalysis 解析规则文本，返回变量分析结果
func parseRuleVarAnalysis(ruleContent string) *ruleVarAnalysis {
	a := &ruleVarAnalysis{
		Defined:      make(map[string]bool),
		Used:         make(map[string]bool),
		Dependencies: make(map[string][]string),
	}
	// 排除 desc 块内容，仅分析规则体（避免 heredoc 内误匹配）
	body := stripDescBlocks(ruleContent)

	// 1. 收集定义：include ... as $x 优先（单独匹配，避免和 as $x 重复）
	for _, m := range reIncludeAs.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Defined[m[1]] = true
		}
	}
	// 2. 收集定义：as $x
	for _, m := range reVarDefAs.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Defined[m[1]] = true
		}
	}

	// 3. 收集使用
	for _, m := range reVarUsedDot.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Used[m[1]] = true
		}
	}
	for _, m := range reVarUsedSpace.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Used[m[1]] = true
		}
	}
	for _, m := range reDataflowTo.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Used[m[1]] = true
		}
	}
	for _, m := range reVarInPattern.FindAllStringSubmatch(body, -1) {
		if len(m) >= 2 && m[1] != "_" {
			a.Used[m[1]] = true
		}
	}

	// 4. 未定义变量：被使用但未定义
	seenUndef := make(map[string]bool)
	for v := range a.Used {
		if !a.Defined[v] && !seenUndef[v] {
			a.Undefined = append(a.Undefined, v)
			seenUndef[v] = true
		}
	}

	// 5. 依赖关系：$a.xxx as $b => $b 依赖 $a；$a #-> xxx as $b => $b 依赖 $a
	// $context.Query(* as $param) as $source => $source 依赖 $context,$param；$param 依赖 $context
	reDepChain := regexp.MustCompile(`\$([a-zA-Z0-9_]+)\.([a-zA-Z0-9_*]+)\s*\([^)]*\)\s*as\s+\$([a-zA-Z0-9_]+)`)
	for _, m := range reDepChain.FindAllStringSubmatch(body, -1) {
		if len(m) >= 4 {
			base, target := m[1], m[3]
			if base != "_" && target != "_" {
				a.Dependencies[target] = appendUniq(a.Dependencies[target], base)
			}
		}
	}
	// $base.xxx as $target（无括号或简化形式）
	reDepSimple := regexp.MustCompile(`\$([a-zA-Z0-9_]+)\.([a-zA-Z0-9_*]+)\s+as\s+\$([a-zA-Z0-9_]+)`)
	for _, m := range reDepSimple.FindAllStringSubmatch(body, -1) {
		if len(m) >= 4 {
			base, target := m[1], m[3]
			if base != "_" && target != "_" {
				a.Dependencies[target] = appendUniq(a.Dependencies[target], base)
			}
		}
	}
	// $a #-> ... as $b：$b 依赖 $a
	reDataflowDep := regexp.MustCompile(`\$([a-zA-Z0-9_]+)\s+#->[^;]*?as\s+\$([a-zA-Z0-9_]+)`)
	for _, m := range reDataflowDep.FindAllStringSubmatch(body, -1) {
		if len(m) >= 3 && m[1] != "_" && m[2] != "_" {
			a.Dependencies[m[2]] = appendUniq(a.Dependencies[m[2]], m[1])
		}
	}
	// (* as $param) 或 (* #-> as $param) 形式：$param 依赖其前的 $context
	reParamInCall := regexp.MustCompile(`\$([a-zA-Z0-9_]+)\.[^;]*?\*\s*(?:#->\s*)?as\s+\$([a-zA-Z0-9_]+)`)
	for _, m := range reParamInCall.FindAllStringSubmatch(body, -1) {
		if len(m) >= 3 {
			base, target := m[1], m[2]
			if base != "_" && target != "_" {
				a.Dependencies[target] = appendUniq(a.Dependencies[target], base)
			}
		}
	}

	return a
}

func stripDescBlocks(s string) string {
	// 简单移除 desc(...) 块，避免 heredoc 内 $ 误匹配
	inDesc := false
	parenDepth := 0
	var out strings.Builder
	i := 0
	for i < len(s) {
		if i+4 <= len(s) && strings.ToLower(s[i:i+4]) == "desc" {
			inDesc = true
			parenDepth = 0
			i += 4
			for i < len(s) && (s[i] == ' ' || s[i] == '(') {
				if s[i] == '(' {
					parenDepth++
				}
				i++
			}
			continue
		}
		if inDesc {
			if s[i] == '(' {
				parenDepth++
			} else if s[i] == ')' {
				parenDepth--
				if parenDepth <= 0 {
					inDesc = false
				}
			}
			i++
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func appendUniq(slice []string, v string) []string {
	for _, x := range slice {
		if x == v {
			return slice
		}
	}
	return append(slice, v)
}

// buildBottomUpZeroChain 从下往上构建断点链：$sink:0 因依赖 $source:0；$source:0 因依赖 $context:0；...
// 用于 diagnosticHint，帮助 AI 理解变量间的逻辑依赖。undefined 为未定义变量名集合，用于在 reason 中标注「未定义」。
func buildBottomUpZeroChain(varOrder []string, diag map[string]int, deps map[string][]string, undefined map[string]bool) string {
	type link struct {
		varName string
		reason  string
	}
	var chain []link
	for i := len(varOrder) - 1; i >= 0; i-- {
		name := varOrder[i]
		cnt, ok := diag[name]
		if !ok {
			continue
		}
		if cnt == 0 {
			reason := ""
			if parents, has := deps[name]; has && len(parents) > 0 {
				for _, p := range parents {
					if undefined[p] {
						reason = "因 $" + p + " 未定义"
					} else if pc, pok := diag[p]; pok && pc == 0 {
						reason = "因依赖 $" + p + ":0"
					} else if !pok {
						reason = "因 $" + p + " 可能未定义"
					}
					if reason != "" {
						break
					}
				}
			}
			if reason == "" {
				if undefined[name] {
					reason = "变量未定义（如 include 缺少 as $" + name + "）"
				} else {
					reason = "其前模式/include 未匹配"
				}
			}
			chain = append(chain, link{name, reason})
		}
	}
	if len(chain) == 0 {
		return ""
	}
	var parts []string
	for j := len(chain) - 1; j >= 0; j-- {
		l := chain[j]
		parts = append(parts, fmt.Sprintf("$%s:0 ← %s", l.varName, l.reason))
	}
	return strings.Join(parts, "；")
}
