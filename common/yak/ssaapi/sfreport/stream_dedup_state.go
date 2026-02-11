package sfreport

import (
	"sync"
	"sync/atomic"
	"time"
)

const streamDedupTTL = 15 * time.Minute
const streamDedupSweepInterval int64 = 60 // seconds

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
		return v.(*streamDedupState)
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
	if last > 0 && now.Unix()-last < streamDedupSweepInterval {
		return
	}
	if !atomic.CompareAndSwapInt64(&streamDedupSweep, last, now.Unix()) {
		return
	}
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
		if !lu.IsZero() && now.Sub(lu) > streamDedupTTL {
			streamDedup.Delete(k)
		}
		return true
	})
}

// markSeen returns true if key is seen for the first time under the given prefix.
// nil-safe: returns true for non-empty key when receiver is nil.
func (s *streamDedupState) markSeen(prefix, key string) bool {
	if key == "" {
		return false
	}
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := prefix + key
	if _, ok := s.seen[k]; ok {
		return false
	}
	s.seen[k] = struct{}{}
	s.lastUsed = time.Now()
	return true
}

// len returns the number of entries tracked by this dedup state.
func (s *streamDedupState) len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.seen)
}

// ResetStreamFileDedup clears per-stream file-content dedup state.
func ResetStreamFileDedup(streamKey string) {
	if streamKey == "" {
		return
	}
	streamDedup.Delete(streamKey)
}
