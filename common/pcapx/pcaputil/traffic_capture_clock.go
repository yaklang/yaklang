package pcaputil

import (
	"container/heap"
	"time"
)

// Explicit offline idle limits use only captured observations in the same
// domain. The indexed heap orders each flow's actual latest timestamp; late
// records must neither rewind that timestamp nor refresh it to a future packet
// from another flow. Domain objects exist only while they contain cached flows.
type flowCaptureClock struct {
	watermark time.Time
	flows     int
	expiry    flowCaptureHeap
}

type flowCaptureHeap []*flowEntry

func (h flowCaptureHeap) Len() int { return len(h) }
func (h flowCaptureHeap) Less(i, j int) bool {
	if h[i].seen.Equal(h[j].seen) {
		return h[i].flow.Index < h[j].flow.Index
	}
	return h[i].seen.Before(h[j].seen)
}
func (h flowCaptureHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].expiryIndex, h[j].expiryIndex = i, j
}
func (h *flowCaptureHeap) Push(value any) {
	e := value.(*flowEntry)
	e.expiryIndex = len(*h)
	*h = append(*h, e)
}
func (h *flowCaptureHeap) Pop() any {
	old := *h
	e := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	e.expiryIndex = -1
	return e
}

// The cache mutex protects both the global resource LRU and these heaps.
func (c *trafficFlowCache) touchCaptureEntry(e *flowEntry, captured time.Time) {
	g := e.captureDomain
	if g == nil {
		if c.captureDomains == nil {
			c.captureDomains = make(map[CaptureDomain]*flowCaptureClock)
		}
		g = c.captureDomains[e.key.domain]
		if g == nil {
			g = &flowCaptureClock{watermark: captured}
			c.captureDomains[e.key.domain] = g
		}
		g.flows++
		e.captureDomain, e.expiryIndex = g, -1
	}
	if captured.IsZero() || !captured.After(e.seen) {
		return // Unknown/regressed capture time cannot extend idle identity.
	}
	e.seen = captured
	if captured.After(g.watermark) {
		g.watermark = captured
	}
	if e.expiryIndex < 0 {
		heap.Push(&g.expiry, e)
	} else {
		heap.Fix(&g.expiry, e.expiryIndex)
	}
}

func (c *trafficFlowCache) removeCaptureEntry(e *flowEntry) {
	g := e.captureDomain
	if g == nil {
		return
	}
	if e.expiryIndex >= 0 {
		heap.Remove(&g.expiry, e.expiryIndex)
	}
	g.flows--
	if g.flows == 0 {
		delete(c.captureDomains, e.key.domain)
	}
	e.captureDomain = nil
}

func (c *trafficFlowCache) expireCaptureDomain(domain CaptureDomain, captured time.Time) {
	g := c.captureDomains[domain]
	if g == nil {
		return
	}
	if captured.After(g.watermark) {
		g.watermark = captured
	}
	for len(g.expiry) > 0 && g.watermark.Sub(g.expiry[0].seen) >= c.ttl {
		c.retire(g.expiry[0], TrafficFlowCloseReason_INACTIVE)
	}
}
