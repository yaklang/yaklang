package utils

import (
	"math/bits"
	"sync/atomic"
)

// BitVector is a bit set backed by a []uint64 words slice.
//
// Copy-on-write: Clone() returns a vector that SHARES the same words backing
// slice instead of copying it (the #1 remaining BitVector allocator on large
// scans — Clone 32.3GB + Or 9.2GB in the hadoop window). The shared flag marks
// a vector whose words slice may be aliased by at least one other vector; any
// mutation (Set / Or / the internal ensure grow) first detaches by copying the
// backing words. Read-only operations (Has / Contains / IsEmpty / ForEach)
// never detach.
//
// The anchor-bits invariant ("anchor bits are NEVER mutated in place") is what
// makes COW safe in practice: the hot shared paths (mergeAnchorBits 2nd branch,
// applyScopedAnchorBits, buildSlotAnchorBitVectors, sf_cfg_native Set calls)
// all Clone before Or/Set, so a shared source's words are never written through
// the alias. New code must keep that convention: to mutate a bitvector that
// may be shared, Clone it first (which then detaches on the actual Set/Or).
//
// The shared flag is atomic so a single source vector can be Clone()d from
// multiple goroutines (e.g. a shared anchor set read concurrently) without a
// data race. COW does not make concurrent mutation of ONE vector safe; callers
// must still serialize writes to a single vector (or clone per writer).
type BitVector struct {
	words  []uint64
	shared atomic.Bool
}

func NewBitVector() *BitVector {
	return &BitVector{}
}

// Clone returns a copy of b. With COW, the copy shares b's words slice (no
// allocation) and both vectors are marked shared; the copy detaches from the
// backing slice on the first mutation.
func (b *BitVector) Clone() *BitVector {
	if b == nil {
		return nil
	}
	if len(b.words) == 0 {
		return &BitVector{}
	}
	// Share the backing slice; mark both as shared so the next mutation detaches.
	b.shared.Store(true)
	c := &BitVector{words: b.words}
	c.shared.Store(true)
	return c
}

// ensure grows b.words to cover index, detaching first if the backing slice is
// shared with another vector.
func (b *BitVector) ensure(index int) {
	if b == nil || index < 0 {
		return
	}
	word := index >> 6
	if word < len(b.words) {
		return
	}
	// Growing always allocates a fresh slice, so it inherently detaches — but
	// we still copy the current (possibly shared) contents into it.
	grow := make([]uint64, word+1)
	copy(grow, b.words)
	b.words = grow
	b.shared.Store(false)
}

// Set sets bit index. If the vector is shared (its words slice is aliased by
// another vector), it detaches first so the other vector is not mutated.
func (b *BitVector) Set(index int) {
	if b == nil || index < 0 {
		return
	}
	b.ensure(index)
	if b.shared.Load() {
		dup := make([]uint64, len(b.words))
		copy(dup, b.words)
		b.words = dup
		b.shared.Store(false)
	}
	word := index >> 6
	bit := uint(index & 63)
	b.words[word] |= 1 << bit
}

func (b *BitVector) Has(index int) bool {
	if b == nil || index < 0 {
		return false
	}
	word := index >> 6
	if word >= len(b.words) {
		return false
	}
	bit := uint(index & 63)
	return (b.words[word] & (1 << bit)) != 0
}

// Or merges other into b. If b is shared, it detaches before mutating.
func (b *BitVector) Or(other *BitVector) {
	if b == nil || other == nil {
		return
	}
	if len(other.words) > len(b.words) {
		grow := make([]uint64, len(other.words))
		copy(grow, b.words)
		b.words = grow
		b.shared.Store(false)
	} else if b.shared.Load() {
		dup := make([]uint64, len(b.words))
		copy(dup, b.words)
		b.words = dup
		b.shared.Store(false)
	}
	for i, word := range other.words {
		b.words[i] |= word
	}
}

// Contains reports whether every bit set in other is also set in b
// (i.e. other is a subset of b). Used to short-circuit idempotent anchor
// merges and skip an allocation.
func (b *BitVector) Contains(other *BitVector) bool {
	if other == nil {
		return true
	}
	if b == nil {
		return other.IsEmpty()
	}
	for i, word := range other.words {
		if i >= len(b.words) {
			if word != 0 {
				return false
			}
			continue
		}
		if b.words[i]|word != b.words[i] {
			return false
		}
	}
	return true
}

func (b *BitVector) IsEmpty() bool {
	if b == nil {
		return true
	}
	for _, word := range b.words {
		if word != 0 {
			return false
		}
	}
	return true
}

func (b *BitVector) ForEach(handler func(index int)) {
	if b == nil || handler == nil {
		return
	}
	for wordIndex, word := range b.words {
		for word != 0 {
			lsb := bits.TrailingZeros64(word)
			handler((wordIndex << 6) + lsb)
			word &= word - 1
		}
	}
}
