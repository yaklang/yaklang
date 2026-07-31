package minirehs

import (
	"math/bits"
	"sort"
	"unicode"
	"unicode/utf8"
)

// 本文件实现 R2 基建: 把多条"无字面量、每条记录都要跑"的 always-on 断言 NFA (hasAssert=true,
// 如身份证/MAC) 合并成一个单趟扫描的位并行自动机, 共享同一份 computeBoundaries 预算的边界条件.
//
// 与 lean 合并 (mvs_merged.go) 的区别: 断言 NFA 带边界守卫 (guard), first/follow/accept 各有
// "无条件部分"和"按 guard 分组的条件部分". 合并时把各成员的位置空间拼接 (offset 平移), guard 不变
// (guard 是位置无关的边界条件), 条件位集按 offset 平移到全局位置空间. 全局字母表取各成员字母表的
// 并集 (cut points 合并), reach 按全局符号重建.
//
// 正确性: 合并自动机对成员 m 的命中判定, 与 m 单独 existsInAssertShared 的判定完全等价:
//   - guard 在合并前后完全相同 (位置无关), 按 bound[i] 门控的逻辑不变;
//   - first/follow/accept 位集仅做 offset 平移, 位语义不变;
//   - reach 用更细的全局字母表, 对每个 rune 的接受集与原 per-member 字母表相同.
//
// A/B 结论: 实测合并后 nword 1→2 (两成员各 nword==1), 多字循环开销 > 省的趟数,
// 净回归. 故默认不接线 (assertMergedEnabled=false), 保留作差分护栏与后续优化基线.
//
// 关键词: assert NFA merge, single-pass, shared boundaries, guard, 位并行, R2

// mvsAssertMergedNFA 是若干断言 NFA 的不相交并集, 共享全局字母表、活跃位集与边界条件.
type mvsAssertMergedNFA struct {
	npos  int
	nword int
	nsym  int

	first      []uint64         // 无条件 first 位集并集 (每步注入)
	condFirst  []guardedBits    // 条件 first (按 bpre 门控, 已平移到全局位置)
	follow     [][]uint64       // 无条件后继 (跨成员无边)
	condFollow [][]guardedBits  // 每位置的条件后继 (已平移, 按 bpre 门控)
	lastAny    []uint64         // 命中位置并集 (任意处接受)
	lastEnd    []uint64         // requireEnd 成员的命中位置并集 (仅末尾接受)
	condAccept []guardedBits    // 条件接受 (按 bpost 门控, 已平移)
	posPat     []int32          // 全局位置 -> 成员 idx (仅命中位置有效, 其余 -1)

	// 字母表
	asciiSym [128]int32
	cuts     []rune
	reach    [][]uint64

	// 成员列表 (用于命中后定位/上报)
	nmem   int
	memIdx []int // [nmem] -> compiledPattern idx
}

// buildAssertMergedNFA 把多条断言 NFA 合并为单趟扫描自动机. 成员数为 0 返回 nil.
func buildAssertMergedNFA(members []mergeMember) *mvsAssertMergedNFA {
	if len(members) == 0 {
		return nil
	}
	nmem := len(members)
	offsets := make([]int, nmem)
	total := 0
	for mi, mem := range members {
		offsets[mi] = total
		total += mem.nfa.npos
	}
	npos := total
	nword := (npos + 63) / 64

	m := &mvsAssertMergedNFA{
		npos:       npos,
		nword:      nword,
		nmem:       nmem,
		memIdx:     make([]int, nmem),
		first:      bsNew(nword),
		follow:     make([][]uint64, npos),
		lastAny:    bsNew(nword),
		lastEnd:    bsNew(nword),
		posPat:     make([]int32, npos),
		condFollow: make([][]guardedBits, npos),
	}
	for p := range m.posPat {
		m.posPat[p] = -1
	}
	for p := range m.follow {
		m.follow[p] = bsNew(nword)
	}

	// 全局字母表 cut points: 取各成员 cuts 的并集.
	cutSet := map[rune]struct{}{0: {}, unicode.MaxRune + 1: {}}
	for _, mem := range members {
		for _, c := range mem.nfa.cuts {
			cutSet[c] = struct{}{}
		}
	}
	cuts := make([]rune, 0, len(cutSet))
	for c := range cutSet {
		if c >= 0 && c <= unicode.MaxRune+1 {
			cuts = append(cuts, c)
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i] < cuts[j] })
	m.cuts = cuts
	m.nsym = len(cuts) - 1
	m.reach = make([][]uint64, m.nsym)
	for s := 0; s < m.nsym; s++ {
		m.reach[s] = bsNew(nword)
	}

	globalSymRange := func(lo, hi rune) (int, int) {
		return symIndex(cuts, lo), symIndex(cuts, hi)
	}

	for mi, mem := range members {
		off := offsets[mi]
		nf := mem.nfa
		m.memIdx[mi] = mem.idx

		forEachSetBit(nf.first, func(q int) { bsSet(m.first, off+q) })
		for _, gb := range nf.condFirst {
			gb2 := guardedBits{g: gb.g, bits: bsNew(nword)}
			forEachSetBit(gb.bits, func(q int) { bsSet(gb2.bits, off+q) })
			m.condFirst = append(m.condFirst, gb2)
		}

		for p := 0; p < nf.npos; p++ {
			gp := off + p
			forEachSetBit(nf.follow[p], func(q int) { bsSet(m.follow[gp], off+q) })
			for _, gb := range nf.condFollow[p] {
				gb2 := guardedBits{g: gb.g, bits: bsNew(nword)}
				forEachSetBit(gb.bits, func(q int) { bsSet(gb2.bits, off+q) })
				m.condFollow[gp] = append(m.condFollow[gp], gb2)
			}
		}

		forEachSetBit(nf.lastAny, func(q int) {
			bsSet(m.lastAny, off+q)
			m.posPat[off+q] = int32(mem.idx)
		})
		forEachSetBit(nf.lastEnd, func(q int) {
			bsSet(m.lastEnd, off+q)
			m.posPat[off+q] = int32(mem.idx)
		})
		for _, gb := range nf.condAccept {
			gb2 := guardedBits{g: gb.g, bits: bsNew(nword)}
			forEachSetBit(gb.bits, func(q int) {
				bsSet(gb2.bits, off+q)
				if m.posPat[off+q] < 0 {
					m.posPat[off+q] = int32(mem.idx)
				}
			})
			m.condAccept = append(m.condAccept, gb2)
		}

		for p := 0; p < nf.npos; p++ {
			ranges := nf.posRanges(p)
			for _, r := range ranges {
				s0, s1 := globalSymRange(r.lo, r.hi)
				for s := s0; s <= s1; s++ {
					bsSet(m.reach[s], off+p)
				}
			}
		}
	}

	for r := rune(0); r < 128; r++ {
		m.asciiSym[r] = int32(symIndex(cuts, r))
	}
	return m
}

