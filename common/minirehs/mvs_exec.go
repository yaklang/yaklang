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
	if nfa.hasAssert {
		return nfa.existsInAssert(data)
	}
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
		c := data[i]
		var sym int
		if c < utf8.RuneSelf {
			sym = int(nfa.asciiSym[c])
			i++
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = nfa.symbolOf(r)
			i += size
		}

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

// findAllLoc 在 data 上枚举本 nfa 的所有非重叠匹配, 每个匹配以精确字节区间 [from,to) 回调
// emit. 语义为 leftmost-longest (POSIX, 等价 regexp.Longest().FindAllIndex): 取最靠左的
// 起点, 同起点取最长终点; 一个匹配确定后从其终点继续找下一个 (非重叠). emit 返回 false 即停止.
//
// 与 existsIn 的区别: existsIn 只判定"是否命中"(布尔, 可提前停); findAllLoc 还要算出"命中在
// 哪里、内容是什么"(data[from:to]), 用于上报匹配内容与定位. 它由 NFA 自身完成定位, 不依赖
// stdlib regexp; 正确性由差分测试逐字节对照 regexp.Longest().FindAllIndex 保证.
//
// 关键词: mvscan, match location, leftmost-longest, 匹配定位, 匹配内容
func (nfa *mvsNFA) findAllLoc(data []byte, sc *scratch, emit func(from, to int) bool) {
	pos := 0
	n := len(data)
	for pos <= n {
		from, to, ok := nfa.findLocFrom(data, pos, sc)
		if !ok {
			return
		}
		if !emit(from, to) {
			return
		}
		if to > pos {
			pos = to
		} else {
			// 防御: 本核拒绝可空根, 不应出现空匹配; 仍兜底推进避免死循环.
			pos++
		}
	}
}

