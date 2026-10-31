package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Native UDP selection is separate from the historical mode3/4 byte-stream
// profile. Control messages have their own twelve-byte header, not timestamps.
func decodeNTPDatagramWire(w []byte) (map[string]any, error) {
	if len(w) == 0 {
		return nil, protocolError(ErrMalformedMessage, "NTP empty datagram")
	}
	v, mode := int(w[0]>>3&7), int(w[0]&7)
	if v < 1 || v > 4 || v != 1 && (mode < 1 || mode > 6) {
		return nil, protocolError(ErrUnsupportedFeature, "NTP version/mode is outside the selected UDP profile")
	}
	ranges := map[string]any{}
	m := map[string]any{"Version": v, "Leap Indicator": int(w[0] >> 6), "Wire Field Ranges": ranges}
	span := func(k string, a, b int) { ranges[k] = []any{a, b} }
	for _, k := range []string{"Version", "Leap Indicator"} {
		span(k, 0, 1)
	}
	if v == 1 {
		m["Legacy Reserved"] = mode
		span("Legacy Reserved", 0, 1)
		if mode != 0 {
			return m, protocolError(ErrMalformedMessage, "NTPv1 low three bits are reserved, not a modern mode")
		}
	} else {
		m["Mode"] = mode
		span("Mode", 0, 1)
	}
	if v != 1 && mode == 6 {
		return decodeNTPControlWire(w, m, span)
	}
	if len(w) < 48 {
		return m, protocolError(ErrMalformedMessage, "NTP time header requires48 bytes, got%d", len(w))
	}
	names := [...]string{"", "Symmetric active", "Symmetric passive", "Client", "Server", "Broadcast"}
	m["Stratum"] = int(w[1])
	m["Poll"], m["Precision"] = int(int8(w[2])), int(int8(w[3]))
	if v <= 2 {
		if v == 1 {
			m["Packet Name"] = "Legacy time header"
		} else {
			m["Packet Name"] = names[mode]
		}
		m["Synchronizing Distance Raw"] = binary.BigEndian.Uint32(w[4:8])
		m["Synchronizing Distance Seconds"] = float64(binary.BigEndian.Uint32(w[4:8])) / 65536
		span("Synchronizing Distance Raw", 4, 8)
		if v == 1 {
			m["Estimated Drift Rate Raw"] = binary.BigEndian.Uint32(w[8:12])
			m["Estimated Drift Rate"] = float64(binary.BigEndian.Uint32(w[8:12])) / 4294967296
			span("Estimated Drift Rate Raw", 8, 12)
			m["Legacy Format Scope"] = "RFC1059: reserved low bits, unsigned16.16 distance and unsigned0.32 drift; no modern mode or dispersion inference"
		} else {
			m["Synchronizing Dispersion Raw"] = binary.BigEndian.Uint32(w[8:12])
			m["Synchronizing Dispersion Seconds"] = float64(binary.BigEndian.Uint32(w[8:12])) / 65536
			span("Synchronizing Dispersion Raw", 8, 12)
			m["Legacy Format Scope"] = "RFC1119: unsigned16.16 synchronizing distance and dispersion; not a signed modern root-delay value"
		}
	} else {
		m["Packet Name"] = names[mode]
		m["Root Delay Raw"], m["Root Dispersion Raw"] = binary.BigEndian.Uint32(w[4:8]), binary.BigEndian.Uint32(w[8:12])
		m["Root Delay Seconds"] = float64(int32(binary.BigEndian.Uint32(w[4:8]))) / 65536
		m["Root Dispersion Seconds"] = float64(binary.BigEndian.Uint32(w[8:12])) / 65536
		span("Root Delay Raw", 4, 8)
		span("Root Dispersion Raw", 8, 12)
	}
	m["Reference ID"] = hex.EncodeToString(w[12:16])
	for _, f := range []struct {
		key  string
		a, b int
	}{{"Stratum", 1, 2}, {"Poll", 2, 3}, {"Precision", 3, 4}, {"Reference ID", 12, 16}} {
		span(f.key, f.a, f.b)
	}
	for i, name := range []string{"Reference", "Origin", "Receive", "Transmit"} {
		at := 16 + 8*i
		m[name+" Timestamp"] = bytes.Clone(w[at : at+8])
		m[name+" Timestamp Seconds"] = binary.BigEndian.Uint32(w[at : at+4])
		m[name+" Timestamp Fraction"] = binary.BigEndian.Uint32(w[at+4 : at+8])
		span(name+" Timestamp", at, at+8)
		span(name+" Timestamp Seconds", at, at+4)
		span(name+" Timestamp Fraction", at+4, at+8)
	}
	m["Timestamp Scope"] = "literal seconds/fraction; era, absolute UTC and synchronized clock state are not inferred"
	if v != 1 && mode == 3 {
		m["Role"] = "request"
	}
	if v != 1 && mode == 4 {
		m["Role"], m["Association"] = "response", "unavailable-stateless-datagram"
	}
	stratumLimit := byte(16)
	if v == 2 {
		stratumLimit = 15
	}
	if v != 1 && w[1] > stratumLimit {
		return m, protocolError(ErrMalformedMessage, "NTPv%d stratum%d is outside0..%d", v, w[1], stratumLimit)
	}
	if len(w) != 48 {
		m["Trailing Bytes"], m["Trailing Content"] = len(w)-48, "extension-or-authentication-unparsed"
		return m, protocolError(ErrUnsupportedFeature, "NTP extensions/authentication require an unsupported profile")
	}
	return m, nil
}

