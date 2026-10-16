package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

// Write has two independent sequences on wire, not LN invoke/priority flags.
// Equal descriptor/Data counts and at most sixteen rows are selected observer
// bounds, not extra ASN.1 constraints or proof of an object/device operation.
func wrapperIsSNWrite(w []byte) bool {
	return len(w) > 8 && w[0] == 0 && w[1] == 1 && (w[8] == 0x06 || w[8] == 0x0d)
}

func decodeWrapperSNWrite(w []byte, limit, depth int) (*wrapperMessage, error) {
	p := w[8:]
	c := wrapperListCursor{wire: p, at: 1, maxDepth: depth}
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, wrapperError(ErrUnsupportedFeature, "SN Write selected nonempty lists only")
	}
	if n > uint64(min(limit, wrapperSNReadItems)) {
		return nil, wrapperError(ErrResourceExceeded, "SN Write list exceeds selected collection budget")
	}
	src, dst := binary.BigEndian.Uint16(w[2:4]), binary.BigEndian.Uint16(w[4:6])
	f := map[string]any{"wrapper_version": uint16(1), "source_wport": src, "destination_wport": dst, "payload_length": len(p), "wrapper_header_hex": hex.EncodeToString(w[:8]), "apdu_hex": hex.EncodeToString(p), "command": p[0], "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": n, "list_count_encoding_hex": enc}
	m := &wrapperMessage{fields: f, source: src, destination: dst, command: p[0], request: p[0] == 0x06, snWrite: true, count: int(n)}
	rows := make([]map[string]any, 0, int(n))
	successes, failures := 0, 0
	for i := uint64(0); i < n; i++ {
		at := c.at
		choice, err := c.take(1)
		if err != nil {
			return nil, err
		}
		row := map[string]any{"index": int(i), "apdu_offset": at}
		if m.request {
			if err := wrapperSNAccess(&c, row, choice[0], limit); err != nil {
				return nil, err
			}
		} else {
			row["result_choice_raw"] = choice[0]
			switch choice[0] {
			case 0:
				// success is NULL: the choice octet is the entire result, no extra zero.
				row["result_choice"], row["result_success"] = "success", true
				successes++
			case 1:
				code, err := c.take(1)
				if err != nil {
					return nil, err
				}
				if !wrapperDataAccessResultKnown(code[0]) {
					return nil, wrapperError(ErrUnsupportedFeature, "SN Write data-access-result outside pinned enumeration")
				}
				row["result_choice"], row["data_access_result"], row["result_success"] = "data-access-error", code[0], false
				failures++
			default:
				return nil, wrapperError(ErrUnsupportedFeature, "SN Write selected response success/error choices0/1 only")
			}
		}
		row["encoded_length"], row["raw_hex"] = c.at-at, hex.EncodeToString(p[at:c.at])
		rows = append(rows, row)
	}
	if m.request {
		count, countEnc, err := c.count()
		if err != nil {
			return nil, err
		}
		if count != n {
			return nil, wrapperError(ErrUnsupportedFeature, "SN Write selected equal descriptor and Data counts only")
		}
		values := make([]map[string]any, 0, int(count))
		for i := uint64(0); i < count; i++ {
			at := c.at
			data, err := c.data(limit, 0)
			if err != nil {
				return nil, err
			}
			values = append(values, map[string]any{"index": int(i), "apdu_offset": at, "data": data, "encoded_length": c.at - at, "raw_hex": hex.EncodeToString(p[at:c.at])})
		}
		f["kind"], f["descriptors"], f["data_count"], f["data_count_encoding_hex"], f["values"] = "WriteRequest", rows, count, countEnc, values
	} else {
		f["kind"], f["results"], f["success_item_count"], f["error_item_count"] = "WriteResponse", rows, successes, failures
	}
	if c.at != len(p) {
		return nil, wrapperError(ErrMalformedMessage, "SN Write has trailing bytes")
	}
	return m, nil
}
