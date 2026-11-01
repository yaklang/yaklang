package pcaputil

import (
	"encoding/binary"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

type captureGREDecoder struct{}

func (captureGREDecoder) Decode(w []byte, b gopacket.PacketBuilder) error {
	// RFC 2637 version one is the PPTP layout, not arbitrary EtherType GRE.
	// Select and bound that parent before an inner IP/PPP/ARP decoder can run.
	if len(w) >= 2 && w[1]&7 == 1 {
		return decodeCapturePPTP(w, b)
	}
	fail := func() error {
		b.SetTruncated()
		return fmt.Errorf("GRE optional header exceeds captured bytes or header budget")
	}
	if len(w) < 4 {
		return fail()
	}
	at := 4
	if w[0]&0xc0 != 0 {
		at += 4
	}
	if w[0]&0x20 != 0 {
		at += 4
	}
	if w[0]&0x10 != 0 {
		at += 4
	}
	if at > len(w) {
		return fail()
	}
	if w[0]&0x40 != 0 {
		for n := 0; ; n++ {
			if n >= 128 || at+4 > len(w) || at > 4096 {
				return fail()
			}
			size := int(w[at+3])
			end := w[at] == 0 && w[at+1] == 0 && size == 0
			at += 4 + size
			if at > len(w) {
				return fail()
			}
			if end {
				break
			}
		}
	}
	if w[1]&0x80 != 0 {
		at += 4
	}
	if at > len(w) || at > 4096 {
		return fail()
	}
	g := &layers.GRE{}
	if err := g.DecodeFromBytes(w, b); err != nil {
		return err
	}
	b.AddLayer(g)
	if g.Protocol == 0x22eb {
		if g.Version != 0 || g.RoutingPresent || !g.SeqPresent {
			return fmt.Errorf("ERSPAN III: unselected GRE profile")
		}
		return b.NextDecoder(captureERSPAN3Decoder{})
	}
	if g.Protocol == layers.EthernetTypeERSPAN && !g.SeqPresent {
		return b.NextDecoder(layers.LayerTypeEthernet) // ERSPAN Type I has no ERSPAN header.
	}
	return b.NextDecoder(g.NextLayerType())
}

// The error retains only bounded literal header facts. No speculative child
// bytes or peer/call association survives a rejected PPTP parent.
type pptpHeaderError struct {
	reason                 string
	flags, protocol        uint16
	available, required    int
	hasProtocol, truncated bool
}

func (e *pptpHeaderError) Error() string { return "PPTP GRE header: " + e.reason }

func decodeCapturePPTP(w []byte, b gopacket.PacketBuilder) error {
	if len(w) < 2 {
		b.SetTruncated()
		return &pptpHeaderError{reason: "incomplete flags/version", available: len(w), required: 2, truncated: true}
	}
	flags := binary.BigEndian.Uint16(w)
	fail := func(reason string, required int, truncated bool) error {
		e := &pptpHeaderError{reason: reason, flags: flags, available: len(w), required: required, truncated: truncated}
		if len(w) >= 4 {
			e.protocol, e.hasProtocol = binary.BigEndian.Uint16(w[2:]), true
		}
		if truncated {
			b.SetTruncated()
		}
		return e
	}
	if len(w) < 4 {
		return fail("incomplete protocol type", 4, true)
	}
	// C/R/s/Recur and reserved Flags are zero; K is one. S and A select
	// actual sequence/ack fields. Version bits have already selected v1.
	if flags&^uint16(0x3081) != 0 || flags&0x2000 == 0 {
		return fail("unselected flags; K is required and reserved bits must be zero", 8, false)
	}
	if binary.BigEndian.Uint16(w[2:]) != uint16(layers.EthernetTypePPP) {
		return fail("protocol type must be PPP (0x880b)", 8, false)
	}
	if len(w) < 8 {
		return fail("incomplete payload length and Call ID", 8, true)
	}
	at := 8
	sequence, ack := flags&0x1000 != 0, flags&0x80 != 0
	if sequence {
		at += 4
	}
	if ack {
		at += 4
	}
	if len(w) < at {
		return fail("incomplete sequence or acknowledgment field", at, true)
	}
	length := int(binary.BigEndian.Uint16(w[4:]))
	if sequence != (length != 0) || !sequence && !ack {
		return fail("data requires S and a payload; acknowledgment-only requires A and zero payload", at, false)
	}
	if length != len(w)-at {
		return fail(fmt.Sprintf("declared payload length %d differs from %d available bytes", length, len(w)-at), at+length, length > len(w)-at)
	}
	if length != 0 {
		// Bound the dependency's PPP prefix reads as well. Address/control
		// compression and a one-byte odd protocol are valid observations; a
		// protocol requiring another byte cannot borrow it from another frame.
		payload := w[at:]
		if payload[0] == 0xff {
			if len(payload) < 2 {
				return fail("incomplete PPP address/control prefix", at+2, true)
			}
			if payload[1] == 0x03 {
				payload = payload[2:]
			}
		}
		if len(payload) == 0 || payload[0]&1 == 0 && len(payload) < 2 {
			return fail("incomplete PPP protocol field", at+length+1, true)
		}
		if payload[0]&1 == 0 && payload[1]&1 == 0 {
			return fail("invalid PPP protocol field", at, false)
		}
	}
	g := &layers.GRE{}
	if err := g.DecodeFromBytes(w, b); err != nil {
		return err // Every read in this selected layout is bounded above.
	}
	b.AddLayer(g)
	if length == 0 {
		return nil // A real ACK-only header has no PPP or child transport.
	}
	return b.NextDecoder(layers.LayerTypePPP)
}

// This also checks observer-layer headers before deriving a stable namespace.
// Payload Length is framing, while the directional Call ID is session identity.
func selectedPPTPGRE(g *layers.GRE) bool {
	if g.Version != 1 || g.Protocol != layers.EthernetTypePPP || g.ChecksumPresent || g.RoutingPresent || !g.KeyPresent || g.StrictSourceRoute || g.RecursionControl != 0 || g.Flags&15 != 0 {
		return false
	}
	at := 8
	if g.SeqPresent {
		at += 4
	}
	if g.AckPresent {
		at += 4
	}
	length := int(g.Key >> 16)
	return len(g.LayerContents()) == at && len(g.LayerPayload()) == length && g.SeqPresent == (length != 0) && (g.SeqPresent || g.AckPresent)
}
