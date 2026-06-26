package minirehs

import (
	"regexp/syntax"
	"strings"
)

// 本文件实现 Rose-lite 双向锚定的编译期"可救性分析": 对每个必需字面量, 按其在 AST 中的 *每个出现处*
// 分别判定 head (match-start 到字面量结尾) 与 tail (字面量结尾到 match-end) 是否有界, 据此给出
//
//	headF: 全部 head 有界出现处的最大 head (前向锚定注入区间 [h.end-headF, h.end] 的界); -1 = 无 head 有界出现处
//	tailR: 全部 tail 有界出现处的最大 tail (反向锚定注入区间 [h.end, h.end+tailR] 的界); -1 = 无 tail 有界出现处
//	ok:    该字面量 *每个* 出现处都至少一侧 (head 或 tail) 有界
//
// 零假阴论证 (核心): 任一匹配 M 必含某必需字面量出现处 O. 若 O 头有界 (<=headF) => 前向锚定覆盖
// (M.start>=hit.end-headF); 若 O 尾有界 (<=tailR) => 反向锚定覆盖 (M.end<=hit.end+tailR). 故当某
// pattern 的每个必需字面量都 ok 时, 前向 ∪ 反向 = 全部匹配, 绝不漏报. 与 computeLitWindow/Heads 同源
// (同一 sumWidthRange 宽度口径与 OpStar/OpPlus/OpRepeat 无界保守约定), 仅改为按出现处分别记录两侧界.
//
// 关键词: Rose-lite, 双向锚定可救性, per-occurrence 边界, headF, tailR, 零假阴

// litBiCover 是单个字面量的双向可救信息 (见文件头).
type litBiCover struct {
	headF int32
	tailR int32
	ok    bool
}

// computeLitBiCover 为 expr (RE2 可解析) 的每个必需字面量计算双向可救信息. 无法解析或字面量集为空
// 时返回空表 (调用方按"未命中=不可救"处理, 安全). lits 为已 ASCII 小写的触发字面量集.
func computeLitBiCover(expr string, lits []string) map[string]litBiCover {
	out := make(map[string]litBiCover, len(lits))
	if len(lits) == 0 {
		return out
	}
	re, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return out
	}
	re = re.Simplify()
	set := make(map[string]struct{}, len(lits))
	for _, l := range lits {
		set[l] = struct{}{}
	}
	acc := &biCoverAcc{set: set, out: map[string]*litBiCover{}}
	acc.walk(re, 0, true, 0, true)
	for lit, c := range acc.out {
		out[lit] = *c
	}
	return out
}

type biCoverAcc struct {
	set map[string]struct{}
	out map[string]*litBiCover
}

// record 累计字面量某出现处的两侧界 (pre/suf 为该结点 [起,止) 之外的上下文宽与有界性).
func (a *biCoverAcc) record(lit string, litLen, pre int, preB bool, suf int, sufB bool) {
	c := a.out[lit]
	if c == nil {
		c = &litBiCover{headF: -1, tailR: -1, ok: true}
		a.out[lit] = c
	}
	headB := preB && pre+litLen <= litWindowCap
	tailB := sufB && suf <= litWindowCap
	if headB {
		if h := int32(pre + litLen); h > c.headF {
			c.headF = h
		}
	}
	if tailB {
		if t := int32(suf); t > c.tailR {
			c.tailR = t
		}
	}
	if !headB && !tailB {
		c.ok = false // 该出现处两侧全无界: 前向反向都救不了 -> 字面量不可救
	}
}

// walk 与 litWindowAcc.walk 完全同构 (同一宽度/有界性继承), 仅把"全 pattern 取 max"换成"按出现处
// 分别 record". OpStar/OpPlus/OpRepeat 一律两侧无界 (与 computeLitWindow 同保守口径).
func (a *biCoverAcc) walk(re *syntax.Regexp, pre int, preB bool, suf int, sufB bool) {
	switch re.Op {
	case syntax.OpLiteral:
		s := strings.ToLower(string(re.Rune))
		if _, ok := a.set[s]; ok {
			a.record(s, len(string(re.Rune)), pre, preB, suf, sufB)
		}
	case syntax.OpCapture:
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, preB, suf, sufB)
		}
	case syntax.OpConcat:
		k := len(re.Sub)
		for i, sub := range re.Sub {
			lw, lb := sumWidthRange(re.Sub, 0, i)
			rw, rb := sumWidthRange(re.Sub, i+1, k)
			a.walk(sub, addSat(pre, lw), preB && lb, addSat(suf, rw), sufB && rb)
		}
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			a.walk(sub, pre, preB, suf, sufB)
		}
	case syntax.OpQuest:
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, preB, suf, sufB)
		}
	case syntax.OpStar, syntax.OpPlus, syntax.OpRepeat:
		if len(re.Sub) == 1 {
			a.walk(re.Sub[0], pre, false, suf, false)
		}
	}
}