// scanExistAssert 单趟扫描 data, 共享边界条件 bound (computeBoundaries 产出), 把命中的成员
// idx (去重, 用 seen 标记) 追加到 out 并返回. 与各成员单独 existsInAssertShared 命中集合等价.
func (m *mvsAssertMergedNFA) scanExistAssert(data []byte, bound []uint8, seen []bool, out []int) []int {
	nword := m.nword
	n := len(data)
	prev := make([]uint64, nword)
	cand := make([]uint64, nword)

	i := 0
	for i < n {
		c := data[i]
		var sym, ni int
		if c < utf8.RuneSelf {
			sym = int(m.asciiSym[c])
			ni = i + 1
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = m.symbolOf(r)
			ni = i + size
		}
		bpre := bound[i]
		bpost := bound[ni]
		atEnd := ni == n

		copy(cand, m.first)
		for _, gb := range m.condFirst {
			if guardHolds(gb.g, bpre) {
				for w := 0; w < nword; w++ {
					cand[w] |= gb.bits[w]
				}
			}
		}
		for w := 0; w < nword; w++ {
			pw := prev[w]
			for pw != 0 {
				p := w*64 + bits.TrailingZeros64(pw)
				pw &= pw - 1
				fp := m.follow[p]
				for k := 0; k < nword; k++ {
					cand[k] |= fp[k]
				}
				for _, gb := range m.condFollow[p] {
					if guardHolds(gb.g, bpre) {
						for k := 0; k < nword; k++ {
							cand[k] |= gb.bits[k]
						}
					}
				}
			}
		}

		rc := m.reach[sym]
		for w := 0; w < nword; w++ {
			v := cand[w] & rc[w]
			prev[w] = v
			if v == 0 {
				continue
			}
			acc := v & m.lastAny[w]
			if atEnd {
				acc |= v & m.lastEnd[w]
			}
			for acc != 0 {
				p := w*64 + bits.TrailingZeros64(acc)
				acc &= acc - 1
				idx := int(m.posPat[p])
				if idx >= 0 && !seen[idx] {
					seen[idx] = true
					out = append(out, idx)
				}
			}
		}
		for _, gb := range m.condAccept {
			if guardHolds(gb.g, bpost) {
				for w := 0; w < nword; w++ {
					acc := prev[w] & gb.bits[w]
					for acc != 0 {
						p := w*64 + bits.TrailingZeros64(acc)
						acc &= acc - 1
						idx := int(m.posPat[p])
						if idx >= 0 && !seen[idx] {
							seen[idx] = true
							out = append(out, idx)
						}
					}
				}
			}
		}

		i = ni
	}
	return out
}

func (m *mvsAssertMergedNFA) symbolOf(r rune) int {
	if r >= 0 && r < 128 {
		return int(m.asciiSym[r])
	}
	if r > unicode.MaxRune {
		r = utf8.RuneError
	}
	return symIndex(m.cuts, r)
}
