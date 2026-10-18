package minirehs

import "unsafe"

// This bounds retained data-sized buffers, not accepted input/rule sizes or
// total Go/C memory. Oversized records still complete; their scratch is closed
// rather than cached afterwards. Rule-sized immutable tables are not counted.
const groupScratchBufferBudget uint64 = 4 << 20

type groupRangeCapacity struct {
	anchor, forward, reverse uint32
}

func (s *scratch) groupPoolOversized() bool {
	return s.groupPoolBufferBytes() > groupScratchBufferBudget
}

func (s *scratch) groupPoolBufferBytes() uint64 {
	if s == nil {
		return 0
	}
	// Sizeof is a compile-time size query, with no unsafe borrowing or alias.
	retained := (uint64(cap(s.cpairs))+uint64(cap(s.cLocs))+uint64(cap(s.anchorSpansLo))+uint64(cap(s.anchorSpansHi)))*4 +
		uint64(cap(s.hits))*uint64(unsafe.Sizeof(litHit{})) +
		uint64(cap(s.lower)) + uint64(cap(s.assertBound)) + uint64(cap(s.gateBound))
	for i := range s.batchResults {
		retained += uint64(cap(s.batchResults[i])) * uint64(unsafe.Sizeof(batchMatch{}))
	}
	if retained > groupScratchBufferBudget {
		return retained
	}
	if len(s.anchorBatch)+len(s.biBatch) > 0 {
		n := len(s.anchorRanges)
		if len(s.biFwdRanges) > n {
			n = len(s.biFwdRanges)
		}
		if len(s.groupRangeCaps) < n {
			grown := make([]groupRangeCapacity, n)
			copy(grown, s.groupRangeCaps)
			s.groupRangeCaps = grown
		}
		update := func(old *uint32, capacity int) bool {
			current := uint64(capacity)
			if current > groupScratchBufferBudget/uint64(unsafe.Sizeof(anchorSpan{})) {
				return false
			}
			if current >= uint64(*old) {
				s.groupRangeBytes += (current - uint64(*old)) * uint64(unsafe.Sizeof(anchorSpan{}))
			} else {
				s.groupRangeBytes -= (uint64(*old) - current) * uint64(unsafe.Sizeof(anchorSpan{}))
			}
			*old = uint32(current)
			return true
		}
		for _, idx := range s.anchorBatch {
			if !update(&s.groupRangeCaps[idx].anchor, cap(s.anchorRanges[idx])) {
				return groupScratchBufferBudget + 1
			}
		}
		for _, idx := range s.biBatch {
			if !update(&s.groupRangeCaps[idx].forward, cap(s.biFwdRanges[idx])) ||
				!update(&s.groupRangeCaps[idx].reverse, cap(s.biRevRanges[idx])) {
				return groupScratchBufferBudget + 1
			}
		}
	}
	retained += s.groupRangeBytes
	if retained > groupScratchBufferBudget {
		return retained
	}
	for _, child := range s.batchLanes {
		if child != nil {
			retained += child.groupPoolBufferBytes()
			if retained > groupScratchBufferBudget {
				return retained
			}
		}
	}
	for _, child := range []*scratch{s.alwaysMergedScratch, s.alwaysAssertScratch, s.gateAsyncScratch, s.anchoredAsyncScratch} {
		if child != nil {
			retained += child.groupPoolBufferBytes()
			if retained > groupScratchBufferBudget {
				return retained
			}
		}
	}
	return retained
}
