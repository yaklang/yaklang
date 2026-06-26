package minirehs

import (
	"math/bits"
	"sort"
	"unicode"
	"unicode/utf8"
)

// 本文件实现 TODO 第 5 节 P2: 把多条"无字面量、每条记录都要跑"的 always-on NFA 合并成一个
// 单趟扫描的位并行自动机. 当前对每条 always-on NFA 各跑一次 existsIn (K 趟全量扫描); 合并后
// 把 K 个 per-pattern NFA 作为"不相交并集"塞进同一个全局字母表 + 同一个活跃位集, 一趟扫描即可
// 判定这 K 条规则各自是否命中. 命中后再用该规则自身的 findAllLoc 取精确偏移与内容 (定位语义
// 与单独执行完全一致). 不同成员之间无 follow 边, 故彼此独立、互不污染.
//
// 正确性: 合并自动机对成员 m 的命中判定, 与 m 单独 existsIn 的判定完全等价 (位置/follow/first/
// accept 仅做下标平移, reach 用更细的全局字母表, 二者对每个 rune 的接受集相同). 差分测试
// (合并命中集合 == 各自 existsIn == oracle) 为护栏. 混合锚定通过 firstUnanchored / firstAnchored
// 与 lastAny / lastEnd 拆分处理.
//
// 关键词: mvscan, merged NFA, single-pass, always-on, 全局字母表, 位并行

// mergeMember 是参与合并的一条 always-on NFA 成员.
type mergeMember struct {
	idx int // 在 compiledPattern 集合中的下标 (也用作命中标识)
	nfa *mvsNFA
}

// mvsMergedNFA 是若干 per-pattern NFA 的不相交并集, 共享全局字母表与活跃位集.
type mvsMergedNFA struct {
	npos  int
	nword int

	firstUnanchored []uint64   // 无锚成员的 first 并集 (每个 rune 都注入)
	firstAnchored   []uint64   // 有锚成员的 first 并集 (仅输入起点注入)
	follow          [][]uint64 // 全局位置后继 (跨成员无边)
	lastAny         []uint64   // 命中位置并集 (任意处接受)
	lastEnd         []uint64   // requireEnd 成员的命中位置并集 (仅输入末尾接受)
	posPat          []int32    // 全局位置 -> 成员 idx (仅命中位置有效, 其余 -1)

	nsym     int
	asciiSym [128]int32
	cuts     []rune
	reach    [][]uint64

	hasAnchored bool
}

// posRanges 从已压缩的字母表 (cuts/reach) 反推位置 p 接受的 rune 区间集合, 供合并重建全局字母表.
func (nfa *mvsNFA) posRanges(p int) []runeRange {
	var out []runeRange
	w := p >> 6
	bit := uint64(1) << uint(p&63)
	for s := 0; s < nfa.nsym; s++ {
		if nfa.reach[s][w]&bit == 0 {
			continue
		}
		lo := nfa.cuts[s]
		hi := nfa.cuts[s+1] - 1
		if len(out) > 0 && out[len(out)-1].hi+1 == lo {
			out[len(out)-1].hi = hi
		} else {
			out = append(out, runeRange{lo, hi})
		}
	}
	return out
}

