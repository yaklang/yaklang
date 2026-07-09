package minirehs

import (
	"math/bits"
	"unicode/utf8"
)

// 本文件实现"锚定式单趟存在性验证"(anchored single-pass): 命中必需字面量后, 只在字面量命中点
// 邻域 (按 per-literal 回看上限 head 推出的注入区间) 注入 NFA 起点 first, 其余位置一律不注入.
//
// 与整段 existsIn / existsInAssertShared 的本质区别 —— 后者每个 rune 步都重新注入 first, 导致活跃
// 集"永不消亡", 含无界尾 (.* / \w+ / [^x]+ 等) 的 pattern 被迫扫到报文末尾; 锚定式只在注入区间内
// 注入, 活跃集可在匹配失败后消亡, 一旦越过所有注入区间即提前返回. 这是逼近 vectorscan "literal ->
// 局部验证 (Rose)" 的关键: 把"有界头 + 无界尾"这类 pattern 的 per-trigger 成本从 O(record) 降到
// O(头部宽 + 匹配深度).
//
// 正确性 (绝不假阴/假阳):
//   - 任一匹配 M 必含某必需字面量 (必需字面量集语义), 设其在 data 命中结束于 h.end, 则
//     M.start >= h.end - head_L (head_L 为该字面量在本 pattern 的回看上限, 见 computeLitHeads).
//     调用方以 [h.end-head_L, h.end] 为注入区间 (head_L<0 退化为 [0,h.end]); 故 M.start 必落某
//     注入区间内 => 锚定式必能从该起点找到 M (无假阴). 区间外不注入 => 只会找到真实起点的匹配
//     (无假阳). 因此 existsInAnchored(spans) 与整段 existsIn 同真伪.
//   - 断言版位置锚 (^ $ \b 等) 编码为 condFirst/condAccept 的 guard, 按真实共享 bound (整段预算,
//     绝对偏移索引) 门控, 故在多注入位置/跳跃处的边界判定与整段逐位一致, 额外注入位置被 guard 自动
//     滤除, 无害.
//
// 关键词: anchored verification, Rose-lite, single-pass, literal anchoring, early death, 锚定式单趟

// anchorSpan 是一个注入区间 [lo,hi): 锚定式扫描只在落入这些区间的 rune 起始处注入 NFA 起点 first.
type anchorSpan struct {
	lo, hi int32
}

// mergeAnchorSpans 原地按 lo 升序排序并合并重叠/相邻区间, 返回合并后的前缀切片.
// 各命中点的注入区间 lo=h.end-head_L 因 head_L 随字面量不同而非单调, 故需排序后合并.
// 用插入排序 (而非 sort.Slice): per-pattern 区间数通常很小 (< 数十), 且插入排序零分配
// (sort.Slice 的闭包+反射会在热路径每次调用分配), 对每报文每锚定 pattern 调用更友好.
func mergeAnchorSpans(spans []anchorSpan) []anchorSpan {
	if len(spans) <= 1 {
		return spans
	}
	for i := 1; i < len(spans); i++ {
		v := spans[i]
		j := i - 1
		for j >= 0 && spans[j].lo > v.lo {
			spans[j+1] = spans[j]
			j--
		}
		spans[j+1] = v
	}
	w := 0
	for r := 1; r < len(spans); r++ {
		if spans[r].lo <= spans[w].hi {
			if spans[r].hi > spans[w].hi {
				spans[w].hi = spans[r].hi
			}
		} else {
			w++
			spans[w] = spans[r]
		}
	}
	return spans[:w+1]
}