// findLocFrom 从 data[searchFrom:] 起, 寻找最靠左 (同起点最长) 的一个匹配, 返回其在 data 中的
// 绝对字节区间 [from,to) 与是否找到. 锚点按 data 的绝对坐标处理 (^ 仅在绝对偏移 0 注入起点,
// $ 仅在 data 末尾接受), 因此对子区间续扫 (findAllLoc 的非重叠推进) 仍语义正确.
//
// 位并行递推在 existsIn 基础上为每个活跃 position 维护一个"起点字节偏移" startOf[p]:
//   - 后继继承前驱起点; 同一 position 被多路汇聚时保留最小起点 (leftmost 优先).
//   - 无锚时每步在当前 rune 处注入 first, 其起点为当前 runeStart.
//   - 命中 (active & accept != 0) 时, 以命中 position 的最小起点与当前终点更新 best;
//     起点更小则替换, 起点相同则取更大终点 (longest).
//
// 当所有"起点 <= best 起点"的活跃线程都消亡时即可停止 (后续注入起点只会更大, 不可能更优).
func (nfa *mvsNFA) findLocFrom(data []byte, searchFrom int, sc *scratch) (int, int, bool) {
	if searchFrom < 0 {
		searchFrom = 0
	}
	if nfa.anchoredStart && searchFrom > 0 {
		// ^ 锚定: 匹配只能始于绝对偏移 0, 续扫 (searchFrom>0) 必不命中.
		return 0, 0, false
	}
	const inf = int(^uint(0) >> 1)
	nword := nfa.nword
	npos := nfa.npos
	// 位并行状态缓冲: 有 scratch 则复用 (零分配热路径), 否则就地分配 (测试/无 sc 调用兜底).
	// 各缓冲均"写后读"语义 (prevActive 每步全写; cand 每步起始清零; candStart/prevStart 仅对
	// 已置位 position 写后读), 故复用无需逐次清零.
	var prevActive, cand []uint64
	var candStart, prevStart []int
	if sc != nil {
		sc.locPrev = ensureU64Len(sc.locPrev, nword)
		sc.locCand = ensureU64Len(sc.locCand, nword)
		sc.locCandStart = ensureIntLen(sc.locCandStart, npos)
		sc.locPrevStart = ensureIntLen(sc.locPrevStart, npos)
		prevActive, cand = sc.locPrev, sc.locCand
		candStart, prevStart = sc.locCandStart, sc.locPrevStart
	} else {
		prevActive = make([]uint64, nword)
		cand = make([]uint64, nword)
		candStart = make([]int, npos)
		prevStart = make([]int, npos)
	}
	anchored := nfa.anchoredStart
	requireEnd := nfa.requireEnd
	n := len(data)

	bestStart, bestEnd := -1, -1
	hasPrev := false

	i := searchFrom
	for i < n {
		runeStart := i
		c := data[i]
		var sym int
		if c < utf8.RuneSelf {
			sym = int(nfa.asciiSym[c])
			i++
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = nfa.symbolOf(r)
			i += size
		}

		for w := range cand {
			cand[w] = 0
		}

		// 后继并集: 继承前驱起点, 汇聚取最小.
		if hasPrev {
			for w := 0; w < nword; w++ {
				pw := prevActive[w]
				for pw != 0 {
					p := w*64 + bits.TrailingZeros64(pw)
					pw &= pw - 1
					sp := prevStart[p]
					fp := nfa.follow[p]
					for fw := 0; fw < nword; fw++ {
						fb := fp[fw]
						for fb != 0 {
							q := fw*64 + bits.TrailingZeros64(fb)
							fb &= fb - 1
							bit := uint64(1) << uint(q&63)
							if cand[fw]&bit == 0 {
								cand[fw] |= bit
								candStart[q] = sp
							} else if sp < candStart[q] {
								candStart[q] = sp
							}
						}
					}
				}
			}
		}

		// 注入起点 (无锚每步; 有锚仅绝对偏移 0).
		if !anchored || runeStart == 0 {
			for w := 0; w < nword; w++ {
				fb := nfa.first[w]
				for fb != 0 {
					q := w*64 + bits.TrailingZeros64(fb)
					fb &= fb - 1
					bit := uint64(1) << uint(q&63)
					if cand[w]&bit == 0 {
						cand[w] |= bit
						candStart[q] = runeStart
					} else if runeStart < candStart[q] {
						candStart[q] = runeStart
					}
				}
			}
		}

		rc := nfa.reach[sym]
		anyActive := false
		minActiveStart := inf
		minAcc := inf
		for w := 0; w < nword; w++ {
			v := cand[w] & rc[w]
			prevActive[w] = v
			if v == 0 {
				continue
			}
			anyActive = true
			var acc uint64
			acc = v & nfa.lastAny[w]
			if requireEnd && i == n {
				acc |= v & nfa.lastEnd[w]
			}
			vv := v
			for vv != 0 {
				q := w*64 + bits.TrailingZeros64(vv)
				vv &= vv - 1
				s := candStart[q]
				prevStart[q] = s
				if s < minActiveStart {
					minActiveStart = s
				}
			}
			for acc != 0 {
				q := w*64 + bits.TrailingZeros64(acc)
				acc &= acc - 1
				if candStart[q] < minAcc {
					minAcc = candStart[q]
				}
			}
		}
		hasPrev = anyActive

		if minAcc != inf {
			end := i
			if bestEnd < 0 || minAcc < bestStart || (minAcc == bestStart && end > bestEnd) {
				bestStart = minAcc
				bestEnd = end
			}
		}

		if !anyActive {
			if anchored {
				break // 有锚且活跃集空: 不会再有新起点.
			}
			if bestEnd >= 0 {
				break // 已得匹配且无活跃线程, 后续起点更大不可能更优.
			}
			continue
		}
		if bestEnd >= 0 && minActiveStart > bestStart {
			break // 所有"起点<=best 起点"的线程均消亡, best 已是 leftmost-longest.
		}
	}

	if bestEnd < 0 {
		return 0, 0, false
	}
	return bestStart, bestEnd, true
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
		// ASCII 快路径: 单字节 rune 直接查 asciiSym, 省去 utf8.DecodeRune + symbolOf 调用开销
		// (语料绝大多数字节为 ASCII; 非 ASCII 才回退完整解码 + 切点二分).
		c := data[i]
		var sym int
		if c < utf8.RuneSelf {
			sym = int(nfa.asciiSym[c])
			i++
		} else {
			r, size := utf8.DecodeRune(data[i:])
			sym = nfa.symbolOf(r)
			i += size
		}

		var cand uint64
		if !anchored || atStart {
			cand = first
		}
		for pw := prev; pw != 0; pw &= pw - 1 {
			cand |= follow[bits.TrailingZeros64(pw)]
		}
		active := cand & reach[sym]
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

// ensureU64Len 返回长度恰为 n 的 []uint64: cap 足够则复用底层数组 (b[:n]), 否则新分配.
// 供定位热路径复用 scratch 缓冲, 避免每次命中重新分配.
func ensureU64Len(b []uint64, n int) []uint64 {
	if cap(b) >= n {
		return b[:n]
	}
	return make([]uint64, n)
}

// ensureIntLen 返回长度恰为 n 的 []int: cap 足够则复用底层数组, 否则新分配.
func ensureIntLen(b []int, n int) []int {
	if cap(b) >= n {
		return b[:n]
	}
	return make([]int, n)
}
