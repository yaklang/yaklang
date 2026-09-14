package pcaputil

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type flowEntry struct {
	key        flowKey
	flow       *TrafficFlow
	seen       time.Time
	prev, next *flowEntry
}

type retiredFlow struct {
	flow   *TrafficFlow
	reason TrafficFlowCloseReason
}

// One expiry worker per capture, with synchronous retirement under the pool's
// lock. A generic TTL cache spawns insertion/eviction goroutines per connection;
// during a connection flood those goroutines can retain an unbounded backlog.
type trafficFlowCache struct {
	mu             sync.Mutex
	items          map[flowKey]*flowEntry
	oldest, newest *flowEntry
	retired        []retiredFlow
	hasRetired     atomic.Bool
	recent         *flowEntry
	capacity       int
	ttl            time.Duration
	stop, done     chan struct{}
	closeOnce      sync.Once
	budget         *reassemblyBudget
	checkExpiry    bool
}

func newTrafficFlowCache(options TCPReassemblyOptions) *trafficFlowCache {
	return &trafficFlowCache{items: make(map[flowKey]*flowEntry), capacity: options.MaxFlows, ttl: options.IdleTimeout, stop: make(chan struct{}), done: make(chan struct{}), checkExpiry: true}
}

func (c *trafficFlowCache) unlink(e *flowEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.oldest = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.newest = e.prev
	}
	e.prev, e.next = nil, nil
}

func (c *trafficFlowCache) touch(e *flowEntry, now time.Time) {
	e.seen = now
	c.recent = e
	if c.newest == e {
		return
	}
	if e.prev != nil || e.next != nil || c.oldest == e {
		c.unlink(e)
	}
	e.prev = c.newest
	if c.newest != nil {
		c.newest.next = e
	} else {
		c.oldest = e
	}
	c.newest = e
}

func (c *trafficFlowCache) retire(e *flowEntry, reason TrafficFlowCloseReason) {
	c.unlink(e)
	delete(c.items, e.key)
	if c.budget != nil {
		c.budget.releaseFlows(1)
	}
	c.retired = append(c.retired, retiredFlow{e.flow, reason})
	c.hasRetired.Store(true)
	if c.recent == e {
		c.recent = nil
	}
}

func (c *trafficFlowCache) Get(key flowKey) (*TrafficFlow, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.recent
	if e == nil || e.key != key {
		e = c.items[key]
	}
	if e == nil {
		return nil, false
	}
	now := time.Now()
	if c.checkExpiry && now.Sub(e.seen) >= c.ttl {
		c.retire(e, TrafficFlowCloseReason_INACTIVE)
		return nil, false
	}
	c.touch(e, now)
	return e.flow, true
}

func (c *trafficFlowCache) Set(key flowKey, flow *TrafficFlow) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old := c.items[key]; old != nil {
		if old.flow == flow {
			c.touch(old, time.Now())
			return true
		}
		c.retire(old, TrafficFlowCloseReason_INACTIVE)
	}
	for len(c.items) >= c.capacity {
		c.retire(c.oldest, TrafficFlowCloseReason_RESOURCE_LIMIT)
	}
	if c.budget != nil {
		for !c.budget.reserveFlow() {
			if c.oldest == nil {
				flow.pool.counters.limits.Add(1)
				flow.pool.parallel.fail(fmt.Errorf("TCP worker global flow limit exceeded"))
				return false
			}
			c.retire(c.oldest, TrafficFlowCloseReason_RESOURCE_LIMIT)
		}
	}
	e := &flowEntry{key: key, flow: flow}
	c.items[key] = e
	c.touch(e, time.Now())
	return true
}

func (c *trafficFlowCache) Remove(key flowKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.items[key]; e != nil {
		c.unlink(e)
		delete(c.items, key)
		if c.budget != nil {
			c.budget.releaseFlows(1)
		}
		if c.recent == e {
			c.recent = nil
		}
	}
}

func (c *trafficFlowCache) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *trafficFlowCache) ForEach(h func(string, *TrafficFlow)) {
	c.mu.Lock()
	flows := make([]*TrafficFlow, 0, len(c.items))
	for _, e := range c.items {
		flows = append(flows, e.flow)
	}
	c.mu.Unlock()
	for _, f := range flows {
		h(f.Hash, f)
	}
}

func (c *trafficFlowCache) expire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for c.oldest != nil && now.Sub(c.oldest.seen) >= c.ttl {
		c.retire(c.oldest, TrafficFlowCloseReason_INACTIVE)
	}
}

func (c *trafficFlowCache) takeRetired() []retiredFlow {
	if !c.hasRetired.Load() {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	retired := c.retired
	c.retired = nil
	c.hasRetired.Store(false)
	return retired
}

func (c *trafficFlowCache) Close() {
	c.closeOnce.Do(func() {
		close(c.stop)
		<-c.done
		c.mu.Lock()
		if c.budget != nil {
			c.budget.releaseFlows(len(c.items))
		}
		c.items = nil
		c.oldest, c.newest = nil, nil
		c.recent = nil
		c.retired = nil
		c.hasRetired.Store(false)
		c.mu.Unlock()
	})
}
