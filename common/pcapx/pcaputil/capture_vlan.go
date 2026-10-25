package pcaputil

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// A VLAN tag's final two bytes can be an IEEE 802.3 length rather than an
// EtherType. gopacket's Dot1Q decoder only dispatches the EtherType form.
// Preserve normal application decoding while fixing that local carrier edge.
type captureCarrierDecoder struct{ inner gopacket.Decoder }
type captureCarrierBuilder struct {
	gopacket.PacketBuilder
	pim gopacket.NetworkLayer
}
type captureVLANDecoder struct{}

func (d captureCarrierDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	return d.inner.Decode(data, &captureCarrierBuilder{PacketBuilder: b})
}
func (b *captureCarrierBuilder) SetNetworkLayer(n gopacket.NetworkLayer) {
	b.PacketBuilder.SetNetworkLayer(n)
	b.pim = pimNetwork(n)
}
func (b *captureCarrierBuilder) NextDecoder(next gopacket.Decoder) error {
	if b.pim != nil {
		next = capturePIMDecoder{network: b.pim}
		b.pim = nil
	}
	if next == layers.LayerTypeGRE {
		next = captureGREDecoder{}
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
	return b.PacketBuilder.NextDecoder(captureCarrierDecoder{next})
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
