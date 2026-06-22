package minirehs

import (
	"regexp/syntax"
	"sort"
	"testing"
)

// classifyFallback 解释一条 expr 为何无法编入 mvsNFA: 返回简短英文原因 (供数据驱动决策).
// nfa 可编入返回 "nfa"; RE2 不可编译返回 "regexp2-only"; 其余走树walk 找首个不支持构造.
func classifyFallback(expr string) string {
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return "regexp2-only"
	}
	simplified := parsed.Simplify()
	if _, ok := compileMVSNFA(simplified); ok {
		return "nfa"
	}
	reason := scanUnsupportedOp(simplified, true)
	if reason == "" {
		reason = "other(nullable-root/empty-first/over)"
	}
	return reason
}

// scanUnsupportedOp 递归找首个本核不支持的 op; top 标记顶层 (顶层首尾锚是支持的).
func scanUnsupportedOp(re *syntax.Regexp, top bool) string {
	switch re.Op {
	case syntax.OpWordBoundary:
		return `\b`
	case syntax.OpNoWordBoundary:
		return `\B`
	case syntax.OpBeginLine:
		return "(?m)^"
	case syntax.OpEndLine:
		return "(?m)$"
	case syntax.OpBeginText:
		if !top {
			return "infix-^/\\A"
		}
	case syntax.OpEndText:
		if !top {
			return "infix-$/\\z"
		}
	case syntax.OpNoMatch:
		return "no-match"
	case syntax.OpCharClass:
		if len(re.Rune) == 0 {
			return "empty-class"
		}
	}
	// 顶层 concat 的首尾文本锚是支持的, 递归时对中间子节点关闭 top.
	for i, sub := range re.Sub {
		childTop := false
		if top && re.Op == syntax.OpConcat {
			if (i == 0 && sub.Op == syntax.OpBeginText) ||
				(i == len(re.Sub)-1 && sub.Op == syntax.OpEndText) {
				childTop = true
			}
		}
		if r := scanUnsupportedOp(sub, childTop); r != "" {
			return r
		}
	}
	return ""
}

// TestMVSFallbackReasons 数据驱动: 报告真实 MITM 规则集中每条 fallback 的原因分布,
// 指导 NFA 覆盖率扩展 (零宽断言等) 的优先级.
func TestMVSFallbackReasons(t *testing.T) {
	patterns, names := compilableMITMPatterns(t)
	counts := map[string]int{}
	byReason := map[string][]string{}
	for _, p := range patterns {
		expr := buildExprWithFlags(p)
		reason := classifyFallback(expr)
		counts[reason]++
		if reason != "nfa" {
			byReason[reason] = append(byReason[reason], names[p.ID]+" :: "+p.Expr)
		}
	}
	reasons := make([]string, 0, len(counts))
	for r := range counts {
		reasons = append(reasons, r)
	}
	sort.Slice(reasons, func(i, j int) bool { return counts[reasons[i]] > counts[reasons[j]] })
	t.Logf("=== MVS fallback reason breakdown (total=%d) ===", len(patterns))
	for _, r := range reasons {
		t.Logf("  %-30s %d", r, counts[r])
	}
	for _, r := range reasons {
		if r == "nfa" {
			continue
		}
		for _, e := range byReason[r] {
			t.Logf("    [%s] %s", r, e)
		}
	}
}
