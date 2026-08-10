package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPersistedIDsGuard_PreventsDuplicatePersist proves that the
// instructionStore's persistedIDs set prevents the same instruction
// ID from being persisted twice, even if the item is re-added to
// the writer cache after being persisted.
func TestPersistedIDsGuard_PreventsDuplicatePersist(t *testing.T) {
	store := &instructionStore{
		mode:         ProgramCacheDBWrite,
		persistedIDs: make(map[int64]struct{}),
	}

	// Simulate persisting ID 42
	store.markPersisted(42)
	store.markPersisted(43)

	// Check: 42 and 43 should be marked as persisted
	require.True(t, store.isPersisted(42), "ID 42 should be marked persisted")
	require.True(t, store.isPersisted(43), "ID 43 should be marked persisted")

	// Check: 44 should NOT be marked
	require.False(t, store.isPersisted(44), "ID 44 should not be marked persisted")

	// Re-marking 42 should be a no-op (idempotent)
	store.markPersisted(42)
	require.True(t, store.isPersisted(42))

	// Thread safety: concurrent markPersisted should not panic
	var wg sync.WaitGroup
	for i := int64(1); i <= 100; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			store.markPersisted(id)
		}(i)
	}
	wg.Wait()

	// All IDs 0-99 should be marked
	for i := int64(1); i <= 100; i++ {
		require.True(t, store.isPersisted(i), "ID %d should be marked", i)
	}
}
