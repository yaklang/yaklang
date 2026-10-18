package minirehs

import "slices"

// Prove that the merged presence machine is exactly one original executable
// NFA. Images may supply different tables: the metadata count alone cannot
// justify substituting a per-rule scan. This runs only before publication.
func singleMergedMember(d *mvsDB) (int, bool) {
	if d == nil || d.reportLoc || d.legacyFold || d.legacyInvalidUTF || d.mergedCount != 1 || d.merged == nil {
		return -1, false
	}
	m := d.merged
	if m.npos < 1 || m.npos > 64 || m.nword != 1 || m.hasAnchored || m.isAnchoredMerge ||
		len(m.firstUnanchored) != 1 || len(m.firstAnchored) != 1 || len(m.lastAny) != 1 || len(m.lastEnd) != 1 ||
		m.firstAnchored[0] != 0 || m.lastEnd[0] != 0 || len(m.posPat) != m.npos {
		return -1, false
	}
	valid := ^uint64(0)
	if m.npos < 64 {
		valid = uint64(1)<<uint(m.npos) - 1
	}
	accept := m.lastAny[0]
	if accept == 0 || (accept|m.firstUnanchored[0])&^valid != 0 {
		return -1, false
	}
	idx := -1
	for p, id := range m.posPat {
		if accept>>uint(p)&1 == 0 {
			continue
		}
		if id < 0 || int(id) >= d.n || (idx >= 0 && idx != int(id)) {
			return -1, false
		}
		idx = int(id)
	}
	if idx < 0 || idx >= len(d.nfas) || idx >= len(d.all) || d.all[idx] == nil || d.all[idx].v == nil || !d.all[idx].v.exact() {
		return -1, false
	}
	n := d.nfas[idx]
	if n == nil || n.hasAssert || n.anchoredStart || n.requireEnd || n.nword != 1 || n.npos != m.npos || n.nsym != m.nsym ||
		!slices.Equal(n.first, m.firstUnanchored) || !slices.Equal(n.lastAny, m.lastAny) || !slices.Equal(n.lastEnd, m.lastEnd) ||
		n.asciiSym != m.asciiSym || !slices.Equal(n.cuts, m.cuts) || len(m.cuts) != m.nsym+1 {
		return -1, false
	}
	if !sameSingleWordRows(n.follow, m.follow, m.npos, valid) || !sameSingleWordRows(n.reach, m.reach, m.nsym, valid) {
		return -1, false
	}
	return idx, true
}

func sameSingleWordRows(a, b [][]uint64, count int, valid uint64) bool {
	if count < 1 || len(a) != count || len(b) != count {
		return false
	}
	for i := range a {
		if len(a[i]) != 1 || len(b[i]) != 1 || a[i][0] != b[i][0] || a[i][0]&^valid != 0 {
			return false
		}
	}
	return true
}
