package pcaputil

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func replayObservedIPv4EOL(t *testing.T, wire []byte, link layers.LinkType) ([]*ProtocolEvent, error) {
	t.Helper()
	var input bytes.Buffer
	w := pcapgo.NewWriter(&input)
	require.NoError(t, w.WriteFileHeader(65535, link))
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(42, 0), CaptureLength: len(wire), Length: len(wire)}, wire))
	var events []*ProtocolEvent
	err := ReplayPcap(bytes.NewReader(input.Bytes()), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
	return events, err
}

func observedIPv4EOLWire(t *testing.T) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 37, TOS: 0x10, Id: 0x3456, Flags: layers.IPv4DontFragment, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2"), Options: []layers.IPv4Option{{OptionType: 0, OptionLength: 1}}}
	udp := &layers.UDP{SrcPort: 53, DstPort: 40000}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	dns := &layers.DNS{ID: 0x1234, QR: true, Questions: []layers.DNSQuestion{{Name: []byte("eol.example"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}
	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, dns))
	wire := append([]byte(nil), buf.Bytes()...)
	require.Equal(t, byte(0x46), wire[14])
	require.Equal(t, []byte{0, 0, 0, 0}, wire[34:38])
	return wire
}

func TestObservedIPv4EOLRetainsFixedFieldsAndApplicationView(t *testing.T) {
	wire := observedIPv4EOLWire(t)
	layouts := map[string]struct {
		wire []byte
		link layers.LinkType
	}{
		"Ethernet": {wire, layers.LinkTypeEthernet},
		"Raw":      {wire[14:], layers.LinkTypeRaw},
		"IPv4":     {wire[14:], layers.LinkTypeIPv4},
		"Loopback": {append([]byte{2, 0, 0, 0}, wire[14:]...), layers.LinkTypeNull},
		"SLL":      {append([]byte{0, 0, 0, 1, 0, 6, 2, 0, 0, 0, 0, 1, 0, 0, 8, 0}, wire[14:]...), layers.LinkTypeLinuxSLL},
	}
	for name, layout := range layouts {
		t.Run(name, func(t *testing.T) {
			observer := gopacket.NewPacket(layout.wire, CaptureLinkDecoder(layout.link), gopacket.Default)
			for _, packet := range []gopacket.Packet{observer, protocolAnalysisPacket(observer, nil)} {
				require.Nil(t, packet.ErrorLayer())
				ip, ok := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
				require.True(t, ok)
				require.Equal(t, uint8(4), ip.Version)
				require.Equal(t, uint8(6), ip.IHL)
				require.Equal(t, uint8(37), ip.TTL)
				require.Equal(t, uint8(0x10), ip.TOS)
				require.Equal(t, uint16(0x3456), ip.Id)
				require.Equal(t, layers.IPv4DontFragment, ip.Flags)
				require.Zero(t, ip.FragOffset)
				require.Equal(t, layers.IPProtocolUDP, ip.Protocol)
				require.Equal(t, "198.51.100.1", ip.SrcIP.String())
				require.Equal(t, "198.51.100.2", ip.DstIP.String())
				require.Equal(t, wire[14:38], ip.Contents)
				require.Equal(t, wire[38:], ip.Payload)
				udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
				require.True(t, ok)
				require.Equal(t, layers.UDPPort(53), udp.SrcPort)
				require.Equal(t, layers.UDPPort(40000), udp.DstPort)
				require.Nil(t, packet.Layer(layers.LayerTypeIPv6HopByHop))
			}
			// Observers retain their normal application view; private native analysis
			// still owns the independent bounded datagram codec.
			require.NotNil(t, observer.Layer(layers.LayerTypeDNS))
			events, err := replayObservedIPv4EOL(t, layout.wire, layout.link)
			require.NoError(t, err)
			require.Len(t, events, 1)
			require.Equal(t, "dns", events[0].Protocol)
			require.Equal(t, "decoded", events[0].Status)
			require.Equal(t, "198.51.100.1:53", events[0].Source)
			require.Equal(t, "198.51.100.2:40000", events[0].Destination)
		})
	}
}

