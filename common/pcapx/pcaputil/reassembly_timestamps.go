package pcaputil

import (
	"sort"
	"sync"
	"time"
)

type streamTimestamp struct {
	end  uint64
	time time.Time
}

// Readers run concurrently with capture. Store byte offsets rather than
// payloads, and find message completion timestamps in O(log n).
type streamTimestamps struct {
	mu    sync.RWMutex
	items []streamTimestamp
}

func (s *streamTimestamps) add(size int, ts time.Time) {
	s.mu.Lock()
	var end uint64
	if len(s.items) > 0 {
		end = s.items[len(s.items)-1].end
	}
	s.items = append(s.items, streamTimestamp{end: end + uint64(size), time: ts})
	s.mu.Unlock()
}

func (s *streamTimestamps) at(offset int) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if offset <= 0 {
		return time.Time{}
	}
	i := sort.Search(len(s.items), func(i int) bool { return s.items[i].end >= uint64(offset) })
	if i == len(s.items) {
		return time.Time{}
	}
	return s.items[i].time
}

func (s *streamTimestamps) clear() {
	s.mu.Lock()
	s.items = nil
	s.mu.Unlock()
}
