package minirehs

func validateImageRoutes(d *mvsDB, kinds []mvsSpecializedKind) error {
	if d.mergedCount < 0 || d.mergedCount > d.n {
		return ErrCorruptImage
	}
	maxNW := 0
	for i, cp := range d.all {
		n := d.nfas[i]
		if d.batchable[i] != (n != nil && !n.hasAssert) ||
			(d.gate[i] || d.re2Loc[i]) && (n == nil || cp.v.exact()) ||
			d.windowable[i] && (n == nil || n.hasAssert || d.gate[i] || cp.winW <= 0 || !d.hasLiterals[i]) ||
			d.anchorable[i] && (n == nil || d.gate[i] || !d.hasLiterals[i]) ||
			d.biAnchorable[i] && (n == nil || n.hasAssert || n.anchoredStart || n.requireEnd || d.gate[i] || !d.hasLiterals[i] || d.imageFlags&imageBiAnchor == 0) ||
			d.biAnchorable[i] != (d.revNFAs[i] != nil) {
			return ErrCorruptImage
		}
		if d.anchorable[i] || d.biAnchorable[i] {
			if n.nword > maxNW {
				maxNW = n.nword
			}
			if r := d.revNFAs[i]; r != nil && r.nword > maxNW {
				maxNW = r.nword
			}
		}
	}
	if d.maxAnchorNword != maxNW {
		return ErrCorruptImage
	}
	// Check the stored index in its canonical first-appearance order, without
	// extracting literals, building an automaton or reparsing an expression.
	ids := make(map[string]int, len(d.literals))
	cursors := make([]int, len(d.literals))
	for _, cp := range d.all {
		for _, lit := range cp.literals {
			if lit == "" {
				return ErrCorruptImage
			}
			id, ok := ids[lit]
			if !ok {
				id = len(ids)
				if id >= len(d.literals) || d.literals[id] != lit {
					return ErrCorruptImage
				}
				ids[lit] = id
			}
			k := cursors[id]
			if k >= len(d.litToPat[id]) || d.litToPat[id][k] != int32(cp.idx) {
				return ErrCorruptImage
			}
			cursors[id]++
		}
	}
	if len(ids) != len(d.literals) {
		return ErrCorruptImage
	}
	for id, pats := range d.litToPat {
		if len(pats) != cursors[id] || len(d.litHead[id]) != len(pats) || len(d.litBiHead[id]) != len(pats) || len(d.litBiTail[id]) != len(pats) {
			return ErrCorruptImage
		}
		for k, idx := range pats {
			h, bh, bt := d.litHead[id][k], d.litBiHead[id][k], d.litBiTail[id][k]
			if h < -1 || bh < -1 || bt < -1 || !d.anchorable[idx] && h != -1 ||
				!d.biAnchorable[idx] && (bh != -1 || bt != -1) || d.biAnchorable[idx] && bh < 0 && bt < 0 {
				return ErrCorruptImage
			}
		}
	}
	seen := make([]bool, d.n)
	mark := func(idx int) bool {
		if idx < 0 || idx >= d.n || seen[idx] || d.hasLiterals[idx] {
			return false
		}
		seen[idx] = true
		return true
	}
	members := make([]bool, d.n)
	if d.merged != nil {
		for p, idx := range d.merged.posPat {
			if idx < 0 {
				continue
			}
			n := d.nfas[idx]
			if n == nil || n.hasAssert || d.hasLiterals[idx] ||
				!(bsTest(d.merged.lastAny, p) || bsTest(d.merged.lastEnd, p)) {
				return ErrCorruptImage
			}
			members[idx] = true
		}
	}
	nm := 0
	for i, on := range members {
		if on {
			if !mark(i) {
				return ErrCorruptImage
			}
			nm++
		}
	}
	if nm != d.mergedCount || (nm > 0) != (d.merged != nil) {
		return ErrCorruptImage
	}
	for _, idx := range d.assertAlwaysOn {
		if !mark(idx) || d.nfas[idx] == nil || !d.nfas[idx].hasAssert {
			return ErrCorruptImage
		}
	}
	ci, gi := 0, 0
	for _, idx := range d.assertAlwaysOn {
		if d.nfas[idx].single {
			if ci >= len(d.assertAlwaysOnCIdxs) || d.assertAlwaysOnCIdxs[ci] != int32(idx) {
				return ErrCorruptImage
			}
			ci++
		} else {
			if gi >= len(d.assertAlwaysOnGoIdxs) || d.assertAlwaysOnGoIdxs[gi] != idx {
				return ErrCorruptImage
			}
			gi++
		}
	}
	if ci != len(d.assertAlwaysOnCIdxs) || gi != len(d.assertAlwaysOnGoIdxs) {
		return ErrCorruptImage
	}
	for _, idx := range d.otherAlwaysOn {
		if !mark(idx) || d.nfas[idx] != nil {
			return ErrCorruptImage
		}
	}
	for _, p := range d.specializedAlwaysOn {
		if !mark(p.idx) || p.kind == mvsSpecializedNone || p.kind > mvsSpecializedWindowsPath || kinds[p.idx] != p.kind || d.nfas[p.idx] == nil {
			return ErrCorruptImage
		}
		d.specializedMask |= 1 << p.kind
	}
	for i, on := range seen {
		if on == d.hasLiterals[i] {
			return ErrCorruptImage
		}
	}
	for i, kind := range kinds {
		if kind != mvsSpecializedNone {
			found := false
			for _, p := range d.specializedAlwaysOn {
				if p.idx == i && p.kind == kind {
					found = true
					break
				}
			}
			if !found {
				return ErrCorruptImage
			}
		}
	}
	return nil
}
