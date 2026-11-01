package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// The dependency indexes ESP's two words without checking its length. Keep a
// bounded observer layer even for a partial header, never a fabricated word.
type captureESPDecoder struct{}

type espHeaderTruncatedError struct{ available int }

func (e *espHeaderTruncatedError) Error() string {
	return fmt.Sprintf("ESP clear header requires 8 bytes, got %d", e.available)
}

func (captureESPDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	l := &layers.IPSecESP{BaseLayer: layers.BaseLayer{Contents: data}}
	if len(data) >= 4 {
		l.SPI = binary.BigEndian.Uint32(data[:4])
	}
	if len(data) >= 8 {
		l.Seq = binary.BigEndian.Uint32(data[4:8])
		l.Encrypted = data[8:] // Dependency compatibility name; not an algorithm claim.
	}
	b.AddLayer(l)
	if len(data) < 8 {
		b.SetTruncated()
		return &espHeaderTruncatedError{available: len(data)}
	}
	return nil
}

// A HeaderLength of zero produces ActualLength=8 in the dependency and then a
// data[12:8] panic. Refuse it before delegation, including empty declarations.
type captureAHDecoder struct{ ipv6 bool }

type ahHeaderError struct{ message string }

func (e *ahHeaderError) Error() string { return e.message }

func (d captureAHDecoder) Decode(data []byte, b gopacket.PacketBuilder) error {
	if len(data) < 12 {
		b.SetTruncated()
		return &ahHeaderError{fmt.Sprintf("AH fixed header requires 12 bytes, got %d", len(data))}
	}
	if data[1] == 0 {
		return &ahHeaderError{"AH payload length cannot be zero"}
	}
	if d.ipv6 && (int(data[1])+2)%2 != 0 {
		return &ahHeaderError{"IPv6 AH header length must be a multiple of 8 bytes"}
	}
	if n := (int(data[1]) + 2) * 4; n > len(data) {
		b.SetTruncated()
		return &ahHeaderError{fmt.Sprintf("AH declared header requires %d bytes, got %d", n, len(data))}
	}
	return layers.LayerTypeIPSecAH.Decode(data, b)
}