func ntpControlOpcode(n byte) string {
	names := [...]string{"Reserved", "Read status", "Read variables", "Write variables", "Read clock variables", "Write clock variables", "Set trap", "Trap", "Runtime configuration", "Export configuration", "Remote address statistics", "Ordered list", "Client nonce"}
	if int(n) < len(names) {
		return names[n]
	}
	if n == 31 {
		return "Unset trap"
	}
	return "Reserved"
}

func decodeNTPControlWire(w []byte, m map[string]any, span func(string, int, int)) (map[string]any, error) {
	m["Packet Name"] = "Control"
	if len(w) < 12 {
		return m, protocolError(ErrMalformedMessage, "NTP control header requires12 bytes")
	}
	r, e, more := w[1]&128 != 0, w[1]&64 != 0, w[1]&32 != 0
	op := w[1] & 31
	m["Response"], m["Control Error"], m["More Fragments"], m["Opcode"], m["Opcode Name"] = r, e, more, int(op), ntpControlOpcode(op)
	for _, k := range []string{"Response", "Control Error", "More Fragments", "Opcode"} {
		span(k, 1, 2)
	}
	for i, k := range []string{"Sequence Number", "Status Word", "Association ID", "Fragment Offset", "Data Count"} {
		m[k] = binary.BigEndian.Uint16(w[2+2*i : 4+2*i])
		span(k, 2+2*i, 4+2*i)
	}
	n, off := int(binary.BigEndian.Uint16(w[10:12])), int(binary.BigEndian.Uint16(w[8:10]))
	if w[0]>>6 != 0 || int(w[0]>>3&7) < 2 {
		return m, protocolError(ErrMalformedMessage, "NTP control requires zero LI and version2..4")
	}
	if n > 468 || n > len(w)-12 {
		return m, protocolError(ErrMalformedMessage, "NTP control data count%d exceeds468 or captured extent%d", n, len(w)-12)
	}
	m["Control Data"] = bytes.Clone(w[12 : 12+n])
	span("Control Data", 12, 12+n)
	tail, pad := w[12+n:], (-n)&3
	unknownTrailer := false
	// Padding may be omitted without an authenticator. Other residual bytes
	// stay opaque instead of being mistaken for control data or verified MAC.
	if len(tail) == 0 {
		m["Padding Bytes"] = []byte{}
	} else if len(tail) >= pad && bytes.Equal(tail[:pad], make([]byte, pad)) {
		m["Padding Bytes"] = bytes.Clone(tail[:pad])
		span("Padding Bytes", 12+n, 12+n+pad)
		if len(tail) > pad {
			unknownTrailer = len(tail)-pad != 20 && len(tail)-pad != 24
			if unknownTrailer {
				m["Unselected Trailer Bytes"] = bytes.Clone(tail[pad:])
				span("Unselected Trailer Bytes", 12+n+pad, len(w))
			} else {
				m["Authenticator Bytes"] = bytes.Clone(tail[pad:])
				span("Authenticator Bytes", 12+n+pad, len(w))
				m["Authentication Verified"] = false
				m["Authenticator Scope"] = "uninterpreted trailer; no algorithm, MAC or sender verification"
			}
		}
	} else {
		return m, protocolError(ErrMalformedMessage, "NTP control padding must be zero and bounded by the data count")
	}
	if !r && (e || more || off != 0 || binary.BigEndian.Uint16(w[4:6]) != 0) {
		return m, protocolError(ErrMalformedMessage, "NTP control request has response/error/fragment/status state")
	}
	if r {
		m["Role"] = "response"
	} else {
		m["Role"] = "request"
	}
	status, assoc := binary.BigEndian.Uint16(w[4:6]), binary.BigEndian.Uint16(w[6:8])
	switch {
	case !r:
		m["Status Word Type"] = "request-reserved"
	case e:
		m["Reported Error Code"], m["Status Word Type"] = int(status>>8), "error"
	case op == 6:
		m["Status Word Type"] = "insignificant-set-trap-response"
	case ntpControlOpcode(op) == "Reserved":
		m["Status Word Type"] = "unselected-opcode"
	case op == 4 || op == 5:
		m["Clock Status High Byte"], m["Clock Status Low Byte"], m["Status Word Type"] = int(status>>8), int(status&255), "clock"
		m["Clock Status Scope"] = "literal high/low bytes; clock-word variants are not negotiated, no hardware state inferred"
	case assoc == 0:
		m["Reported Leap Indicator"], m["Reported Clock Source"], m["Status Word Type"] = int(status>>14), int(status>>8&63), "system"
	default:
		m["Reported Peer Flags"], m["Reported Peer Selection"], m["Status Word Type"] = int(status>>11), int(status>>8&7), "peer"
	}
	if r && !e && op != 4 && op != 5 && op != 6 && ntpControlOpcode(op) != "Reserved" {
		m["Reported Event Count"], m["Reported Event Code"] = int(status>>4&15), int(status&15)
	}
	m["Status Scope"] = "reported control status; no local authentication, reachability or synchronization verification"
	m["Control Summary"] = fmt.Sprintf("%s seq=%d assoc=%d off=%d count=%d", ntpControlOpcode(op), binary.BigEndian.Uint16(w[2:4]), assoc, off, n)
	if unknownTrailer {
		return m, protocolError(ErrUnsupportedFeature, "NTP control trailer length is outside selected20/24-byte authenticator envelopes")
	}
	return m, nil
}
