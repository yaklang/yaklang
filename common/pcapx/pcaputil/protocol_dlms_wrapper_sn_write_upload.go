package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

// Write choice7 carries no raw bytes in the access specification. Its separate
// list-of-data contains the selected single raw OCTETSTRING. Read choice6 has
// different framing; neither the ASN grammar nor the source handler permits
// silently parsing these two carrier forms interchangeably.
func wrapperIsSNWriteUpload(w []byte) bool {
	if len(w) < 10 || w[0] != 0 || w[1] != 1 || w[8] != 6 && w[8] != 13 {
		return false
	}
	_, at, ok := wrapperSNDefinitePrefix(w, 9)
	return ok && at < len(w) && (w[8] == 6 && w[at] == 7 || w[8] == 13 && w[at] == 2)
}
func wrapperSNWriteUploadRefusalHint(w []byte) bool {
	if !wrapperIsSNWriteUpload(w) || int(binary.BigEndian.Uint16(w[6:8])) != len(w)-8 {
		return false
	}
	n, at, ok := wrapperSNDefinitePrefix(w, 9)
	if !ok || n != 1 {
		return false
	}
	if w[8] == 13 {
		return len(w) == at+3
	}
	if len(w) < at+5 || w[at+1] > 1 {
		return false
	}
	n, at, ok = wrapperSNDefinitePrefix(w, at+4)
	if !ok || n != 1 || at >= len(w) || w[at] != 9 {
		return false
	}
	n, end, ok := wrapperSNDefinitePrefix(w, at+1)
	return ok && n == uint64(len(w)-end)
}
func decodeWrapperSNWriteUpload(w []byte, limit, depth int) (*wrapperMessage, error) {
	p := w[8:]
	c := wrapperListCursor{wire: p, at: 1, maxDepth: depth}
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, wrapperError(ErrUnsupportedFeature, "SN Write upload selected single access/result only")
	}
	if limit < 1 {
		return nil, wrapperError(ErrResourceExceeded, "SN Write upload collection budget")
	}
	at := c.at
	ch, err := c.take(1)
	if err != nil {
		return nil, err
	}
	src, dst := binary.BigEndian.Uint16(w[2:4]), binary.BigEndian.Uint16(w[4:6])
	f := map[string]any{"wrapper_version": uint16(1), "source_wport": src, "destination_wport": dst, "wrapper_header_hex": hex.EncodeToString(w[:8]), "payload_length": len(p), "apdu_hex": hex.EncodeToString(p), "command": p[0], "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": n, "list_count_encoding_hex": enc}
	m := &wrapperMessage{fields: f, source: src, destination: dst, command: p[0], request: p[0] == 6, snWrite: true, snServiceBlock: true, snUpload: true, count: 1}
	row := map[string]any{"index": 0, "apdu_offset": at}
	if m.request {
		if ch[0] != 7 {
			return nil, wrapperError(ErrUnsupportedFeature, "SN Write upload selected access7 only")
		}
		last, err := c.take(1)
		if err != nil {
			return nil, err
		}
		if last[0] > 1 {
			return nil, wrapperError(ErrUnsupportedFeature, "SN Write upload last-block selected canonical0/1 only")
		}
		m.lastBlock = last[0] == 1
		row["access_choice_raw"], row["access_choice"], row["last_block_raw"], row["last_block"] = ch[0], "write-data-block-access", last[0], m.lastBlock
		f["kind"], f["transfer_complete"] = "WriteRequest", false
	} else {
		if ch[0] != 2 {
			return nil, wrapperError(ErrUnsupportedFeature, "SN Write upload selected response2 only")
		}
		row["result_choice_raw"], row["result_choice"] = ch[0], "block-number"
		f["kind"] = "WriteResponse"
	}
	bn, err := c.take(2)
	if err != nil {
		return nil, err
	}
	m.blockNumber = uint32(binary.BigEndian.Uint16(bn))
	row["block_number"] = uint16(m.blockNumber)
	row["encoded_length"], row["raw_hex"] = c.at-at, hex.EncodeToString(p[at:c.at])
	if m.request {
		count, countEnc, err := c.count()
		if err != nil {
			return nil, err
		}
		if count != 1 {
			return nil, wrapperError(ErrUnsupportedFeature, "SN Write upload selected one OCTETSTRING only")
		}
		at = c.at
		tag, err := c.take(1)
		if err != nil {
			return nil, err
		}
		if tag[0] != 9 {
			return nil, wrapperError(ErrUnsupportedFeature, "SN Write upload selected Data OCTETSTRING only")
		}
		if depth < 4 {
			return nil, wrapperError(ErrResourceExceeded, "SN Write upload raw container depth budget")
		}
		size, sizeEnc, err := c.count()
		if err != nil {
			return nil, err
		}
		if size > wrapperSNBlockBytes {
			return nil, wrapperError(ErrResourceExceeded, "SN Write upload raw byte budget")
		}
		m.blockData, err = c.take(int(size))
		if err != nil {
			return nil, err
		}
		value := map[string]any{"type": byte(9), "length": size, "length_encoding_hex": sizeEnc, "value_hex": hex.EncodeToString(m.blockData), "raw_hex": hex.EncodeToString(p[at:c.at])}
		f["descriptors"], f["data_count"], f["data_count_encoding_hex"], f["values"] = []map[string]any{row}, count, countEnc, []map[string]any{{"index": 0, "apdu_offset": at, "data": value, "raw_hex": hex.EncodeToString(p[at:c.at]), "encoded_length": c.at - at}}
	} else {
		f["results"] = []map[string]any{row}
	}
	if c.at != len(p) {
		return nil, wrapperError(ErrMalformedMessage, "SN Write upload trailing bytes")
	}
	return m, nil
}

// Reserve one node for the outer raw Data9 container before projecting the
// complete body. The body cursor shares the remaining pool across every access
// parameter and value, instead of giving two graphs the full caller allowance.
func decodeSNWriteUploadBody(body []byte, limit, depth int) (map[string]any, error) {
	w := make([]byte, 9+len(body))
	w[1] = 1
	binary.BigEndian.PutUint16(w[6:8], uint16(1+len(body)))
	w[8] = 6
	copy(w[9:], body)
	m, err := decodeWrapperSNWriteReserved(w, limit, depth, 1)
	if err != nil {
		return nil, err
	}
	f := m.fields
	for _, k := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex", "apdu_hex", "payload_length"} {
		delete(f, k)
	}
	f["kind"], f["body_hex"], f["body_length"] = "WriteRequestBody", hex.EncodeToString(body), len(body)
	for _, key := range []string{"descriptors", "values"} {
		for _, r := range f[key].([]map[string]any) {
			r["body_offset"] = r["apdu_offset"].(int) - 1
			delete(r, "apdu_offset")
		}
	}
	return f, nil
}
