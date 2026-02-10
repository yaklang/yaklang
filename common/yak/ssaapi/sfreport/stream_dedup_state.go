package sfreport

import (
	"sync"
	"sync/atomic"
	"time"
)

// streamDedupState is a per-stream (usually per task_id) dedup set used by streaming converters.
// It prevents resending identical file/dataflow payloads across many per-risk events.
type streamDedupState struct {
	mu       sync.Mutex
	seen     map[string]struct{}
	lastUsed time.Time
}

var (
	streamDedup      sync.Map // key(string) -> *streamDedupState
	streamDedupSweep int64
)

func getStreamDedupState(key string) *streamDedupState {
	if key == "" {
		return nil
	}
	if v, ok := streamDedup.Load(key); ok {
		st := v.(*streamDedupState)
		st.mu.Lock()
		st.lastUsed = time.Now()
		st.mu.Unlock()
		return st
	}
	st := &streamDedupState{seen: make(map[string]struct{}, 256), lastUsed: time.Now()}
	if v, loaded := streamDedup.LoadOrStore(key, st); loaded {
		return v.(*streamDedupState)
	}
	return st
}

func maybeSweepStreamDedup() {
	now := time.Now()
	last := atomic.LoadInt64(&streamDedupSweep)
	if last > 0 && now.Unix()-last < 60 {
		return
	}
	if !atomic.CompareAndSwapInt64(&streamDedupSweep, last, now.Unix()) {
		return
	}
	ttl := 15 * time.Minute
	streamDedup.Range(func(k, v any) bool {
		key, _ := k.(string)
		st, _ := v.(*streamDedupState)
		if key == "" || st == nil {
			streamDedup.Delete(k)
			return true
		}
		st.mu.Lock()
		lu := st.lastUsed
		st.mu.Unlock()
		if !lu.IsZero() && now.Sub(lu) > ttl {
			streamDedup.Delete(k)
		}
		return true
	})
}

// ResetStreamFileDedup clears per-stream file-content dedup state.
// Intended to be called at the end of a scan task (streamKey=task_id).
func ResetStreamFileDedup(streamKey string) {
	if streamKey == "" {
		return
	}
	streamDedup.Delete(streamKey)
}

