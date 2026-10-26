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
func protocolAnalysisPacket(packet gopacket.Packet, selections map[uint16]string) gopacket.Packet {
	decoded := packet.Layers()
	if len(decoded) == 0 || decoded[0].LayerType() == gopacket.LayerTypeDecodeFailure {
		return packet
	}
	if _, ok := decoded[0].(*socketCANLayer); ok {
		// This local link decoder already keeps only the capture record. Its
		// Payload layer tag must not be used to recreate an untyped packet.
		return packet
	}
	decoder := gopacket.Decoder(decoded[0].LayerType())
	if _, ok := decoded[0].(*capturePPILayer); ok {
		decoder = capturePPIDecoder{}
	}
	if _, ok := decoded[0].(*captureMPacket); ok {
		decoder = captureMPacketDecoder{}
	}
	ci := packet.Metadata().CaptureInfo
	p := gopacket.NewPacket(packet.Data(), protocolPacketDecoder{decoder: decoder, captureTruncated: ci.CaptureLength < ci.Length, datagramDecodeAs: selections}, gopacket.DecodeOptions{Lazy: true, NoCopy: true, DecodeStreamsAsDatagrams: false})
	p.Metadata().CaptureInfo = ci
	return p
}

type protocolPacketDecoder struct {
	decoder          gopacket.Decoder
	network          gopacket.NetworkLayer
	captureTruncated bool
	datagramDecodeAs map[uint16]string
}

