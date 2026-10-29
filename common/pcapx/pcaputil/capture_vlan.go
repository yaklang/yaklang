package pcaputil

import (
	"encoding/binary"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// A VLAN tag's final two bytes can be an IEEE 802.3 length rather than an
// EtherType. gopacket's Dot1Q decoder only dispatches the EtherType form.
// Preserve normal application decoding while fixing that local carrier edge.
type captureCarrierDecoder struct {
	inner   gopacket.Decoder
	network gopacket.NetworkLayer
}
type captureCarrierBuilder struct {
	gopacket.PacketBuilder
	pim     gopacket.NetworkLayer
	udp     *layers.UDP
	last    gopacket.Layer
	network gopacket.NetworkLayer
}
type captureVLANDecoder struct{}

func (d captureCarrierDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	// Apply the same parent-envelope boundary in packet observers and native
	// ingress. Dependency reslicing must not expose unused backing capacity.
	data = data[:len(data):len(data)]
	wrapped := &captureCarrierBuilder{PacketBuilder: b, network: d.network}
	if d.inner == layers.LinkTypeRaw && len(data) > 0 {
		switch data[0] >> 4 {
		case 4:
			d.inner = layers.LayerTypeIPv4
		case 6:
			d.inner = layers.LayerTypeIPv6
		}
	}
	if typed, ok := d.inner.(interface{ LayerType() gopacket.LayerType }); ok && (typed.LayerType() == layers.LayerTypeIPv4 || typed.LayerType() == layers.LayerTypeIPv6) {
		d.inner = typed.LayerType()
	}
	if d.inner == layers.LayerTypeIPv4 || d.inner == layers.IPProtocolIPv4 || d.inner == layers.EthernetTypeIPv4 {
		ip := &layers.IPv4{}
		err := ip.DecodeFromBytes(data, wrapped)
		if err == nil {
			restoreIPv4FixedFieldsAfterOptions(ip, data)
		}
		wrapped.AddLayer(ip)
		wrapped.SetNetworkLayer(ip)
		if err != nil {
			return err
		}
		return wrapped.NextDecoder(ip.NextLayerType())
	}
	if (d.inner == layers.LayerTypeIPv6 || d.inner == layers.IPProtocolIPv6 || d.inner == layers.EthernetTypeIPv6) && len(data) >= 40 && data[6] == 0 && binary.BigEndian.Uint16(data[4:6]) != 0 {
		// First HBH options are decoded inside IPv6 by the dependency. Use the
		// same bounded adapter for observers; retain legacy jumbo handling.
		ip := &layers.IPv6{}
		err := decodeProtocolIPv6(ip, data, wrapped)
		wrapped.AddLayer(ip)
		wrapped.SetNetworkLayer(ip)
		if ip.HopByHop != nil {
			wrapped.AddLayer(ip.HopByHop)
		}
		if err != nil {
			return err
		}
		return wrapped.NextDecoder(ip.NextLayerType())
	}
	return d.inner.Decode(data, wrapped)
}
func (b *captureCarrierBuilder) AddLayer(layer gopacket.Layer) {
	b.PacketBuilder.AddLayer(layer)
	b.last = layer
}
func (b *captureCarrierBuilder) SetNetworkLayer(n gopacket.NetworkLayer) {
	b.PacketBuilder.SetNetworkLayer(n)
	b.pim = pimNetwork(n)
	b.network = n
}
func (b *captureCarrierBuilder) SetTransportLayer(layer gopacket.TransportLayer) {
	b.PacketBuilder.SetTransportLayer(layer)
	b.udp, _ = layer.(*layers.UDP)
}
func (b *captureCarrierBuilder) NextDecoder(next gopacket.Decoder) error {
	if _, ipv4 := b.network.(*layers.IPv4); ipv4 && (next == layers.LayerTypeOSPF || next == layers.IPProtocolOSPF) {
		next = captureOSPFv2Decoder{}
	}
	if _, ipv6 := b.network.(*layers.IPv6); ipv6 && (next == layers.LayerTypeOSPF || next == layers.IPProtocolOSPF) {
		next = captureOSPFv3Decoder{network: b.network}
	}
	if next == layers.LayerTypeIPSecESP || next == layers.IPProtocolESP {
		next = captureESPDecoder{}
	}
	if next == layers.LayerTypeIPSecAH || next == layers.IPProtocolAH {
		_, v6 := b.network.(*layers.IPv6)
		next = captureAHDecoder{ipv6: v6}
	}
	if next == layers.LayerTypeIPv6Destination || next == layers.IPProtocolIPv6Destination {
		next = captureIPv6OptionsDecoder{}
	}
	if next == layers.LayerTypeIPv6HopByHop || next == layers.IPProtocolIPv6HopByHop {
		next = captureIPv6OptionsDecoder{hop: true}
	}
	if b.pim != nil {
		next = capturePIMDecoder{network: b.pim}
		b.pim = nil
	}
	if next == layers.LayerTypeGRE {
		next = captureGREDecoder{}
	}
	if next == layers.LayerTypeVXLAN {
		if b.udp != nil && b.udp.DstPort.LayerType() != layers.LayerTypeVXLAN {
			// A source-port fallback must not present an unselected inner
			// Ethernet/application view to packet observers either.
			next = gopacket.LayerTypePayload
		} else {
			next = captureVXLANObserverDecoder{}
		}
	}
	if next == layers.EthernetType(0x22eb) {
		next = captureERSPAN3Decoder{}
	}
	if next == layers.LayerTypeDot1Q || next == layers.EthernetTypeDot1Q || next == layers.EthernetTypeQinQ {
		next = captureVLANDecoder{}
	}
	if next == layers.LayerTypeLLC || next == layers.EthernetTypeLLC {
		next = captureLLCDecoder{}
	}
	if next == layers.LayerTypeSNAP {
		next = captureSNAPDecoder{}
	}
	if b.last != nil && len(b.last.LayerPayload()) == 0 {
		switch d := next.(type) {
		case captureOSPFv2Decoder:
			return d.Decode(nil, b.PacketBuilder)
		case captureOSPFv3Decoder:
			return d.Decode(nil, b.PacketBuilder)
		case captureESPDecoder:
			return d.Decode(nil, b.PacketBuilder)
		case captureAHDecoder:
			return d.Decode(nil, b.PacketBuilder)
		case captureIPv6OptionsDecoder:
			return d.Decode(nil, b.PacketBuilder)
		}
	}
	return b.PacketBuilder.NextDecoder(captureCarrierDecoder{inner: next, network: b.network})
}

type captureLLCDecoder struct{}

func (captureLLCDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	l := &layers.LLC{}
	if err := l.DecodeFromBytes(data, b); err != nil {
		return err
	}
	b.AddLayer(l)
	if data[0]&0xfe == 0xfe {
		// ISO NLPIDs need their own complete native grammar. A masked DSAP
		// must not turn a response/group SAP or another control into IS-IS.
		return b.NextDecoder(gopacket.LayerTypePayload)
	}
	// Generic decoders select STP from a masked DSAP alone. Keep it raw until
	// native admission checks both SAPs, UI control and the complete BPDU.
	if data[0]&0xfe == 0x42 {
		if data[0] == 0x42 && data[1] == 0x42 && data[2] == 3 {
			if _, err := stream_parser.DecodeSTPWire(l.Payload, false, 4096); err == nil {
				return b.NextDecoder(layers.LayerTypeSTP)
			}
		}
		return b.NextDecoder(gopacket.LayerTypePayload)
	}
	if data[0]&0xfe == 0xaa && (data[0] != 0xaa || data[1] != 0xaa || data[2] != 3) {
		return b.NextDecoder(gopacket.LayerTypePayload)
	}
	return b.NextDecoder(l.NextLayerType())
}

type captureSNAPDecoder struct{}

func (captureSNAPDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	s := &layers.SNAP{}
	if err := s.DecodeFromBytes(data, b); err != nil {
		return err
	}
	b.AddLayer(s)
	if data[0] == 0 && data[1] == 0 && data[2] == 12 && data[3] == 1 && data[4] == 11 {
		return b.NextDecoder(gopacket.LayerTypePayload)
	}
	return b.NextDecoder(s.NextLayerType())
}
func (captureVLANDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	v := &layers.Dot1Q{}
	if err := v.DecodeFromBytes(data, b); err != nil {
		return err
	}
	if v.Type <= 1500 {
		length := int(v.Type)
		if length > len(v.Payload) {
			b.SetTruncated()
			return fmt.Errorf("802.1Q IEEE 802.3 payload length %d exceeds captured %d", length, len(v.Payload))
		}
		v.Payload = v.Payload[:length] // Ethernet padding is not LLC contents.
		b.AddLayer(v)
		return b.NextDecoder(layers.LayerTypeLLC)
	}
	if v.Type < 1536 {
		return fmt.Errorf("802.1Q reserved length/type value %d", v.Type)
	}
	b.AddLayer(v)
	return b.NextDecoder(v.NextLayerType())
}
