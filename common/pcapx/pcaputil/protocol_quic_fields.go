package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
)

// QUIC clear-header ranges belong to this logical wire packet. Short-header CID
// size comes from observed context; neither a version nor an SCID is carried.
func quicHeaderWireRanges(h quicHdr, raw []byte) map[string]any {
	r := map[string]any{}
	span := func(k string, lo, hi int) { r[k] = []any{lo, hi} }
	if len(raw) == 0 || (raw[0]&0x80 != 0) != h.long {
		return r
	}
	if !h.long {
		if len(raw) >= 1+len(h.dcid) && bytes.Equal(raw[1:1+len(h.dcid)], h.dcid) {
			span("DCID", 1, 1+len(h.dcid))
		}
		return r
	}
	if len(raw) < 7+len(h.dcid)+len(h.scid) || int(raw[5]) != len(h.dcid) || int(raw[6+len(h.dcid)]) != len(h.scid) ||
		!bytes.Equal(raw[6:6+len(h.dcid)], h.dcid) || !bytes.Equal(raw[7+len(h.dcid):7+len(h.dcid)+len(h.scid)], h.scid) {
		return r
	}
	span("Version", 1, 5)
	span("DCID Length", 5, 6)
	span("DCID", 6, 6+len(h.dcid))
	span("SCID Length", 6+len(h.dcid), 7+len(h.dcid))
	span("SCID", 7+len(h.dcid), 7+len(h.dcid)+len(h.scid))
	if quicSupportedVersion(h.version) {
		span("Long Packet Type", 0, 1)
	}
	return r
}

func (e *ProtocolEvent) ownedQUICHeaderField(key string) (any, bool) {
	if e == nil || !e.nativeQUICObservation || e.semanticSession == nil || len(e.Raw) == 0 {
		return nil, false
	}
	m := e.semanticSession
	ranges, _ := m["Wire Field Ranges"].(map[string]any)
	span, _ := ranges[key].([]any)
	if len(span) != 2 {
		return nil, false
	}
	lo, ok := displayUint(span[0])
	hi, ok2 := displayUint(span[1])
	if !ok || !ok2 || hi < lo || hi > uint64(len(e.Raw)) {
		return nil, false
	}
	w := e.Raw[lo:hi]
	switch key {
	case "DCID", "SCID":
		b, ok := m[key].([]byte)
		if !ok || len(b) > 20 || !bytes.Equal(w, b) || key == "SCID" && m["Header Form"] != "long" {
			return nil, false
		}
		return hex.EncodeToString(b), true
	case "Version":
		v, ok := displayUint(m["Version"])
		if m["Header Form"] == "long" && len(w) == 4 && ok && uint64(binary.BigEndian.Uint32(w)) == v {
			return v, true
		}
	case "DCID Length", "SCID Length":
		name := "DCID"
		if key == "SCID Length" {
			name = "SCID"
		}
		b, ok := m[name].([]byte)
		if m["Header Form"] == "long" && len(w) == 1 && ok && len(b) == int(w[0]) {
			return uint64(w[0]), true
		}
	case "Long Packet Type":
		version, ok := displayUint(m["Version"])
		if m["Header Form"] == "long" && len(w) == 1 && ok && (version == uint64(quicVersion1) || version == uint64(quicVersion2)) {
			return uint64(w[0] >> 4 & 3), true
		}
	}
	return nil, false
}
