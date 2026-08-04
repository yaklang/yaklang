package synscanx

import (
	"net"
	"reflect"
	"testing"
	"time"
)

func TestGetRouteCachesBySampleIP(t *testing.T) {
	var calls []string
	restore := stubRouteLookup(t, func(timeout time.Duration, sampleIP string) (*net.Interface, net.IP, net.IP, error) {
		calls = append(calls, sampleIP)
		return &net.Interface{Name: "iface-" + sampleIP}, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), nil
	})
	defer restore()

	loopIface, _, _, err := getRoute("127.0.0.1")
	if err != nil {
		t.Fatalf("get loopback route: %v", err)
	}
	publicIface, _, _, err := getRoute("175.178.223.47")
	if err != nil {
		t.Fatalf("get public route: %v", err)
	}

	if loopIface.Name == publicIface.Name {
		t.Fatalf("routes for different sample IPs reused iface %q", loopIface.Name)
	}
	wantCalls := []string{"127.0.0.1", "175.178.223.47"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("route lookup calls = %#v, want %#v", calls, wantCalls)
	}
}

func TestGetRouteReusesCachedSampleIP(t *testing.T) {
	var calls []string
	restore := stubRouteLookup(t, func(timeout time.Duration, sampleIP string) (*net.Interface, net.IP, net.IP, error) {
		calls = append(calls, sampleIP)
		return &net.Interface{Name: "iface-" + sampleIP}, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), nil
	})
	defer restore()

	for i := 0; i < 2; i++ {
		iface, _, _, err := getRoute("175.178.223.47")
		if err != nil {
			t.Fatalf("get route iteration %d: %v", i, err)
		}
		if iface.Name != "iface-175.178.223.47" {
			t.Fatalf("iface name = %q, want iface-175.178.223.47", iface.Name)
		}
	}

	wantCalls := []string{"175.178.223.47"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("route lookup calls = %#v, want %#v", calls, wantCalls)
	}
}

func stubRouteLookup(t *testing.T, lookup func(time.Duration, string) (*net.Interface, net.IP, net.IP, error)) func() {
	t.Helper()

	originalLookup := routeLookup
	routeLookup = lookup
	resetRouteCache()

	return func() {
		routeLookup = originalLookup
		resetRouteCache()
	}
}
