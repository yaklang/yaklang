package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// This is a physical Source0 observation, never a native PDU, accepted segment
// or connection identity. A decoder error alone is insufficient: the literal
// selected parent and this TCP header must independently prove their failure.
// Tunnels and IPv6 extension profiles are deliberately left to their owners.
type tcpHeaderFailure struct {
	layers                          []string
	prefix                          []tcpFailurePrefix
	network                         capturedNetworkHeader
	start, end, available, required int
	headerComplete                  bool
	legacyZeroLength                bool
	status, reason                  string
	fixed                           []byte
	options                         []tcpFailurePrefix
}

type tcpFailurePrefix struct {
	label      string
	start, end int
}

func inspectTCPHeaderFailure(raw *capturedPacket, p gopacket.Packet) *tcpHeaderFailure {
	if p == nil || p.ErrorLayer() == nil || !bytes.Equal(p.Data(), raw.data) {
		return nil
	}
	b := literalTCPHeaderFailure(raw)
	if b == nil {
		return nil // A complete TCP header followed by a bad child stays intact.
	}
	if raw.presentation.networkRefused || presentEvents(raw.protocolEvents()).networkRefused {
		return nil // An unselected or invalid parent owns its own child boundary.
	}
	return b
}

func literalTCPHeaderFailure(raw *capturedPacket) *tcpHeaderFailure {
	wire := raw.data
	at, kind := 0, uint16(0)
	b := &tcpHeaderFailure{}
	addPrefix := func(label string, size int) bool {
		if size > len(wire)-at {
			return false
		}
		b.prefix = append(b.prefix, tcpFailurePrefix{label: label, start: at, end: at + size})
		b.layers = append(b.layers, label)
		at += size
		return true
	}
	switch raw.link {
	case layers.LinkTypeEthernet:
		if !addPrefix("Ethernet", 14) {
			return nil
		}
		kind = binary.BigEndian.Uint16(wire[12:14])
	case layers.LinkTypeLinuxSLL:
		if !addPrefix("Linux SLL", 16) {
			return nil
		}
		kind = binary.BigEndian.Uint16(wire[14:16])
	case layers.LinkTypeLinuxSLL2:
		if !addPrefix("Linux SLL2", 20) {
			return nil
		}
		kind = binary.BigEndian.Uint16(wire[:2])
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		if !addPrefix("Loopback", 4) {
			return nil
		}
		family := binary.LittleEndian.Uint32(wire[:4])
		if family > 255 {
			family = binary.BigEndian.Uint32(wire[:4])
		}
		switch family {
		case 2:
			kind = 0x0800
		case 10, 24, 28, 30:
			kind = 0x86dd
		default:
			return nil
		}
	case layers.LinkTypeIPv4:
		kind = 0x0800
	case layers.LinkTypeIPv6:
		kind = 0x86dd
	case layers.LinkTypeRaw:
		if len(wire) == 0 {
			return nil
		}
		switch wire[0] >> 4 {
		case 4:
			kind = 0x0800
		case 6:
			kind = 0x86dd
		default:
			return nil
		}
	default:
		return nil
	}
	for count := 0; kind == 0x8100 || kind == 0x88a8; count++ {
		if count == 8 || at+4 > len(wire) {
			return nil
		}
		kind = binary.BigEndian.Uint16(wire[at+2 : at+4])
		if !addPrefix("Dot1Q", 4) {
			return nil
		}
	}
	limit := len(wire)
	start := at
	switch kind {
	case 0x0800:
		if limit-at < 20 || wire[at]>>4 != 4 {
			return nil
		}
		header, total := int(wire[at]&15)*4, int(binary.BigEndian.Uint16(wire[at+2:at+4]))
		if header < 20 || header > limit-at || total != 0 && total < header || wire[at+9] != 6 || binary.BigEndian.Uint16(wire[at+6:at+8])&0x3fff != 0 {
			return nil
		}
		// Validate the own IPv4 option prefix. EOL terminates options; following
		// captured padding is preserved, not mistaken for another option.
		for option := at + 20; option < at+header; {
			t := wire[option]
			if t == 0 {
				break
			}
			if t == 1 {
				option++
				continue
			}
			if option+2 > at+header || wire[option+1] < 2 || int(wire[option+1]) > at+header-option {
				return nil
			}
			option += int(wire[option+1])
		}
		b.legacyZeroLength = total == 0
		if total != 0 {
			limit = min(limit, at+total)
		}
		b.network = capturedNetworkHeader{label: "IPv4", source: net.IP(wire[at+12 : at+16]).String(), destination: net.IP(wire[at+16 : at+20]).String(), start: at, end: at + header, sourceStart: at + 12, sourceEnd: at + 16, destStart: at + 16, destEnd: at + 20}
		at += header
	case 0x86dd:
		if limit-at < 40 || wire[at]>>4 != 6 || wire[at+6] != 6 {
			return nil
		}
		payload := int(binary.BigEndian.Uint16(wire[at+4 : at+6]))
		if payload == 0 {
			return nil // No jumbo-payload profile is inferred in this inspector.
		}
		limit = min(limit, at+40+payload)
		b.network = capturedNetworkHeader{label: "IPv6", source: net.IP(wire[at+8 : at+24]).String(), destination: net.IP(wire[at+24 : at+40]).String(), start: at, end: at + 40, sourceStart: at + 8, sourceEnd: at + 24, destStart: at + 24, destEnd: at + 40}
		at += 40
	default:
		return nil
	}
	if at > limit {
		return nil
	}
	b.prefix = append(b.prefix, tcpFailurePrefix{label: b.network.label, start: start, end: at})
	b.layers = append(b.layers, b.network.label, "TCP")
	b.start, b.end, b.available, b.required = at, limit, limit-at, 20
	b.fixed = bytes.Clone(wire[at:min(limit, at+20)])
	if b.available >= 13 {
		b.required = int(wire[at+12]>>4) * 4
	}
	if b.available < 20 {
		b.status, b.reason = "incomplete", fmt.Sprintf("TCP fixed header incomplete (%d/20 bytes)", b.available)
		return b
	}
	if b.required < 20 {
		b.status, b.reason = "malformed", fmt.Sprintf("TCP data offset %d words is below the 5-word fixed header", wire[at+12]>>4)
		return b
	}
	if b.required > b.available {
		b.status, b.reason = "incomplete", fmt.Sprintf("TCP header incomplete (%d/%d bytes)", b.available, b.required)
		return b
	}
	b.headerComplete = true
	for option := at + 20; option < at+b.required; {
		t := wire[option]
		if t == 0 {
			b.options = append(b.options, tcpFailurePrefix{label: "End of option list", start: option, end: option + 1})
			break
		}
		if t == 1 {
			b.options = append(b.options, tcpFailurePrefix{label: "No-operation", start: option, end: option + 1})
			option++
			continue
		}
		remaining := at + b.required - option
		if remaining < 2 {
			b.options = append(b.options, tcpFailurePrefix{label: fmt.Sprintf("Option %d: length byte unavailable", t), start: option, end: option + 1})
			b.status, b.reason = "malformed", fmt.Sprintf("TCP option %d length byte unavailable at header offset %d", t, option-at)
			return b
		}
		size := int(wire[option+1])
		if size < 2 || size > remaining {
			b.options = append(b.options, tcpFailurePrefix{label: fmt.Sprintf("Rejected option prefix: kind %d, length %d", t, size), start: option, end: option + 2})
			b.status, b.reason = "malformed", fmt.Sprintf("TCP option %d length %d invalid at header offset %d (%d header bytes available)", t, size, option-at, remaining)
			return b
		}
		label := fmt.Sprintf("Option %d: %d bytes", t, size)
		b.options = append(b.options, tcpFailurePrefix{label: label, start: option, end: option + size})
		if t == 30 {
			if reason := tcpMPTCPDimensionFailure(wire[option : option+size]); reason != "" {
				b.status, b.reason = "malformed", fmt.Sprintf("%s at TCP header offset %d", reason, option-at)
				return b
			}
		}
		option += size
	}
	return nil // Do not turn an unrelated application decoder error into TCP failure.
}

