//go:build hids && linux

package runtime

import "sync/atomic"

// These counters describe local queue admission, not durable transport delivery.
// Keep overflow observable without blocking the detection loop on a slow sink.
type queueHealth struct {
	enqueued atomic.Uint64
	dropped  atomic.Uint64
}

func (q *queueHealth) snapshot(depth, capacity int) map[string]any {
	return map[string]any{
		"enqueued":    q.enqueued.Load(),
		"dropped":     q.dropped.Load(),
		"queue_depth": depth,
		"capacity":    capacity,
	}
}
