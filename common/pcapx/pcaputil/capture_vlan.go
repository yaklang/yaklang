package pcaputil

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
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
	return b.PacketBuilder.NextDecoder(captureCarrierDecoder{next})
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
