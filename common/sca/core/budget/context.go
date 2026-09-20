package budget

import (
	"context"
	"fmt"
	"sync"
)

type key struct{}
type State struct {
	Limits    Limits
	mu        sync.Mutex
	entries   int
	expanded  int64
	read      int64
	exhausted bool
}

func Bind(ctx context.Context, l Limits) context.Context {
	return context.WithValue(ctx, key{}, &State{Limits: l})
}
func From(ctx context.Context) *State {
	if s, ok := ctx.Value(key{}).(*State); ok {
		return s
	}
	l, _ := (Limits{}).Normalize()
	return &State{Limits: l}
}
func (s *State) Archive(entries int, expanded int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expanded < 0 || entries < 0 || expanded > s.Limits.MaxExpandedBytes-s.expanded || entries > s.Limits.MaxArchiveEntries-s.entries {
		s.exhausted = true
		return fmt.Errorf("resource_limit: archive entries or expanded bytes")
	}
	s.entries += entries
	s.expanded += expanded
	return nil
}

func (s *State) Read(n int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 0 || n > s.Limits.MaxTotalReadBytes-s.read {
		s.exhausted = true
		return fmt.Errorf("resource_limit: cumulative snapshot and parser reads")
	}
	s.read += n
	return nil
}

// Exhausted makes failure of a shared budget observable independently of worker
// scheduling. Callers discard the concurrently produced partial graph on failure.
func (s *State) Exhausted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exhausted
}
