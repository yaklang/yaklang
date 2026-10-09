package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Packet observers keep gopacket's original application view. Native protocol
// parsing uses a separate network/transport view: a legacy UDP application
// decoder must not mark a valid datagram truncated before our codec sees it.
func protocolAnalysisPacket(packet gopacket.Packet) gopacket.Packet {
	decoded := packet.Layers()
	if len(decoded) == 0 || decoded[0].LayerType() == gopacket.LayerTypeDecodeFailure {
		return packet
	}
	if _, ok := decoded[0].(*socketCANLayer); ok {
		// This local link decoder already keeps only the capture record. Its
		// Payload layer tag must not be used to recreate an untyped packet.
		return packet
	}
	p := gopacket.NewPacket(packet.Data(), protocolPacketDecoder{decoder: decoded[0].LayerType()}, gopacket.DecodeOptions{Lazy: true, NoCopy: true, DecodeStreamsAsDatagrams: false})
	p.Metadata().CaptureInfo = packet.Metadata().CaptureInfo
	return p
}

type protocolPacketDecoder struct{ decoder gopacket.Decoder }

func (d protocolPacketDecoder) Decode(data []byte, builder gopacket.PacketBuilder) error {
	b := &protocolPacketBuilder{PacketBuilder: builder}
	decoder := d.decoder
	if link, ok := decoder.(layers.LinkType); ok {
		decoder = captureLinkDecoder(link)
		if link == layers.LinkTypeRaw && len(data) > 0 && data[0]>>4 == 4 {
			decoder = layers.LayerTypeIPv4
		}
	}
	if typed, ok := decoder.(interface{ LayerType() gopacket.LayerType }); ok && typed.LayerType() == layers.LayerTypeIPv4 {
		decoder = layers.LayerTypeIPv4
	}
	if layer, ok := decoder.(gopacket.LayerType); ok && layer == layers.LayerTypeIPv4 {
		ip := &layers.IPv4{}
		err := decodeProtocolIPv4(ip, data, b)
		b.AddLayer(ip)
		b.SetNetworkLayer(ip)
		if err != nil {
			return err
		}
		return b.NextDecoder(ip.NextLayerType())
	}
	return decoder.Decode(data, b)
}

type protocolPacketBuilder struct {
	gopacket.PacketBuilder
	udp bool
}

func (b *protocolPacketBuilder) SetTransportLayer(layer gopacket.TransportLayer) {
	b.PacketBuilder.SetTransportLayer(layer)
	_, b.udp = layer.(*layers.UDP)
}

func udpEncapsulation(next gopacket.LayerType) bool {
	return next == layers.LayerTypeVXLAN || next == layers.LayerTypeGeneve || next == layers.LayerTypeGTPv1U
}

func (b *protocolPacketBuilder) NextDecoder(next gopacket.Decoder) error {
	// These complete Ethernet PDUs have bounded native codecs. Do not invoke
	// gopacket's unknown-EtherType decoder before their ingress can run.
	if kind, ok := next.(layers.EthernetType); ok && (kind == 0x88ba || kind == 0x88a4) {
		next = gopacket.LayerTypePayload
	}
	// The bounded native LLDP codec validates its own complete TLV layout.
	// Observer packets retain the original gopacket view.
	if layer, ok := next.(gopacket.LayerType); ok && layer == layers.LayerTypeLinkLayerDiscovery {
		next = gopacket.LayerTypePayload
	}
	if b.udp {
		layer, ok := next.(gopacket.LayerType)
		if !ok || !udpEncapsulation(layer) {
			next = gopacket.LayerTypePayload
		}
	}
	return b.PacketBuilder.NextDecoder(protocolPacketDecoder{decoder: next})
}

// gopacket 1.3.1 returns at the IPv4 end-of-options marker before assigning
// fixed header fields. Reset reusable state and finish those fields only after
// its length/option checks succeed. Both fresh and private decoding share this
// adapter, so valid EOL padding is accepted without borrowing prior endpoints.
func decodeProtocolIPv4(ip *layers.IPv4, data []byte, feedback gopacket.DecodeFeedback) error {
	*ip = layers.IPv4{Options: ip.Options[:0]}
	if err := ip.DecodeFromBytes(data, feedback); err != nil {
		return err
	}
	if ip.Version == 0 && data[0]>>4 == 4 {
		flags := binary.BigEndian.Uint16(data[6:8])
		ip.Version, ip.TOS = data[0]>>4, data[1]
		ip.Id = binary.BigEndian.Uint16(data[4:6])
		ip.Flags, ip.FragOffset = layers.IPv4Flag(flags>>13), flags&0x1fff
		ip.TTL, ip.Protocol = data[8], layers.IPProtocol(data[9])
		ip.Checksum = binary.BigEndian.Uint16(data[10:12])
		ip.SrcIP, ip.DstIP = net.IP(data[12:16]), net.IP(data[16:20])
	}
	if ip.Version != 4 {
		return fmt.Errorf("invalid IPv4 version %d", data[0]>>4)
	}
	return nil
}
