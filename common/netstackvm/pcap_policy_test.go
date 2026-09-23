package netstackvm

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"
)

func mockPCAP(ctx context.Context, s *stack.Stack, writes *atomic.Int64) *PCAPEndpoint {
	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan gopacket.Packet, 32)
	p := &PCAPEndpoint{Endpoint: channel.New(128, 1500, tcpip.LinkAddress("\x02\x00\x00\x00\x00\x01")), ctx: ctx, cancel: cancel, stack: s, wg: new(sync.WaitGroup), mtu: 1500, ipToMac: new(sync.Map), gatewayFound: utils.NewAtomicBool(), tcpKillMap: make(map[string]struct{}), netBridge: &pcapBridge{internal: net.HardwareAddr{2, 0, 0, 0, 0, 1}, external: net.HardwareAddr{2, 0, 0, 0, 0, 2}}}
	p.adaptor = newPcapBroker(ch, func() { close(ch) }, func([]byte) error { writes.Add(1); return nil })
	p.adaptor.linkType = layers.LinkTypeEthernet
	p.readOnly.Store(true)
	return p
}
func tcpCapture(t *testing.T, flags uint8) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IPv4(10, 80, 0, 2), DstIP: net.IPv4(10, 80, 0, 1), Protocol: layers.IPProtocolTCP}
	tcp := &layers.TCP{SrcPort: 12345, DstPort: 443, Seq: 1, Window: 65535, SYN: flags&2 != 0, ACK: flags&16 != 0, RST: flags&4 != 0, FIN: flags&1 != 0}
	tcp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp))
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func TestPCAPPassiveGateAllWritePaths(t *testing.T) {
	var writes atomic.Int64
	p := mockPCAP(context.Background(), nil, &writes)
	defer p.Close()
	// Permissive filters and legacy TCP-killer requests cannot override the gate.
	p.SetPCAPOutboundFilter(func(gopacket.Packet) bool { return true })
	p.DisallowTCP("10.80.0.1:443")
	for _, flag := range []uint8{2, 16, 4, 1} {
		packet := tcpCapture(t, flag)
		require.NoError(t, p.writeFrame(packet.Data(), layers.LayerTypeEthernet))
		dropped, e := p.generateRSTFromPacket(packet)
		require.NoError(t, e)
		require.False(t, dropped)
		raw := packet.NetworkLayer().LayerContents()
		raw = append(append([]byte{}, raw...), packet.NetworkLayer().LayerPayload()...)
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(raw)})
		require.NoError(t, p.writePacket(pkt))
		_, e = p.sendRSTPacket(packet.LinkLayer().(*layers.Ethernet), packet.NetworkLayer().(*layers.IPv4), &layers.TCP{RST: true})
		require.NoError(t, e)
	}
	// Even malformed/unknown packets are denied before decoding or encapsulation.
	for _, raw := range [][]byte{nil, {0}, {0x60, 1, 2, 3}, {0x08, 0x06}} {
		require.NoError(t, p.writeFrame(raw, layers.LayerTypeEthernet))
	}
	require.Zero(t, writes.Load())
	p.readOnly.Store(false)
	require.NoError(t, p.writeFrame(tcpCapture(t, 2).Data(), layers.LayerTypeEthernet))
	require.EqualValues(t, 1, writes.Load())
	p.SetPCAPOutboundFilter(func(gopacket.Packet) bool { return false })
	_, e := p.sendRSTPacket(&layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}, &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IPv4(10, 80, 0, 1), DstIP: net.IPv4(10, 80, 0, 2), Protocol: layers.IPProtocolTCP}, &layers.TCP{RST: true})
	require.NoError(t, e)
	require.EqualValues(t, 1, writes.Load())
}
func TestPCAPPassiveReceivesWithoutAutomaticRST(t *testing.T) {
	var writes atomic.Int64
	seen := make(chan struct{}, 1)
	var p *PCAPEndpoint
	v, e := newNetworkVM(context.Background(), NetworkVMConfig{Mode: NetworkPCAP, Device: "mock", OnPacket: func(gopacket.Packet) { seen <- struct{}{} }}, func(ctx context.Context, s *stack.Stack, _ string, _ net.HardwareAddr, _ bool) (*PCAPEndpoint, error) {
		p = mockPCAP(ctx, s, &writes)
		return p, nil
	})
	require.NoError(t, e)
	defer v.Close()
	require.NoError(t, eToStd(v.Stack().AddProtocolAddress(v.nic, tcpip.ProtocolAddress{Protocol: header.IPv4ProtocolNumber, AddressWithPrefix: tcpip.AddressWithPrefix{Address: tcpip.AddrFrom4([4]byte{10, 80, 0, 1}), PrefixLen: 24}}, stack.AddressProperties{})))
	v.Stack().SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: v.nic}})
	p.adaptor.inChan <- tcpCapture(t, 2)
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Fatal("capture not delivered")
	}
	// Wait for gVisor to generate the closed-port response, proving the actual
	// capture->stack->outbound path is exercised, rather than just filter predicates.
	require.Eventually(t, func() bool { return v.Stack().Stats().TCP.SegmentsSent.Value() > 0 }, time.Second, time.Millisecond)
	require.Zero(t, writes.Load())
	_, e = v.DialContext(context.Background(), "tcp", "10.80.0.2:80")
	require.ErrorIs(t, e, ErrPassiveNetwork)
	_, e = v.ListenTCP("10.80.0.1:80")
	require.ErrorIs(t, e, ErrPassiveNetwork)
	require.NoError(t, v.Close())
	require.Zero(t, writes.Load())
}
func eToStd(e tcpip.Error) error {
	if e == nil {
		return nil
	}
	return &stackTestError{s: e.String()}
}

