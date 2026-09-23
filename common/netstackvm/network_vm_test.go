package netstackvm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/dhcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
)

func testNetworkVM(t *testing.T, cfg NetworkVMConfig) *NetworkVM {
	t.Helper()
	v, e := NewNetworkVM(context.Background(), cfg)
	require.NoError(t, e)
	t.Cleanup(func() { v.Close() })
	return v
}
func TestNetworkVMIsolatedTCPUDP(t *testing.T) {
	a := testNetworkVM(t, NetworkVMConfig{Address: netip.MustParsePrefix("10.71.0.2/24")})
	b := testNetworkVM(t, NetworkVMConfig{Address: netip.MustParsePrefix("10.71.0.3/24")})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); forwardChannel(ctx, a.PacketEndpoint(), b.PacketEndpoint()) }()
	go func() { defer wg.Done(); forwardChannel(ctx, b.PacketEndpoint(), a.PacketEndpoint()) }()
	defer func() { cancel(); wg.Wait() }()
	ln, e := b.ListenTCP("10.71.0.3:8000")
	require.NoError(t, e)
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		c, e := ln.Accept()
		if e != nil {
			done <- e
			return
		}
		defer c.Close()
		_, e = io.Copy(c, c)
		done <- e
	}()
	c, e := a.DialContext(ctx, "tcp", ln.Addr().String())
	require.NoError(t, e)
	require.Equal(t, "10.71.0.2", c.LocalAddr().(*net.TCPAddr).IP.String())
	c.SetDeadline(time.Now().Add(3 * time.Second))
	_, e = c.Write([]byte("independent TCP"))
	require.NoError(t, e)
	data := make([]byte, 15)
	_, e = io.ReadFull(c, data)
	require.NoError(t, e)
	require.Equal(t, "independent TCP", string(data))
	c.Close()
	require.NoError(t, <-done)
	u, e := gonet.DialUDP(b.Stack(), &tcpip.FullAddress{NIC: b.nic, Addr: tcpip.AddrFrom4([4]byte{10, 71, 0, 3}), Port: 8001}, nil, header.IPv4ProtocolNumber)
	require.NoError(t, e)
	defer u.Close()
	client, e := a.DialContext(ctx, "udp", "10.71.0.3:8001")
	require.NoError(t, e)
	defer client.Close()
	payload := bytes.Repeat([]byte("x"), 4096)
	client.SetDeadline(time.Now().Add(3 * time.Second))
	u.SetDeadline(time.Now().Add(3 * time.Second))
	_, e = client.Write(payload)
	require.NoError(t, e)
	buf := make([]byte, 65535)
	n, remote, e := u.ReadFrom(buf)
	require.NoError(t, e)
	require.Equal(t, payload, buf[:n])
	_, e = u.WriteTo(buf[:n], remote)
	require.NoError(t, e)
	n, e = client.Read(buf)
	require.NoError(t, e)
	require.Equal(t, payload, buf[:n])
}

func TestNetworkVMSharedTCPUDP(t *testing.T) {
	// Real host sockets on loopback verify the complete private-stack gateway.
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, e)
	defer ln.Close()
	go func() {
		c, e := ln.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		io.Copy(c, c)
	}()
	u, e := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, e)
	defer u.Close()
	go func() {
		b := make([]byte, 65535)
		for {
			n, a, e := u.ReadFrom(b)
			if e != nil {
				return
			}
			u.WriteTo(b[:n], a)
		}
	}()
	v := testNetworkVM(t, NetworkVMConfig{Mode: NetworkShared, Address: netip.MustParsePrefix("10.72.0.2/24"), HostDialContext: func(ctx context.Context, n, a string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, n, strings.Replace(a, "198.18.0.1", "127.0.0.1", 1))
	}})
	require.Nil(t, v.PacketEndpoint())
	for _, tc := range []struct {
		network, target string
		size            int
	}{{"tcp", ln.Addr().String(), 8192}, {"udp", u.LocalAddr().String(), 8192}} {
		t.Run(tc.network, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c, e := v.DialContext(ctx, tc.network, strings.Replace(tc.target, "127.0.0.1", "198.18.0.1", 1))
			require.NoError(t, e)
			defer c.Close()
			c.SetDeadline(time.Now().Add(3 * time.Second))
			payload := bytes.Repeat([]byte{42}, tc.size)
			_, e = c.Write(payload)
			require.NoError(t, e)
			buf := make([]byte, len(payload))
			if tc.network == "tcp" {
				_, e = io.ReadFull(c, buf)
			} else {
				var n int
				n, e = c.Read(buf)
				require.Equal(t, len(payload), n)
			}
			require.NoError(t, e)
			require.Equal(t, payload, buf)
		})
	}
}

