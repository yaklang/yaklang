package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueConcurrentMemberRelations(t *testing.T) {
	value := NewValue()
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 64; i++ {
				id := int64(worker*64 + i + 1)
				value.appendMemberPairIDs(id, id)
				value.appendOwnerPairIDs(id, id)
				_ = value.IsMember()
				_ = value.IsObject()
			}
		}(worker)
	}
	wg.Wait()
	require.Len(t, value.snapshotMemberPairs(), 1024)
	require.Len(t, value.snapshotOwnerPairs(), 1024)
	seen := make(map[int64]bool)
	for _, pair := range value.snapshotMemberPairs() {
		require.False(t, seen[pair.key])
		seen[pair.key] = true
	}
}

func TestMemberReplacementPreservesReaderSnapshot(t *testing.T) {
	lib := &ExternLib{anValue: NewValue()}
	from, to := &Call{anValue: NewValue()}, &Call{anValue: NewValue()}
	from.SetId(1)
	to.SetId(2)
	lib.appendMemberPairIDs(3, 1)
	before := lib.snapshotMemberPairs()
	lib.ReplaceValue(from, to)
	require.Equal(t, int64(1), before[0].member)
	require.Equal(t, int64(2), lib.snapshotMemberPairs()[0].member)
	SetMemberPairs(lib, nil)
	require.Len(t, before, 1)
	require.Empty(t, lib.snapshotMemberPairs())
}
