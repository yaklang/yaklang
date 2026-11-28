// Package boundindex provides an amortized O(log N) address successor index
// shared across architecture boundary scanners.
package boundindex

import (
	"slices"
	"sort"
)

const flushThreshold = 64

// Index tracks discovered function start addresses and answers next-boundary
// queries in O(log N) time instead of rescanning the full boundary map.
type Index struct {
	sorted  []uint64
	pending []uint64
}

// Add records a newly inserted boundary address. Callers must only pass
// addresses not already present in the index.
func (x *Index) Add(addr uint64) {
	x.pending = append(x.pending, addr)
	if len(x.pending) >= flushThreshold {
		x.Flush()
	}
}

// Flush merges pending addresses into the sorted slice and deduplicates it.
func (x *Index) Flush() {
	if len(x.pending) == 0 {
		return
	}
	x.sorted = append(x.sorted, x.pending...)
	x.pending = x.pending[:0]
	slices.Sort(x.sorted)
	x.sorted = slices.Compact(x.sorted)
}

// Next returns the smallest indexed address strictly greater than addr, or 0
// if none exists.
func (x *Index) Next(addr uint64) uint64 {
	if len(x.pending) >= flushThreshold {
		x.Flush()
	}
	var next uint64
	if pos := sort.Search(len(x.sorted), func(i int) bool { return x.sorted[i] > addr }); pos < len(x.sorted) {
		next = x.sorted[pos]
	}
	for _, cand := range x.pending {
		if cand > addr && (next == 0 || cand < next) {
			next = cand
		}
	}
	return next
}

// Sorted returns a deduplicated ascending slice of all indexed addresses.
func (x *Index) Sorted() []uint64 {
	x.Flush()
	return x.sorted
}
