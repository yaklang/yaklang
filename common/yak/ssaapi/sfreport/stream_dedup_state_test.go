package sfreport

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStreamDedupState_EmptyKey(t *testing.T) {
	assert.Nil(t, getStreamDedupState(""))
}

func TestGetStreamDedupState_NewAndReuse(t *testing.T) {
	key := fmt.Sprintf("test-new-reuse-%d", time.Now().UnixNano())
	defer ResetStreamFileDedup(key)

	s1 := getStreamDedupState(key)
	require.NotNil(t, s1)

	s2 := getStreamDedupState(key)
	assert.Equal(t, s1, s2, "should return the same instance for the same key")
}

func TestMarkSeen_BasicDedup(t *testing.T) {
	key := fmt.Sprintf("test-mark-seen-%d", time.Now().UnixNano())
	defer ResetStreamFileDedup(key)

	st := getStreamDedupState(key)

	assert.True(t, st.markSeen("file:", "h1"))
	assert.False(t, st.markSeen("file:", "h1"), "duplicate should be rejected")
	assert.True(t, st.markSeen("flow:", "h1"), "different prefix is independent")
	assert.True(t, st.markSeen("file:", "h2"), "different key should pass")
	assert.Equal(t, 3, st.len())
}

func TestMarkSeen_EmptyKey(t *testing.T) {
	st := &streamDedupState{seen: make(map[string]struct{})}
	assert.False(t, st.markSeen("file:", ""))
	assert.Equal(t, 0, st.len())
}

func TestMarkSeen_NilReceiver(t *testing.T) {
	var st *streamDedupState
	assert.True(t, st.markSeen("file:", "h1"), "nil state should allow non-empty key")
	assert.False(t, st.markSeen("file:", ""), "nil state should reject empty key")
	assert.Equal(t, 0, st.len())
}

func TestStreamDedupLen(t *testing.T) {
	var nilSt *streamDedupState
	assert.Equal(t, 0, nilSt.len())

	st := &streamDedupState{seen: make(map[string]struct{})}
	assert.Equal(t, 0, st.len())

	st.markSeen("a:", "1")
	st.markSeen("b:", "2")
	assert.Equal(t, 2, st.len())
}

func TestResetStreamFileDedup(t *testing.T) {
	key := fmt.Sprintf("test-reset-%d", time.Now().UnixNano())

	st := getStreamDedupState(key)
	st.markSeen("file:", "h1")
	require.Equal(t, 1, st.len())

	ResetStreamFileDedup(key)

	// After reset, a new state should be created.
	st2 := getStreamDedupState(key)
	assert.Equal(t, 0, st2.len())

	// Clean up.
	ResetStreamFileDedup(key)
}

func TestResetStreamFileDedup_EmptyKey(t *testing.T) {
	// Should not panic.
	ResetStreamFileDedup("")
}

func TestMaybeSweepStreamDedup_CleansExpired(t *testing.T) {
	key := fmt.Sprintf("test-sweep-%d", time.Now().UnixNano())
	st := getStreamDedupState(key)
	st.markSeen("file:", "h1")

	// Manually set lastUsed to past to simulate expiration.
	st.mu.Lock()
	st.lastUsed = time.Now().Add(-streamDedupTTL - time.Minute)
	st.mu.Unlock()

	// Force sweep by resetting the sweep timestamp.
	streamDedupSweep = 0
	maybeSweepStreamDedup()

	// The key should be cleaned up.
	_, loaded := streamDedup.Load(key)
	assert.False(t, loaded, "expired key should be swept")
}

func TestMaybeSweepStreamDedup_KeepsFresh(t *testing.T) {
	key := fmt.Sprintf("test-sweep-fresh-%d", time.Now().UnixNano())
	defer ResetStreamFileDedup(key)

	st := getStreamDedupState(key)
	st.markSeen("file:", "h1")

	streamDedupSweep = 0
	maybeSweepStreamDedup()

	_, loaded := streamDedup.Load(key)
	assert.True(t, loaded, "fresh key should not be swept")
}

func TestMarkSeen_Concurrent(t *testing.T) {
	key := fmt.Sprintf("test-concurrent-%d", time.Now().UnixNano())
	defer ResetStreamFileDedup(key)

	st := getStreamDedupState(key)
	const goroutines = 50
	const keysPerGoroutine = 100

	var wg sync.WaitGroup
	firstSeen := sync.Map{}

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for k := 0; k < keysPerGoroutine; k++ {
				key := fmt.Sprintf("k%d", k)
				if st.markSeen("test:", key) {
					if _, loaded := firstSeen.LoadOrStore(key, gid); loaded {
						t.Errorf("key %s was marked as first-seen by multiple goroutines", key)
					}
				}
			}
		}(g)
	}
	wg.Wait()

	// Each of the 100 unique keys should be seen exactly once.
	count := 0
	firstSeen.Range(func(_, _ any) bool {
		count++
		return true
	})
	assert.Equal(t, keysPerGoroutine, count)
	assert.Equal(t, keysPerGoroutine, st.len())
}
