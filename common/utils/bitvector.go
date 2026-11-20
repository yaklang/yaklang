package utils

import (
	"math/bits"
	"sort"
	"sync/atomic"
)

// BitVector is a bit set with two backings:
//
//   - sparse: a sorted list of set bit indices;
//   - dense:  a []uint64 word slice, one bit per index.
//
// The sparse form exists because anchor bits are almost always singletons with
// a large index: a SyntaxFlow anchor scope assigns slot `base+i` to value i, so
// the dense form had to grow to i/64 words for a single bit. Over one scope of
// N values that is O(N²/16) bytes (a 4.6M-instruction program reached 15 GB of
// heap). A singleton sparse entry costs 8 bytes instead.
//
// A vector switches to dense once its set bits are dense enough that the word
// slice is smaller (`count > maxIndex/32`, i.e. 4 bytes per bit vs 1 bit per
// bit). Both forms iterate in ascending index order, so callers cannot tell
// them apart.
//
// Copy-on-write: Clone() returns a vector that SHARES the backing slice
// instead of copying it. The shared flag marks a vector whose backing slice may
// be aliased by at least one other vector; any mutation (Set / Or / promotion)
// first detaches by copying the backing slice. Read-only operations (Has /
// Contains / IsEmpty / ForEach) never detach.
//
// The shared flag is atomic so a single source vector can be Clone()d from
// multiple goroutines without a data race. COW does not make concurrent
// mutation of ONE vector safe; callers must still serialize writes to a single
// vector (or clone per writer).
type BitVector struct {
	words  []uint64
	sparse []uint32 // sorted set bits, used when words is nil
	shared atomic.Bool
}

func NewBitVector() *BitVector {
	return &BitVector{}
}

// Clone returns a copy of b that shares the backing slice until either side is
// mutated.
func (b *BitVector) Clone() *BitVector {
	if b == nil {
		return nil
	}
	if len(b.sparse) == 0 && len(b.words) == 0 {
		return &BitVector{}
	}
	b.shared.Store(true)
	c := &BitVector{words: b.words, sparse: b.sparse}
	c.shared.Store(true)
	return c
}

func (b *BitVector) maxIndex() int {
	if b == nil {
		return -1
	}
	if b.words == nil {
		if len(b.sparse) == 0 {
			return -1
		}
		return int(b.sparse[len(b.sparse)-1])
	}
	for i := len(b.words) - 1; i >= 0; i-- {
		if b.words[i] != 0 {
			return (i << 6) + 63 - bits.LeadingZeros64(b.words[i])
		}
	}
	return -1
}

// promoteLocked converts the sparse form to the dense one. The caller must have
// ensured the vector is detached (not shared).
func (b *BitVector) promote() {
	if b.words != nil {
		return
	}
	max := b.maxIndex()
	if max < 0 {
		return
	}
	words := make([]uint64, (max>>6)+1)
	for _, i := range b.sparse {
		words[i>>6] |= 1 << (i & 63)
	}
	b.words = words
	b.sparse = nil
}

// shouldPromote reports whether the dense form is smaller for the current
// content. The sparse list costs 4 bytes per bit; the dense form costs
// maxIndex/8 bytes, so promote when 4*count > maxIndex/8.
func (b *BitVector) shouldPromote() bool {
	if len(b.sparse) < 64 {
		return false
	}
	max := b.maxIndex()
	if max < 0 {
		return false
	}
	return len(b.sparse) > max/32
}

// detach makes the backing slice private (COW). It is a no-op when the vector
// is not shared.
func (b *BitVector) detach() {
	if !b.shared.Load() {
		return
	}
	if b.words != nil {
		dup := make([]uint64, len(b.words))
		copy(dup, b.words)
		b.words = dup
	} else if b.sparse != nil {
		dup := make([]uint32, len(b.sparse))
		copy(dup, b.sparse)
		b.sparse = dup
	}
	b.shared.Store(false)
}

// ensure grows the dense form to cover index. Sparse vectors are converted by
// promote() only when that is cheaper; otherwise the bit is added sparsely.
func (b *BitVector) ensure(index int) {
	if b == nil || index < 0 {
		return
	}
	if b.words == nil {
		return
	}
	word := index >> 6
	if word < len(b.words) {
		return
	}
	grow := make([]uint64, word+1)
	copy(grow, b.words)
	b.words = grow
	b.shared.Store(false)
}

