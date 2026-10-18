// Frozen parent producer for byte/row differential checks only.
package minirehs

func storageReferenceBlob(nfas []*mvsNFA, merged *mvsMergedNFA, assertNecFactors []necFactor, hasLiterals []bool) []byte {
	npat := len(nfas)
	slotUnit := make([]int32, npat)
	var units []mvsUnit
	for idx := 0; idx < npat; idx++ {
		if nfas[idx] == nil {
			slotUnit[idx] = -1
			continue
		}
		if nfas[idx].hasAssert {
			slotUnit[idx] = int32(len(units))
			units = append(units, unitFromAssertNFA(nfas[idx], idx, getNecFactor(assertNecFactors, idx)))
			continue
		}
		hl := true // 默认有字面量 (保守: 不构建 DFA)
		if hasLiterals != nil && idx < len(hasLiterals) {
			hl = hasLiterals[idx]
		}
		slotUnit[idx] = int32(len(units))
		units = append(units, unitFromNFA(nfas[idx], idx, hl))
	}
	mergedUnit := int32(-1)
	if merged != nil {
		mergedUnit = int32(len(units))
		units = append(units, unitFromMerged(merged))
	}

	// 先编码各 unit 得长度, 再算偏移.
	unitBytes := make([][]byte, len(units))
	for i := range units {
		unitBytes[i] = storageReferenceUnit(units[i])
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
func storageReferenceUnit(u mvsUnit) []byte {
	flags := uint32(0)
	if u.hasAnchored {
		flags |= 1
	}
	if u.hasAssert {
		flags |= 2 // bit1 = hasAssert
	}
	if u.hasDFA {
		flags |= 4 // bit2 = hasDFA
	}
	if u.hasLimEx {
		flags |= 8 // bit3 = hasLimEx (v3)
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
	// LimEx 多字扩展 (v3, flags bit3).
	if u.hasLimEx {
		b = putU64s(b, u.chainTarget)
		b = putU64s(b, u.excMask)
		b = putU64s(b, u.excFollow)
	}
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
		// 必要条件预过滤 (4 个 i32)
		b = putI32(b, u.necMinRunLen)
		b = putI32(b, u.necRunClass)
		b = putI32(b, u.necRequiredByte)
		b = putI32(b, u.necRequiredCount)
	}
	// DFA 扩展 (bit2 = hasDFA, 在所有字段最后)
	if u.hasDFA {
		b = putI32(b, u.dfaNstates)
		b = putI32s(b, u.dfaNext)
		for _, a := range u.dfaAccept {
			b = append(b, a)
		}
	}
	return b
}

func storageReferenceLimEx(m *mvsMergedNFA) *mvsLimEx {
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
