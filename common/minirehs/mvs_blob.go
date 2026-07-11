package minirehs

// 本文件把前端 (Go) 编译出的 rune 级 Glushkov 位并行 NFA 序列化为"平台无关只读 blob"
// (小端、显式编码、与机器字节序/对齐无关), 作为 Go 前端与纯 C 运行期内核 (native/mvscan)
// 的解耦契约 (见 MINI_VECTOR_SCAN_IMPL.md 第 7.3 节). C 侧 mvscan_db_open 零依赖地解析它.
//
// 本文件是纯 Go、不带 build tag: 即使不启用 cgo 也能编译/单测 (可对 blob 往返做结构校验),
// 真正消费它的是 cgo 内核 (mvs_cgo.go). per-pattern NFA 与合并 always-on NFA 统一序列化为
// "unit": 用 firstUnanchored/firstAnchored 承载锚点语义, posPat 承载命中位置->成员映射.
//
// 关键词: mvscan, blob, 序列化, 平台无关, 小端, 前后端契约

// mvsBlobMagic / mvsBlobVersion 是 blob 头, C 侧校验. 契约一旦冻结不得随意改 (改则升 version).
var mvsBlobMagic = [4]byte{'M', 'V', 'S', '1'}

const mvsBlobVersion uint32 = 2

// mvsUnit 是 per-pattern NFA 与合并 NFA 的统一可序列化视图.
type mvsUnit struct {
	npos        int
	nword       int
	nsym        int
	hasAnchored bool

	firstUnanchored []uint64 // [nword] 每步注入的起点集 (无锚成员)
	firstAnchored   []uint64 // [nword] 仅输入起点注入 (有锚成员)
	lastAny         []uint64 // [nword] 任意处接受的命中位置集
	lastEnd         []uint64 // [nword] 仅输入末尾接受 ($/\z)
	follow          []uint64 // [npos*nword] 行优先展平
	reach           []uint64 // [nsym*nword] 行优先展平
	cuts            []int32  // [nsym+1] 升序切点
	asciiSym        []int32  // [128]
	posPat          []int32  // [npos] 命中位置->成员 idx (-1 表示非命中位置)

	// 断言扩展 (v2 blob, 仅 hasAssert NFA). 对应 C mvs_nfa assert 字段.
	hasAssert bool
	// LimEx 单字字段
	chainTarget1    uint64
	excMask1        uint64
	excFollow1Flat  []uint64 // [npos]
	condFollowMask1 uint64
	// condFirst (条件起点注入)
	condFirstGuard []uint8
	condFirstBits  []uint64
	// condFollow (条件后继, 扁平三元组)
	condFollowPos   []int32
	condFollowGuard []uint8
	condFollowBits  []uint64
	// condAccept (条件接受)
	condAcceptGuard []uint8
	condAcceptBits  []uint64
}

// unitFromNFA 把一条 per-pattern NFA 转为 unit. reportedIdx 填入命中位置的 posPat
// (存在性执行不使用 posPat, 仅为统一结构/可观测).
func unitFromNFA(nfa *mvsNFA, reportedIdx int) mvsUnit {
	zero := make([]uint64, nfa.nword)
	u := mvsUnit{
		npos:        nfa.npos,
		nword:       nfa.nword,
		nsym:        nfa.nsym,
		hasAnchored: nfa.anchoredStart,
		lastAny:     cloneU64(nfa.lastAny),
		lastEnd:     cloneU64(nfa.lastEnd),
		follow:      flattenRows(nfa.follow, nfa.npos, nfa.nword),
		reach:       flattenRows(nfa.reach, nfa.nsym, nfa.nword),
		cuts:        runesToI32(nfa.cuts),
		asciiSym:    nfa.asciiSym[:],
		posPat:      make([]int32, nfa.npos),
	}
	// 锚点分桶: 无锚成员 first 每步注入; 有锚成员仅起点注入. 与 existsIn 的
	// "if !anchored || atStart: cand |= first" 等价.
	if nfa.anchoredStart {
		u.firstUnanchored = cloneU64(zero)
		u.firstAnchored = cloneU64(nfa.first)
	} else {
		u.firstUnanchored = cloneU64(nfa.first)
		u.firstAnchored = cloneU64(zero)
	}
	for p := range u.posPat {
		u.posPat[p] = -1
	}
	setAcceptPos(u.posPat, nfa.lastAny, int32(reportedIdx))
	setAcceptPos(u.posPat, nfa.lastEnd, int32(reportedIdx))
	return u
}

