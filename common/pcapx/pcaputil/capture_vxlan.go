package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// RFC 7348 section 5 makes I mandatory but explicitly requires receivers to
// ignore every reserved flag/byte. GBP remains Ethernet-compatible (the GBP
// draft's section 3); observing its fields does not execute a group policy.
func observedVXLAN(data []byte) *layers.VXLAN {
	v := &layers.VXLAN{}
	if len(data) > 0 {
		v.ValidIDFlag = data[0]&0x08 != 0
		v.GBPExtension = data[0]&0x80 != 0
	}
	if len(data) >= 7 {
		v.VNI = uint32(data[4])<<16 | uint32(data[5])<<8 | uint32(data[6])
	}
	if v.GBPExtension && len(data) >= 4 {
		v.GBPDontLearn = data[1]&0x40 != 0
		v.GBPApplied = data[1]&0x08 != 0
		v.GBPGroupPolicyID = binary.BigEndian.Uint16(data[2:4])
	}
	if len(data) < 8 {
		v.BaseLayer = layers.BaseLayer{Contents: data}
	} else {
		v.BaseLayer = layers.BaseLayer{Contents: data[:8], Payload: data[8:]}
	}
	return v
}

// Observers retain their historical application view. Correct the literal GBP
// A bit there too: the dependency currently reads bit 0x80 instead of 0x08.
type captureVXLANObserverDecoder struct{}

func (captureVXLANObserverDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	if len(data) < 8 {
		return fmt.Errorf("vxlan packet too small")
	}
	b.AddLayer(observedVXLAN(data))
	return b.NextDecoder(layers.LinkTypeEthernet)
}

type captureVXLANDecoder struct{}

type vxlanHeaderError struct {
	layer          *layers.VXLAN
	reason         string
	available      int
	truncated      bool
	innerTruncated bool
}

func (e *vxlanHeaderError) Error() string { return "VXLAN header: " + e.reason }

func (captureVXLANDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	v := observedVXLAN(data)
	b.AddLayer(v)
	if len(data) < 8 {
		b.SetTruncated()
		return &vxlanHeaderError{layer: v, reason: fmt.Sprintf("incomplete fixed header: %d of 8 bytes available", len(data)), available: len(data), truncated: true}
	}
	if !v.ValidIDFlag {
		return &vxlanHeaderError{layer: v, reason: "I bit is clear; VNI is not valid", available: len(data)}
	}
	if len(v.LayerPayload()) < 14 {
		b.SetTruncated()
		return &vxlanHeaderError{layer: v, reason: fmt.Sprintf("Ethernet child prefix incomplete: %d of 14 bytes available", len(v.LayerPayload())), available: len(data), truncated: true, innerTruncated: true}
	}
	return b.NextDecoder(layers.LinkTypeEthernet)
}

func selectedVXLAN(v *layers.VXLAN) bool {
	return v != nil && len(v.LayerContents()) == 8 && v.ValidIDFlag
}

// GPE is a separate service/profile, not an interpretation of RFC 7348's
// must-ignore bits on the ordinary VXLAN port. Only a complete coherent v0
// header on its actual destination service selects this observation. Unknown
// versions/incoherent prefixes remain UDP observations, not identified GPE.
// No GPE payload decoder, OAM handler or forwarding policy is implemented here.
type captureVXLANGPE struct{ layers.BaseLayer }

func (v *captureVXLANGPE) LayerType() gopacket.LayerType { return layers.LayerTypeVXLAN }

func observedVXLANGPE(data []byte) bool {
	if len(data) < 8 || data[0]&0x08 == 0 || data[0]&0x30 != 0 {
		return false
	}
	return data[0]&0x04 == 0 && data[3] == 0 || data[0]&0x04 != 0 && data[3] != 0
}

type captureVXLANGPEDecoder struct{}

func (captureVXLANGPEDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	if !observedVXLANGPE(data) {
		return fmt.Errorf("VXLAN-GPE header is not selected")
	}
	b.AddLayer(&captureVXLANGPE{layers.BaseLayer{Contents: data[:8], Payload: data[8:]}})
	return b.NextDecoder(gopacket.LayerTypePayload)
}