// existsInAnchored 是 lean NFA (无零宽断言) 的锚定式单趟存在性判定. spans 须已排序合并 (mergeAnchorSpans).
// prev/cand/active 为调用方提供的可复用工作缓冲 (长度须 >= nfa.nword), 避免热路径分配.
//
// 仅用于非 anchoredStart 的 lean NFA (^ 锚定 pattern 不走此路径, 见 compile 资格判定): 故每个注入
// 区间位置注入 first 均为合法匹配起点. requireEnd ($) 仍在到达真实报文末尾 n 时按 lastEnd 接受.
func (nfa *mvsNFA) existsInAnchored(data []byte, spans []anchorSpan, prev, cand, active []uint64) bool {
	if len(spans) == 0 {
		return false
	}
	nword := nfa.nword
	n := len(data)
	prev = prev[:nword]
	cand = cand[:nword]
	active = active[:nword]
	for w := 0; w < nword; w++ {
		prev[w] = 0
	}
	lastHi := int(spans[len(spans)-1].hi)
	si := 0
	hasActive := false

	i := alignRuneStart(data, int(spans[0].lo))
	for i < n {
		runeStart := i
		c := data[i]
		var sym, ni int
		if c < utf8.RuneSelf {
			sym = int(nfa.asciiSym[c])
			ni = i + 1
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = nfa.symbolOf(r)
			ni = i + size
		}

		for si < len(spans) && runeStart >= int(spans[si].hi) {
			si++
		}
		inject := si < len(spans) && runeStart >= int(spans[si].lo)

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
		if nfa.requireEnd && ni == n {
			for w := 0; w < nword; w++ {
				if active[w]&nfa.lastEnd[w] != 0 {
					return true
				}
			}
		}

		// 提前消亡: 活跃集空且已越过所有注入区间 (不会再注入新起点) => 不可能再命中, 立即返回.
		// (注入区间之间的"空洞"逐 rune 廉价走过即可; 不做向前跳跃 —— 跳跃在非法 UTF-8/二进制流上
		//  可能因连续 continuation 字节而回退, 破坏前向单调性, 故弃用, 改由提前消亡兜住主要收益。)
		hasActive = anyActive != 0
		if !hasActive && (si >= len(spans) || runeStart >= lastHi) {
			return false
		}
		copy(prev, active)
		i = ni
	}
	return false
}

// existsInAnchored1 是 existsInAnchored 的 nword==1 (位置数<=64) 标量快路径: 活跃集为单个 uint64,
// 全程寄存器位运算, 零分配 (无需调用方 prev/cand/active 缓冲). 语义与 existsInAnchored 完全一致,
// 仅用于 nfa.single 的 lean NFA. 含 ASCII 快路径 (省去 utf8.DecodeRune + symbolOf 调用开销).
func (nfa *mvsNFA) existsInAnchored1(data []byte, spans []anchorSpan) bool {
	if len(spans) == 0 {
		return false
	}
	first := nfa.first1
	lastAny := nfa.lastAny1
	lastEnd := nfa.lastEnd1
	follow := nfa.follow1
	reach := nfa.reach1
	requireEnd := nfa.requireEnd
	n := len(data)
	nspan := len(spans)
	lastHi := int(spans[nspan-1].hi)
	si := 0
	// 缓存当前 span 的 lo/hi 到局部变量, 推进时更新, 避免每 rune 的 spans[si].hi/lo 数组索引
	// + int32->int 转换开销 (span 推进是 existsInAnchored1 的最大热点, profile ~24% flat).
	curLo := int(spans[0].lo)
	curHi := int(spans[0].hi)
	var prev uint64
	hasActive := false

	i := alignRuneStart(data, curLo)
	for i < n {
		runeStart := i
		c := data[i]
		var sym, ni int
		if c < utf8.RuneSelf {
			sym = int(nfa.asciiSym[c])
			ni = i + 1
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = nfa.symbolOf(r)
			ni = i + size
		}

		// span 推进: si 单调递增, 推进时更新缓存的 curLo/curHi.
		for runeStart >= curHi {
			si++
			if si >= nspan {
				break
			}
			curLo = int(spans[si].lo)
			curHi = int(spans[si].hi)
		}
		var cand uint64
		if si < nspan && runeStart >= curLo {
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
		if requireEnd && ni == n && active&lastEnd != 0 {
			return true
		}
		hasActive = active != 0
		if !hasActive && (si >= nspan || runeStart >= lastHi) {
			return false
		}
		prev = active
		i = ni
	}
	return false
}

// existsInAssertAnchored 是断言 NFA 的锚定式单趟存在性判定 (与 existsInAssertShared 同语义, 但仅在
// spans 注入区间内注入起点, 支持提前消亡). bound 为整段共享边界 (computeBoundaries, 长度 len(data)+1),
// 按绝对偏移索引以保证 \b/^$ 等条件取自完整报文邻字符. prev/cand 为可复用缓冲 (长度 >= nfa.nword).
func (nfa *mvsNFA) existsInAssertAnchored(data []byte, bound []uint8, spans []anchorSpan, prev, cand []uint64) bool {
	if len(spans) == 0 {
		return false
	}
	nword := nfa.nword
	n := len(data)
	prev = prev[:nword]
	cand = cand[:nword]
	for w := 0; w < nword; w++ {
		prev[w] = 0
	}
	lastHi := int(spans[len(spans)-1].hi)
	si := 0
	hasActive := false

	i := alignRuneStart(data, int(spans[0].lo))
	for i < n {
		runeStart := i
		c := data[i]
		var sym, ni int
		if c < utf8.RuneSelf {
			sym = int(nfa.asciiSym[c])
			ni = i + 1
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = nfa.symbolOf(r)
			ni = i + size
		}
		bpre := bound[runeStart]

		for si < len(spans) && runeStart >= int(spans[si].hi) {
			si++
		}
		inject := si < len(spans) && runeStart >= int(spans[si].lo)

		if inject {
			copy(cand, nfa.first)
			for _, gb := range nfa.condFirst {
				if guardHolds(gb.g, bpre) {
					for w := 0; w < nword; w++ {
						cand[w] |= gb.bits[w]
					}
				}
			}
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
					for _, gb := range nfa.condFollow[p] {
						if guardHolds(gb.g, bpre) {
							for k := 0; k < nword; k++ {
								cand[k] |= gb.bits[k]
							}
						}
					}
				}
			}
		}

		rc := nfa.reach[sym]
		var anyActive uint64
		for w := 0; w < nword; w++ {
			v := cand[w] & rc[w]
			prev[w] = v
			anyActive |= v
			if v&nfa.lastAny[w] != 0 {
				return true
			}
		}
		if len(nfa.condAccept) > 0 {
			bpost := bound[ni]
			for _, gb := range nfa.condAccept {
				if guardHolds(gb.g, bpost) {
					for w := 0; w < nword; w++ {
						if prev[w]&gb.bits[w] != 0 {
							return true
						}
					}
				}
			}
		}

		// 提前消亡 (同 existsInAnchored): 活跃集空且越过所有注入区间即返回.
		hasActive = anyActive != 0
		if !hasActive && (si >= len(spans) || runeStart >= lastHi) {
			return false
		}
		i = ni
	}
	return false
}

