package pcaputil

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// CaptureLinkDecoder selects the same bounded link adapter for SDK replay,
// packet summaries, byte inspection and connection tracking.
func CaptureLinkDecoder(link layers.LinkType) gopacket.Decoder { return captureLinkDecoder(link) }

var captureMPacketType = gopacket.RegisterLayerType(200001, gopacket.LayerTypeMetadata{Name: "FPP", Decoder: captureMPacketDecoder{}})

type captureMPacket struct{ contents, payload []byte }

func (*captureMPacket) LayerType() gopacket.LayerType { return captureMPacketType }
func (p *captureMPacket) LayerContents() []byte       { return p.contents }
func (p *captureMPacket) LayerPayload() []byte        { return p.payload }

type captureMPacketDecoder struct{}

// LINKTYPE_ETHERNET_MPACKET includes the MAC merge preamble and trailing FCS.
// Only complete express frames are admitted here. A preemptable fragment must
// never be handed to Ethernet as if it were a complete packet.
// Format reference: Wireshark epan/dissectors/packet-fpp.c.
func (captureMPacketDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	fail := func(s string) error { return fmt.Errorf("Ethernet mPacket: %s", s) }
	at := 0
	if len(data) > 0 && data[0] == 0x50 {
		at++
	}
	for at < len(data) && data[at] == 0x55 {
		at++
		if at > 64 {
			return fail("preamble exceeds profile limit")
		}
	}
	if at >= len(data) {
		return fail("truncated preamble")
	}
	if data[at] != 0xd5 {
		return fail("unselected MAC merge fragment/control delimiter")
	}
	at++
	if len(data)-at < 18 {
		return fail("truncated express Ethernet frame/FCS")
	}
	payload := data[at : len(data)-4]
	if crc32.ChecksumIEEE(payload) != binary.LittleEndian.Uint32(data[len(data)-4:]) {
		return fail("invalid express frame FCS")
	}
	b.AddLayer(&captureMPacket{contents: data[:at], payload: payload})
	return (captureCarrierDecoder{inner: layers.LinkTypeEthernet}).Decode(payload, b)
}
