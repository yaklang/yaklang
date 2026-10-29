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
	// A child may only read its declared input, even when the parent slice
	// retains capacity for frame padding, another record or a reusable buffer.
	data = data[:len(data):len(data)]
	b := &protocolPacketBuilder{PacketBuilder: builder, network: d.network, captureTruncated: d.captureTruncated, datagramDecodeAs: d.datagramDecodeAs}
	decoder := d.decoder
	if _, observed := decoder.(captureOSPFv3Decoder); observed {
		decoder = gopacket.LayerTypePayload
	}
	if _, ipv6 := d.network.(*layers.IPv6); ipv6 && (decoder == layers.LayerTypeOSPF || decoder == layers.IPProtocolOSPF) {
		// Native admission owns the complete bounded OSPFv3 codec. The
		// dependency's LSA prefix walker mixes bit counts and byte offsets.
		decoder = gopacket.LayerTypePayload
	}
	if decoder == layers.LayerTypeGRE {
		decoder = captureGREDecoder{}
	}
	if decoder == layers.LayerTypeIPSecESP || decoder == layers.IPProtocolESP {
		decoder = captureESPDecoder{}
	}
	if decoder == layers.LayerTypeIPSecAH || decoder == layers.IPProtocolAH {
		_, v6 := d.network.(*layers.IPv6)
		decoder = captureAHDecoder{ipv6: v6}
	}
	if _, ah := decoder.(captureAHDecoder); ah {
		_, v6 := d.network.(*layers.IPv6)
		decoder = captureAHDecoder{ipv6: v6}
	}
	if decoder == layers.LayerTypeIPv6Destination || decoder == layers.IPProtocolIPv6Destination {
		decoder = captureIPv6OptionsDecoder{}
	}
	if decoder == layers.LayerTypeIPv6HopByHop || decoder == layers.IPProtocolIPv6HopByHop {
		decoder = captureIPv6OptionsDecoder{hop: true}
	}
	if decoder == layers.LayerTypeVXLAN {
		decoder = captureVXLANDecoder{}
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
	lastLayer        gopacket.Layer
}

func (b *protocolPacketBuilder) AddLayer(layer gopacket.Layer) {
	b.PacketBuilder.AddLayer(layer)
	b.lastLayer = layer
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
	if _, observerVXLAN := next.(captureVXLANObserverDecoder); observerVXLAN {
		// The observer fixes field bits without imposing native admission.
		// Recover the literal carrier kind for bounds and strict I validation.
		next = layers.LayerTypeVXLAN
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
		gpe := b.udp.DstPort == 4790 && b.udp.DstPort.LayerType() == gopacket.LayerTypePayload && observedVXLANGPE(b.udp.LayerPayload())
		// Source-port fallback is a hint, not a configured tunnel receiver.
		// Honor real registered destination ports, including custom VXLAN ports.
		sourceOnlyVXLAN := !gpe && ok && layer == layers.LayerTypeVXLAN && b.udp.DstPort.LayerType() != layers.LayerTypeVXLAN
		if selected || sourceOnlyVXLAN || !gpe && (!ok || !udpEncapsulation(layer)) || b.network != nil && udpCarrierError(b.network, b.udp, gopacket.CaptureInfo{}, b.captureTruncated) != nil {
			// A user-selected native codec owns the complete UDP payload before
			// a legacy tunnel port can substitute an inner network/transport.
			// Its own grammar and carrier checks still decide acceptance.
			// Do not let a service-selected tunnel expose an inner IP/transport
			// before its literal outer UDP bounds are known. Keep the outer UDP
			// layer so normal dispatch can publish its precise typed refusal and
			// invalidate owned protocol state. Observer packets stay unchanged.
			next = gopacket.LayerTypePayload
		} else if gpe {
			next = captureVXLANGPEDecoder{}
		}
	}
	decoder := protocolPacketDecoder{decoder: next, network: b.network, captureTruncated: b.captureTruncated, datagramDecodeAs: b.datagramDecodeAs}
	if b.lastLayer != nil && len(b.lastLayer.LayerPayload()) == 0 {
		layer, ok := next.(gopacket.LayerType)
		if !ok {
			if typed, found := next.(interface{ LayerType() gopacket.LayerType }); found {
				layer = typed.LayerType()
			}
		}
		if layer == layers.LayerTypeIPv4 || layer == layers.LayerTypeIPv6 {
			// The dependency normally stops at any empty payload. A declared
			// fixed IP header still requires bytes: report the zero-byte case
			// with the same typed truncation path as other incomplete prefixes.
			return decoder.Decode(nil, b.PacketBuilder)
		}
		if layer == layers.LayerTypeVXLAN || layer == layers.LayerTypeIPSecESP || layer == layers.LayerTypeIPSecAH || layer == layers.LayerTypeIPv6Destination || layer == layers.LayerTypeIPv6HopByHop || next == layers.IPProtocolESP || next == layers.IPProtocolAH {
			return decoder.Decode(nil, b.PacketBuilder)
		}
	}
	return b.PacketBuilder.NextDecoder(decoder)
}