// existsInAssertAnchored1 是 existsInAssertAnchored 的 nword==1 标量快路径 (活跃集单 uint64, 零分配).
// 语义与 existsInAssertAnchored 完全一致, 仅用于 nfa.single 的断言 NFA. guard 位集取 gb.bits[0]
// (nword==1 时长度为 1). 含 ASCII 快路径. bound 仍为整段共享边界 (绝对偏移索引).
func (nfa *mvsNFA) existsInAssertAnchored1(data []byte, bound []uint8, spans []anchorSpan) bool {
	if len(spans) == 0 {
		return false
	}
	first := nfa.first1
	lastAny := nfa.lastAny1
	follow := nfa.follow1
	reach := nfa.reach1
	n := len(data)
	nspan := len(spans)
	lastHi := int(spans[nspan-1].hi)
	si := 0
	// 缓存当前 span lo/hi (同 existsInAnchored1, 省每 rune 数组索引 + int32 转换).
	curLo := int(spans[0].lo)
	curHi := int(spans[0].hi)
	var prev uint64
	hasActive := false

	i := alignRuneStart(data, curLo)
	for i < n {
		runeStart := i
		c := data[i]
		var sym, ni int
		if c < utf8.RuneSelf {
			sym = int(nfa.asciiSym[c])
			ni = i + 1
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = nfa.symbolOf(r)
			ni = i + size
		}
		bpre := bound[runeStart]

		for runeStart >= curHi {
			si++
			if si >= nspan {
				break
			}
			curLo = int(spans[si].lo)
			curHi = int(spans[si].hi)
		}
		var cand uint64
		if si < nspan && runeStart >= curLo {
			cand = first
			for _, gb := range nfa.condFirst {
				if guardHolds(gb.g, bpre) {
					cand |= gb.bits[0]
				}
			}
		}
		if hasActive {
			for pw := prev; pw != 0; pw &= pw - 1 {
				p := bits.TrailingZeros64(pw)
				cand |= follow[p]
				for _, gb := range nfa.condFollow[p] {
					if guardHolds(gb.g, bpre) {
						cand |= gb.bits[0]
					}
				}
			}
		}
		active := cand & reach[sym]
		if active&lastAny != 0 {
			return true
		}
		if len(nfa.condAccept) > 0 {
			bpost := bound[ni]
			for _, gb := range nfa.condAccept {
				if guardHolds(gb.g, bpost) && active&gb.bits[0] != 0 {
					return true
				}
			}
		}
		hasActive = active != 0
		if !hasActive && (si >= nspan || runeStart >= lastHi) {
			return false
		}
		prev = active
		i = ni
	}
	return false
}

// alignRuneStart 把字节偏移 off 向左吸附到最近的 rune 起始 (off<=0 归 0; off>=len 归 len).
// 共享 bound 仅在 rune 起始与末尾处写入真实值, 故注入区间端点与跳跃目标须 rune 对齐.
func alignRuneStart(data []byte, off int) int {
	n := len(data)
	if off <= 0 {
		return 0
	}
	if off >= n {
		return n
	}
	for off > 0 && !utf8.RuneStart(data[off]) {
		off--
	}
	return off
}
