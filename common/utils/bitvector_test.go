package utils

import (
	"errors"
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

// TestBitVector_CanMutateInPlace guards the O1 ownership probe: a freshly-built
// vector is a unique owner (can Or in place, 0 alloc); a Clone() shares the
// backing slice so it is NOT a unique owner (Or must detach).
func TestBitVector_CanMutateInPlace(t *testing.T) {
	// Fresh vector: unique owner, no shared backing → can mutate in place.
	fresh := NewBitVector()
	fresh.Set(3)
	require.True(t, fresh.CanMutateInPlace(), "fresh vector must be a unique owner")

	// Clone: the clone shares the source's backing slice → NOT unique.
	clone := fresh.Clone()
	require.False(t, clone.CanMutateInPlace(), "clone must not be a unique owner (shares backing)")
	require.False(t, fresh.CanMutateInPlace(), "source is now shared after Clone, must not be unique owner")

	// After a mutation, the vector detaches and becomes a unique owner again.
	clone.Set(100)
	require.True(t, clone.CanMutateInPlace(), "clone detached on Set, must be unique owner again")
	require.False(t, fresh.CanMutateInPlace(), "source still shared (clone holds its old backing until detach)")

	// Empty vector is trivially a unique owner.
	require.True(t, NewBitVector().CanMutateInPlace(), "empty vector is a unique owner")
}
