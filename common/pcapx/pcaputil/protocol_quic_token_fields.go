package pcaputil

import "bytes"

// Token contents are opaque clear-header octets. Keeping the observed encoding
// does not validate an address, a resumption ticket, or the sending peer.
func quicHeaderTokenFacts(h quicHdr, raw []byte) (map[string]any, map[string]any) {
	if !h.long || !quicSupportedVersion(h.version) {
		return nil, nil
	}
	off := 7 + len(h.dcid) + len(h.scid)
	if off > len(raw) {
		return nil, nil
	}
	values, ranges := map[string]any{}, map[string]any{}
	span := func(key string, lo, hi int) { ranges[key] = []any{lo, hi} }
	switch h.typeName {
	case "Initial":
		length, width, err := quicVarint(raw[off:])
		if err != nil || length > 20<<10 || uint64(len(raw)-off-width) < length || !bytes.Equal(h.token, raw[off+width:off+width+int(length)]) {
			return nil, nil
		}
		values["Initial Token Length"], values["Initial Token Length Encoding Bytes"] = length, width
		values["Initial Token"] = bytes.Clone(h.token)
		span("Initial Token Length", off, off+width)
		span("Initial Token", off+width, off+width+int(length))
	case "Retry":
		if len(raw)-off < 16 {
			return nil, nil
		}
		values["Retry Token"] = bytes.Clone(raw[off : len(raw)-16])
		values["Retry Integrity Tag"] = bytes.Clone(raw[len(raw)-16:])
		span("Retry Token", off, len(raw)-16)
		span("Retry Integrity Tag", len(raw)-16, len(raw))
	}
	return values, ranges
}