// Set sets bit index. If the vector is shared, it detaches first so the other
// vector is not mutated.
func (b *BitVector) Set(index int) {
	if b == nil || index < 0 {
		return
	}
	idx := uint32(index)
	if b.words == nil {
		// Sparse fast path: anchor scopes set indices in ascending order.
		if n := len(b.sparse); n == 0 || b.sparse[n-1] < idx {
			b.detach()
			b.sparse = append(b.sparse, idx)
			if b.shouldPromote() {
				b.promote()
			}
			return
		}
		pos := sort.Search(len(b.sparse), func(i int) bool { return b.sparse[i] >= idx })
		if pos < len(b.sparse) && b.sparse[pos] == idx {
			return
		}
		b.detach()
		b.sparse = append(b.sparse, 0)
		copy(b.sparse[pos+1:], b.sparse[pos:])
		b.sparse[pos] = idx
		if b.shouldPromote() {
			b.promote()
		}
		return
	}

	b.detach()
	b.ensure(index)
	word := index >> 6
	bit := uint(index & 63)
	b.words[word] |= 1 << bit
}

func (b *BitVector) Has(index int) bool {
	if b == nil || index < 0 {
		return false
	}
	if b.words == nil {
		idx := uint32(index)
		pos := sort.Search(len(b.sparse), func(i int) bool { return b.sparse[i] >= idx })
		return pos < len(b.sparse) && b.sparse[pos] == idx
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
	if b == nil || other == nil || other.IsEmpty() {
		return
	}
	if b.words == nil && other.words == nil {
		b.detach()
		b.sparse = mergeSortedBits(b.sparse, other.sparse)
		if b.shouldPromote() {
			b.promote()
		}
		return
	}
	// Mixed or dense: work in the dense form.
	if b.words == nil {
		b.detach()
		b.promote()
	} else {
		b.detach()
	}
	ob := other
	if ob.words == nil {
		ob = &BitVector{words: make([]uint64, (other.maxIndex()>>6)+1)}
		for _, i := range other.sparse {
			ob.words[i>>6] |= 1 << (i & 63)
		}
	}
	if len(ob.words) > len(b.words) {
		grow := make([]uint64, len(ob.words))
		copy(grow, b.words)
		b.words = grow
	}
	for i, word := range ob.words {
		b.words[i] |= word
	}
}

// mergeSortedBits returns the union of two ascending bit lists.
func mergeSortedBits(a, b []uint32) []uint32 {
	if len(a) == 0 {
		out := make([]uint32, len(b))
		copy(out, b)
		return out
	}
	out := make([]uint32, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			out = append(out, a[i])
			i++
		case a[i] > b[j]:
			out = append(out, b[j])
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	out = append(out, a[i:]...)
	out = append(out, b[j:]...)
	return out
}

// Contains reports whether every bit set in other is also set in b
// (i.e. other is a subset of b). Used to short-circuit idempotent anchor
// merges and skip an allocation.
func (b *BitVector) Contains(other *BitVector) bool {
	if other == nil || other.IsEmpty() {
		return true
	}
	if b == nil || b.IsEmpty() {
		return false
	}
	if b.words == nil && other.words == nil {
		// Two-pointer subset over ascending lists.
		i, j := 0, 0
		for i < len(b.sparse) && j < len(other.sparse) {
			switch {
			case b.sparse[i] == other.sparse[j]:
				i++
				j++
			case b.sparse[i] < other.sparse[j]:
				i++
			default:
				return false
			}
		}
		return j == len(other.sparse)
	}
	if b.words == nil || other.words == nil {
		// Cross-form: check each of other's bits with Has.
		missing := false
		other.ForEach(func(index int) {
			if missing {
				return
			}
			if !b.Has(index) {
				missing = true
			}
		})
		return !missing
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
	if b.words == nil {
		return len(b.sparse) == 0
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
	if b.words == nil {
		for _, i := range b.sparse {
			handler(int(i))
		}
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