package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestCapturedDecoderRejectsPPPoEPayloadOutsideItsInput(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, declared := range []uint16{3, 12, 256, 65529, 65530, 65535} {
			wire := []byte{0x11, 0, 0x12, 0x34, 0, 0, 0, 0}
			binary.BigEndian.PutUint16(wire[4:6], declared)
			for _, capacity := range []int{len(wire), 1024, 65536} {
				backing := bytes.Repeat([]byte{0xa5}, capacity)
				copy(backing, wire)
				input := backing[:len(wire)]
				var decoder gopacket.Decoder = captureCarrierDecoder{inner: layers.LayerTypePPPoE}
				if native {
					decoder = protocolPacketDecoder{decoder: layers.LayerTypePPPoE}
				}
				p := gopacket.NewPacket(input, decoder, gopacket.DecodeOptions{NoCopy: true})
				require.Equal(t, wire, p.Data())
				require.NotNil(t, p.ErrorLayer(), "native=%t declared=%d capacity=%d", native, declared, capacity)
				require.Nil(t, p.Layer(layers.LayerTypePPPoE), "missing payload cannot manufacture a namespace from spare buffer capacity")
				require.Empty(t, packetEvidence(p).Ref.Domain.Encapsulation)
			}
		}
	}
}

func TestCapturedDecoderChildCannotReadEthernetPaddingPastIPLength(t *testing.T) {
	innerMAC := net.HardwareAddr{2, 0, 0, 0, 0, 3}
	outer := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocol(97), SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2")}
	inner := make([]byte, 2+14+8)
	inner[0] = 0x30
	copy(inner[2:8], innerMAC)
	copy(inner[8:14], innerMAC)
	binary.BigEndian.PutUint16(inner[14:16], uint16(layers.EthernetTypePPPoESession))
	copy(inner[16:], []byte{0x11, 0, 0x12, 0x34, 0, 12, 0, 0})
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, outer, ip, gopacket.Payload(inner)))
	frame := bytes.Clone(buffer.Bytes())
	// These bytes physically exist, but are beyond the IP envelope. Limiting
	// only the outermost frame's capacity would still expose them to PPPoE.
	frame = append(frame, bytes.Repeat([]byte{0xa5}, 64)...)
	for _, native := range []bool{false, true} {
		var decoder gopacket.Decoder = CaptureLinkDecoder(layers.LinkTypeEthernet)
		if native {
			decoder = protocolPacketDecoder{decoder: layers.LinkTypeEthernet}
		}
		p := gopacket.NewPacket(frame, decoder, gopacket.DecodeOptions{NoCopy: true})
		require.NotNil(t, p.Layer(layers.LayerTypeEtherIP))
		require.NotNil(t, p.ErrorLayer())
		require.Nil(t, p.Layer(layers.LayerTypePPPoE))
		require.Empty(t, packetEvidence(p).Ref.Domain.Encapsulation)
	}
}

func TestCapturedDecoderRetainsCompletePPPoEHeaderAndExactPayload(t *testing.T) {
	wire := []byte{0x11, 0, 0x12, 0x34, 0, 4, 0, 0x21, 0x45, 0}
	for _, native := range []bool{false, true} {
		var decoder gopacket.Decoder = captureCarrierDecoder{inner: layers.LayerTypePPPoE}
		if native {
			decoder = protocolPacketDecoder{decoder: layers.LayerTypePPPoE}
		}
		p := gopacket.NewPacket(wire, decoder, gopacket.DecodeOptions{NoCopy: true})
		pppoe, ok := p.Layer(layers.LayerTypePPPoE).(*layers.PPPoE)
		require.True(t, ok)
		require.Equal(t, wire[:6], pppoe.LayerContents())
		require.Equal(t, wire[6:], pppoe.LayerPayload())
		require.Equal(t, "/pppoe:4660", packetEvidence(p).Ref.Domain.Encapsulation)
		// A malformed child does not invalidate its complete parent header.
		require.NotNil(t, p.ErrorLayer())
	}
}
