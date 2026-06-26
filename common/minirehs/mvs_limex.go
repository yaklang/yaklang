package minirehs

import (
	"math/bits"
	"unicode/utf8"
)

// 本文件实现 LimEx 式位并行递推 (Phase 3 核心去风险, 纯 Go 参考). 朴素合并自动机 (mvsMergedNFA)
// 每字节代价 = 活跃位置数 × nword (逐个活跃位置 OR 其 follow); 当合并规模大 (npos 数千, nword 数十)
// 时反而比"字面量门控 + 逐模式 existsIn"更慢 (实测 1.26 MB/s)。
//
// LimEx 思路 (源自 Hyperscan): 把 Glushkov follow 边拆成两类——
//   - "链边" p -> p+1: 用整条状态向量一次左移 (prev << 1) & chainTarget 同时推进, O(nword), 与活跃数无关;
//   - "异常边" (follow[p] 去掉 p+1 后剩余的目标): 仅对活跃的异常位置逐个 OR, O(活跃异常 × nword).
// 因 Glushkov 对"连接"产生的恰是 p->p+1 链 (字面量/序列), 大量 follow 落入链边, 异常稀疏, 故每字节
// 趋近 O(nword), 不随活跃位置线性增长。这是把"全并单趟"做快、最终逼近 SIMD 的前提。
//
// 正确性: LimEx 递推与 mvsMergedNFA.scanExist 对同一成员集逐报文命中集合完全一致 (差分护栏
// TestMVSLimExVsMerged*)。不做位置重排时, chainTarget 只捕获"恰好 p->p+1"的边, 其余全归异常——
// 仍 100% 正确, 只是异常更多 (性能不极致); 重排是后续纯性能优化, 不影响正确性。
//
// 关键词: mvscan, LimEx, bit-parallel, shift, exception, 单趟全并, Glushkov

// mvsLimEx 在 mvsMergedNFA 之上叠加 LimEx 递推所需的链/异常拆分, 复用其字母表 / reach / first /
// last / posPat (不复制, 仅引用)。
type mvsLimEx struct {
	m *mvsMergedNFA

	chainTarget []uint64   // 位 q 置位 <=> 存在链边 (q-1) -> q, 即 follow[q-1] 含 q
	excMask     []uint64   // 位 p 置位 <=> 位置 p 有异常边 (excFollow[p] != 0)
	excFollow   [][]uint64 // 位置 p 的异常后继 (= follow[p] 去掉链边目标 p+1); 仅 excMask[p] 为真时有效

	excCount int // 异常位置总数 (去风险指标: 越少越接近 O(nword)/字节)
}

// buildLimEx 从已构建的合并自动机派生 LimEx 形式。无位置重排 (正确性与重排无关)。
func buildLimEx(m *mvsMergedNFA) *mvsLimEx {
	if m == nil {
		return nil
	}
	nword := m.nword
	le := &mvsLimEx{
		m:           m,
		chainTarget: bsNew(nword),
		excMask:     bsNew(nword),
		excFollow:   make([][]uint64, m.npos),
	}
	for p := 0; p < m.npos; p++ {
		f := m.follow[p]
		hasChain := p+1 < m.npos && bsTest(f, p+1)
		if hasChain {
			bsSet(le.chainTarget, p+1)
		}
		exc := make([]uint64, nword)
		copy(exc, f)
		if hasChain {
			bsClear(exc, p+1)
		}
		if !bsIsZero(exc) {
			le.excFollow[p] = exc
			bsSet(le.excMask, p)
			le.excCount++
		}
	}
	return le
}

// scanExist 单趟 LimEx 递推扫描 data, 命中成员 idx (去重) 追加到 out。语义同 mvsMergedNFA.scanExist。
func (le *mvsLimEx) scanExist(data []byte, seen []bool, out []int) []int {
	m := le.m
	nword := m.nword
	prev := make([]uint64, nword)
	cand := make([]uint64, nword)
	shifted := make([]uint64, nword)
	n := len(data)

	i := 0
	for i < n {
		atStart := i == 0
		r, size := utf8.DecodeRune(data[i:])
		i += size
		sym := m.symbolOf(r)

		// 链边: 整条状态向量左移 1 位, 再用 chainTarget 屏蔽掉"无链边"的伪进位。
		shiftLeft1(shifted, prev)
		for w := 0; w < nword; w++ {
			cand[w] = (shifted[w] & le.chainTarget[w]) | m.firstUnanchored[w]
		}
		if atStart && m.hasAnchored {
			for w := 0; w < nword; w++ {
				cand[w] |= m.firstAnchored[w]
			}
		}

		// 异常边: 仅对活跃的异常位置逐个并入其异常后继。
		for w := 0; w < nword; w++ {
			ex := prev[w] & le.excMask[w]
			for ex != 0 {
				p := w*64 + bits.TrailingZeros64(ex)
				ex &= ex - 1
				ef := le.excFollow[p]
				for k := 0; k < nword; k++ {
					cand[k] |= ef[k]
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

// shiftLeft1 把 src 视为 nword*64 位的大整数左移 1 位写入 dst (位 p -> p+1, 跨字进位)。
func shiftLeft1(dst, src []uint64) {
	var carry uint64
	for w := 0; w < len(src); w++ {
		v := src[w]
		dst[w] = (v << 1) | carry
		carry = v >> 63
	}
}

func bsTest(bs []uint64, i int) bool { return bs[i>>6]&(1<<uint(i&63)) != 0 }

func bsClear(bs []uint64, i int) { bs[i>>6] &^= 1 << uint(i&63) }

func bsIsZero(bs []uint64) bool {
	for _, w := range bs {
		if w != 0 {
			return false
		}
	}
	return true
}
