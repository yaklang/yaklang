package ssa

import (
	"fmt"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/utils/memedit"
)

// offsetStore wraps OffsetMap and OffsetSortedSlice with a RWMutex to
// protect concurrent access during syntaxflow scan parallelism.
//
// LazyInstruction materialization from multiple goroutines (triggered by
// concurrent syntaxflow scan rules) can invoke setValue/setVariable
// simultaneously on the same Program. Without locking, this triggers
// "fatal error: concurrent map writes".
type offsetStore struct {
	mu            sync.RWMutex
	offsetMap     map[int]*OffsetItem
	sortedOffsets []int
}

func newOffsetStore() *offsetStore {
	return &offsetStore{
		offsetMap:     make(map[int]*OffsetItem),
		sortedOffsets: make([]int, 0),
	}
}

func (s *offsetStore) setVariable(v *Variable, r *memedit.Range) {
	if r == nil {
		return
	}
	endOffset := r.GetEndOffset()
	s.mu.Lock()
	defer s.mu.Unlock()
	if item, ok := s.offsetMap[endOffset]; ok && item.rangeLength <= r.Len() {
		return
	}
	s.sortedOffsets = InsertSortedIntSlice(s.sortedOffsets, endOffset)
	s.offsetMap[endOffset] = &OffsetItem{
		variable:    v,
		value:       v.GetValue(),
		rangeLength: r.Len(),
	}
}

func (s *offsetStore) setValue(v Value, r *memedit.Range, force bool) {
	if r == nil {
		return
	}
	endOffset := r.GetEndOffset()
	s.mu.Lock()
	defer s.mu.Unlock()
	if item, ok := s.offsetMap[endOffset]; !force && ok && item.rangeLength <= r.Len() {
		return
	}
	s.sortedOffsets = InsertSortedIntSlice(s.sortedOffsets, endOffset)
	s.offsetMap[endOffset] = &OffsetItem{
		variable:    nil,
		value:       v,
		rangeLength: r.Len(),
	}
}

func (s *offsetStore) get(offset int) (*OffsetItem, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.offsetMap[offset]
	return item, ok
}

func (s *offsetStore) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.offsetMap)
}

func (s *offsetStore) searchIndexAndOffset(searchOffset int) (index int, offset int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	index = sort.Search(len(s.sortedOffsets), func(i int) bool {
		return s.sortedOffsets[i] >= searchOffset
	})
	if index >= len(s.sortedOffsets) && len(s.sortedOffsets) > 0 {
		index = len(s.sortedOffsets) - 1
	}
	if len(s.sortedOffsets) > 0 {
		offset = s.sortedOffsets[index]
	}
	return
}

func (s *offsetStore) getFrontValue(searchOffset int) (offset int, value Value) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	index := sort.Search(len(s.sortedOffsets), func(i int) bool {
		return s.sortedOffsets[i] >= searchOffset
	})
	if index >= len(s.sortedOffsets) && len(s.sortedOffsets) > 0 {
		index = len(s.sortedOffsets) - 1
	}
	if len(s.sortedOffsets) > 0 {
		offset = s.sortedOffsets[index]
	}
	if offset > searchOffset {
		if index > 0 {
			index -= 1
		}
		offset = s.sortedOffsets[index]
	}
	if item, ok := s.offsetMap[offset]; ok {
		value = item.GetValue()
	}
	return
}

func (s *offsetStore) showAll() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := 0; i < len(s.sortedOffsets); i++ {
		offset := s.sortedOffsets[i]
		value := s.offsetMap[offset].GetValue()
		fmt.Printf("%d: %s\n", offset, value.String())
	}
}

func (s *offsetStore) getAllOffsetItemsBefore(offset int) []*OffsetItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	index := sort.SearchInts(s.sortedOffsets, offset)
	if index < len(s.sortedOffsets) && s.sortedOffsets[index] > offset && index > 0 {
		index--
	}
	beforeSlice := s.sortedOffsets[:index]
	result := make([]*OffsetItem, 0, len(beforeSlice))
	for _, off := range beforeSlice {
		if item := s.offsetMap[off]; item != nil && item.GetVariable() != nil {
			result = append(result, item)
		}
	}
	return result
}