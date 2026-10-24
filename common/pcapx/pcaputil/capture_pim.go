package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

var capturePIMType = gopacket.RegisterLayerType(200004, gopacket.LayerTypeMetadata{Name: "PIMv2"})

// This is a bounded Register carrier, not a multicast routing state machine.
// RFC 7761 section 4.9.3: Null-Registers contain dummy IP headers and must
// never admit their contents as captured application traffic.
type capturePIM struct {
	Version, Type        uint8
	Checksum             uint16
	NullRegister, Border bool
	ChecksumVerified     bool
	ChecksumScope        string
	contents, payload    []byte
}

func (*capturePIM) LayerType() gopacket.LayerType { return capturePIMType }
func (p *capturePIM) LayerContents() []byte       { return p.contents }
func (p *capturePIM) LayerPayload() []byte        { return p.payload }

func pimNetwork(n gopacket.NetworkLayer) gopacket.NetworkLayer {
	switch ip := n.(type) {
	case *layers.IPv4:
		if ip.Protocol == 103 && ip.FragOffset == 0 && ip.Flags&layers.IPv4MoreFragments == 0 {
			return n
		}
	case *layers.IPv6:
		if ip.NextHeader == 103 {
			return n
		}
	}
	return nil
}

type capturePIMDecoder struct{ network gopacket.NetworkLayer }

func pimChecksum(w []byte, network gopacket.NetworkLayer) uint16 {
	var sum uint32
	add := func(data []byte) {
		for len(data) >= 2 {
			sum += uint32(binary.BigEndian.Uint16(data))
			data = data[2:]
		}
		if len(data) > 0 {
			sum += uint32(data[0]) << 8
		}
	}
	if ip, ok := network.(*layers.IPv6); ok {
		add(ip.SrcIP.To16())
		add(ip.DstIP.To16())
		// The Register pseudo-header length is eight, also when checking the
		// interoperable whole-message checksum permitted by RFC 7761.
		add([]byte{0, 0, 0, 8, 0, 0, 0, 103})
	}
	add(w)
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

func (d capturePIMDecoder) Decode(w []byte, b gopacket.PacketBuilder) error {
	if len(w) < 4 {
		b.SetTruncated()
		return fmt.Errorf("PIM: short common header")
	}
	if w[0]>>4 != 2 {
		return fmt.Errorf("PIM: unselected version")
	}
	p := &capturePIM{Version: 2, Type: w[0] & 15, Checksum: binary.BigEndian.Uint16(w[2:]), contents: w[:4], payload: w[4:]}
	if p.Type != 1 {
		b.AddLayer(p)
		return b.NextDecoder(gopacket.LayerTypePayload) // Control bodies remain opaque.
	}
	if len(w) < 8 {
		b.SetTruncated()
		return fmt.Errorf("PIM Register: short flags")
	}
	p.NullRegister, p.Border = w[4]&0x40 != 0, w[4]&0x80 != 0
	p.contents, p.payload = w[:8], w[8:]
	p.ChecksumScope = "header"
	if pimChecksum(w[:8], d.network) != 0 {
		if pimChecksum(w, d.network) != 0 {
			return fmt.Errorf("PIM Register: checksum failed")
		}
		p.ChecksumScope = "message-interop"
	}
	p.ChecksumVerified = true // Integrity only; no sender authentication claim.
	b.AddLayer(p)
	if p.NullRegister {
		return b.NextDecoder(gopacket.LayerTypePayload)
	}
	if len(p.payload) == 0 {
		b.SetTruncated()
		return fmt.Errorf("PIM Register: missing inner IP")
	}
	version := byte(0)
	switch d.network.(type) {
	case *layers.IPv4:
		version = 4
	case *layers.IPv6:
		version = 6
	}
	if version == 0 || p.payload[0]>>4 != version {
		return fmt.Errorf("PIM Register: inner/outer address family mismatch")
	}
	var destination net.IP
	if version == 4 && len(p.payload) >= 20 {
		destination = net.IP(p.payload[16:20])
	}
	if version == 6 && len(p.payload) >= 40 {
		destination = net.IP(p.payload[24:40])
	}
	if destination == nil {
		b.SetTruncated()
		return fmt.Errorf("PIM Register: short inner IP header")
	}
	if !destination.IsMulticast() {
		return fmt.Errorf("PIM Register: inner destination is not multicast")
	}
	return b.NextDecoder(layers.LinkTypeRaw)
}
