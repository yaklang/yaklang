//go:build hids && linux

package runtime

import (
	"container/heap"
	"time"
)

// observationTimeIndex keeps exactly one heap node per live key. Fixing the
// existing node supports out-of-order timestamps without accumulating stale
// expiry records for frequently updated keys.
type observationTimeIndex struct {
	items []*observationTimeEntry
	byKey map[string]*observationTimeEntry
}

type observationTimeEntry struct {
	key      string
	at       time.Time
	position int
}

func (index *observationTimeIndex) Len() int { return len(index.items) }
func (index *observationTimeIndex) Less(left, right int) bool {
	a, b := index.items[left], index.items[right]
	if a.at.Equal(b.at) {
		return a.key < b.key
	}
	return a.at.Before(b.at)
}
func (index *observationTimeIndex) Swap(left, right int) {
	index.items[left], index.items[right] = index.items[right], index.items[left]
	index.items[left].position = left
	index.items[right].position = right
}
func (index *observationTimeIndex) Push(value any) {
	entry := value.(*observationTimeEntry)
	entry.position = len(index.items)
	index.items = append(index.items, entry)
	index.byKey[entry.key] = entry
}
func (index *observationTimeIndex) Pop() any {
	last := len(index.items) - 1
	entry := index.items[last]
	index.items[last] = nil
	index.items = index.items[:last]
	delete(index.byKey, entry.key)
	return entry
}
func (index *observationTimeIndex) set(key string, at time.Time) {
	if entry, exists := index.byKey[key]; exists {
		if !entry.at.Equal(at) {
			entry.at = at
			heap.Fix(index, entry.position)
		}
		return
	}
	if index.byKey == nil {
		index.byKey = make(map[string]*observationTimeEntry)
	}
	heap.Push(index, &observationTimeEntry{key: key, at: at})
}
func (index *observationTimeIndex) remove(key string) {
	if entry, exists := index.byKey[key]; exists {
		heap.Remove(index, entry.position)
	}
}
func (index *observationTimeIndex) first() (string, time.Time, bool) {
	if len(index.items) == 0 {
		return "", time.Time{}, false
	}
	entry := index.items[0]
	return entry.key, entry.at, true
}