type ipVersionError struct{ observed, expected uint8 }

func (e *ipVersionError) Error() string {
	return fmt.Sprintf("invalid IPv%d version %d", e.expected, e.observed)
}

type ipHeaderTruncatedError struct {
	expected            uint8
	available, required int
	observed            uint8
	hasVersion          bool
	cause               error
}

func (e *ipHeaderTruncatedError) Error() string { return e.cause.Error() }
func (e *ipHeaderTruncatedError) Unwrap() error { return e.cause }

func truncatedIPHeader(data []byte, expected uint8, required int, cause error) error {
	e := &ipHeaderTruncatedError{expected: expected, available: len(data), required: required, cause: cause}
	if len(data) > 0 {
		e.observed, e.hasVersion = data[0]>>4, true
	}
	return e
}

// gopacket 1.3.1 returns at the IPv4 end-of-options marker before assigning
// fixed header fields. Reset reusable state and finish those fields only after
// its length/option checks succeed. Both fresh and private decoding share this
// adapter, so valid EOL padding is accepted without borrowing prior endpoints.
func decodeProtocolIPv4(ip *layers.IPv4, data []byte, feedback gopacket.DecodeFeedback) error {
	*ip = layers.IPv4{Options: ip.Options[:0]}
	// Validate the complete fixed header before the dependency can expose
	// partial addresses/fragment flags and return an unrelated option error.
	// A short captured header still takes its existing truncation path.
	if len(data) >= 20 && data[0]>>4 != 4 {
		return &ipVersionError{observed: data[0] >> 4, expected: 4}
	}
	if err := ip.DecodeFromBytes(data, feedback); err != nil {
		if len(data) < 20 {
			return truncatedIPHeader(data, 4, 20, err)
		}
		return err
	}
	restoreIPv4FixedFieldsAfterOptions(ip, data)
	if ip.Version != 4 {
		return &ipVersionError{observed: data[0] >> 4, expected: 4}
	}
	return nil
}

// Call only after DecodeFromBytes succeeds. The dependency returns at EOL
// before filling these fixed fields, even though the complete header/options
// have already been checked. Keep observer and native views on the same bytes.
func restoreIPv4FixedFieldsAfterOptions(ip *layers.IPv4, data []byte) {
	if ip.Version != 0 || len(data) < 20 || data[0]>>4 != 4 {
		return
	}
	flags := binary.BigEndian.Uint16(data[6:8])
	ip.Version, ip.TOS = data[0]>>4, data[1]
	ip.Id = binary.BigEndian.Uint16(data[4:6])
	ip.Flags, ip.FragOffset = layers.IPv4Flag(flags>>13), flags&0x1fff
	ip.TTL, ip.Protocol = data[8], layers.IPProtocol(data[9])
	ip.Checksum = binary.BigEndian.Uint16(data[10:12])
	ip.SrcIP, ip.DstIP = net.IP(data[12:16]), net.IP(data[16:20])
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
		return truncatedIPHeader(data, 6, 40, fmt.Errorf("IPv6 header truncated: %d of 40 bytes", len(data)))
	}
	if data[0]>>4 != 6 {
		return &ipVersionError{observed: data[0] >> 4, expected: 6}
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
	if data[6] == byte(layers.IPProtocolIPv6HopByHop) {
		// Preserve the complete, validated fixed parent even if an option is
		// malformed. Only a successful extension may strip its payload prefix.
		ip.Version, ip.TrafficClass, ip.FlowLabel = data[0]>>4, uint8(binary.BigEndian.Uint16(data[:2])>>4), binary.BigEndian.Uint32(data[:4])&0xfffff
		ip.Length, ip.NextHeader, ip.HopLimit = uint16(declared), layers.IPProtocol(data[6]), data[7]
		ip.SrcIP, ip.DstIP = net.IP(data[8:24]), net.IP(data[24:40])
		ip.Contents, ip.Payload = bounded[:40], bounded[40:]
		hop, err := decodeBoundedIPv6HopByHop(bounded[40:], feedback)
		if err != nil {
			return err
		}
		for _, option := range hop.Options {
			if option.OptionType == layers.IPv6HopByHopOptionJumbogram {
				return fmt.Errorf("IPv6 jumbo option requires an unsupported zero-length carrier profile")
			}
		}
		ip.Payload, ip.HopByHop = hop.Payload, hop
		return nil
	}
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