type stackTestError struct{ s string }

func (e *stackTestError) Error() string { return e.s }

func TestPCAPFanoutIndependentSubscribers(t *testing.T) {
	a, b := make(chan gopacket.Packet, 1), make(chan gopacket.Packet, 1)
	p := &pcapFanOut{chans: map[string]chan gopacket.Packet{"a": a, "b": b}}
	raw := tcpCapture(t, 2).Data()
	p.dispatch(raw, gopacket.CaptureInfo{}, layers.LayerTypeEthernet)
	pa, pb := <-a, <-b
	pa.LinkLayer().(*layers.Ethernet).SrcMAC[0] = 0xff
	require.Equal(t, byte(2), pb.LinkLayer().(*layers.Ethernet).SrcMAC[0])
	require.Equal(t, byte(2), raw[6])
	// A full subscriber must not stall the other subscriber.
	a <- pa
	p.dispatch(raw, gopacket.CaptureInfo{}, layers.LayerTypeEthernet)
	require.Len(t, b, 1)
	var closes atomic.Int64
	broker := newPcapBroker(nil, func() { closes.Add(1) }, nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); broker.Close() }()
	}
	wg.Wait()
	require.EqualValues(t, 1, closes.Load())
}
func TestDisableForwardingOption(t *testing.T) {
	c := NewDefaultConfig()
	require.True(t, c.pcapReadOnly)
	require.NoError(t, WithDisableForwarding(true)(c))
	require.True(t, c.DisableForwarding)
}

func TestPCAPLegacyAPIsFailBeforeHostFallback(t *testing.T) {
	var writes atomic.Int64
	p := mockPCAP(context.Background(), nil, &writes)
	defer p.Close()
	entry := &NetStackVirtualMachineEntry{driver: p}
	_, e := entry.DialTCP(time.Second, "127.0.0.1:80")
	require.ErrorIs(t, e, ErrPassiveNetwork)
	_, e = entry.ListenTCP("127.0.0.1:80")
	require.ErrorIs(t, e, ErrPassiveNetwork)
	require.ErrorIs(t, entry.StartDHCP(), ErrPassiveNetwork)
	require.Zero(t, writes.Load())
}
