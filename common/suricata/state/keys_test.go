package state

import (
	"net/netip"
	"testing"
)

func TestOwnedComparableKeysCanonicalize(t *testing.T) {
	domain := testDomain(1)
	a := netip.MustParseAddrPort("192.0.2.1:1234")
	b := netip.MustParseAddrPort("[2001:db8::1]:443")
	forward := NewFlowKey(7, domain, 3, 6, a, b)
	reverse := NewFlowKey(7, domain, 3, 6, b, a)
	if forward != reverse {
		t.Fatalf("bidirectional tuple not canonicalized: %#v != %#v", forward, reverse)
	}

	mapped := netip.MustParseAddrPort("[::ffff:192.0.2.1]:1234")
	if got := NewFlowKey(7, domain, 3, 6, mapped, b); got != forward {
		t.Fatalf("IPv4-mapped address not normalized: %#v != %#v", got, forward)
	}

	hostA := netip.MustParseAddr("203.0.113.1")
	hostB := netip.MustParseAddr("203.0.113.2")
	if NewPairKey(7, domain, hostA, hostB) != NewPairKey(7, domain, hostB, hostA) {
		t.Fatal("pair key is not unordered")
	}

	// Compile-time comparability assertions.
	_ = map[CaptureDomain]bool{domain: true}
	_ = map[FlowKey]bool{forward: true}
	_ = map[HostKey]bool{NewHostKey(7, domain, hostA): true}
	_ = map[PairKey]bool{NewPairKey(7, domain, hostA, hostB): true}
}

func TestTupleGenerationDomainEpochIsolation(t *testing.T) {
	store := testStore(t, nil)
	base := testFlow(1, 1, 1)
	keys := []FlowKey{
		base,
		testFlow(1, 1, 2),
		testFlow(1, 2, 1),
		testFlow(2, 1, 1),
	}
	if err := store.SetFlowBit(base, "seen"); err != nil {
		t.Fatal(err)
	}
	for i, key := range keys {
		got, err := store.TestFlowBit(key, "seen")
		if err != nil {
			t.Fatal(err)
		}
		if got != (i == 0) {
			t.Fatalf("key %d isolation mismatch: got %v", i, got)
		}
	}

	for i, key := range keys {
		xkey := FlowXBitKey(key, "shared")
		if i == 0 {
			if err := store.SetXBit(xkey, 0); err != nil {
				t.Fatal(err)
			}
		}
		got, err := store.TestXBit(xkey)
		if err != nil {
			t.Fatal(err)
		}
		if got != (i == 0) {
			t.Fatalf("xbit key %d isolation mismatch: got %v", i, got)
		}
	}
}
