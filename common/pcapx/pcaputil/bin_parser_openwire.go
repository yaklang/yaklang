package pcaputil

import (
	"bytes"
	"encoding/binary"
)

const (
	openWireInfoType       = byte(1)
	openWireMagic          = "ActiveMQ"
	openWireMaxPropertyLen = 4096
	// A WireFormatInfo body contains type, fixed eight-byte magic, version,
	// nullable-byte-array marker, and (when non-null) a uint32 property size.
	openWireInfoBodyHeader = 1 + 8 + 4 + 1 + 4
	openWireInfoMaxBytes   = 4 + openWireInfoBodyHeader + openWireMaxPropertyLen
)

// probeOpenWire admits only a complete, size-prefixed WireFormatInfo exchange
// command. Its larger protocol-specific inspection bound is still capped at
// the OpenWire specification's 4 KiB WireFormatInfo property limit.
func probeOpenWire(w []byte, maxBytes int) ProbeResult {
	if maxBytes <= 0 || maxBytes > openWireInfoMaxBytes {
		maxBytes = openWireInfoMaxBytes
	}
	if len(w) > maxBytes {
		w = w[:maxBytes]
	}
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	// The supported WFI command is at most 4,118 bytes, so its first two
	// length bytes must be zero. Use that constraint to avoid retaining an
	// arbitrary short binary prefix while still allowing TCP fragmentation.
	if len(w) < 4 {
		if w[0] != 0 || len(w) > 1 && w[1] != 0 || len(w) > 2 && w[2] > 0x10 {
			return ProbeResult{Verdict: ProbeReject}
		}
		return openWireProbeNeed(w, 4, "incomplete OpenWire length prefix")
	}
	bodyLength := uint64(binary.BigEndian.Uint32(w[:4]))
	if bodyLength < 14 || bodyLength > openWireInfoBodyHeader+openWireMaxPropertyLen || bodyLength+4 > uint64(maxBytes) {
		return ProbeResult{Verdict: ProbeReject, Reason: "WireFormatInfo length is outside the bounded profile"}
	}
	total := int(bodyLength + 4)
	if len(w) < 5 {
		return openWireProbeNeed(w, 5, "incomplete OpenWire command type")
	}
	if w[4] != openWireInfoType {
		return ProbeResult{Verdict: ProbeReject, Reason: "first OpenWire command is not WireFormatInfo"}
	}
	for i := 5; i < min(len(w), 13); i++ {
		if w[i] != openWireMagic[i-5] {
			return ProbeResult{Verdict: ProbeReject, Reason: "invalid OpenWire WireFormatInfo magic"}
		}
	}
	if len(w) < 13 {
		return openWireProbeNeed(w, 13, "incomplete OpenWire WireFormatInfo magic")
	}
	if len(w) < 17 {
		return openWireProbeNeed(w, 17, "incomplete OpenWire WireFormatInfo version")
	}
	if binary.BigEndian.Uint32(w[13:17]) == 0 {
		return ProbeResult{Verdict: ProbeReject, Reason: "OpenWire WireFormatInfo version must be positive"}
	}
	if len(w) < 18 {
		return openWireProbeNeed(w, 18, "incomplete OpenWire properties marker")
	}
	present := w[17]
	if present > 1 {
		return ProbeResult{Verdict: ProbeReject, Reason: "invalid OpenWire properties marker"}
	}
	if present == 0 {
		if bodyLength != 14 {
			return ProbeResult{Verdict: ProbeReject, Reason: "null OpenWire properties have trailing bytes"}
		}
		if len(w) < total {
			return openWireProbeNeed(w, total, "incomplete OpenWire WireFormatInfo")
		}
		return probeAccept("openwire", "wireformatinfo", 98)
	}
	if len(w) < 22 {
		return openWireProbeNeed(w, 22, "incomplete OpenWire properties length")
	}
	propertiesLength := uint64(binary.BigEndian.Uint32(w[18:22]))
	if propertiesLength > openWireMaxPropertyLen || bodyLength != openWireInfoBodyHeader+propertiesLength {
		return ProbeResult{Verdict: ProbeReject, Reason: "OpenWire properties length does not match WireFormatInfo"}
	}
	if len(w) < total {
		return openWireProbeNeed(w, total, "incomplete OpenWire WireFormatInfo properties")
	}
	return probeAccept("openwire", "wireformatinfo", 98)
}

func openWireProbeNeed(w []byte, want int, reason string) ProbeResult {
	return ProbeResult{
		Verdict:    ProbeNeedMore,
		Protocol:   "openwire",
		Version:    "wireformatinfo",
		NeedBytes:  want - len(w),
		Confidence: 60,
		Reason:     reason,
	}
}

// OpenWire command identifiers below are the command/object identifiers in
// the Apache ActiveMQ OpenWire v2 table, excluding nested value-only types.
// Framing is supported only while the standard size prefix is present.
func validOpenWireCommandType(command byte) bool {
	switch command {
	case 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
		14, 15, 16, 17, 18,
		21, 22, 23, 24, 25, 26, 27, 28, 30, 31, 32, 33, 34,
		40, 50, 52, 53, 54, 55, 60, 61, 65, 90, 91:
		return true
	default:
		return false
	}
}

func (f *binFlow) frameOpenWire(w []byte) (int, *binSpec, error) {
	if len(w) < 4 {
		return 0, nil, nil
	}
	bodyLength := uint64(binary.BigEndian.Uint32(w[:4]))
	if bodyLength == 0 {
		return 0, nil, protocolError(ErrMalformedMessage, "openwire: command length must include a type byte")
	}
	maxFrame := f.a.budget.MaxFrameBytes
	if maxFrame < 5 || bodyLength+4 > uint64(maxFrame) {
		return maxFrame + 1, nil, nil
	}
	if len(w) < 5 {
		return 0, nil, nil
	}
	command := w[4]
	if !validOpenWireCommandType(command) {
		return 0, nil, protocolError(ErrMalformedMessage, "openwire: unsupported command type %d", command)
	}
	total := int(bodyLength + 4)
	if len(w) < total {
		return total, nil, nil
	}
	if command == openWireInfoType {
		return total, f.spec("extended_protocols", "ActiveMQOpenWire"), nil
	}
	return total, f.spec("extended_protocols", "ActiveMQOpenWireOpaque"), nil
}

func openWireInfoHeader(w []byte) bool {
	return len(w) >= 13 && w[4] == openWireInfoType && bytes.Equal(w[5:13], []byte(openWireMagic))
}
