package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

// A local observer bound, not a wire/ASN.1 maximum. SN Read has no invoke-id
// and neither this decoder nor matching wPorts can establish a paired service,
// object model, negotiated association or successful device operation.
const wrapperSNReadItems = 16

func wrapperIsSNRead(w []byte) bool {
	return len(w) > 8 && w[0] == 0 && w[1] == 1 && (w[8] == 0x05 || w[8] == 0x0c)
}

// Observe complete plaintext variable-name requests and ordered Data/error
// responses. One bounded cursor owns the aggregate Data node/depth accounting
// across every result, including compact-array expansion and empty containers.
func decodeWrapperSNRead(w []byte, limit, depth int) (*wrapperMessage, error) {
	p := w[8:]
	c := wrapperListCursor{wire: p, at: 1, maxDepth: depth}
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, wrapperError(ErrUnsupportedFeature, "SN Read selected nonempty list only")
	}
	if n > uint64(min(limit, wrapperSNReadItems)) {
		return nil, wrapperError(ErrResourceExceeded, "SN Read list exceeds selected collection budget")
	}
	src, dst := binary.BigEndian.Uint16(w[2:4]), binary.BigEndian.Uint16(w[4:6])
	f := map[string]any{
		"wrapper_version": uint16(1), "source_wport": src, "destination_wport": dst,
		"payload_length": len(p), "wrapper_header_hex": hex.EncodeToString(w[:8]), "apdu_hex": hex.EncodeToString(p),
		"command": p[0], "reference_mode": "short-name", "association_observed": false,
		"authentication_verified": false, "object_model_verified": false, "operation_success_verified": false,
		"list_count": n, "list_count_encoding_hex": enc,
	}
	m := &wrapperMessage{fields: f, source: src, destination: dst, command: p[0], request: p[0] == 0x05, snRead: true, count: int(n)}
	items := make([]map[string]any, 0, int(n))
	dataCount, errorCount := 0, 0
	for i := uint64(0); i < n; i++ {
		start := c.at
		choice, err := c.take(1)
		if err != nil {
			return nil, err
		}
		row := map[string]any{"index": int(i), "apdu_offset": start}
		if m.request {
			if choice[0] != 2 {
				return nil, wrapperError(ErrUnsupportedFeature, "SN Read selected variable-name choice2 only")
			}
			name, err := c.take(2)
			if err != nil {
				return nil, err
			}
			// Preserve the full address bit pattern. It is not evidence of a
			// resolved COSEM class/attribute, and high-bit names are not rejected.
			row["access_choice_raw"], row["access_choice"] = choice[0], "variable-name"
			row["variable_name"], row["variable_name_hex"] = binary.BigEndian.Uint16(name), hex.EncodeToString(name)
		} else {
			row["result_choice_raw"] = choice[0]
			switch choice[0] {
			case 0:
				data, err := c.data(limit, 0)
				if err != nil {
					return nil, err
				}
				row["result_choice"], row["data"] = "data", data
				dataCount++
			case 1:
				result, err := c.take(1)
				if err != nil {
					return nil, err
				}
				if !wrapperDataAccessResultKnown(result[0]) {
					return nil, wrapperError(ErrUnsupportedFeature, "SN Read data-access-result outside pinned enumeration")
				}
				row["result_choice"], row["data_access_result"] = "data-access-error", result[0]
				errorCount++
			default:
				return nil, wrapperError(ErrUnsupportedFeature, "SN Read selected response Data/error choices0/1 only")
			}
		}
		row["encoded_length"], row["raw_hex"] = c.at-start, hex.EncodeToString(p[start:c.at])
		items = append(items, row)
	}
	if c.at != len(p) {
		return nil, wrapperError(ErrMalformedMessage, "SN Read list has trailing bytes")
	}
	if m.request {
		f["kind"], f["descriptors"] = "ReadRequest", items
	} else {
		f["kind"], f["results"] = "ReadResponse", items
		f["data_item_count"], f["error_item_count"] = dataCount, errorCount
	}
	return m, nil
}
