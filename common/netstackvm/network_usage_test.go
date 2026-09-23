package netstackvm

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"
)

func TestNetworkVMHTTPAndDNS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a := testNetworkVM(t, NetworkVMConfig{Address: netip.MustParsePrefix("10.78.0.1/24"), DNS: []netip.Addr{netip.MustParseAddr("10.78.0.2")}})
	b := testNetworkVM(t, NetworkVMConfig{Address: netip.MustParsePrefix("10.78.0.2/24")})
	go forwardChannel(ctx, a.PacketEndpoint(), b.PacketEndpoint())
	go forwardChannel(ctx, b.PacketEndpoint(), a.PacketEndpoint())
	dns, e := b.ListenUDP("10.78.0.2:53")
	require.NoError(t, e)
	defer dns.Close()
	queries := make(chan string, 8)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, addr, e := dns.ReadFrom(buf)
			if e != nil {
				return
			}
			var msg dnsmessage.Message
			if msg.Unpack(buf[:n]) != nil || len(msg.Questions) == 0 {
				return
			}
			queries <- msg.Questions[0].Name.String()
			msg.Header.Response = true
			msg.Header.RecursionAvailable = true
			msg.Answers = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: msg.Questions[0].Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60}, Body: &dnsmessage.AResource{A: [4]byte{10, 78, 0, 2}}}}
			data, e := msg.Pack()
			if e != nil {
				return
			}
			dns.WriteTo(data, addr)
		}
	}()
	ln, e := b.ListenTCP("10.78.0.2:8080")
	require.NoError(t, e)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "VM HTTP "+r.Host) })}
	defer server.Close()
	go server.Serve(ln)
	transport := &http.Transport{DialContext: a.DialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	req, e := http.NewRequestWithContext(ctx, "GET", "http://vm.test:8080/", nil)
	require.NoError(t, e)
	response, e := client.Do(req)
	require.NoError(t, e)
	defer response.Body.Close()
	body, e := io.ReadAll(response.Body)
	require.NoError(t, e)
	require.Equal(t, "VM HTTP vm.test:8080", string(body))
	select {
	case name := <-queries:
		require.Equal(t, "vm.test.", name)
	case <-ctx.Done():
		t.Fatal("no VM DNS query")
	}
}

func TestPCAPLiveLoopbackOptional(t *testing.T) {
	device := os.Getenv("NETSTACKVM_PCAP_DEVICE")
	if device == "" {
		t.Skip("set NETSTACKVM_PCAP_DEVICE to a loopback device for live passive capture")
	}
	// Only ordinary host UDP sockets generate traffic; both VM captures are passive.
	listener, e := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, e)
	defer listener.Close()
	port := listener.LocalAddr().(*net.UDPAddr).Port
	var first, second atomic.Int64
	observe := func(counter *atomic.Int64) func(gopacket.Packet) {
		return func(p gopacket.Packet) {
			if u, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP); ok && int(u.DstPort) == port {
				counter.Add(1)
			}
		}
	}
	a := testNetworkVM(t, NetworkVMConfig{Mode: NetworkPCAP, Device: device, OnPacket: observe(&first)})
	b := testNetworkVM(t, NetworkVMConfig{Mode: NetworkPCAP, Device: device, OnPacket: observe(&second)})
	host, e := net.Dial("udp4", listener.LocalAddr().String())
	require.NoError(t, e)
	defer host.Close()
	require.Eventually(t, func() bool { host.Write([]byte("passive capture")); return first.Load() > 0 && second.Load() > 0 }, 3*time.Second, 25*time.Millisecond)
	require.NoError(t, a.Close())
	previous := second.Load()
	require.Eventually(t, func() bool { host.Write([]byte("remaining subscriber")); return second.Load() > previous }, 3*time.Second, 25*time.Millisecond)
	require.NoError(t, b.Close())
	fanouts.Lock()
	count := len(fanouts.entries)
	fanouts.Unlock()
	require.Zero(t, count)
}

// A shared VM can be used directly as the dialer of standard Go clients.
func ExampleNewNetworkVM() {
	vm, err := NewNetworkVM(context.Background(), NetworkVMConfig{Mode: NetworkShared, Address: netip.MustParsePrefix("10.90.0.2/24"), DNS: []netip.Addr{netip.MustParseAddr("1.1.1.1")}, HostDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		// Policies see every destination before a host connection is opened.
		if strings.HasSuffix(address, ":25") {
			return nil, fmt.Errorf("SMTP is disabled")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}})
	if err != nil {
		panic(err)
	}
	defer vm.Close()
	transport := &http.Transport{DialContext: vm.DialContext}
	defer transport.CloseIdleConnections()
	_ = &http.Client{Transport: transport, Timeout: 10 * time.Second}
}