// buildMergedNFA 把成员 NFA 合并. 成员数为 0 返回 nil.
func buildMergedNFA(members []mergeMember) *mvsMergedNFA {
	if len(members) == 0 {
		return nil
	}
	// 1) 全局位置空间 = 各成员位置顺序拼接; 同时收集每个全局位置的 rune 区间.
	offsets := make([]int, len(members))
	total := 0
	for mi, mem := range members {
		offsets[mi] = total
		total += mem.nfa.npos
	}
	npos := total
	nword := (npos + 63) / 64

	m := &mvsMergedNFA{
		npos:            npos,
		nword:           nword,
		firstUnanchored: bsNew(nword),
		firstAnchored:   bsNew(nword),
		follow:          make([][]uint64, npos),
		lastAny:         bsNew(nword),
		lastEnd:         bsNew(nword),
		posPat:          make([]int32, npos),
	}
	for p := range m.posPat {
		m.posPat[p] = -1
	}
	for p := range m.follow {
		m.follow[p] = bsNew(nword)
	}

	posClass := make([][]runeRange, npos)

	for mi, mem := range members {
		off := offsets[mi]
		nf := mem.nfa
		for p := 0; p < nf.npos; p++ {
			posClass[off+p] = nf.posRanges(p)
			// follow 平移.
			forEachSetBit(nf.follow[p], func(q int) {
				bsSet(m.follow[off+p], off+q)
			})
		}
		// first 平移 (按成员是否锚定分桶).
		dst := m.firstUnanchored
		if nf.anchoredStart {
			dst = m.firstAnchored
			m.hasAnchored = true
		}
		forEachSetBit(nf.first, func(q int) { bsSet(dst, off+q) })
		// accept 平移 + 位置->成员映射.
		forEachSetBit(nf.lastAny, func(q int) {
			bsSet(m.lastAny, off+q)
			m.posPat[off+q] = int32(mem.idx)
		})
		forEachSetBit(nf.lastEnd, func(q int) {
			bsSet(m.lastEnd, off+q)
			m.posPat[off+q] = int32(mem.idx)
		})
	}

	m.buildAlphabet(posClass)
	return m
}

// buildAlphabet 与 mvsNFA.buildAlphabet 同构: 用所有位置类边界切分码点空间为符号, 建 reach.
func (m *mvsMergedNFA) buildAlphabet(posClass [][]runeRange) {
	cutSet := map[rune]struct{}{0: {}, unicode.MaxRune + 1: {}}
	for _, cls := range posClass {
		for _, r := range cls {
			cutSet[r.lo] = struct{}{}
			cutSet[r.hi+1] = struct{}{}
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
		m.reach[s] = bsNew(m.nword)
	}
	for p := 0; p < len(posClass); p++ {
		for _, r := range posClass[p] {
			s0 := symIndex(cuts, r.lo)
			s1 := symIndex(cuts, r.hi)
			for s := s0; s <= s1; s++ {
				bsSet(m.reach[s], p)
			}
		}
	}
	for r := rune(0); r < 128; r++ {
		m.asciiSym[r] = int32(symIndex(cuts, r))
	}
}

func (m *mvsMergedNFA) symbolOf(r rune) int {
	if r >= 0 && r < 128 {
		return int(m.asciiSym[r])
	}
	if r > unicode.MaxRune {
		r = utf8.RuneError
	}
	return symIndex(m.cuts, r)
}

// scanExist 单趟扫描 data, 把命中的成员 idx (去重, 用 seen 标记) 追加到 out 并返回.
// seen 由调用方提供且尺寸 >= 最大 idx+1; 命中即置位, 用于去重 (复用 scratch.fullDone).
func (m *mvsMergedNFA) scanExist(data []byte, seen []bool, out []int) []int {
	nword := m.nword
	prev := make([]uint64, nword)
	cand := make([]uint64, nword)
	n := len(data)

	i := 0
	for i < n {
		atStart := i == 0
		r, size := utf8.DecodeRune(data[i:])
		i += size
		sym := m.symbolOf(r)

		copy(cand, m.firstUnanchored)
		if atStart && m.hasAnchored {
			for w := 0; w < nword; w++ {
				cand[w] |= m.firstAnchored[w]
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
			}
		}

		rc := m.reach[sym]
		atEnd := i == n
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
	}
	return out
}

// forEachSetBit 对 bitset 中每个置位下标调用 fn.
func forEachSetBit(bs []uint64, fn func(i int)) {
	for w := 0; w < len(bs); w++ {
		x := bs[w]
		for x != 0 {
			fn(w*64 + bits.TrailingZeros64(x))
			x &= x - 1
		}
	}
}
