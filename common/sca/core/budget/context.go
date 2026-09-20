package budget

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

type key struct{}
type State struct {
	Limits    Limits
	mu        sync.Mutex
	entries   int
	objects   int
	expanded  int64
	read      int64
	result    int64
	seen      map[string]struct{}
	exhausted bool
}

func Bind(ctx context.Context, l Limits) context.Context {
	if n, err := l.Normalize(); err == nil {
		l = n
	}
	return context.WithValue(ctx, key{}, &State{Limits: l, seen: map[string]struct{}{}})
}
func From(ctx context.Context) *State {
	if ctx != nil {
		if s, ok := ctx.Value(key{}).(*State); ok {
			return s
		}
	}
	l, _ := (Limits{}).Normalize()
	return &State{Limits: l, seen: map[string]struct{}{}}
}
func (s *State) Archive(entries int, expanded int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expanded < 0 || entries < 0 || expanded > s.Limits.MaxExpandedBytes-s.expanded || entries > s.Limits.MaxArchiveEntries-s.entries {
		s.exhausted = true
		return scanerr.New(scanerr.ResourceLimit, "archive entries or expanded bytes")
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
		return scanerr.New(scanerr.ResourceLimit, "cumulative snapshot and parser reads")
	}
	s.read += n
	return nil
}

// Add charges logical objects and result bytes. It must be called before the
// corresponding allocation, map insert, or slice append.
func (s *State) Add(objects int, bytes int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addLocked(objects, bytes)
}

func (s *State) addLocked(objects int, bytes int64) error {
	if objects < 0 || bytes < 0 {
		s.exhausted = true
		return scanerr.New(scanerr.ResourceLimit, "negative result charge")
	}
	maxNodes := s.Limits.MaxExpressionNodes
	if maxNodes == 0 {
		maxNodes = 100000
	}
	if maxNodes < 0 || (objects > 0 && objects > maxNodes-s.objects) {
		s.exhausted = true
		return scanerr.New(scanerr.ResourceLimit, "expression nodes")
	}
	maxResult := s.Limits.MaxResultBytes
	if maxResult == 0 {
		maxResult = 256 << 20
	}
	if maxResult < 0 || (bytes > 0 && bytes > maxResult-s.result) {
		s.exhausted = true
		return scanerr.New(scanerr.ResourceLimit, "result memory estimate")
	}
	s.objects += objects
	s.result += bytes
	return nil
}

func (s *State) Result(n int64) error { return s.Add(0, n) }

func (s *State) Node() error { return s.Add(1, SizeObject) }

func (s *State) Working(n int64) error { return s.Add(0, n) }

// Insert charges a map key and pointer before the corresponding map write.
func (s *State) Insert(key string) error {
	return s.Add(0, SizeOfString(key)+SizePtr)
}

// Once charges shared material or a cached DTO only the first time id is seen.
func (s *State) Once(id string, objects int, bytes int64) error {
	if id == "" {
		return s.Add(objects, bytes)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == nil {
		s.seen = map[string]struct{}{}
	}
	if _, ok := s.seen[id]; ok {
		return nil
	}
	if err := s.addLocked(objects, bytes+SizeOfString(id)+SizeMap); err != nil {
		return err
	}
	s.seen[id] = struct{}{}
	return nil
}

func (s *State) ResultBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}

func (s *State) ObjectCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.objects
}

// Exhausted makes failure of a shared budget observable independently of worker
// scheduling. Callers discard the concurrently produced partial graph on failure.
func (s *State) Exhausted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exhausted
}
