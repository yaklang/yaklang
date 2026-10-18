package minirehs

import (
	"math/bits"
)

// 本文件实现 always-on NFA 的 DFA 转换: 对小规模 NFA (npos <= DFA_MAX_NPOS)
// 构建确定性有限自动机 (子集构造法), 序列化为紧凑转移表, 在 C 内核中用单字查表
// 替代逐字节位递推. DFA 每字节仅一次 next[state*256+byte] 查表, 无位运算.
//
// 对 npos=3 (JSON) 或 npos=8 (Windows) 的小 NFA, DFA 仅需 5-20 个状态,
// 转移表 5*256=1280 ~ 20*256=5120 字节, 远小于 NFA 的 reach/follow 表.
// 每字节成本: DFA = 1 次数组查 (O(1)); NFA = O(npos/nword) 位运算.
//
// 关键词: DFA, subset construction, determinization, table lookup, always-on

// DFA_MAX_NPOS 限制 DFA 转换的最大 NFA 位置数 (避免状态爆炸).
const DFA_MAX_NPOS = 64

type mvsDFA struct {
	nstates int     // DFA 状态数
	accept  []byte  // [nstates] 接受状态标记 (0/1)
	next    []int32 // [nstates * 256] 转移表: next[state*256 + byte] (-1 = 死状态)
}

// buildDFAFromNFA 用子集构造法把 NFA 转为 DFA. 仅对 npos <= DFA_MAX_NPOS 的 NFA 调用.
// 返回 nil 表示转换失败 (状态爆炸).
func buildDFAFromNFA(nfa *mvsNFA) *mvsDFA {
	if nfa.npos > DFA_MAX_NPOS || nfa.npos == 0 || nfa.nword != 1 ||
		nfa.hasAssert || nfa.anchoredStart || nfa.requireEnd {
		return nil
	}

	// DFA 状态 = NFA 位置集 (用 uint64 位集表示, nword 个字)
	nword := nfa.nword

	// Each state contains positions consumed by the preceding input, not the
	// positions eligible to consume the next symbol. Initially none were consumed;
	// computeSuccessorSet injects first before consuming every unanchored symbol.
	initSet := make([]uint64, nword)

	// DFA 状态表: 位集 -> 状态 ID
	stateMap := map[string]int{}
	var stateSets [][]uint64

	// 添加初始状态
	key0 := bitsKey(initSet)
	stateMap[key0] = 0
	stateSets = append(stateSets, initSet)

	// BFS 构造
	maxStates := 256 // 限制状态数避免爆炸

	// State IDs are assigned in discovery order. Scanning those IDs is the same
	// BFS without a second queue or an unused parallel string-key slice.
	for sid := 0; sid < len(stateSets) && len(stateSets) < maxStates; sid++ {
		curSet := stateSets[sid]

		// 对每个 ASCII 字节计算后继状态集
		for b := 0; b < 128; b++ {
			sym := int(nfa.asciiSym[byte(b)])
			nextSet := computeSuccessorSet(nfa, curSet, sym, nword)
			if isZero(nextSet) {
				continue // 死转移
			}
			key := bitsKey(nextSet)
			nextID, ok := stateMap[key]
			if !ok {
				nextID = len(stateSets)
				if nextID >= maxStates {
					return nil // 状态爆炸
				}
				stateMap[key] = nextID
				stateSets = append(stateSets, nextSet)
			}
			_ = nextID
		}
	}

	nstates := len(stateSets)
	if nstates > maxStates-1 {
		return nil
	}

	// 构建转移表: next[state*256 + byte] -> 下一状态
	dfa := &mvsDFA{
		nstates: nstates,
		accept:  make([]byte, nstates),
		next:    make([]int32, nstates*256),
	}
	// 初始化转移表为 -1 (死状态)
	for i := range dfa.next {
		dfa.next[i] = -1
	}

	// 填充转移表
	for sid := 0; sid < nstates; sid++ {
		curSet := stateSets[sid]
		for b := 0; b < 128; b++ {
			sym := int(nfa.asciiSym[byte(b)])
			nextSet := computeSuccessorSet(nfa, curSet, sym, nword)
			if isZero(nextSet) {
				continue
			}
			key := bitsKey(nextSet)
			nextID := stateMap[key]
			dfa.next[sid*256+b] = int32(nextID)
		}
		// 非 ASCII 字节: 暂不处理 (保持 -1)
	}

	// 标记接受状态
	for sid := 0; sid < nstates; sid++ {
		curSet := stateSets[sid]
		for w := 0; w < nword; w++ {
			if curSet[w]&nfa.lastAny[w] != 0 {
				dfa.accept[sid] = 1
				break
			}
			if nfa.requireEnd && curSet[w]&nfa.lastEnd[w] != 0 {
				dfa.accept[sid] = 1
				break
			}
		}
	}

	return dfa
}

// computeSuccessorSet 计算 NFA 从状态集 curSet 消费符号 sym 后的后继状态集.
func computeSuccessorSet(nfa *mvsNFA, curSet []uint64, sym, nword int) []uint64 {
	cand := make([]uint64, nword)
	// 无锚 NFA: 每步注入 first (模拟 unanchored 扫描)
	if !nfa.anchoredStart {
		for w := 0; w < nword; w++ {
			cand[w] = nfa.first[w]
		}
	}
	for w := 0; w < nword; w++ {
		pw := curSet[w]
		for pw != 0 {
			p := w*64 + bits.TrailingZeros64(pw)
			pw &= pw - 1
			for k := 0; k < nword; k++ {
				cand[k] |= nfa.follow[p][k]
			}
		}
	}
	// active = cand & reach[sym]
	rc := nfa.reach[sym]
	active := make([]uint64, nword)
	for w := 0; w < nword; w++ {
		active[w] = cand[w] & rc[w]
	}
	return active
}

func bitsKey(bits []uint64) string {
	// 简单: 把 bits 转为字符串键
	b := make([]byte, len(bits)*8)
	for i, v := range bits {
		for j := 0; j < 8; j++ {
			b[i*8+j] = byte(v >> (uint(j) * 8))
		}
	}
	return string(b)
}

func isZero(bits []uint64) bool {
	for _, v := range bits {
		if v != 0 {
			return false
		}
	}
	return true
}
