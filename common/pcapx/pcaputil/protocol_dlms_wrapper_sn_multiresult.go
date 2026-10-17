package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

// Explicit selected outer Wrapper or complete M0 HDLC observations use this
// bounded syntax decoder. The default shared decoder for joined information,
// nested GBT and single-result service blocks stays unchanged. A row list
// carries no invoke or per-row transfer owner.
func wrapperSNMultiEnvelope(w []byte) bool {
	if len(w) < 10 || w[0] != 0 || w[1] != 1 || int(binary.BigEndian.Uint16(w[6:8])) != len(w)-8 || w[8] != 12 && w[8] != 13 {
		return false
	}
	n, _, ok := wrapperSNDefinitePrefix(w, 9)
	return ok && n >= 2
}

func decodeWrapperSNMultiResult(w []byte, limit, depth int) (*wrapperMessage, error) {
	p := w[8:]
	c := wrapperListCursor{wire: p, at: 1, maxDepth: depth}
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	if n > uint64(min(limit, wrapperSNReadItems)) {
		service := "Read"
		if p[0] == 13 {
			service = "Write"
		}
		return nil, wrapperError(ErrResourceExceeded, "SN "+service+" list exceeds selected collection budget")
	}
	src, dst := binary.BigEndian.Uint16(w[2:4]), binary.BigEndian.Uint16(w[4:6])
	f := map[string]any{"wrapper_version": uint16(1), "source_wport": src, "destination_wport": dst, "payload_length": len(p), "wrapper_header_hex": hex.EncodeToString(w[:8]), "apdu_hex": hex.EncodeToString(p), "command": p[0], "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": n, "list_count_encoding_hex": enc}
	m := &wrapperMessage{fields: f, source: src, destination: dst, command: p[0], snRead: p[0] == 12, snWrite: p[0] == 13, count: int(n)}
	fail := func(err error) (*wrapperMessage, error) { return m, err }
	rows := make([]map[string]any, 0, int(n))
	dataCount, errorCount, successCount, totalRaw := 0, 0, 0, 0
	for i := 0; i < int(n); i++ {
		at := c.at
		choice, err := c.take(1)
		if err != nil {
			return fail(err)
		}
		row := map[string]any{"index": i, "apdu_offset": at, "result_choice_raw": choice[0]}
		switch choice[0] {
		case 0:
			if m.snRead {
				data, err := c.data(limit, 0)
				if err != nil {
					return fail(err)
				}
				row["result_choice"], row["data"] = "data", data
				dataCount++
			} else {
				row["result_choice"], row["result_success"] = "success", true
				successCount++
			}
		case 1:
			v, err := c.take(1)
			if err != nil {
				return fail(err)
			}
			if !wrapperDataAccessResultKnown(v[0]) {
				if m.snRead {
					return fail(wrapperError(ErrUnsupportedFeature, "SN Read data-access-result outside pinned enumeration"))
				}
				return fail(wrapperError(ErrUnsupportedFeature, "SN Write data-access-result outside pinned enumeration"))
			}
			row["result_choice"], row["data_access_result"] = "data-access-error", v[0]
			errorCount++
			if m.snWrite {
				row["result_success"] = false
			}
		case 2:
			m.snMultiResult = true
			if m.snRead {
				v, err := c.take(1)
				if err != nil {
					return fail(err)
				}
				if v[0] > 1 {
					return fail(wrapperError(ErrUnsupportedFeature, "SN multiresult last-block selected canonical0/1 only"))
				}
				row["result_choice"], row["last_block_raw"], row["last_block"] = "data-block-result", v[0], v[0] == 1
			} else {
				row["result_choice"] = "block-number"
			}
			v, err := c.take(2)
			if err != nil {
				return fail(err)
			}
			row["block_number"] = binary.BigEndian.Uint16(v)
			if m.snRead {
				size, encoding, err := c.count()
				if err != nil {
					return fail(err)
				}
				if size > uint64(min(limit, wrapperSNBlockBytes)-totalRaw) {
					return fail(wrapperError(ErrResourceExceeded, "SN multiresult aggregate opaque data exceeds selected/caller byte budget"))
				}
				raw, err := c.take(int(size))
				if err != nil {
					return fail(err)
				}
				totalRaw += len(raw)
				row["raw_data_length"], row["length_encoding_hex"], row["raw_data_hex"] = size, encoding, hex.EncodeToString(raw)
			}
		case 3:
			if m.snWrite {
				return fail(wrapperError(ErrUnsupportedFeature, "SN Write selected response success/error choices0/1 only"))
			}
			m.snMultiResult = true
			v, err := c.take(2)
			if err != nil {
				return fail(err)
			}
			row["result_choice"], row["block_number"] = "block-number", binary.BigEndian.Uint16(v)
		default:
			if m.snRead {
				return fail(wrapperError(ErrUnsupportedFeature, "SN Read selected response Data/error choices0/1 only"))
			}
			return fail(wrapperError(ErrUnsupportedFeature, "SN Write selected response success/error choices0/1 only"))
		}
		row["encoded_length"], row["raw_hex"] = c.at-at, hex.EncodeToString(p[at:c.at])
		rows = append(rows, row)
	}
	if c.at != len(p) {
		if m.snRead {
			return fail(wrapperError(ErrMalformedMessage, "SN Read list has trailing bytes"))
		}
		return fail(wrapperError(ErrMalformedMessage, "SN Write has trailing bytes"))
	}
	f["results"], f["error_item_count"] = rows, errorCount
	if m.snRead {
		f["kind"], f["data_item_count"] = "ReadResponse", dataCount
	} else {
		f["kind"], f["success_item_count"] = "WriteResponse", successCount
	}
	return m, nil
}

// Normalize to the observed caller orientation, not an unordered endpoint pair.
// Multirow response block choices do not identify which old row/group they
// belong to. Retire matching SN modes only after the complete list is valid.
func (s *binDLMSWrapper) retireSNMultiResult(m *wrapperMessage, dir int) {
	canonical := wrapperGBTKey{1 - dir, m.destination, m.source}
	for k := range s.snGroups {
		owner := k.wrapperGBTKey
		if !k.upload {
			owner = wrapperGBTKey{1 - owner.dir, owner.destination, owner.source}
		}
		if owner == canonical {
			s.snGroups[k].retire()
		}
	}
}
