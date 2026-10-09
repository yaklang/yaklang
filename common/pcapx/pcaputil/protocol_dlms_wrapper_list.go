package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// These are local selected-profile bounds, not DLMS wire maxima. Structured
// Data, block transfer, ciphering and object/selector semantics remain separate.
const wrapperListItems = 64
const wrapperListOctets = 1024

type wrapperListCursor struct {
	wire []byte
	at   int
}

func (c *wrapperListCursor) take(n int) ([]byte, error) {
	if n < 0 || n > len(c.wire)-c.at {
		return nil, wrapperError(ErrMalformedMessage, "selected list field truncated")
	}
	w := c.wire[c.at : c.at+n]
	c.at += n
	return w, nil
}
func (c *wrapperListCursor) count() (uint64, string, error) {
	start := c.at
	w, err := c.take(1)
	if err != nil {
		return 0, "", err
	}
	first := w[0]
	n := uint64(first)
	if first == 0x80 {
		return 0, "", wrapperError(ErrUnsupportedFeature, "literal80 determinant outside selected definite forms")
	}
	if first > 0x80 {
		width := int(first & 127)
		if width < 1 || width > 4 {
			return 0, "", wrapperError(ErrUnsupportedFeature, "list count width outside pinned definite forms1..4")
		}
		w, err = c.take(width)
		if err != nil {
			return 0, "", err
		}
		n = 0
		for _, v := range w {
			n = n<<8 | uint64(v)
		}
	}
	return n, hex.EncodeToString(c.wire[start:c.at]), nil
}
func wrapperScalarSize(tag byte) int {
	switch tag {
	case 0:
		return 0
	case 3, 15, 17, 22:
		return 1
	case 16, 18:
		return 2
	case 5, 6:
		return 4
	case 20, 21:
		return 8
	default:
		return -1
	}
}
func (c *wrapperListCursor) scalar(limit int) (map[string]any, error) {
	start := c.at
	w, err := c.take(1)
	if err != nil {
		return nil, err
	}
	tag := w[0]
	if n := wrapperScalarSize(tag); n >= 0 {
		if _, err = c.take(n); err != nil {
			return nil, err
		}
		return wrapperScalar(c.wire[start:c.at], limit)
	}
	if tag != 9 {
		return nil, wrapperError(ErrUnsupportedFeature, "Data type outside bounded scalar/octet-string profile")
	}
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	if n > uint64(min(limit, wrapperListOctets)) {
		return nil, wrapperError(ErrResourceExceeded, "list octet value exceeds selected byte/collection budget")
	}
	w, err = c.take(int(n))
	if err != nil {
		return nil, err
	}
	return map[string]any{"type": tag, "length": n, "length_encoding_hex": enc, "value_hex": hex.EncodeToString(w), "raw_hex": hex.EncodeToString(c.wire[start:c.at])}, nil
}
func decodeWrapperList(m *wrapperMessage, p []byte, limit int) error {
	c := wrapperListCursor{wire: p, at: 3}
	n, enc, err := c.count()
	if err != nil {
		return err
	}
	if n == 0 {
		return wrapperError(ErrUnsupportedFeature, "empty list outside selected profile; no universal wire-invalid claim")
	}
	if n > uint64(min(limit, wrapperListItems)) {
		return wrapperError(ErrResourceExceeded, "announced list count exceeds selected collection budget")
	}
	m.count = int(n)
	m.fields["list_count"], m.fields["list_count_encoding_hex"] = n, enc
	items := make([]map[string]any, 0, int(n))
	dataCount, errorCount := 0, 0
	for i := 0; i < int(n); i++ {
		start := c.at
		item := map[string]any{"index": i, "apdu_offset": start}
		if m.request {
			w, err := c.take(10)
			if err != nil {
				return err
			}
			if int8(w[8]) <= 0 {
				return wrapperError(ErrUnsupportedFeature, "positive signed8 attribute profile")
			}
			if w[9] != 0 && w[9] != 1 {
				return wrapperError(ErrUnsupportedFeature, "selected optional access selection0/1")
			}
			item["class_id"], item["logical_name_hex"], item["logical_name"] = binary.BigEndian.Uint16(w), hex.EncodeToString(w[2:8]), fmt.Sprintf("%d.%d.%d.%d.%d.%d", w[2], w[3], w[4], w[5], w[6], w[7])
			item["attribute_id"], item["selective_access"], item["access_selection_raw"] = int8(w[8]), w[9] == 1, w[9]
			if w[9] == 1 {
				selector, err := c.take(1)
				if err != nil {
					return err
				}
				parameter, err := c.scalar(limit)
				if err != nil {
					return err
				}
				item["access_selector"], item["access_parameters"], item["selector_semantics_verified"] = selector[0], parameter, false
			}
		} else {
			w, err := c.take(1)
			if err != nil {
				return err
			}
			item["result_choice_raw"] = w[0]
			switch w[0] {
			case 0:
				data, err := c.scalar(limit)
				if err != nil {
					return err
				}
				item["result_choice"], item["data"] = "data", data
				dataCount++
			case 1:
				code, err := c.take(1)
				if err != nil {
					return err
				}
				switch code[0] {
				case 0, 1, 2, 3, 4, 9, 11, 12, 13, 14, 15, 16, 17, 18, 19, 250:
				default:
					return wrapperError(ErrUnsupportedFeature, "data-access-result outside pinned enumeration")
				}
				item["result_choice"], item["data_access_result"] = "data-access-result", code[0]
				errorCount++
			default:
				return wrapperError(ErrUnsupportedFeature, "selected result discriminator0/1")
			}
		}
		item["raw_hex"], item["encoded_length"] = hex.EncodeToString(p[start:c.at]), c.at-start
		items = append(items, item)
	}
	if c.at != len(p) {
		return wrapperError(ErrMalformedMessage, "Get list has unexplained trailing bytes")
	}
	if m.request {
		m.fields["kind"], m.fields["descriptors"] = "GetRequestWithList", items
	} else {
		m.fields["kind"], m.fields["results"] = "GetResponseWithList", items
		m.fields["data_item_count"], m.fields["error_item_count"], m.fields["operation_success_verified"] = dataCount, errorCount, false
	}
	return nil
}
func wrapperIsList(w []byte) bool { return len(w) > 9 && w[9] == 3 }
func wrapperProfile(w []byte) string {
	if wrapperIsList(w) {
		return "dlms-wrapper-v1-get-list"
	}
	return "dlms-wrapper-v1-get-normal"
}
func wrapperProbeVersion(w []byte) string {
	if wrapperIsList(w) {
		return "v1-get-list"
	}
	return "v1-get-normal"
}
func wrapperProjection(w []byte, n int) int64 {
	if !wrapperIsList(w) {
		return wrapperProjectionBytes + 128*int64(n)
	}
	// A null result needs at least two wire bytes but creates a map. Reserve the
	// worst selected list graph and all owned public/native/string projections
	// before raw bytes, item slices, fields or association state are allocated.
	items := min(wrapperListItems, max(0, n-12)/2)
	return 32768 + 512*int64(n) + 8192*int64(items)
}
