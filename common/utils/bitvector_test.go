package utils

import (
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBitVector_SetAndHas(t *testing.T) {
	bits := NewBitVector()
	bits.Set(0)
	bits.Set(63)
	bits.Set(64)

	require.True(t, bits.Has(0))
	require.True(t, bits.Has(63))
	require.True(t, bits.Has(64))
	require.False(t, bits.Has(1))
	require.False(t, bits.Has(-1))
}

func TestBitVector_CloneAndOr(t *testing.T) {
	left := NewBitVector()
	left.Set(1)
	left.Set(66)

	right := left.Clone()
	right.Set(130)

	require.True(t, left.Has(1))
	require.True(t, left.Has(66))
	require.False(t, left.Has(130))

	left.Or(right)
	require.True(t, left.Has(1))
	require.True(t, left.Has(66))
	require.True(t, left.Has(130))
}

func TestBitVector_ForEachAndEmpty(t *testing.T) {
	bits := NewBitVector()
	require.True(t, bits.IsEmpty())

	bits.Set(2)
	bits.Set(64)
	bits.Set(129)
	require.False(t, bits.IsEmpty())

	var got []int
	bits.ForEach(func(index int) {
		got = append(got, index)
	})
	require.Equal(t, []int{2, 64, 129}, got)
}

// TestBitVector_COW_CloneSharesThenDetachesOnMutate verifies that Clone shares
// the backing slice (no per-clone allocation) and that a mutation on the clone
// detaches so the original is not corrupted.
func TestBitVector_COW_CloneSharesThenDetachesOnMutate(t *testing.T) {
	src := NewBitVector()
	src.Set(1)
	src.Set(66)

	clone := src.Clone()
	require.True(t, src.Has(1))
	require.True(t, clone.Has(1))
	require.True(t, clone.Has(66))

	// Mutate the clone: it must detach, leaving src unchanged.
	clone.Set(130)
	require.True(t, clone.Has(130))
	require.False(t, src.Has(130), "source must not gain clone's new bit")
	require.True(t, src.Has(1))
	require.True(t, src.Has(66))

	// src can still be mutated independently after the detach.
	src.Set(200)
	require.True(t, src.Has(200))
	require.False(t, clone.Has(200), "clone must not gain src's new bit")
}

// TestBitVector_COW_OrDetachesWhenShared verifies Or on a shared vector detaches
// so the original alias is unchanged.
func TestBitVector_COW_OrDetachesWhenShared(t *testing.T) {
	src := NewBitVector()
	src.Set(5)
	clone := src.Clone() // shares words

	other := NewBitVector()
	other.Set(9)

	clone.Or(other) // must detach before Or
	require.True(t, clone.Has(5))
	require.True(t, clone.Has(9))
	require.False(t, src.Has(9), "source must not gain clone's Or bit")
	require.True(t, src.Has(5))
}

// TestBitVector_COW_ConcurrentCloneSetRace verifies the COW BitVector is safe
// under concurrent read/Clone + independent Set on separate goroutines (each
// goroutine clones from a shared source then mutates its own copy; the source
// is only read). Run with -race.
func TestBitVector_COW_ConcurrentCloneSetRace(t *testing.T) {
	src := NewBitVector()
	for i := 0; i < 64; i++ {
		src.Set(i * 5)
	}

	const goroutines = 16
	const iters = 2000
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				clone := src.Clone() // shares src's words (read-only on src)
				clone.Set(1000 + seed + i)
				// src must never have the clone's bit, and the clone must retain
				// the shared source's bits.
				if src.Has(1000 + seed + i) {
					errs <- errors.New("source leaked clone bit")
					return
				}
				if !clone.Has(0) {
					errs <- errors.New("clone lost shared source bits")
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// A single high bit must not allocate a dense word slice. SyntaxFlow anchor
// scopes set slot `base+i` per value, so a dense form costs O(index/64) words
// per value and made a 4.6M-instruction program run out of memory.
func TestBitVector_SparseHighIndexStaysSmall(t *testing.T) {
	b := NewBitVector()
	b.Set(4_000_000)
	require.Nil(t, b.words, "singleton bit must stay sparse")
	require.True(t, b.Has(4_000_000))
	require.False(t, b.Has(3_999_999))

	c := NewBitVector()
	c.Set(4_000_001)
	b.Or(c)
	require.Nil(t, b.words, "two sparse bits must stay sparse")
	var got []int
	b.ForEach(func(i int) { got = append(got, i) })
	require.Equal(t, []int{4_000_000, 4_000_001}, got)
}

// Dense-looking content promotes to the word form, and both forms agree.
func TestBitVector_PromotesWhenDense(t *testing.T) {
	b := NewBitVector()
	for i := 0; i < 4096; i++ {
		b.Set(i)
	}
	require.NotNil(t, b.words, "dense content must use the word form")
	for i := 0; i < 4096; i++ {
		require.True(t, b.Has(i))
	}
	require.False(t, b.Has(4096))
}

func TestBitVector_SparseOrContains(t *testing.T) {
	a := NewBitVector()
	a.Set(1_000_000)
	a.Set(2_000_000)

	b := NewBitVector()
	b.Set(2_000_000)

	require.True(t, a.Contains(b))
	require.False(t, b.Contains(a))

	c := NewBitVector()
	c.Set(3_000_000)
	a.Or(c)
	require.True(t, a.Has(3_000_000))
	require.True(t, a.Has(1_000_000))
}

// Clone must share the sparse slice and detach on mutation.
func TestBitVector_SparseCloneDetaches(t *testing.T) {
	a := NewBitVector()
	a.Set(100)
	b := a.Clone()
	require.Equal(t, a.sparse, b.sparse)

	b.Set(200)
	require.False(t, a.Has(200), "mutating the clone must not touch the source")
	require.True(t, b.Has(200))
	require.True(t, b.Has(100))
}

// Sparse and dense vectors must produce identical results for mixed operation
// sequences.
func TestBitVector_SparseDenseEquivalence(t *testing.T) {
	indices := []int{0, 63, 64, 1000, 5000, 5001, 200_000}
	var sparse *BitVector = NewBitVector()
	dense := NewBitVector()
	// Force the dense form by setting a contiguous run first.
	for i := 0; i < 1024; i++ {
		dense.Set(i)
		sparse.Set(i)
	}
	for _, i := range indices {
		sparse.Set(i + 4096)
		dense.Set(i + 4096)
	}
	var gotSparse, gotDense []int
	sparse.ForEach(func(i int) { gotSparse = append(gotSparse, i) })
	dense.ForEach(func(i int) { gotDense = append(gotDense, i) })
	require.Equal(t, gotDense, gotSparse)
	for _, i := range append(indices, 5, 500) {
		require.Equal(t, dense.Has(i+4096), sparse.Has(i+4096))
	}
}

// TestBitVector_AnchorScopeAllocationIsLinear pins the memory shape of an
// anchor scope: one vector per value, each with one bit at its slot index.
// The dense backing grew to index/64 words per vector, i.e. O(n²/16) bytes for
// the whole scope, which is what made a 4.6M-instruction SyntaxFlow query
// exhaust the heap. The sparse backing keeps one entry per vector.
func TestBitVector_AnchorScopeAllocationIsLinear(t *testing.T) {
	const n = 50_000
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	vectors := make([]*BitVector, n)
	for i := 0; i < n; i++ {
		b := NewBitVector()
		b.Set(i)
		vectors[i] = b
	}

	runtime.GC()
	runtime.ReadMemStats(&after)
	grown := after.HeapAlloc - before.HeapAlloc
	// Dense backing for this shape costs ~n²/16 bytes (~156 MB at n=50k); the
	// sparse form costs 4 bytes per slot plus the slice headers (~3 MB). The
	// bound leaves room for slice growth while staying far below dense.
	const limit = 24 << 20
	require.Less(t, grown, uint64(limit), "anchor scope allocation must be linear, got %d bytes", grown)
	require.True(t, vectors[n-1].Has(n-1))
	require.False(t, vectors[n-1].Has(n-2))
}
