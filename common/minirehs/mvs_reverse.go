package minirehs

import (
	"math/bits"
	"regexp/syntax"
	"unicode/utf8"
)

// 本文件实现 Rose-lite 双向锚定的"反向"半边: 反向 NFA 构造 (结构反转 rune 级 bnode 树后复用
// Glushkov 构造器) + 反向锚定单趟存在性 (自尾向头扫描, 仅在"匹配终点候选"邻域注入起点).
//
// 动机: 形如 "value":keyword (无界 value 在 keyword 之前) 的分支, keyword 的回看 (head) 无界 =>
// 前向锚定 (existsInAnchored) 退化整段; 但其前看 (keyword 到 match-end, tail) 有界. 反向 NFA 接受
// 反转语言, 自 match-end 向 match-start 扫, 在 [h.end, h.end+tailMax] 注入起点即可提前消亡, 把这类
// 分支的 per-trigger 成本从 O(record) 降到 O(尾宽 + 匹配深度). 与前向锚定取并集后:
//   - 任一匹配必属某 AST 出现处; 该处 head 有界 (前向覆盖) 或 tail 有界 (反向覆盖);
//   - 故"每个出现处 head 或 tail 至少一侧有界"的 pattern, 前向 ∪ 反向 = 全部匹配, 绝不漏报.
//
// 正确性 (绝不假阴/假阳):
//   - 反向 NFA = 正向 bnode 结构反转后的同构 Glushkov 自动机, 接受 L^R (反转语言). 对 data 自尾向头
//     逐 rune 递推, 等价于正向 NFA 在反转 rune 序列上的存在性. 注入区间以"匹配终点偏移 e"(=runeEnd)
//     计: 命中字面量结尾 h.end, 任一 tail<=tailMax 的匹配满足 e<=h.end+tailMax, 故 e 落注入区间 =>
//     必能自 e 反扫到 match-start (无假阴); 区间外不注入 => 只判定真实终点的匹配 (无假阳).
//   - rune 切分: 反向用 utf8.DecodeLastRune. 其在合法 UTF-8 上与正向 DecodeRune 边界一致; 非法字节
//     两者均按"单字节 RuneError"处理. 该一致性由差分护栏 (含大量非法 UTF-8) 强校验 —— 一旦发现分歧
//     即改走"正向切分边界反向遍历", 保零假阴.
//   - 仅用于无文本锚 (anchoredStart/requireEnd 均 false) 的 lean NFA; 含锚/断言者 compileReverseExprToNFA
//     返回 nil, 退回原整段/前向路径, 安全.
//
// 关键词: Rose-lite, 反向锚定, reverse NFA, suffix anchoring, 双向并集, 零假阴

// biAnchorEnabled 是双向锚定 (Rose-lite 完全体) 的编译期总开关, 默认开启. 仅供 A/B 基准
// (Test/Benchmark) 临时关闭以量化纯增量收益; 生产恒为 true.
var biAnchorEnabled = true

// reverseBnode 结构化反转 rune 级 AST: concat 子序列翻转并各自递归反转; alt/star/plus/quest 递归
// 反转子树; 叶子 (bClass/bEmpty/bAssert) 原样. bAssert 不出现在 lean 反向路径 (含断言者不进此路径).
func reverseBnode(n *bnode) *bnode {
	switch n.kind {
	case bConcat:
		k := len(n.sub)
		rev := make([]*bnode, k)
		for i, s := range n.sub {
			rev[k-1-i] = reverseBnode(s)
		}
		return &bnode{kind: bConcat, sub: rev}
	case bAlt:
		rev := make([]*bnode, len(n.sub))
		for i, s := range n.sub {
			rev[i] = reverseBnode(s)
		}
		return &bnode{kind: bAlt, sub: rev}
	case bStar, bPlus, bQuest:
		if len(n.sub) == 1 {
			return &bnode{kind: n.kind, sub: []*bnode{reverseBnode(n.sub[0])}}
		}
		return n
	default:
		return n // bClass / bEmpty / bAssert
	}
}

// compileReverseExprToNFA 解析 expr -> 剥锚 -> rune 级树 -> 结构反转 -> Glushkov, 得反向 NFA.
// 仅对无文本锚的 lean pattern 有效 (含 ^/$ 或零宽断言者返回 nil, 调用方退回原路径).
func compileReverseExprToNFA(expr string) *mvsNFA {
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil
	}
	s := parsed.Simplify()
	anchoredStart, requireEnd, core := stripEndAnchors(s)
	if anchoredStart || requireEnd {
		return nil // 文本锚: 反向语义需额外锚处理, 不在本期范围, 退回安全.
	}
	root, ok := synToRune(core)
	if !ok {
		return nil
	}
	rev := reverseBnode(root)
	nfa, ok := glushkovNFA(rev, false, false)
	if !ok {
		return nil
	}
	if nfa.hasAssert {
		return nil // 双保险: 反向锚定仅 lean.
	}
	return nfa
}

