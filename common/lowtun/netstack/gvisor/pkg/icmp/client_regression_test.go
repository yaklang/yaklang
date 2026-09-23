package icmp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv6"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	transporticmp "github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/icmp"
)

func TestPingScanInvalidConfig(t *testing.T) {
	c := NewClient(nil)
	for _, option := range []ScanConfigOpt{WithConcurrent(0), WithConcurrent(-1), WithRetryTimes(-1)} {
		if _, err := c.PingScan(context.Background(), "192.0.2.1", option); err == nil {
			t.Fatal("expected invalid configuration error")
		}
	}
}

func TestPingScanCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := NewClient(nil)
	if _, err := c.Ping(ctx, "192.0.2.1", time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	results, err := c.PingScan(ctx, "192.0.2.0/24")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-results:
		if ok {
			t.Fatal("unexpected result")
		}
	case <-time.After(time.Second):
		t.Fatal("scan did not close")
	}
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{transporticmp.NewProtocol4, transporticmp.NewProtocol6},
		HandleLocal:        true,
	})
	t.Cleanup(func() { s.Close(); s.Wait() })
	if err := s.CreateNIC(1, channel.New(16, 1500, "")); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"192.0.2.1", "2001:db8::1"} {
		addr := tcpip.AddrFromSlice(net.ParseIP(target).To4())
		proto := ipv4.ProtocolNumber
		prefix := 24
		if target == "2001:db8::1" {
			addr = tcpip.AddrFromSlice(net.ParseIP(target).To16())
			proto = ipv6.ProtocolNumber
			prefix = 64
		}
		pa := tcpip.ProtocolAddress{Protocol: proto, AddressWithPrefix: tcpip.AddressWithPrefix{Address: addr, PrefixLen: prefix}}
		if err := s.AddProtocolAddress(1, pa, stack.AddressProperties{}); err != nil {
			t.Fatal(err)
		}
	}
	s.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: 1}, {Destination: header.IPv6EmptySubnet, NIC: 1}})
	return NewClient(s)
}

func TestPingInMemory(t *testing.T) {
	c := newTestClient(t)
	for _, target := range []string{"192.0.2.1", "2001:db8::1"} {
		t.Run(target, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			result, err := c.Ping(ctx, target, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Ok || result.Address.String() != target {
				t.Fatalf("unexpected echo result: %+v", result)
			}
		})
	}
}

func TestPingScanCancelInFlight(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results, err := c.PingScan(ctx, "192.0.2.0/24", WithConcurrent(100), WithTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	// The local .1 reply proves the scan started; other targets remain pending.
	select {
	case r := <-results:
		if r == nil || !r.Ok {
			t.Fatalf("unexpected result: %+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("scan did not start")
	}
	cancel()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-results:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("in-flight probes did not stop")
		}
	}
}
