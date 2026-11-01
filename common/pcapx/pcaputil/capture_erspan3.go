package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

var captureERSPAN3Type = gopacket.RegisterLayerType(200003, gopacket.LayerTypeMetadata{Name: "ERSPANIII", Decoder: captureERSPAN3Decoder{}})

// Type III (GRE protocol 0x22eb), draft-foschiano-erspan-03 section 4.3.
// The optional eight-byte platform header contains both stable source identity
// and changing timestamps; only the former belongs in a capture domain.
type captureERSPAN3 struct {
	Version                                     uint8
	VLAN, SessionID                             uint16
	CoS, BadShortOversized                      uint8
	Truncated                                   bool
	Timestamp                                   uint32
	SecurityGroup                               uint16
	EthernetPDU                                 bool
	FrameType, HardwareID, TimestampGranularity uint8
	Egress                                      bool
	OptionalPlatform                            bool
	PlatformID                                  uint8
	SourceIndex                                 uint32
	contents, payload                           []byte
}

func (*captureERSPAN3) LayerType() gopacket.LayerType { return captureERSPAN3Type }
func (p *captureERSPAN3) LayerContents() []byte       { return p.contents }
func (p *captureERSPAN3) LayerPayload() []byte        { return p.payload }

type captureERSPAN3Decoder struct{}

func (captureERSPAN3Decoder) Decode(w []byte, b gopacket.PacketBuilder) error {
	fail := func(s string) error { return fmt.Errorf("ERSPAN III: %s", s) }
	if len(w) < 12 {
		b.SetTruncated()
		return fail("short base header")
	}
	first, flags, last := binary.BigEndian.Uint16(w), binary.BigEndian.Uint16(w[2:]), binary.BigEndian.Uint16(w[10:])
	if first>>12 != 2 {
		return fail("unselected version")
	}
	p := &captureERSPAN3{Version: 2, VLAN: first & 0xfff, SessionID: flags & 0x3ff, CoS: uint8(flags >> 13), BadShortOversized: uint8(flags>>11) & 3, Truncated: flags&0x400 != 0, Timestamp: binary.BigEndian.Uint32(w[4:]), SecurityGroup: binary.BigEndian.Uint16(w[8:]), EthernetPDU: last&0x8000 != 0, FrameType: uint8(last>>10) & 31, HardwareID: uint8(last>>4) & 63, Egress: last&8 != 0, TimestampGranularity: uint8(last>>1) & 3, OptionalPlatform: last&1 != 0}
	n := 12
	if p.OptionalPlatform {
		if len(w) < 20 {
			b.SetTruncated()
			return fail("short optional platform header")
		}
		platform := binary.BigEndian.Uint32(w[12:])
		p.PlatformID = uint8(platform >> 26)
		switch p.PlatformID {
		case 1: // VSM domain and full port index
			p.SourceIndex = binary.BigEndian.Uint32(w[16:])
			// Preserve VSM domain in the owned header/domain projection below.
		case 3:
			p.SourceIndex = platform & 0x3fff
		case 5, 6:
			p.SourceIndex = platform & 0x03ffffff // switch ID + port
		case 0, 7:
			p.SourceIndex = platform & 0x000fffff
		case 4: // no source identity in the reserved platform body
		default:
			return fail("unselected optional platform identity")
		}
		n = 20
	}
	if p.FrameType != 0 && p.FrameType != 2 {
		return fail("unselected inner frame type")
	}
	p.contents, p.payload = w[:n], w[n:]
	b.AddLayer(p)
	if p.Truncated {
		b.SetTruncated()
	}
	if p.FrameType == 2 {
		return b.NextDecoder(layers.LinkTypeRaw)
	}
	return b.NextDecoder(layers.LayerTypeEthernet)
}
