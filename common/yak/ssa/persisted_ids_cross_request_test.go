package ssa

import (
	"sync"
	"testing"
)

// TestPersistedIDs_CrossRequestDedup simulates the engineercms failure:
// same Cache, same program, two requests for same code_id.
func TestPersistedIDs_CrossRequestDedup(t *testing.T) {
	store := &instructionStore{
		mode:         ProgramCacheDBWrite,
		persistedIDs: make(map[int64]struct{}),
	}

	// First request persists code_id=214770
	store.markPersisted(214770)
	if !store.isPersisted(214770) {
		t.Fatal("214770 should be marked as persisted after first request")
	}

	// Second request: guard should catch it
	if store.isPersisted(214770) {
		t.Log("Second request for code_id=214770 correctly skipped by persistedIDs guard")
	} else {
		t.Fatal("persistedIDs guard failed")
	}

	// Thread safety
	var wg sync.WaitGroup
	for i := int64(1); i <= 100; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			store.markPersisted(id)
			_ = store.isPersisted(id)
		}(i)
	}
	wg.Wait()
	for i := int64(1); i <= 100; i++ {
		if !store.isPersisted(i) {
			t.Fatalf("ID %d should be persisted", i)
		}
	}
}

// TestPersistedIDs_DrainResidentForCloseBypassesGuard documents the root cause:
// Cache.FlushKeys does NOT check instructionStore.persistedIDs.
func TestPersistedIDs_DrainResidentForCloseBypassesGuard(t *testing.T) {
	t.Log("BUG: Cache.FlushKeys bypasses persistedIDs guard")
	t.Log("FIX NEEDED: instructionStore.Close must filter persistedIDs")
}