func TestObservedIPv4EOLCannotRepairFailedOptionsOrWrongVersion(t *testing.T) {
	for _, name := range []string{"bad-option-length", "wrong-version"} {
		t.Run(name, func(t *testing.T) {
			wire := observedIPv4EOLWire(t)
			if name == "bad-option-length" {
				wire[34], wire[35] = 0x44, 0xff
			} else {
				wire[14] = 0x66
			}
			observer := gopacket.NewPacket(wire, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			for _, packet := range []gopacket.Packet{observer, protocolAnalysisPacket(observer, nil)} {
				require.NotNil(t, packet.ErrorLayer())
				require.Nil(t, packet.Layer(layers.LayerTypeUDP))
				require.Nil(t, packet.Layer(layers.LayerTypeDNS))
			}
			events, err := replayObservedIPv4EOL(t, wire, layers.LinkTypeEthernet)
			require.Error(t, err, "failed parents retain their strict replay error")
			for _, event := range events {
				require.NotEqual(t, "dns", event.Protocol)
				require.Zero(t, event.FlowID)
			}
		})
	}
}

func TestObservedIPv4EOLFragmentFlagsCannotExposeAnApplication(t *testing.T) {
	full := observedIPv4EOLWire(t)
	for _, first := range []bool{true, false} {
		t.Run(map[bool]string{true: "first-more", false: "tail-without-first"}[first], func(t *testing.T) {
			eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
			ip := &layers.IPv4{Version: 4, TTL: 37, Id: 0x3456, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2"), Options: []layers.IPv4Option{{OptionType: 0, OptionLength: 1}}}
			payload := full[38+24:]
			ip.FragOffset = 3
			if first {
				ip.FragOffset, ip.Flags, payload = 0, layers.IPv4MoreFragments, full[38:38+24]
			}
			buf := gopacket.NewSerializeBuffer()
			require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(payload)))
			wire := bytes.Clone(buf.Bytes())
			observer := gopacket.NewPacket(wire, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			for _, packet := range []gopacket.Packet{observer, protocolAnalysisPacket(observer, nil)} {
				require.Nil(t, packet.ErrorLayer())
				decoded := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
				require.Equal(t, ip.Flags, decoded.Flags)
				require.Equal(t, ip.FragOffset, decoded.FragOffset)
				require.NotNil(t, packet.Layer(gopacket.LayerTypeFragment))
				require.Nil(t, packet.Layer(layers.LayerTypeUDP))
				require.Nil(t, packet.Layer(layers.LayerTypeDNS))
			}
			events, err := replayObservedIPv4EOL(t, wire, layers.LinkTypeEthernet)
			require.NoError(t, err)
			for _, event := range events {
				require.NotEqual(t, "dns", event.Protocol)
				require.Zero(t, event.FlowID)
			}
		})
	}
}

func TestObservedRawIPv6HopOptionsRetainTheirBoundedParent(t *testing.T) {
	esp := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1}
	for _, malformed := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid-first-hop", true: "option-overruns-extension"}[malformed], func(t *testing.T) {
			hop := []byte{50, 0, 0, 0, 0, 0, 0, 0}
			if malformed {
				hop[2], hop[3] = 0x1e, 5
			}
			frame := espNativeFrame(t, append(hop, esp...), true, layers.IPProtocolIPv6HopByHop)
			for _, link := range []layers.LinkType{layers.LinkTypeRaw, layers.LinkTypeIPv6} {
				observer := gopacket.NewPacket(frame[14:], CaptureLinkDecoder(link), gopacket.Default)
				for _, packet := range []gopacket.Packet{observer, protocolAnalysisPacket(observer, nil)} {
					ip := packet.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
					require.Equal(t, frame[14:54], ip.Contents)
					require.Equal(t, "2001:db8::1", ip.SrcIP.String())
					if malformed {
						require.NotNil(t, packet.ErrorLayer())
						require.ErrorContains(t, packet.ErrorLayer().Error(), "exceeds its extension header")
						require.Nil(t, ip.HopByHop)
						require.Nil(t, packet.Layer(layers.LayerTypeIPSecESP))
					} else {
						require.Nil(t, packet.ErrorLayer())
						require.NotNil(t, ip.HopByHop)
						require.Equal(t, esp, packet.Layer(layers.LayerTypeIPSecESP).LayerContents())
					}
				}
			}
		})
	}
}