func tcpMPTCPDimensionFailure(option []byte) string {
	if len(option) < 3 {
		return "MPTCP option subtype unavailable"
	}
	var name string
	var sizes []int
	switch option[2] >> 4 {
	case 0:
		name, sizes = "MP_CAPABLE", []int{4, 12, 20, 22, 24}
	case 1:
		name, sizes = "MP_JOIN", []int{12, 16, 24}
	case 2:
		name = "DSS"
		if len(option) < 4 {
			return "MPTCP DSS flags unavailable"
		}
		flags, size := option[3], 4
		if flags&1 != 0 {
			size += 4
			if flags&2 != 0 {
				size += 4
			}
		}
		if flags&4 != 0 {
			size += 10
			if flags&8 != 0 {
				size += 4
			}
			sizes = []int{size, size + 2} // Only a mapping can carry a checksum.
		} else {
			sizes = []int{size}
		}
	case 4:
		if len(option) >= 4 {
			return ""
		}
		name, sizes = "REMOVE_ADDR", []int{4}
	case 5:
		name, sizes = "MP_PRIO", []int{3, 4}
	case 6:
		name, sizes = "MP_FAIL", []int{12}
	case 7:
		name, sizes = "MP_FASTCLOSE", []int{12}
	case 8:
		name, sizes = "MP_TCPRST", []int{4}
	default:
		// ADD_ADDR depends on the MPTCP version/profile. Unknown subtypes
		// and this unselected profile are not rejected just for their kind.
		return ""
	}
	for _, size := range sizes {
		if len(option) == size {
			return ""
		}
	}
	return fmt.Sprintf("MPTCP %s option length %d inconsistent with its fields (expected %v)", name, len(option), sizes)
}

