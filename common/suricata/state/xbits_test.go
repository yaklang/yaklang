package state

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestXBitScopesExpiryBoundaryAndRollback(t *testing.T) {
	start := time.Unix(100, 0)
	clock := newFakeClock(start)
	store := testStore(t, clock)
	domain := testDomain(1)
	keys := []XBitKey{
		FlowXBitKey(testFlow(1, 1, 1), "tag"),
		HostXBitKey(NewHostKey(1, domain, netip.MustParseAddr("192.0.2.1")), "tag"),
		PairXBitKey(NewPairKey(1, domain, netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2")), "tag"),
	}
	for _, key := range keys {
		if err := store.SetXBit(key, time.Second); err != nil {
			t.Fatal(err)
		}
	}
	clock.Set(start.Add(time.Second - time.Nanosecond))
	for _, key := range keys {
		if got, err := store.TestXBit(key); err != nil || !got {
			t.Fatalf("end-1ns got=%v err=%v", got, err)
		}
	}
	clock.Set(start.Add(time.Second))
	for _, key := range keys {
		if got, err := store.TestXBit(key); err != nil || got {
			t.Fatalf("exact end got=%v err=%v", got, err)
		}
	}

	key := keys[0]
	clock.Set(start.Add(10 * time.Second))
	if err := store.SetXBit(key, time.Second); err != nil {
		t.Fatal(err)
	}
	clock.Set(start)
	if got, err := store.TestXBit(key); err != nil || !got {
		t.Fatalf("rollback expired xbit got=%v err=%v", got, err)
	}
	clock.Set(start.Add(11 * time.Second))
	if got, err := store.TestXBit(key); err != nil || got {
		t.Fatalf("expiry after rollback got=%v err=%v", got, err)
	}
}

func TestXBitEpochDomainAndCapacityIsolation(t *testing.T) {
	cfg := testConfig()
	cfg.MaxXBits = 2
	cfg.MaxNameLength = 4
	store, err := NewStore(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, key := range []XBitKey{
		FlowXBitKey(testFlow(1, 1, 1), "tag"),
		FlowXBitKey(testFlow(2, 1, 1), "tag"),
	} {
		if err := store.SetXBit(key, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetXBit(FlowXBitKey(testFlow(1, 2, 1), "tag"), 0); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity err=%v", err)
	}
	if err := store.SetXBit(FlowXBitKey(testFlow(1, 1, 1), "longer"), 0); !errors.Is(err, ErrNameTooLong) {
		t.Fatalf("name limit err=%v", err)
	}
}
