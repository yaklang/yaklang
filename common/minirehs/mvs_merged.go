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

	// R1-Anchor: span-injected merged NFA 字段. 当 isAnchoredMerge=true 时, firstUnanchored
	// 为零 (不每步注入), 改由 scanExistAnchored 按 per-member span 注入 firstPerMember[mi].
	isAnchoredMerge bool       // 是否为 R1 锚定式合并 (span-gated first 注入)
	nmem            int        // 成员数 (R1)
	offsets         []int      // [nmem] 各成员的全局位置偏移 (R1, 诊断用)
	firstPerMember  [][]uint64 // [nmem][nword] 各成员的 first (已按 offset 平移) (R1)
	memIdx          []int      // [nmem] -> compiledPattern idx (R1, == mergeMember.idx)
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

// buildMergedAnchoredNFA 构造 R1-Anchor 锚定式合并自动机: 与 buildMergedNFA 同构, 但
// unanchored 成员的 first 不 OR 进 firstUnanchored (保持零), 而是存入 firstPerMember[mi]
// (已按 offset 平移). scanExistAnchored 在扫描时按 per-member span 注入对应 first.
// anchoredStart 成员不应进入 R1 合并 (调用方须排除). 成员数为 0 返回 nil.
func buildMergedAnchoredNFA(members []mergeMember) *mvsMergedNFA {
	if len(members) == 0 {
		return nil
	}
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
		firstUnanchored: bsNew(nword), // R1: 保持零 (不每步注入)
		firstAnchored:   bsNew(nword), // R1: 不应有 anchored 成员, 保持零
		follow:          make([][]uint64, npos),
		lastAny:         bsNew(nword),
		lastEnd:         bsNew(nword),
		posPat:          make([]int32, npos),
		isAnchoredMerge: true,
		nmem:            len(members),
		offsets:         offsets,
		firstPerMember:  make([][]uint64, len(members)),
		memIdx:          make([]int, len(members)),
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
		m.memIdx[mi] = mem.idx
		// R1: per-member first (平移), 不进 firstUnanchored.
		fm := bsNew(nword)
		forEachSetBit(nf.first, func(q int) { bsSet(fm, off+q) })
		m.firstPerMember[mi] = fm
		// follow 平移.
		for p := 0; p < nf.npos; p++ {
			posClass[off+p] = nf.posRanges(p)
			forEachSetBit(nf.follow[p], func(q int) {
				bsSet(m.follow[off+p], off+q)
			})
		}
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

// scanExistAnchored 是 R1-Anchor 的 span-injected 单趟扫描: 每个 rune 处, 仅注入
// 当前 runeStart 落入其 span 的成员的 firstPerMember[mi], 其余成员不注入 (提前消亡).
// spansPerMember[mi] 是成员 mi 的已排序合并注入区间 (可为空 = 本报文无字面量命中, 不注入).
// 命中成员 idx (去重, 用 seen) 追加到 out. 与各成员单独 existsInAnchored 命中集合等价 (差分护栏).
//
// 正确性: 任一匹配 M 必含某必需字面量 (命中于 h.end), M.start >= h.end - head_L 落入该成员
// 的某 span => merged scan 必能从该起点注入并经 follow 传播到 accept => 无假阴. span 外不注入
// => 只会找到真实起点的匹配 => 无假阳. 不同成员 follow 无跨边, 彼此独立.
func (m *mvsMergedNFA) scanExistAnchored(data []byte, spansPerMember [][]anchorSpan, seen []bool, out []int) []int {
	nword := m.nword
	nmem := m.nmem
	prev := make([]uint64, nword)
	cand := make([]uint64, nword)
	n := len(data)

	// per-member span 游标: si[mi] 单调推进, curLo/curHi 缓存当前 span (避免每 rune 数组索引).
	si := make([]int, nmem)
	curLo := make([]int, nmem)
	curHi := make([]int, nmem)
	hasSpan := make([]bool, nmem)
	for mi := 0; mi < nmem; mi++ {
		spans := spansPerMember[mi]
		if len(spans) > 0 {
			hasSpan[mi] = true
			curLo[mi] = int(spans[0].lo)
			curHi[mi] = int(spans[0].hi)
		}
	}
	// lastHi[mi] = 最后一个 span 的 hi, 用于提前消亡判定.
	lastHi := make([]int, nmem)
	for mi := 0; mi < nmem; mi++ {
		spans := spansPerMember[mi]
		if len(spans) > 0 {
			lastHi[mi] = int(spans[len(spans)-1].hi)
		}
	}

	// 起始位置: 各成员 span 的最小 lo, rune 对齐.
	minLo := n
	for mi := 0; mi < nmem; mi++ {
		if hasSpan[mi] && curLo[mi] < minLo {
			minLo = curLo[mi]
		}
	}
	if minLo < 0 {
		minLo = 0
	}

	i := alignRuneStart(data, minLo)
	if i > n {
		i = n
	}
	for i < n {
		runeStart := i
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

		// cand 清零 + per-member span-gated first 注入.
		for w := 0; w < nword; w++ {
			cand[w] = 0
		}
		for mi := 0; mi < nmem; mi++ {
			if !hasSpan[mi] {
				continue
			}
			// 推进游标: 跳过已过去的 span.
			for runeStart >= curHi[mi] {
				si[mi]++
				if si[mi] >= len(spansPerMember[mi]) {
					hasSpan[mi] = false // 该成员 span 耗尽
					break
				}
				spans := spansPerMember[mi]
				curLo[mi] = int(spans[si[mi]].lo)
				curHi[mi] = int(spans[si[mi]].hi)
			}
			if hasSpan[mi] && runeStart >= curLo[mi] {
				fm := m.firstPerMember[mi]
				for w := 0; w < nword; w++ {
					cand[w] |= fm[w]
				}
			}
		}

		// follow 展开 (与 scanExist 同).
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
		atEnd := ni == n
		anyActive := false
		for w := 0; w < nword; w++ {
			v := cand[w] & rc[w]
			prev[w] = v
			if v == 0 {
				continue
			}
			anyActive = true
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

		// 提前消亡: 活跃集空且所有成员 span 耗尽 => 不可能再命中.
		if !anyActive {
			allDone := true
			nextLo := n
			for mi := 0; mi < nmem; mi++ {
				if !hasSpan[mi] {
					continue
				}
				if lastHi[mi] > runeStart {
					allDone = false
				}
				// 推导下一次可能注入的位置，但不修改 cursor（下一轮仍由原有
				// 推进逻辑统一处理）。若 ni 仍落在当前 span 内，不能跳过。
				s := si[mi]
				spans := spansPerMember[mi]
				for s < len(spans) && int(spans[s].hi) <= ni {
					s++
				}
				if s < len(spans) {
					if int(spans[s].lo) < ni {
						nextLo = ni // 当前/下一 rune 仍可注入，禁止 jump
						break
					}
					if lo := int(spans[s].lo); lo < nextLo {
						nextLo = lo
					}
				}
			}
			if allDone {
				break
			}
			// 与单条 anchored verifier 同一原则：活跃集已空时，跨度之间没有
			// 状态依赖；直接跳到所有成员下一次注入的最早位置，避免扫描空洞。
			if nextLo > ni+gapJumpMin {
				jump := alignRuneStart(data, nextLo)
				if jump > i {
					i = jump
					continue
				}
			}
		}
		i = ni
	}
	return out
}
func forEachSetBit(bs []uint64, fn func(i int)) {
	for w := 0; w < len(bs); w++ {
		x := bs[w]
		for x != 0 {
			fn(w*64 + bits.TrailingZeros64(x))
			x &= x - 1
		}
	}
}
