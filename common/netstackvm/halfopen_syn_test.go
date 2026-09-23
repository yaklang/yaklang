package netstackvm

import (
	"context"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

func TestOpenHalfOpenSYNRejectsMissingInterface(t *testing.T) {
	_, err := OpenHalfOpenSYN(context.Background(), HalfOpenSYNConfig{})
	if err == nil {
		t.Fatal("expected an error without an interface")
	}
}

func TestHalfOpenSYNACKIsReportedOnceAndNotInjected(t *testing.T) {
	var opened []string
	h := &HalfOpenSYN{
		flights: make(map[flightKey]*synFlight),
		onOpen: func(ip net.IP, port int) {
			opened = append(opened, ip.String()+":"+itoaPort(port))
		},
	}
	h.register(net.ParseIP("192.0.2.10"), 40000, net.ParseIP("192.0.2.20"), 80)
	syn := mustPacket(t, layers.LinkTypeEthernet,
		&layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4},
		&layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.ParseIP("192.0.2.10"), DstIP: net.ParseIP("192.0.2.20")},
		&layers.TCP{SrcPort: 40000, DstPort: 80, SYN: true, Seq: 1000, Window: 1024})
	if !h.observeOutbound(syn) {
		t.Fatal("bare SYN was dropped")
	}

	ackOnly := mustPacket(t, layers.LinkTypeRaw,
		&layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.ParseIP("192.0.2.10"), DstIP: net.ParseIP("192.0.2.20")},
		&layers.TCP{SrcPort: 40000, DstPort: 80, ACK: true, Seq: 1001, Ack: 1})
	if h.observeOutbound(ackOnly) {
		t.Fatal("handshake ACK was allowed out")
	}
	rst := mustPacket(t, layers.LinkTypeRaw,
		&layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.ParseIP("192.0.2.10"), DstIP: net.ParseIP("192.0.2.20")},
		&layers.TCP{SrcPort: 40000, DstPort: 80, RST: true, Seq: 1001})
	if h.observeOutbound(rst) {
		t.Fatal("RST was allowed out")
	}
	arp := mustPacket(t, layers.LinkTypeEthernet,
		&layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, EthernetType: layers.EthernetTypeARP},
		&layers.ARP{AddrType: layers.LinkTypeEthernet, Protocol: layers.EthernetTypeIPv4, HwAddressSize: 6, ProtAddressSize: 4, Operation: layers.ARPRequest, SourceHwAddress: []byte{2, 0, 0, 0, 0, 1}, SourceProtAddress: []byte{192, 0, 2, 10}, DstHwAddress: []byte{0, 0, 0, 0, 0, 0}, DstProtAddress: []byte{192, 0, 2, 1}})
	if !h.observeOutbound(arp) || !h.observeInbound(arp) {
		t.Fatal("ARP must pass so neighbor resolution can finish")
	}

	synAck := mustPacket(t, layers.LinkTypeEthernet,
		&layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, EthernetType: layers.EthernetTypeIPv4},
		&layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.ParseIP("192.0.2.20"), DstIP: net.ParseIP("192.0.2.10")},
		&layers.TCP{SrcPort: 80, DstPort: 40000, SYN: true, ACK: true, Seq: 500, Ack: 1001})
	if h.observeInbound(synAck) {
		t.Fatal("SYN-ACK was injected into the stack")
	}
	if h.observeInbound(synAck) {
		t.Fatal("duplicate SYN-ACK was injected")
	}
	wrong := mustPacket(t, layers.LinkTypeRaw,
		&layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.ParseIP("192.0.2.20"), DstIP: net.ParseIP("192.0.2.10")},
		&layers.TCP{SrcPort: 80, DstPort: 40000, SYN: true, ACK: true, Seq: 9, Ack: 1})
	h.observeInbound(wrong)
	if len(opened) != 1 || opened[0] != "192.0.2.20:80" {
		t.Fatalf("opened = %v", opened)
	}
}

func TestHalfOpenLoopbackSYNIsVisible(t *testing.T) {
	h := &HalfOpenSYN{flights: make(map[flightKey]*synFlight)}
	h.register(net.ParseIP("127.0.0.1"), 9, net.ParseIP("127.0.0.1"), 80)
	pkt := mustPacket(t, layers.LinkTypeLoop,
		&layers.Loopback{Family: layers.ProtocolFamilyIPv4},
		&layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.ParseIP("127.0.0.1"), DstIP: net.ParseIP("127.0.0.1")},
		&layers.TCP{SrcPort: 9, DstPort: 80, SYN: true, Seq: 7})
	if !h.observeOutbound(pkt) {
		t.Fatal("loopback SYN was not recognized")
	}
	key := flightKey{local: ip4key(net.ParseIP("127.0.0.1")), port: 9}
	if !h.flights[key].haveISN || h.flights[key].isn != 7 {
		t.Fatalf("flight = %+v", h.flights[key])
	}
}

func mustPacket(t *testing.T, link layers.LinkType, parts ...gopacket.SerializableLayer) gopacket.Packet {
	t.Helper()
	var network gopacket.NetworkLayer
	for _, part := range parts {
		if n, ok := part.(gopacket.NetworkLayer); ok {
			network = n
		}
	}
	for _, part := range parts {
		tcpLayer, ok := part.(*layers.TCP)
		if ok && network != nil {
			if err := tcpLayer.SetNetworkLayerForChecksum(network); err != nil {
				t.Fatal(err)
			}
		}
	}
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, parts...); err != nil {
		t.Fatal(err)
	}
	decode := link
	if link == layers.LinkTypeRaw || link == layers.LinkTypeIPv4 {
		decode = layers.LinkTypeIPv4
	}
	return gopacket.NewPacket(buf.Bytes(), decode, gopacket.Default)
}

func itoaPort(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
