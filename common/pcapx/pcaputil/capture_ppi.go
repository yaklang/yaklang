package pcaputil

import (
	"encoding/binary"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Version0 PPI carrying Ethernet, optionally one aggregation-interface field.
// This local adapter never guesses a PPI prefix on another declared link type.
// Unknown metadata is refused until its capture-domain implications are known.
type capturePPILayer struct {
	contents, payload []byte
	aggregation       bool
	interfaceID       uint32
}

func (p *capturePPILayer) LayerType() gopacket.LayerType { return gopacket.LayerTypePayload }
func (p *capturePPILayer) LayerContents() []byte         { return p.contents }
func (p *capturePPILayer) LayerPayload() []byte          { return p.payload }

type capturePPIDecoder struct{}

func (capturePPIDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	fail := func(reason string) error { return fmt.Errorf("Unable to decode LinkType PPI: %s", reason) }
	if len(data) < 8 {
		return fail("short header")
	}
	if data[0] != 0 || data[1] != 0 {
		return fail("unselected version/flags")
	}
	n := int(binary.LittleEndian.Uint16(data[2:4]))
	if n < 8 || n > 4096 || n > len(data) {
		return fail("header length")
	}
	if binary.LittleEndian.Uint32(data[4:8]) != 1 {
		return fail("unselected inner link type")
	}
	p := &capturePPILayer{contents: data[:n], payload: data[n:]}
	count := 0
	for at := 8; at < n; {
		count++
		if count > 64 || n-at < 4 {
			return fail("field count/header")
		}
		kind := binary.LittleEndian.Uint16(data[at : at+2])
		size := int(binary.LittleEndian.Uint16(data[at+2 : at+4]))
		at += 4
		if size > n-at {
			return fail("field exceeds header")
		}
		if kind != 8 || size != 4 || p.aggregation {
			return fail("unselected/ambiguous aggregation metadata")
		}
		p.aggregation = true
		p.interfaceID = binary.LittleEndian.Uint32(data[at : at+4])
		at += size
	}
	b.AddLayer(p)
	return b.NextDecoder(layers.LayerTypeEthernet)
}