// unitFromMerged 把合并 always-on NFA 转为 unit.
func unitFromMerged(m *mvsMergedNFA) mvsUnit {
	return mvsUnit{
		npos:            m.npos,
		nword:           m.nword,
		nsym:            m.nsym,
		hasAnchored:     m.hasAnchored,
		firstUnanchored: cloneU64(m.firstUnanchored),
		firstAnchored:   cloneU64(m.firstAnchored),
		lastAny:         cloneU64(m.lastAny),
		lastEnd:         cloneU64(m.lastEnd),
		follow:          flattenRows(m.follow, m.npos, m.nword),
		reach:           flattenRows(m.reach, m.nsym, m.nword),
		cuts:            runesToI32(m.cuts),
		asciiSym:        m.asciiSym[:],
		posPat:          cloneI32(m.posPat),
	}
}

// unitFromAssertNFA 把断言 NFA (hasAssert, nword==1) 转为 unit, 含 LimEx + guard 字段.
func unitFromAssertNFA(nfa *mvsNFA, reportedIdx int) mvsUnit {
	u := unitFromNFA(nfa, reportedIdx)
	u.hasAssert = true
	u.chainTarget1 = nfa.chainTarget1
	u.excMask1 = nfa.excMask1
	u.excFollow1Flat = make([]uint64, nfa.npos)
	copy(u.excFollow1Flat, nfa.excFollow1)
	u.condFollowMask1 = nfa.condFollowMask1
	// condFirst
	for _, gb := range nfa.condFirst {
		u.condFirstGuard = append(u.condFirstGuard, uint8(gb.g))
		u.condFirstBits = append(u.condFirstBits, gb.bits[0])
	}
	// condFollow (扁平三元组)
	for p := 0; p < nfa.npos; p++ {
		for _, gb := range nfa.condFollow[p] {
			u.condFollowPos = append(u.condFollowPos, int32(p))
			u.condFollowGuard = append(u.condFollowGuard, uint8(gb.g))
			u.condFollowBits = append(u.condFollowBits, gb.bits[0])
		}
	}
	// condAccept
	for _, gb := range nfa.condAccept {
		u.condAcceptGuard = append(u.condAcceptGuard, uint8(gb.g))
		u.condAcceptBits = append(u.condAcceptBits, gb.bits[0])
	}
	return u
}

// buildMVSBlob 把整个 db 的 per-pattern NFA (按 idx) + 合并 NFA 序列化为单段 blob.
// v2: 断言 NFA (hasAssert) 也序列化进 blob (C 内核 nfa_run_assert_1 执行).
// nfas[idx]==nil (走 verifier 兜底) 的 slotUnit 记 -1.
func buildMVSBlob(nfas []*mvsNFA, merged *mvsMergedNFA) []byte {
	npat := len(nfas)
	slotUnit := make([]int32, npat)
	var units []mvsUnit
	for idx := 0; idx < npat; idx++ {
		if nfas[idx] == nil {
			slotUnit[idx] = -1
			continue
		}
		if nfas[idx].hasAssert {
			// 断言 NFA: 仅 nword==1 (single) 且 excFollow1 已初始化的才序列化进 C
			if nfas[idx].single {
				slotUnit[idx] = int32(len(units))
				units = append(units, unitFromAssertNFA(nfas[idx], idx))
				continue
			}
			// 多字断言 NFA 仍走 Go (C 暂不支持)
			slotUnit[idx] = -1
			continue
		}
		slotUnit[idx] = int32(len(units))
		units = append(units, unitFromNFA(nfas[idx], idx))
	}
	mergedUnit := int32(-1)
	if merged != nil {
		mergedUnit = int32(len(units))
		units = append(units, unitFromMerged(merged))
	}

	// 先编码各 unit 得长度, 再算偏移.
	unitBytes := make([][]byte, len(units))
	for i := range units {
		unitBytes[i] = encodeUnit(units[i])
	}

	headFixed := 20
	slotBytes := npat * 4
	offBytes := len(units) * 4
	lenBytes := len(units) * 4
	dataStart := headFixed + slotBytes + offBytes + lenBytes

	offsets := make([]uint32, len(units))
	lengths := make([]uint32, len(units))
	cur := dataStart
	for i := range units {
		offsets[i] = uint32(cur)
		lengths[i] = uint32(len(unitBytes[i]))
		cur += len(unitBytes[i])
	}

	b := make([]byte, 0, cur)
	b = append(b, mvsBlobMagic[:]...)
	b = putU32(b, mvsBlobVersion)
	b = putU32(b, uint32(npat))
	b = putI32(b, mergedUnit)
	b = putU32(b, uint32(len(units)))
	for _, s := range slotUnit {
		b = putI32(b, s)
	}
	for _, o := range offsets {
		b = putU32(b, o)
	}
	for _, l := range lengths {
		b = putU32(b, l)
	}
	for i := range unitBytes {
		b = append(b, unitBytes[i]...)
	}
	return b
}

