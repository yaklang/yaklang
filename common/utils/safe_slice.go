package utils

import "sync"

// SafeSlice guards one ordered sequence. A plain slice is safe only when one
// goroutine owns it. Share the sequence across goroutines through SafeSlice.
// A map that must keep key order belongs in omap, not here.
type SafeSlice[T any] struct {
	mu    sync.RWMutex
	items []T
}

func (s *SafeSlice[T]) Len() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// Snapshot copies the current items. The result is nil when the slice is empty.
func (s *SafeSlice[T]) Snapshot() []T {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.items) == 0 {
		return nil
	}
	out := make([]T, len(s.items))
	copy(out, s.items)
	return out
}

// Append adds one item. Concurrent Append and Snapshot are safe.
func (s *SafeSlice[T]) Append(v T) {
	if s == nil {
		return
	}
	s.Update(func(cur []T) ([]T, bool) {
		return append(cur, v), true
	})
}

// Update calls fn with the live items while holding the write lock.
// fn must not retain the slice. write false leaves the slice unchanged;
// otherwise next replaces it.
func (s *SafeSlice[T]) Update(fn func(cur []T) (next []T, write bool)) bool {
	if s == nil || fn == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next, write := fn(s.items)
	if !write {
		return false
	}
	s.items = next
	return true
}
