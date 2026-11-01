package pcaputil

import (
	"net"
	"testing"
	"time"
)

func FuzzCaptureIdleHeapIsolation(f *testing.F) {
	f.Add([]byte{1, 0, 10, 1, 1, 20, 0, 0, 1, 2, 1, 127, 3, 0, 0})
	f.Add([]byte{1, 0, 0, 1, 0, 255, 0, 0, 1})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 4096 {
			input = input[:4096]
		}
		opts, err := (TCPReassemblyOptions{MaxFlows: 8, IdleTimeout: 10 * time.Second, captureReplay: true}).normalized()
		if err != nil {
			t.Fatal(err)
		}
		cache := newTrafficFlowCache(opts)
		close(cache.done) // This private cache has no expiry worker.
		defer cache.Close()
		var generation uint64
		for at := 0; at+2 < len(input); at += 3 {
			op, owner, stamp := input[at], input[at+1], input[at+2]
			key, _ := makeFlowKey(net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2), uint16(50000+int(owner>>3)), 65001, false)
			key.domain = CaptureDomain{Interface: int(owner & 7)}
			var ts time.Time
			if stamp != 0 {
				ts = time.Unix(1000+int64(int8(stamp)), 0)
			}
			switch op & 3 {
			case 0:
				cache.GetAt(key, ts)
			case 1:
				generation++
				cache.SetAt(key, &TrafficFlow{Index: generation}, ts)
			case 2:
				cache.Remove(key)
			case 3:
				cache.expire() // Offline state cannot be expired by the wall clock.
			}
			cache.takeRetired()
			if len(cache.items) > 8 || len(cache.captureDomains) > len(cache.items) {
				t.Fatal("unbounded offline clock state")
			}
			counts := map[*flowCaptureClock]int{}
			for key, entry := range cache.items {
				if entry.captureDomain == nil || entry.captureDomain != cache.captureDomains[key.domain] {
					t.Fatal("clock ownership crossed capture domain")
				}
				counts[entry.captureDomain]++
				if entry.seen.IsZero() && entry.expiryIndex != -1 {
					t.Fatal("unknown time received a synthetic expiry")
				}
			}
			for _, clock := range cache.captureDomains {
				if clock.flows != counts[clock] || len(clock.expiry) > clock.flows {
					t.Fatal("clock retained an evicted generation")
				}
				for i, entry := range clock.expiry {
					if entry.expiryIndex != i || entry.captureDomain != clock || cache.items[entry.key] != entry || entry.seen.IsZero() {
						t.Fatal("invalid indexed expiry/provenance")
					}
					if i > 0 && clock.expiry.Less(i, (i-1)/2) {
						t.Fatal("late time broke idle expiry order")
					}
				}
			}
		}
	})
}
