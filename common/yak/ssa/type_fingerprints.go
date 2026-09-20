package ssa

import "crypto/sha256"

// Type IDs are allocated sequentially. Store their full fingerprints in small
// pages instead of a hash-map entry per type. The page map also bounds allocation
// for imported/sparse IDs; a large ID never allocates a slice up to that ID.
// Only typeStore.flush uses this table, under flushMu.
type typeFingerprints map[int64]*typeFingerprintPage

type typeFingerprintPage struct {
	present uint32
	values  [32][sha256.Size]byte
}

func (p typeFingerprints) get(id int64) ([sha256.Size]byte, bool) {
	page := p[id>>5]
	if page == nil || page.present&(1<<uint(id&31)) == 0 {
		return [sha256.Size]byte{}, false
	}
	return page.values[id&31], true
}

func (p typeFingerprints) set(id int64, value [sha256.Size]byte) {
	page := p[id>>5]
	if page == nil {
		page = new(typeFingerprintPage)
		p[id>>5] = page
	}
	page.values[id&31] = value
	page.present |= 1 << uint(id&31)
}