// existsInReverseAnchored 是反向 NFA 的反向锚定单趟存在性判定. spans 为"匹配终点候选"区间 (绝对字节
// 偏移, [lo,hi] 表示匹配可终止于该范围内某 rune 边界), 须已排序合并 (mergeAnchorSpans). 自尾向头扫:
// 当前 rune 终点 runeEnd 落某 span 时注入反向 NFA 起点 first; 活跃集空且越过所有 span 即提前消亡.
// prev/cand/active 为可复用缓冲 (长度 >= nfa.nword). 仅用于 lean (非断言) 反向 NFA.
func (nfa *mvsNFA) existsInReverseAnchored(data []byte, spans []anchorSpan, prev, cand, active []uint64) bool {
	if len(spans) == 0 {
		return false
	}
	nword := nfa.nword
	prev = prev[:nword]
	cand = cand[:nword]
	active = active[:nword]
	for w := 0; w < nword; w++ {
		prev[w] = 0
	}
	n := len(data)
	firstLo := int(spans[0].lo)
	maxHi := int(spans[len(spans)-1].hi)
	if maxHi > n {
		maxHi = n
	}
	si := len(spans) - 1
	hasActive := false

	i := alignRuneStart(data, maxHi) // 自最高匹配终点候选 (rune 边界) 起, 向头扫.
	for i > 0 {
		r, size := utf8.DecodeLastRune(data[:i])
		runeEnd := i
		j := i - size
		sym := nfa.symbolOf(r)

		// 定位包含 runeEnd 的 span (descending): 跳过 lo 高于 runeEnd 的 span.
		for si >= 0 && runeEnd < int(spans[si].lo) {
			si--
		}
		inject := si >= 0 && runeEnd >= int(spans[si].lo) && runeEnd <= int(spans[si].hi)

		if inject {
			copy(cand, nfa.first)
		} else {
			for w := 0; w < nword; w++ {
				cand[w] = 0
			}
		}
		if hasActive {
			for w := 0; w < nword; w++ {
				pw := prev[w]
				for pw != 0 {
					p := w*64 + bits.TrailingZeros64(pw)
					pw &= pw - 1
					fp := nfa.follow[p]
					for k := 0; k < nword; k++ {
						cand[k] |= fp[k]
					}
				}
			}
		}

		rc := nfa.reach[sym]
		var anyActive uint64
		for w := 0; w < nword; w++ {
			v := cand[w] & rc[w]
			active[w] = v
			anyActive |= v
			if v&nfa.lastAny[w] != 0 {
				return true
			}
		}

		// 提前消亡: 活跃集空且后续不会再注入 (越过所有 span 的下界) => 立即返回.
		hasActive = anyActive != 0
		if !hasActive && (si < 0 || j < firstLo) {
			return false
		}
		copy(prev, active)
		i = j
	}
	return false
}

// existsInReverseAnchored1 是 existsInReverseAnchored 的 nword==1 标量零分配快路径. 语义完全一致.
func (nfa *mvsNFA) existsInReverseAnchored1(data []byte, spans []anchorSpan) bool {
	if len(spans) == 0 {
		return false
	}
	first := nfa.first1
	lastAny := nfa.lastAny1
	follow := nfa.follow1
	reach := nfa.reach1
	n := len(data)
	firstLo := int(spans[0].lo)
	maxHi := int(spans[len(spans)-1].hi)
	if maxHi > n {
		maxHi = n
	}
	si := len(spans) - 1
	var prev uint64
	hasActive := false

	i := alignRuneStart(data, maxHi)
	for i > 0 {
		r, size := utf8.DecodeLastRune(data[:i])
		runeEnd := i
		j := i - size
		sym := nfa.symbolOf(r)

		for si >= 0 && runeEnd < int(spans[si].lo) {
			si--
		}
		var cand uint64
		if si >= 0 && runeEnd >= int(spans[si].lo) && runeEnd <= int(spans[si].hi) {
			cand = first
		}
		if hasActive {
			for pw := prev; pw != 0; pw &= pw - 1 {
				cand |= follow[bits.TrailingZeros64(pw)]
			}
		}
		active := cand & reach[sym]
		if active&lastAny != 0 {
			return true
		}
		hasActive = active != 0
		if !hasActive && (si < 0 || j < firstLo) {
			return false
		}
		prev = active
		i = j
	}
	return false
}
