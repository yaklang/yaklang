package ssa

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// residentKeys used to be a SafeMap with its own lock, so "is this id indexed?"
// and the published index list could drift apart between two locks. It is now a
// plain map under residentMu, the same lock that mutates the lists. This test
// drives every mutating entry point from many goroutines so the race detector
// sees the map, and checks the published lists stay deduplicated.
func TestResidentIndexKeysStayConsistentUnderConcurrency(t *testing.T) {
	p := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, p)()
	b := p.GetAndCreateFunctionBuilder("", "resident-keys-concurrent")
	s := p.Cache.indexes

	const workers = 16
	insts := make([]Instruction, workers)
	names := make([]string, workers)
	for i := 0; i < workers; i++ {
		insts[i] = b.EmitUndefined(fmt.Sprintf("v%d", i))
		names[i] = fmt.Sprintf("own-%d", i)
	}

	// Publishing the same name twice from racing goroutines must register the
	// membership once and store one id.
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			s.AddVariable(names[w], insts[w])
			s.AddVariable(names[w], insts[w])
		}(w)
	}
	wg.Wait()
	for w := 0; w < workers; w++ {
		ids, ok := s.variable.Get(names[w])
		require.True(t, ok, names[w])
		require.Equal(t, []int64{insts[w].GetId()}, ids, names[w])
	}

	// Add and remove the same key concurrently. The surviving set is order
	// dependent, but no id may be published twice: membership and the list move
	// under one lock.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 32; i++ {
				s.AddVariable("shared", insts[w])
				s.RemoveVariable("shared", insts[w])
			}
		}(w)
	}
	wg.Wait()
	ids, _ := s.variable.Get("shared")
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		_, dup := seen[id]
		require.False(t, dup, "resident membership and the published list must agree")
		seen[id] = struct{}{}
	}

	// Removing every own key must clear the list again.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			s.RemoveVariable(names[w], insts[w])
		}(w)
	}
	wg.Wait()
	for w := 0; w < workers; w++ {
		ids, _ := s.variable.Get(names[w])
		require.Empty(t, ids, names[w])
	}
	b.Finish()
}