func (b *tcpHeaderFailure) apply(row packetSummary) packetSummary {
	row.Protocol, row.Transport = "TCP", "tcp"
	row.Source, row.Destination = b.network.source, b.network.destination
	row.PhysicalSource, row.PhysicalDestination = "", ""
	row.SourcePort, row.DestinationPort = 0, 0
	if len(b.fixed) >= 2 {
		row.SourcePort = binary.BigEndian.Uint16(b.fixed[:2])
	}
	if len(b.fixed) >= 4 {
		row.DestinationPort = binary.BigEndian.Uint16(b.fixed[2:4])
	}
	row.Layers, row.ProtocolStack = append([]string(nil), b.layers...), append([]string(nil), b.layers...)
	row.ProtocolStatus, row.ProtocolScope, row.Visibility = b.status, "captured TCP header diagnostic", "headers"
	row.ProtocolEvidence = "captured TCP header failure; segment not admitted to a connection or application"
	row.Info = b.reason
	if b.headerComplete {
		row.Info += fmt.Sprintf("; captured TCP payload=%d bytes (header boundary observed)", b.available-b.required)
	} else {
		row.Info += "; payload boundary unknown"
	}
	if b.legacyZeroLength {
		row.Info += "; IPv4 total length zero: legacy captured-prefix observation"
	}
	if row.Length > row.CapturedLength {
		row.Info += fmt.Sprintf(" [truncated capture: %d/%d bytes]", row.CapturedLength, row.Length)
	}
	row.TCPFlowID, row.TCPFlowDomain, row.TCPFlowState = 0, nil, ""
	row.MessageSource, row.MessageDestination, row.MessageTransport, row.MessageDomain = "", "", "", nil
	row.ConnectionProtocol, row.ConnectionStack, row.ConnectionEvidence = "", nil, ""
	row.StreamID, row.ConversationID, row.TransportView = 0, 0, nil
	row.packetProtocol, row.packetInfo, row.connectionAnnotated = row.Protocol, row.Info, false
	row.payloadBytes, row.opaqueTransport = 0, false
	row.tcpHeaderFailure = b
	return row
}

