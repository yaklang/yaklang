package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net/netip"
)

// WithSessionRTPS13Parameters selects literal legacy inline parameter fields.
// A reported locator is not a verified endpoint. Filter signature word records
// are neither a filter evaluation nor a cryptographic authentication result.
func WithSessionRTPS13Parameters(port uint16) ProtocolSessionOption {
	return withSessionRTPS13Profile(port, "rtps13-parameters")
}
func decodeRTPS13Parameters(w []byte, budget ParserBudget) (map[string]any, error) {
	return decodeRTPS13Mode(w, budget, true)
}
func rtps13ParameterFields(id uint16, b []byte, o binary.ByteOrder, budget ParserBudget, remaining *int) (map[string]any, error) {
	if id >= 0x2f && id <= 0x33 {
		if len(b) < 24 {
			return nil, rtpsMalformed("legacy locator fixed fields are truncated")
		}
		if budget.MaxRecursionDepth < 6 {
			return nil, protocolError(ErrResourceExceeded, "legacy locator projection depth")
		}
		kind := int32(o.Uint32(b))
		f := map[string]any{"Kind": kind, "Port": o.Uint32(b[4:]), "Address Bytes": bytes.Clone(b[8:24]), "Interpretation": "unselected-locator-kind"}
		switch kind {
		case 1:
			f["Interpretation"] = "reported-udp-v4-locator"
			f["IPv4 Address"] = netip.AddrFrom4([4]byte(b[20:24])).String()
			f["IPv4 Prefix Bytes"] = bytes.Clone(b[8:20])
		case 2:
			f["Interpretation"] = "reported-udp-v6-locator"
			f["IPv6 Address"] = netip.AddrFrom16([16]byte(b[8:24])).String()
		}
		if len(b) > 24 {
			f["Extension Bytes"] = bytes.Clone(b[24:])
		}
		return f, nil
	}
	if id != 0x55 {
		return nil, nil
	}
	if len(b) < 8 {
		return nil, rtpsMalformed("legacy content-filter sequences are truncated")
	}
	n := uint64(o.Uint32(b))
	if n > uint64((len(b)-8)/4) {
		return nil, rtpsMalformed("legacy content-filter bitmap is truncated")
	}
	at := 4 + int(n)*4
	count := uint64(o.Uint32(b[at:]))
	at += 4
	// The selected observer is bounded to the four records in the pinned IDL.
	// Do not label a wider legacy encoding globally invalid by inference.
	if count > 4 {
		return nil, protocolError(ErrResourceExceeded, "legacy content-filter observer supports four signature records")
	}
	if count > uint64((len(b)-at)/16) {
		return nil, rtpsMalformed("legacy content-filter signature records are truncated")
	}
	depth := 6
	if n > 0 {
		depth = 7
	}
	if count > 0 {
		depth = 8
	}
	if budget.MaxRecursionDepth < depth {
		return nil, protocolError(ErrResourceExceeded, "legacy content-filter projection depth")
	}
	charge := n + count*5 // bitmap words, record entries and their four words share the datagram allowance.
	if charge > uint64(*remaining) {
		return nil, protocolError(ErrResourceExceeded, "legacy content-filter collection budget")
	}
	*remaining -= int(charge)
	ws := make([]uint32, int(n))
	for i := range ws {
		ws[i] = o.Uint32(b[4+i*4:])
	}
	ss := make([]any, int(count))
	for i := range ss {
		record := make([]uint32, 4)
		for j := range record {
			record[j] = o.Uint32(b[at+i*16+j*4:])
		}
		ss[i] = record
	}
	f := map[string]any{"Interpretation": "unverified-content-filter-word-records", "Bitmap Words": ws, "Signature Words": ss}
	at += int(count) * 16
	if at < len(b) {
		f["Extension Bytes"] = bytes.Clone(b[at:])
	}
	return f, nil
}