func TestNetworkVMSharedPolicyAndClose(t *testing.T) {
	called := make(chan string, 1)
	v := testNetworkVM(t, NetworkVMConfig{Mode: NetworkShared, Address: netip.MustParsePrefix("10.73.0.2/24"), HostDialContext: func(ctx context.Context, n, a string) (net.Conn, error) {
		called <- n + " " + a
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	done := make(chan error, 1)
	go func() { _, e := v.DialContext(context.Background(), "tcp", "203.0.113.1:80"); done <- e }()
	select {
	case s := <-called:
		require.Equal(t, "tcp 203.0.113.1:80", s)
	case <-time.After(3 * time.Second):
		t.Fatal("gateway policy not called")
	}
	require.NoError(t, v.Close())
	select {
	case e := <-done:
		require.Error(t, e)
	case <-time.After(3 * time.Second):
		t.Fatal("dial not canceled on close")
	}
	require.ErrorIs(t, v.WaitReady(context.Background()), net.ErrClosed)
	require.NoError(t, v.Close())
}

func TestNetworkVMLeaseRoutingAndLoss(t *testing.T) {
	v := testNetworkVM(t, NetworkVMConfig{Address: netip.MustParsePrefix("10.74.0.2/24")})
	lease := tcpip.AddressWithPrefix{Address: tcpip.AddrFrom4([4]byte{10, 74, 0, 8}), PrefixLen: 24}
	cfg := dhcp.Config{ServerAddress: tcpip.AddrFrom4([4]byte{10, 74, 0, 99}), Router: []tcpip.Address{tcpip.AddrFrom4([4]byte{10, 74, 0, 1})}, DNS: []tcpip.Address{tcpip.AddrFrom4([4]byte{10, 74, 0, 53})}}
	v.applyLease(context.Background(), tcpip.AddressWithPrefix{}, lease, cfg)
	require.Equal(t, "10.74.0.8/24", v.Address().String())
	require.Equal(t, "10.74.0.1", v.Gateway().String())
	routes := v.Stack().GetRouteTable()
	require.Len(t, routes, 2)
	require.Equal(t, "10.74.0.1", routes[1].Gateway.String())
	require.Equal(t, []netip.Addr{netip.MustParseAddr("10.74.0.53")}, v.DNSServers())
	cfg.Router = nil
	v.applyLease(context.Background(), lease, lease, cfg)
	require.False(t, v.Gateway().IsValid())
	require.Len(t, v.Stack().GetRouteTable(), 1)
	v.applyLease(context.Background(), lease, tcpip.AddressWithPrefix{}, dhcp.Config{})
	require.False(t, v.Address().IsValid())
	require.Empty(t, v.Stack().GetRouteTable())
	require.Empty(t, v.DNSServers())
	require.Error(t, v.WaitReady(context.Background()))
}

func TestNetworkVMValidationAndNoFallback(t *testing.T) {
	for _, cfg := range []NetworkVMConfig{
		{Mode: "invalid"}, {Mode: NetworkShared}, {Mode: NetworkBridged, DHCP: true},
		{Mode: NetworkPCAP, Device: "mock", DHCP: true},
		{Mode: NetworkPCAP, Device: "mock", Address: netip.MustParsePrefix("10.0.0.1/24")},
		{Address: netip.MustParsePrefix("10.0.0.1/24"), Gateway: netip.MustParseAddr("10.1.0.1")},
		{Mode: NetworkShared, Address: netip.MustParsePrefix("10.0.0.1/24"), Link: channel.New(1, 1500, "")},
	} {
		v, e := NewNetworkVM(context.Background(), cfg)
		require.Error(t, e)
		require.Nil(t, v)
	}
	v := testNetworkVM(t, NetworkVMConfig{Address: netip.MustParsePrefix("10.75.0.2/24")})
	_, e := v.DialContext(context.Background(), "tcp", "localhost:80")
	require.ErrorContains(t, e, "DNS")
	for _, target := range []string{"bad", "1.1.1.1:65536", "[::1]:80"} {
		_, e = v.DialContext(context.Background(), "tcp", target)
		require.Error(t, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = v.DialContext(ctx, "tcp", "1.1.1.1:80")
	require.True(t, errors.Is(e, context.Canceled))
}
