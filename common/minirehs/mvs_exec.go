package minirehs

import (
	"math/bits"
	"unicode/utf8"
)

// existsIn 报告 nfa 是否在 data 中存在至少一次命中 (存在性语义, 不取偏移).
//
// 输入按 Go regexp 完全相同的方式逐 rune 解码 (utf8.DecodeRune: 非法字节 -> RuneError 单字节),
// 每个 rune 经字母表压缩为符号 id, 再做位并行 Glushkov 递推:
//
//	cand   = startSet | OR(follow[p] for p in prev)   (无锚每步注入 first; 锚定仅首 rune 注入)
//	active = cand & reach[sym]
//	命中   <=> active & lastAny != 0, 或 (requireEnd 且已到输入末尾) active & lastEnd != 0
//
// 关键词: mvscan, bit-parallel NFA, existence scan, rune decode, RuneError
func (nfa *mvsNFA) existsIn(data []byte) bool {
	if nfa.single {
		return nfa.existsIn1(data)
	}
	nword := nfa.nword
	prev := make([]uint64, nword)
	cand := make([]uint64, nword)
	active := make([]uint64, nword)
	n := len(data)

	i := 0
	for i < n {
		atStart := i == 0
		r, size := utf8.DecodeRune(data[i:])
		i += size
		sym := nfa.symbolOf(r)

		if !nfa.anchoredStart || atStart {
			copy(cand, nfa.first)
		} else {
			for w := range cand {
				cand[w] = 0
			}
		}
		for w := 0; w < nword; w++ {
			pw := prev[w]
			for pw != 0 {
				p := w*64 + bits.TrailingZeros64(pw)
				fp := nfa.follow[p]
				for k := 0; k < nword; k++ {
					cand[k] |= fp[k]
				}
				pw &= pw - 1
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
		if nfa.requireEnd && i == n {
			for w := 0; w < nword; w++ {
				if active[w]&nfa.lastEnd[w] != 0 {
					return true
				}
			}
		}
		if anyActive == 0 && nfa.anchoredStart {
			return false
		}
		copy(prev, active)
	}
	return false
}

// existsIn1 是 nword==1 (位置数 <=64) 的零分配快路径: 活跃集是单个 uint64, 全程寄存器位运算.
// 绝大多数真实 pattern 走此路径.
func (nfa *mvsNFA) existsIn1(data []byte) bool {
	first := nfa.first1
	lastAny := nfa.lastAny1
	lastEnd := nfa.lastEnd1
	follow := nfa.follow1
	reach := nfa.reach1
	anchored := nfa.anchoredStart
	requireEnd := nfa.requireEnd
	n := len(data)

	var prev uint64
	i := 0
	for i < n {
		atStart := i == 0
		r, size := utf8.DecodeRune(data[i:])
		i += size

		var cand uint64
		if !anchored || atStart {
			cand = first
		}
		for pw := prev; pw != 0; pw &= pw - 1 {
			cand |= follow[bits.TrailingZeros64(pw)]
		}
		active := cand & reach[nfa.symbolOf(r)]
		if active&lastAny != 0 {
			return true
		}
		if requireEnd && i == n && active&lastEnd != 0 {
			return true
		}
		if active == 0 && anchored {
			return false
		}
		prev = active
	}
	return false
}
