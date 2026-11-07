package state

import (
	"net/netip"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(now time.Time) *fakeClock { return &fakeClock{now: now} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

func testConfig() Config {
	cfg := DefaultConfig()
	cfg.Shards = 8
	cfg.MaxFlows = 128
	cfg.MaxXBits = 128
	cfg.MaxThresholds = 128
	return cfg
}

func testStore(t *testing.T, clock Clock) *Store {
	t.Helper()
	store, err := NewStore(testConfig(), clock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testDomain(id uint64) CaptureDomain {
	return CaptureDomain{Sensor: "sensor", Interface: "eth0", ID: id}
}

func testFlow(epoch, domain, generation uint64) FlowKey {
	return NewFlowKey(
		epoch,
		testDomain(domain),
		generation,
		6,
		netip.MustParseAddrPort("192.0.2.10:12345"),
		netip.MustParseAddrPort("198.51.100.20:443"),
	)
}
