package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// A contradictory wire length is malformed even if the capture also lost a
// suffix. Absence alone proves only incompleteness. Never infer an application
// identity from the bytes left behind by either kind of carrier failure.
func udpCarrierError(network gopacket.NetworkLayer, udp *layers.UDP, ci gopacket.CaptureInfo, truncated bool) *ProtocolError {
	if _, ok := network.(*layers.IPv4); ok {
		b := network.LayerContents()
		if len(b) >= 20 {
			h, n := int(b[0]&15)*4, int(binary.BigEndian.Uint16(b[2:4]))
			// The dependency substitutes captured size for TotalLength0. That
			// convenience is not literal evidence of a valid UDP carrier.
			if b[0]>>4 != 4 || h < 20 || h > len(b) || n < h {
				return &ProtocolError{Kind: ErrMalformedMessage, Message: "UDP IPv4 carrier has an invalid literal header/total length"}
			}
		}
	}
	if udp.Length < 8 {
		return &ProtocolError{Kind: ErrMalformedMessage, Message: fmt.Sprintf("UDP length %d is smaller than the 8-byte header", udp.Length)}
	}
	if capacity, known := udpIPCapacity(network); known && int(udp.Length) > capacity {
		return &ProtocolError{Kind: ErrMalformedMessage, Message: fmt.Sprintf("UDP length %d exceeds declared IP payload available to UDP (%d bytes)", udp.Length, capacity)}
	}
	if truncated || ci.CaptureLength < ci.Length || int(udp.Length) > len(udp.Payload)+8 {
		return &ProtocolError{Kind: ErrNeedMore, Message: fmt.Sprintf("UDP carrier incomplete: declared %d bytes, available %d; capture %d/%d bytes", udp.Length, len(udp.Payload)+8, ci.CaptureLength, ci.Length)}
	}
	if int(udp.Length) != len(udp.Payload)+8 {
		return &ProtocolError{Kind: ErrMalformedMessage, Message: "UDP decoded payload contradicts its declared length"}
	}
	return nil
}

// Literal IP headers provide an upper bound independent of the UDP decoder's
// own truncation flag. Session-only callers have no wire IP header: retain their
// existing datagram-length check without fabricating an IP capacity.
func udpIPCapacity(network gopacket.NetworkLayer) (int, bool) {
	b := network.LayerContents()
	switch ip := network.(type) {
	case *layers.IPv4:
		if len(b) < 20 || b[0]>>4 != 4 || b[9] != 17 {
			return 0, false
		}
		h := int(b[0]&15) * 4
		n := int(binary.BigEndian.Uint16(b[2:4]))
		if h < 20 || h > len(b) || n < h {
			return 0, false
		}
		return n - h, true
	case *layers.IPv6:
		if len(b) < 40 || b[0]>>4 != 6 {
			return 0, false
		}
		remaining := int(binary.BigEndian.Uint16(b[4:6]))
		if remaining == 0 {
			return 0, false
		} // jumbograms have a separate grammar
		next := layers.IPProtocol(b[6])
		payload := ip.LayerPayload()
		if next == layers.IPProtocolIPv6HopByHop && ip.HopByHop != nil {
			remaining -= ip.HopByHop.ActualLength
			next = ip.HopByHop.NextHeader
		}
		for depth := 0; depth < 32 && remaining >= 0; depth++ {
			if next == layers.IPProtocolUDP {
				return remaining, true
			}
			if len(payload) < 2 {
				return 0, false
			}
			n := 0
			switch next {
			case layers.IPProtocolIPv6HopByHop, layers.IPProtocolIPv6Routing, layers.IPProtocolIPv6Destination:
				n = (int(payload[1]) + 1) * 8
			case layers.IPProtocolAH:
				n = (int(payload[1]) + 2) * 4
			case layers.IPProtocolIPv6Fragment:
				n = 8
			default:
				return 0, false
			}
			if n > remaining || n > len(payload) {
				return 0, false
			}
			next, payload, remaining = layers.IPProtocol(payload[0]), payload[n:], remaining-n
		}
	}
	return 0, false
}
