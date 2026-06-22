package minirehs

import (
	"regexp/syntax"
	"strings"
)

// extractRequiredLiterals 从 RE2 语法树提取"必需字面量"集合: 任何命中都必然包含其中
// 至少一个字面量 (OR 关系). 这些字面量灌入 prefilter, 数据中不出现任一字面量的位置可被
// 整体跳过, 从而避免对每条正则全量扫描.
//
// 返回的字面量已小写化 (供大小写无关预过滤), 并保证每个长度 >= minLen.
// 若无法提取出满足条件的必需字面量, 返回 nil, 该 pattern 归入 always-on 集合.
//
// 提取策略 (先简后繁, 保证正确性优先):
//   - 对整棵树递归求 "required literal set": 任一命中必含该集合中某个字面量.
//   - OpLiteral: 该字面量本身.
//   - OpConcat: 取各子节点中"最长的那个必需字面量集合" (concat 要求所有子串都出现,
//     故任取其一即为必需; 选最长以最大化过滤力).
//   - OpAlternate: 各分支必需集合的并集 (任一分支命中即可), 但只有当每个分支都能提供
//     非空必需字面量时, 整体才有必需字面量; 任一分支无字面量 -> 整体无 (返回 nil).
//   - OpCapture/OpPlus (>=1次): 透传子节点 (至少出现一次).
//   - OpStar/OpQuest (可零次)/OpAnyChar/OpCharClass 等: 无必需字面量.
//
// 关键词: literal factoring, 必需字面量, string factor, prefilter
func extractRequiredLiterals(re *syntax.Regexp, minLen int) []string {
	lits := requiredLiterals(re)
	if lits == nil {
		return nil
	}
	out := make([]string, 0, len(lits))
	seen := make(map[string]struct{}, len(lits))
	for _, l := range lits {
		if len([]byte(l)) < minLen {
			// 任一必需字面量过短 -> 整体过滤力不足, 退化为 always-on 以避免高假阳预过滤.
			return nil
		}
		low := strings.ToLower(l)
		if _, ok := seen[low]; ok {
			continue
		}
		seen[low] = struct{}{}
		out = append(out, low)
	}
	return out
}

// requiredLiterals 返回"命中必含其一"的字面量集合; nil 表示无法保证任何必需字面量.
func requiredLiterals(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpLiteral:
		if len(re.Rune) == 0 {
			return nil
		}
		// prefilter 在 ASCII 小写域比较 (字面量与数据都做 ASCII 小写), 保证大小写无关时
		// 不漏报. 但非 ASCII 字符的大小写折叠会改变字节, ASCII 小写无法覆盖, 可能漏报;
		// 因此 FoldCase 且含非 ASCII 字符时放弃提取, 退化为 always-on (正确性优先).
		if re.Flags&syntax.FoldCase != 0 {
			for _, r := range re.Rune {
				if r > 127 {
					return nil
				}
			}
		}
		return []string{string(re.Rune)}

	case syntax.OpConcat:
		var best []string
		bestLen := 0
		for _, sub := range re.Sub {
			cand := requiredLiterals(sub)
			if cand == nil {
				continue
			}
			l := minLiteralLen(cand)
			if best == nil || l > bestLen {
				best = cand
				bestLen = l
			}
		}
		return best

	case syntax.OpAlternate:
		var all []string
		for _, sub := range re.Sub {
			cand := requiredLiterals(sub)
			if cand == nil {
				// 某分支无必需字面量 -> 整体无法保证 (该分支可不含任何字面量命中).
				return nil
			}
			all = append(all, cand...)
		}
		return all

	case syntax.OpCapture:
		if len(re.Sub) == 1 {
			return requiredLiterals(re.Sub[0])
		}
		return nil

	case syntax.OpPlus:
		// x+ 至少出现一次 x, 故 x 的必需字面量也是整体必需的.
		if len(re.Sub) == 1 {
			return requiredLiterals(re.Sub[0])
		}
		return nil

	case syntax.OpRepeat:
		// {n,m}: 当 n>=1 时子节点必需字面量被保留.
		if re.Min >= 1 && len(re.Sub) == 1 {
			return requiredLiterals(re.Sub[0])
		}
		return nil

	default:
		// OpStar / OpQuest / OpAnyChar / OpCharClass / OpEmptyMatch / 锚点 等: 无必需字面量.
		return nil
	}
}

// minLiteralLen 返回字面量集合中最短者的字节长度.
func minLiteralLen(lits []string) int {
	m := -1
	for _, l := range lits {
		n := len([]byte(l))
		if m < 0 || n < m {
			m = n
		}
	}
	if m < 0 {
		return 0
	}
	return m
}
