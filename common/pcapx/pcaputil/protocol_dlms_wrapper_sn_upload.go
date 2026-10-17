package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

func wrapperIsSNUpload(w []byte) bool {
	if len(w) < 10 || w[0] != 0 || w[1] != 1 || w[8] != 5 && w[8] != 12 {
		return false
	}
	_, at, ok := wrapperSNDefinitePrefix(w, 9)
	return ok && at < len(w) && (w[8] == 5 && w[at] == 6 || w[8] == 12 && w[at] == 3)
}
func wrapperSNUploadRefusalHint(w []byte) bool {
	if !wrapperIsSNUpload(w) || int(binary.BigEndian.Uint16(w[6:8])) != len(w)-8 {
		return false
	}
	n, at, ok := wrapperSNDefinitePrefix(w, 9)
	if !ok || n != 1 {
		return false
	}
	if w[8] == 12 {
		return len(w) == at+3
	}
	if len(w) < at+5 || w[at+1] > 1 {
		return false
	}
	n, end, ok := wrapperSNDefinitePrefix(w, at+4)
	return ok && n == uint64(len(w)-end)
}
func decodeWrapperSNUpload(w []byte, limit int) (*wrapperMessage, error) {
	p := w[8:]
	c := wrapperListCursor{wire: p, at: 1}
	n, det, err := c.count()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, wrapperError(ErrUnsupportedFeature, "SN upload selected single access/result only")
	}
	if limit < 1 {
		return nil, wrapperError(ErrResourceExceeded, "SN upload collection budget")
	}
	start := c.at
	ch, err := c.take(1)
	if err != nil {
		return nil, err
	}
	src, dst := binary.BigEndian.Uint16(w[2:4]), binary.BigEndian.Uint16(w[4:6])
	f := map[string]any{"wrapper_version": uint16(1), "source_wport": src, "destination_wport": dst, "wrapper_header_hex": hex.EncodeToString(w[:8]), "payload_length": len(p), "apdu_hex": hex.EncodeToString(p), "command": p[0], "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": n, "list_count_encoding_hex": det}
	m := &wrapperMessage{fields: f, source: src, destination: dst, command: p[0], request: p[0] == 5, snRead: true, snServiceBlock: true, snUpload: true, count: 1}
	row := map[string]any{"index": 0, "apdu_offset": start}
	if m.request {
		if ch[0] != 6 {
			return nil, wrapperError(ErrUnsupportedFeature, "SN upload request selected choice6 only")
		}
		last, err := c.take(1)
		if err != nil {
			return nil, err
		}
		if last[0] > 1 {
			return nil, wrapperError(ErrUnsupportedFeature, "SN upload last-block selected canonical0/1 only")
		}
		m.lastBlock = last[0] == 1
		row["access_choice_raw"], row["access_choice"], row["last_block_raw"], row["last_block"] = ch[0], "read-data-block-access", last[0], m.lastBlock
		f["kind"] = "ReadRequest"
		f["transfer_complete"] = false
	} else {
		if ch[0] != 3 {
			return nil, wrapperError(ErrUnsupportedFeature, "SN upload echo selected response3 only")
		}
		row["result_choice_raw"], row["result_choice"] = ch[0], "block-number"
		f["kind"] = "ReadResponse"
	}
	bn, err := c.take(2)
	if err != nil {
		return nil, err
	}
	m.blockNumber = uint32(binary.BigEndian.Uint16(bn))
	row["block_number"] = uint16(m.blockNumber)
	if m.request {
		n, enc, err := c.count()
		if err != nil {
			return nil, err
		}
		if n > wrapperSNBlockBytes {
			return nil, wrapperError(ErrResourceExceeded, "SN upload raw block byte budget")
		}
		m.blockData, err = c.take(int(n))
		if err != nil {
			return nil, err
		}
		row["raw_data_length"], row["length_encoding_hex"], row["raw_data_hex"] = n, enc, hex.EncodeToString(m.blockData)
	}
	if c.at != len(p) {
		return nil, wrapperError(ErrMalformedMessage, "SN upload trailing bytes")
	}
	row["encoded_length"], row["raw_hex"] = c.at-start, hex.EncodeToString(p[start:c.at])
	if m.request {
		f["descriptors"] = []map[string]any{row}
	} else {
		f["results"] = []map[string]any{row}
	}
	return m, nil
}

// The source handler passes the concatenated unread bytes to ReadRequest's
// count/access parser. Only the command is known from context; no count/choice
// bytes may be invented. Offsets below are in this literal assembled body.
func decodeSNUploadBody(body []byte, limit, depth int) (map[string]any, error) {
	wire := make([]byte, 9+len(body))
	wire[1] = 1
	binary.BigEndian.PutUint16(wire[6:8], uint16(1+len(body)))
	wire[8] = 5
	copy(wire[9:], body)
	m, err := decodeWrapperSNRead(wire, limit, depth)
	if err != nil {
		return nil, err
	}
	f := m.fields
	for _, k := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex", "apdu_hex", "payload_length"} {
		delete(f, k)
	}
	f["kind"], f["body_hex"], f["body_length"] = "ReadRequestBody", hex.EncodeToString(body), len(body)
	for _, row := range f["descriptors"].([]map[string]any) {
		row["body_offset"] = row["apdu_offset"].(int) - 1
		delete(row, "apdu_offset")
	}
	return f, nil
}
func wrapperSNGroupProfile(key wrapperSNKey) string {
	if key.write {
		return "dlms-wrapper-v1-sn-write-upload"
	}
	if key.upload {
		return "dlms-wrapper-v1-sn-read-upload"
	}
	return "dlms-wrapper-v1-sn-read-block"
}

func wrapperSNGroupCloseSummary(key wrapperSNKey) string {
	if key.write {
		return "DLMS Wrapper SN Write upload wire group ended before complete request body"
	}
	if key.upload {
		return "DLMS Wrapper SN Read upload wire group ended before complete request body"
	}
	return "DLMS Wrapper SN Read wire group ended before complete Data"
}
func wrapperSNGroupCloseFields(key wrapperSNKey, p *wrapperSNGroup) map[string]any {
	f := map[string]any{"Buffered SN Groups": 1, "source_wport": key.source, "destination_wport": key.destination, "observed_block_count": p.blocks, "block_number": p.last}
	if key.upload {
		f["assembled_body_length"] = len(p.data)
	} else {
		f["assembled_data_length"] = len(p.data)
	}
	return f
}
