package diagnostics

import (
	"sync/atomic"
	"time"
)

// LatestTimestamp retains the latest recorded time across concurrent workers.
type LatestTimestamp struct {
	nanos atomic.Int64
}

func (t *LatestTimestamp) Record(at time.Time) {
	nanos := at.UnixNano()
	for previous := t.nanos.Load(); nanos > previous; previous = t.nanos.Load() {
		if t.nanos.CompareAndSwap(previous, nanos) {
			return
		}
	}
}

// UnixNano returns zero until the first time has been recorded.
func (t *LatestTimestamp) UnixNano() int64 {
	return t.nanos.Load()
}