func (d protocolPacketDecoder) Decode(data []byte, builder gopacket.PacketBuilder) error {
	b := &protocolPacketBuilder{PacketBuilder: builder, network: d.network, captureTruncated: d.captureTruncated, datagramDecodeAs: d.datagramDecodeAs}
	decoder := d.decoder
	if decoder == layers.LayerTypeGRE {
		decoder = captureGREDecoder{}
	}
	if decoder == layers.EthernetType(0x22eb) {
		decoder = captureERSPAN3Decoder{}
	}
	if decoder == layers.LayerTypeDot1Q {
		decoder = captureVLANDecoder{}
	}
	if link, ok := decoder.(layers.LinkType); ok {
		decoder = captureLinkDecoder(link)
		if link == layers.LinkTypeRaw && len(data) > 0 {
			switch data[0] >> 4 {
			case 4:
				decoder = layers.LayerTypeIPv4
			case 6:
				decoder = layers.LayerTypeIPv6
			}
		}
	}
	if typed, ok := decoder.(interface{ LayerType() gopacket.LayerType }); ok && (typed.LayerType() == layers.LayerTypeIPv4 || typed.LayerType() == layers.LayerTypeIPv6) {
		decoder = typed.LayerType()
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
	if layer, ok := decoder.(gopacket.LayerType); ok && layer == layers.LayerTypeIPv6 {
		ip := &layers.IPv6{}
		err := decodeProtocolIPv6(ip, data, b)
		b.AddLayer(ip)
		b.SetNetworkLayer(ip)
		if ip.HopByHop != nil {
			b.AddLayer(ip.HopByHop)
		}
		if err != nil {
			return err
		}
		return b.NextDecoder(ip.NextLayerType())
	}
	return decoder.Decode(data, b)
}

type protocolPacketBuilder struct {
	gopacket.PacketBuilder
	udp              *layers.UDP
	network          gopacket.NetworkLayer
	captureTruncated bool
	datagramDecodeAs map[uint16]string
	pim              gopacket.NetworkLayer
}

func (b *protocolPacketBuilder) SetNetworkLayer(n gopacket.NetworkLayer) {
	b.PacketBuilder.SetNetworkLayer(n)
	b.network = n
	b.pim = pimNetwork(n)
}

func (b *protocolPacketBuilder) SetTransportLayer(layer gopacket.TransportLayer) {
	b.PacketBuilder.SetTransportLayer(layer)
	b.udp, _ = layer.(*layers.UDP)
}

func udpEncapsulation(next gopacket.LayerType) bool {
	return next == layers.LayerTypeVXLAN || next == layers.LayerTypeGeneve || next == layers.LayerTypeGTPv1U
}

func (b *protocolPacketBuilder) NextDecoder(next gopacket.Decoder) error {
	if b.pim != nil {
		next = capturePIMDecoder{network: b.pim}
		b.pim = nil
	}
	// The carrier adapter preserves legacy display decoding. Native dispatch
	// must inspect the actual next layer before deciding whether a UDP payload
	// is an encapsulation; an opaque wrapper must not hide Geneve/VXLAN here.
	if carrier, ok := next.(captureCarrierDecoder); ok {
		next = carrier.inner
	}
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
	if b.udp != nil {
		layer, ok := next.(gopacket.LayerType)
		selected := b.datagramDecodeAs[uint16(b.udp.DstPort)] != "" || b.datagramDecodeAs[uint16(b.udp.SrcPort)] != ""
		if selected || !ok || !udpEncapsulation(layer) || b.network != nil && udpCarrierError(b.network, b.udp, gopacket.CaptureInfo{}, b.captureTruncated) != nil {
			// A user-selected native codec owns the complete UDP payload before
			// a legacy tunnel port can substitute an inner network/transport.
			// Its own grammar and carrier checks still decide acceptance.
			// Do not let a service-selected tunnel expose an inner IP/transport
			// before its literal outer UDP bounds are known. Keep the outer UDP
			// layer so normal dispatch can publish its precise typed refusal and
			// invalidate owned protocol state. Observer packets stay unchanged.
			next = gopacket.LayerTypePayload
		}
	}
	return b.PacketBuilder.NextDecoder(protocolPacketDecoder{decoder: next, network: b.network, captureTruncated: b.captureTruncated, datagramDecodeAs: b.datagramDecodeAs})
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

type protocolDecodeFeedback struct{ truncated bool }

func (f *protocolDecodeFeedback) SetTruncated() { f.truncated = true }

// RFC 8200 Payload Length includes the Hop-by-Hop header. gopacket 1.3.1
// removes that header, then applies the original length to the remaining bytes.
// Bound its input to the literal IP envelope and repair only this proven offset;
// observers keep the dependency's original view and malformed options still fail.
func decodeProtocolIPv6(ip *layers.IPv6, data []byte, feedback gopacket.DecodeFeedback) error {
	*ip = layers.IPv6{}
	if len(data) < 40 {
		feedback.SetTruncated()
		return fmt.Errorf("IPv6 header truncated: %d of 40 bytes", len(data))
	}
	if data[0]>>4 != 6 {
		return fmt.Errorf("invalid IPv6 version %d", data[0]>>4)
	}
	declared := int(binary.BigEndian.Uint16(data[4:6]))
	if declared == 0 {
		// The native path has no selected jumbogram profile. In particular do
		// not pass the dependency's HBH-prefixed jumbo payload to a UDP decoder.
		return fmt.Errorf("IPv6 zero payload length/jumbogram requires an unsupported native carrier profile")
	}
	end := 40 + declared
	if end > len(data) {
		feedback.SetTruncated()
		end = len(data)
	}
	bounded := data[:end]
	var local protocolDecodeFeedback
	if err := ip.DecodeFromBytes(bounded, &local); err != nil {
		if local.truncated {
			feedback.SetTruncated()
		}
		return err
	}
	if ip.HopByHop != nil {
		n := ip.HopByHop.ActualLength
		if n > declared || 40+n > end {
			return fmt.Errorf("IPv6 Hop-by-Hop header exceeds declared payload length")
		}
		ip.Payload = bounded[40+n:]
		ip.HopByHop.Payload = ip.Payload
	} else if local.truncated {
		feedback.SetTruncated()
	}
	return nil
}

// capturedTCPPayloadMissing admits only a reader-proven capture suffix after
// complete IP/TCP headers. Wire-invalid lengths, truncated options and fragments
// remain rejected. Missing bytes are never manufactured or reserved as payload.
func capturedTCPPayloadMissing(packet gopacket.Packet, tcp *layers.TCP) (uint32, bool) {
	ci := packet.Metadata().CaptureInfo
	if ci.CaptureLength != len(packet.Data()) || ci.Length <= ci.CaptureLength || tcp == nil || len(tcp.LayerContents()) < 20 || len(tcp.LayerContents()) != int(tcp.DataOffset)*4 || packet.ErrorLayer() != nil {
		return 0, false
	}
	var declared, captured int
	switch ip := packet.NetworkLayer().(type) {
	case *layers.IPv4:
		if ip.Version != 4 || ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 || ip.Protocol != layers.IPProtocolTCP {
			return 0, false
		}
		declared = int(ip.Length)
		captured = len(ip.LayerContents()) + len(ip.LayerPayload())
	case *layers.IPv6:
		if ip.Version != 6 || ip.Length == 0 || packet.Layer(layers.LayerTypeIPv6Fragment) != nil {
			return 0, false
		}
		declared = 40 + int(ip.Length)
		captured = len(ip.LayerContents()) + len(ip.LayerPayload())
		if ip.HopByHop != nil {
			captured += ip.HopByHop.ActualLength
		}
	default:
		return 0, false
	}
	offset := len(packet.Data()) - captured
	if offset < 0 || declared <= captured || declared > ci.Length-offset || captured < len(tcp.LayerContents())+len(tcp.Payload) {
		return 0, false
	}
	return uint32(declared - captured), true
}
