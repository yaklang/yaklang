package minirehs

import (
	"bytes"
	"unicode/utf8"
)

// decodeImageUnit validates the complete unit before it can reach C. The native
// blob was originally an internal compiler interface; its offsets, symbol IDs,
// position bits and DFA transitions must not be trusted at the file boundary.
func decodeImageUnit(data []byte, npat int) (mvsUnit, error) {
	r := &imageReader{b: data}
	u := mvsUnit{npos: int(r.u32()), nword: int(r.u32()), nsym: int(r.u32())}
	flags := r.u32()
	if u.npos <= 0 || u.npos > 0x7fffffbf || u.nword != (u.npos+63)/64 ||
		u.nsym <= 0 || u.nsym > utf8.MaxRune+1 || flags & ^uint32(15) != 0 {
		return u, ErrCorruptImage
	}
	need := (4+uint64(u.npos)+uint64(u.nsym))*uint64(u.nword)*8 + (uint64(u.nsym)+129+uint64(u.npos))*4
	if need > uint64(len(data)-r.off) {
		return u, ErrCorruptImage
	}
	u.hasAnchored, u.hasAssert, u.hasDFA, u.hasLimEx = flags&1 != 0, flags&2 != 0, flags&4 != 0, flags&8 != 0
	u.firstUnanchored = r.words(u.nword)
	u.firstAnchored = r.words(u.nword)
	u.lastAny = r.words(u.nword)
	u.lastEnd = r.words(u.nword)
	u.follow = r.words(u.npos * u.nword)
	u.reach = r.words(u.nsym * u.nword)
	u.cuts = r.ints(u.nsym + 1)
	u.asciiSym = r.ints(128)
	u.posPat = r.ints(u.npos)
	if u.hasLimEx {
		u.chainTarget, u.excMask = r.words(u.nword), r.words(u.nword)
		u.excFollow = r.words(u.npos * u.nword)
	}
	if u.hasAssert {
		u.chainTarget1, u.excMask1 = r.u64(), r.u64()
		u.excFollow1Flat = r.words(u.npos)
		u.condFollowMask1 = r.u64()
		n := r.count(1 + 8*u.nword)
		u.condFirstGuard = r.take(n)
		u.condFirstBits = r.words(n * u.nword)
		n = r.count(5 + 8*u.nword)
		u.condFollowPos = r.ints(n)
		u.condFollowGuard = r.take(n)
		u.condFollowBits = r.words(n * u.nword)
		n = r.count(1 + 8*u.nword)
		u.condAcceptGuard = r.take(n)
		u.condAcceptBits = r.words(n * u.nword)
		u.necMinRunLen, u.necRunClass = r.i32(), r.i32()
		u.necRequiredByte, u.necRequiredCount = r.i32(), r.i32()
		if u.necMinRunLen < 0 || u.necRunClass < 0 || u.necRunClass > 2 ||
			u.necRequiredByte < -1 || u.necRequiredByte > 127 || u.necRequiredCount < 0 {
			return u, ErrCorruptImage
		}
		var conditionalSources1 uint64
		for _, p := range u.condFollowPos {
			if p < 0 || int(p) >= u.npos {
				return u, ErrCorruptImage
			}
			if u.nword == 1 {
				conditionalSources1 |= uint64(1) << uint(p)
			}
		}
		if u.nword == 1 && u.condFollowMask1 != conditionalSources1 {
			return u, ErrCorruptImage
		}
		for _, gs := range [][]byte{u.condFirstGuard, u.condFollowGuard, u.condAcceptGuard} {
			for _, g := range gs {
				if g > 63 {
					return u, ErrCorruptImage
				}
			}
		}
	}
	if u.hasDFA {
		u.dfaNstates = r.i32()
		if u.dfaNstates <= 0 || u.dfaNstates > 512 {
			return u, ErrCorruptImage
		}
		u.dfaNext = r.ints(int(u.dfaNstates) * 256)
		u.dfaAccept = r.take(int(u.dfaNstates))
		for _, s := range u.dfaNext {
			if s < -1 || s >= u.dfaNstates {
				return u, ErrCorruptImage
			}
		}
		for _, a := range u.dfaAccept {
			if a > 1 {
				return u, ErrCorruptImage
			}
		}
	}
	if !r.done() {
		return u, ErrCorruptImage
	}
	if u.cuts[0] != 0 || u.cuts[u.nsym] != utf8.MaxRune+1 {
		return u, ErrCorruptImage
	}
	for i := 1; i < len(u.cuts); i++ {
		if u.cuts[i] <= u.cuts[i-1] {
			return u, ErrCorruptImage
		}
	}
	for b, s := range u.asciiSym {
		if s < 0 || int(s) >= u.nsym || int32(b) < u.cuts[s] || int32(b) >= u.cuts[s+1] {
			return u, ErrCorruptImage
		}
	}
	for _, p := range u.posPat {
		if p < -1 || int(p) >= npat {
			return u, ErrCorruptImage
		}
	}
	// All position bitsets, including extension tables, must have zero padding.
	for _, ws := range [][]uint64{u.firstUnanchored, u.firstAnchored, u.lastAny, u.lastEnd, u.follow, u.reach,
		u.chainTarget, u.excMask, u.excFollow, u.condFirstBits, u.condFollowBits, u.condAcceptBits} {
		if !validImageBits(ws, u.nword, u.npos) {
			return u, ErrCorruptImage
		}
	}
	if u.hasAssert && u.nword == 1 {
		for _, ws := range [][]uint64{{u.chainTarget1, u.excMask1, u.condFollowMask1}, u.excFollow1Flat} {
			if !validImageBits(ws, 1, u.npos) {
				return u, ErrCorruptImage
			}
		}
	}
	return u, nil
}