// encodeUnit 按 native/mvscan/mvscan.c parse_unit 约定的布局编码一个 unit.
func encodeUnit(u mvsUnit) []byte {
	flags := uint32(0)
	if u.hasAnchored {
		flags |= 1
	}
	if u.hasAssert {
		flags |= 2 // bit1 = hasAssert
	}
	b := make([]byte, 0, 16+(len(u.follow)+len(u.reach)+u.nword*4)*8+(len(u.cuts)+128+u.npos)*4)
	b = putU32(b, uint32(u.npos))
	b = putU32(b, uint32(u.nword))
	b = putU32(b, uint32(u.nsym))
	b = putU32(b, flags)
	b = putU64s(b, u.firstUnanchored)
	b = putU64s(b, u.firstAnchored)
	b = putU64s(b, u.lastAny)
	b = putU64s(b, u.lastEnd)
	b = putU64s(b, u.follow)
	b = putU64s(b, u.reach)
	b = putI32s(b, u.cuts)
	b = putI32s(b, u.asciiSym)
	b = putI32s(b, u.posPat)
	// 断言扩展 (v2, 仅 hasAssert):
	if u.hasAssert {
		b = putU64(b, u.chainTarget1)
		b = putU64(b, u.excMask1)
		b = putU64s(b, u.excFollow1Flat)
		b = putU64(b, u.condFollowMask1)
		// condFirst
		b = putI32(b, int32(len(u.condFirstGuard)))
		for _, g := range u.condFirstGuard {
			b = append(b, g)
		}
		b = putU64s(b, u.condFirstBits)
		// condFollow
		b = putI32(b, int32(len(u.condFollowPos)))
		b = putI32s(b, u.condFollowPos)
		for _, g := range u.condFollowGuard {
			b = append(b, g)
		}
		b = putU64s(b, u.condFollowBits)
		// condAccept
		b = putI32(b, int32(len(u.condAcceptGuard)))
		for _, g := range u.condAcceptGuard {
			b = append(b, g)
		}
		b = putU64s(b, u.condAcceptBits)
	}
	return b
}

// ---- 小端编码 helper ----

func putU32(b []byte, v uint32) []byte {
	return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func putI32(b []byte, v int32) []byte { return putU32(b, uint32(v)) }

func putU64(b []byte, v uint64) []byte {
	return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

func putU64s(b []byte, vs []uint64) []byte {
	for _, v := range vs {
		b = putU64(b, v)
	}
	return b
}

func putI32s(b []byte, vs []int32) []byte {
	for _, v := range vs {
		b = putI32(b, v)
	}
	return b
}

// ---- 结构辅助 ----

func cloneU64(in []uint64) []uint64 {
	out := make([]uint64, len(in))
	copy(out, in)
	return out
}

func cloneI32(in []int32) []int32 {
	out := make([]int32, len(in))
	copy(out, in)
	return out
}

func runesToI32(in []rune) []int32 {
	out := make([]int32, len(in))
	for i, r := range in {
		out[i] = int32(r)
	}
	return out
}

// flattenRows 把 rows*cols 的二维位集行优先展平; 行缺失/不足时补零, 保证长度恒为 rows*cols.
func flattenRows(rows [][]uint64, nrows, cols int) []uint64 {
	out := make([]uint64, nrows*cols)
	for r := 0; r < nrows && r < len(rows); r++ {
		row := rows[r]
		for c := 0; c < cols && c < len(row); c++ {
			out[r*cols+c] = row[c]
		}
	}
	return out
}

// setAcceptPos 把 bitset bs 中每个置位下标 p 的 posPat[p] 设为 idx.
func setAcceptPos(posPat []int32, bs []uint64, idx int32) {
	forEachSetBit(bs, func(p int) {
		if p >= 0 && p < len(posPat) {
			posPat[p] = idx
		}
	})
}