func (b *tcpHeaderFailure) appendFields(d *packetDetail) {
	d.protocol = "TCP"
	d.fields = append(d.fields, field{text: "Captured network prefix", branch: true})
	for _, p := range b.prefix {
		d.fields = append(d.fields, field{depth: 1, text: p.label + " header", branch: true, start: p.start, end: p.end, hasRange: p.end > p.start})
	}
	h := b.network
	d.fields = append(d.fields, field{depth: 2, text: "Source: " + h.source, start: h.sourceStart, end: h.sourceEnd, hasRange: true}, field{depth: 2, text: "Destination: " + h.destination, start: h.destStart, end: h.destEnd, hasRange: true})
	end := min(b.end, b.start+max(20, b.required))
	d.fields = append(d.fields, field{text: "TCP header observation", branch: true, start: b.start, end: end, hasRange: end > b.start}, field{depth: 1, text: "Status: " + b.status}, field{depth: 1, text: "Error: " + b.reason})
	add := func(name string, value any, offset, size int) {
		d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("%s: %v", name, value), start: b.start + offset, end: b.start + offset + size, hasRange: true})
	}
	words := []struct {
		name         string
		offset, size int
	}{{"Source Port", 0, 2}, {"Destination Port", 2, 2}, {"Sequence Number", 4, 4}, {"Acknowledgment Number", 8, 4}, {"Window", 14, 2}, {"Checksum (unverified)", 16, 2}, {"Urgent Pointer", 18, 2}}
	for _, word := range words {
		if word.offset+word.size > len(b.fixed) {
			continue
		}
		var value uint32
		if word.size == 2 {
			value = uint32(binary.BigEndian.Uint16(b.fixed[word.offset : word.offset+2]))
		} else {
			value = binary.BigEndian.Uint32(b.fixed[word.offset : word.offset+4])
		}
		add(word.name, value, word.offset, word.size)
	}
	if len(b.fixed) >= 13 {
		add("Data Offset (32-bit words)", b.fixed[12]>>4, 12, 1)
	}
	if len(b.fixed) >= 14 {
		add("TCP Flags (reserved bits excluded)", fmt.Sprintf("0x%03x", uint16(b.fixed[12]&1)<<8|uint16(b.fixed[13])), 12, 2)
		add("Reserved Bits", b.fixed[12]>>1&7, 12, 1)
	}
	d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("Available TCP Bytes: %d", b.available)}, field{depth: 1, text: "Minimum TCP Fixed Header Bytes: 20"})
	if len(b.fixed) >= 13 {
		d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("Declared TCP Header Bytes: %d", b.required)})
	} else {
		d.fields = append(d.fields, field{depth: 1, text: "Declared TCP Header Bytes: unavailable"})
	}
	if b.legacyZeroLength {
		d.fields = append(d.fields, field{depth: 1, text: "Parent scope: IPv4 total length zero; legacy captured-prefix observation, not a valid declared IPv4 extent"})
	}
	for _, option := range b.options {
		d.fields = append(d.fields, field{depth: 1, text: option.label, start: option.start, end: option.end, hasRange: option.end > option.start})
	}
	if b.headerComplete {
		at := b.start + b.required
		d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("Captured TCP Payload: %d bytes; not admitted to reassembly", b.end-at), start: at, end: b.end, hasRange: b.end > at})
	} else {
		d.fields = append(d.fields, field{depth: 1, text: "TCP Payload Boundary: unknown; the fixed/header-options prefix is incomplete or invalid"})
		if b.required > b.available && b.available > 20 {
			d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("Captured TCP option prefix: %d bytes; header incomplete", b.available-20), start: b.start + 20, end: b.end, hasRange: true})
		}
	}
	// No cipher/stream Source1 is synthesized from rejected transport bytes.
	d.fields = append(d.fields, field{depth: 1, text: "Observation scope: captured header fields only; no connection, application, MPTCP negotiation or checksum validation"})
}