func validImageBits(ws []uint64, nw, np int) bool {
	if np&63 == 0 {
		return true
	}
	mask := ^uint64(0) << uint(np&63)
	for i := nw - 1; i < len(ws); i += nw {
		if ws[i]&mask != 0 {
			return false
		}
	}
	return true
}

func imageRows(flat []uint64, nword int) [][]uint64 {
	rows := make([][]uint64, len(flat)/nword)
	for i := range rows {
		rows[i] = flat[i*nword : (i+1)*nword : (i+1)*nword]
	}
	return rows
}

func nfaFromImageUnit(u mvsUnit) *mvsNFA {
	n := &mvsNFA{npos: u.npos, nword: u.nword, nsym: u.nsym, first: u.firstUnanchored,
		lastAny: u.lastAny, lastEnd: u.lastEnd, follow: imageRows(u.follow, u.nword), reach: imageRows(u.reach, u.nword),
		cuts: u.cuts, anchoredStart: u.hasAnchored, requireEnd: !bsIsZero(u.lastEnd), hasAssert: u.hasAssert}
	if u.hasAnchored {
		n.first = u.firstAnchored
	}
	copy(n.asciiSym[:], u.asciiSym)
	if u.hasAssert {
		n.condFollow = make([][]guardedBits, u.npos)
		for i, g := range u.condFirstGuard {
			n.condFirst = append(n.condFirst, guardedBits{g: guard(g), bits: u.condFirstBits[i*u.nword : (i+1)*u.nword]})
		}
		for i, g := range u.condFollowGuard {
			p := u.condFollowPos[i]
			n.condFollow[p] = append(n.condFollow[p], guardedBits{g: guard(g), bits: u.condFollowBits[i*u.nword : (i+1)*u.nword]})
		}
		for i, g := range u.condAcceptGuard {
			n.condAccept = append(n.condAccept, guardedBits{g: guard(g), bits: u.condAcceptBits[i*u.nword : (i+1)*u.nword]})
		}
	}
	n.initScalar()
	return n
}

func mergedFromImageUnit(u mvsUnit) *mvsMergedNFA {
	m := &mvsMergedNFA{npos: u.npos, nword: u.nword, nsym: u.nsym, firstUnanchored: u.firstUnanchored,
		firstAnchored: u.firstAnchored, lastAny: u.lastAny, lastEnd: u.lastEnd, follow: imageRows(u.follow, u.nword),
		reach: imageRows(u.reach, u.nword), cuts: u.cuts, posPat: u.posPat, hasAnchored: u.hasAnchored}
	copy(m.asciiSym[:], u.asciiSym)
	return m
}

func decodeImageBlob(blob []byte, d *mvsDB) error {
	r := &imageReader{b: blob}
	if !bytes.Equal(r.take(4), mvsBlobMagic[:]) || r.u32() != mvsBlobVersion || r.u32() != uint32(d.n) {
		return ErrCorruptImage
	}
	merged := r.i32()
	nu := r.count(8)
	if nu > d.n+1 || merged < -1 || int(merged) >= nu {
		return ErrCorruptImage
	}
	slots := r.ints(d.n)
	offs, lens := r.ints(nu), r.ints(nu)
	if r.err != nil {
		return r.err
	}
	next := 0
	for idx, s := range slots {
		if s == -1 {
			continue
		}
		if next >= nu || s != int32(next) || int(s) == int(merged) {
			return ErrCorruptImage
		}
		u, err := imageBlobUnit(r, offs, lens, next, d.n)
		if err != nil || u.hasLimEx {
			return ErrCorruptImage
		}
		for _, p := range u.posPat {
			if p != -1 && int(p) != idx {
				return ErrCorruptImage
			}
		}
		if u.hasAnchored && !bsIsZero(u.firstUnanchored) || !u.hasAnchored && !bsIsZero(u.firstAnchored) {
			return ErrCorruptImage
		}
		d.nfas[idx] = nfaFromImageUnit(u)
		d.nfaCount++
		if u.hasAssert {
			nf := necFactor{minRunLen: int(u.necMinRunLen), runClass: byteClass(u.necRunClass)}
			if u.necRequiredByte >= 0 && u.necRequiredCount <= 32767 {
				nf.requiredBytes[u.necRequiredByte] = int16(u.necRequiredCount)
			}
			nf.hasFactor = nf.minRunLen > 0 || u.necRequiredCount > 0
			d.assertNecFactor[idx] = nf
		}
		next++
	}
	if merged != -1 {
		if int(merged) != next {
			return ErrCorruptImage
		}
		u, err := imageBlobUnit(r, offs, lens, next, d.n)
		if err != nil || u.hasAssert || u.hasDFA || !u.hasLimEx {
			return ErrCorruptImage
		}
		for p := 0; p < u.npos; p++ {
			if (bsTest(u.lastAny, p) || bsTest(u.lastEnd, p)) && u.posPat[p] < 0 {
				return ErrCorruptImage
			}
		}
		d.merged = mergedFromImageUnit(u)
		next++
	}
	if next != nu || !r.done() {
		return ErrCorruptImage
	}
	return nil
}

func imageBlobUnit(r *imageReader, offs, lens []int32, i, npat int) (mvsUnit, error) {
	if offs[i] < 0 || int(offs[i]) != r.off || lens[i] <= 0 {
		return mvsUnit{}, ErrCorruptImage
	}
	return decodeImageUnit(r.take(int(lens[i])), npat)
}
