package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"unicode/utf8"
)

// These are local selected-profile bounds, not DLMS wire maxima.
// Data types other than scalar/float/octet/bit-string/selected-text/array/structure, GBT, ciphering
// and object/selector semantics remain separate.
const wrapperListItems = 64
const wrapperListOctets = 1024
const wrapperDataNodes = 256
const wrapperDataLevels = 8

type wrapperListCursor struct {
	wire            []byte
	at              int
	nodes, maxDepth int
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
	case 5, 6, 23:
		return 4
	case 20, 21, 24:
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
		if tag == 23 || tag == 24 {
			return wrapperListFloat(c.wire[start:c.at]), nil
		}
		return wrapperScalar(c.wire[start:c.at], limit)
	}
	if tag != 9 && tag != 4 && tag != 10 && tag != 12 {
		return nil, wrapperError(ErrUnsupportedFeature, "Data type outside bounded scalar/octet/bit-string/selected-text profile")
	}
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	bytes := n
	if tag == 4 {
		// A-XDR bit-string counts bits, MSB first, rather than value bytes.
		// Bound the unsigned determinant before conversion or value allocation.
		// Preserve unused low bits in value_hex; they are not semantic bits and
		// their nonzero value alone is not an invalid-message assertion.
		if n > uint64(min(limit, wrapperListOctets))*8 {
			return nil, wrapperError(ErrResourceExceeded, "list bit-string exceeds selected byte/collection budget")
		}
		bytes = (n + 7) / 8
	} else if bytes > uint64(min(limit, wrapperListOctets)) {
		return nil, wrapperError(ErrResourceExceeded, "list octet value exceeds selected byte/collection budget")
	}
	w, err = c.take(int(bytes))
	if err != nil {
		return nil, err
	}
	out := map[string]any{"type": tag, "length_encoding_hex": enc, "value_hex": hex.EncodeToString(w), "raw_hex": hex.EncodeToString(c.wire[start:c.at])}
	if tag == 4 {
		out["bit_length"], out["unused_bits"] = n, (8-n%8)%8
	} else {
		out["length"] = n
	}
	if tag == 10 || tag == 12 {
		encoding := "utf-8"
		if tag == 10 {
			// This profile selects printable US-ASCII. Other visible-string repertoires
			// remain unsupported, rather than being labelled universally malformed.
			for _, b := range w {
				if b < 0x20 || b > 0x7e {
					return nil, wrapperError(ErrUnsupportedFeature, "visible-string outside selected printable ASCII repertoire")
				}
			}
			encoding = "ascii"
		} else if !utf8.Valid(w) {
			// RFC3629: no overlong sequences, surrogates or scalar values above U+10FFFF.
			return nil, wrapperError(ErrMalformedMessage, "Data UTF-8 string contains invalid encoding")
		}
		out["text_encoding"], out["value"], out["code_points"] = encoding, string(w), utf8.RuneCount(w)
	}
	return out, nil
}

// Float Data retains the exact IEEE value bits. Nonfinite values are legal wire
// values, represented as strings so every owned projection remains JSON-safe.
// This does not assign an engineering unit or assert meter measurement validity.
func wrapperListFloat(w []byte) map[string]any {
	width, exponentBits, fractionBits := 64, 11, 52
	bits := uint64(0)
	var number float64
	if w[0] == 23 {
		width, exponentBits, fractionBits = 32, 8, 23
		bits = uint64(binary.BigEndian.Uint32(w[1:]))
		number = float64(math.Float32frombits(uint32(bits)))
	} else {
		bits = binary.BigEndian.Uint64(w[1:])
		number = math.Float64frombits(bits)
	}
	exponent := (bits >> fractionBits) & ((1 << exponentBits) - 1)
	fraction := bits & ((1 << fractionBits) - 1)
	negative := bits>>(width-1) != 0
	class := "normal"
	var value any = number
	if exponent == (1<<exponentBits)-1 {
		if fraction != 0 {
			class, value = "nan", "NaN"
		} else {
			class, value = "infinite", "Infinity"
			if negative {
				value = "-Infinity"
			}
		}
	} else if exponent == 0 {
		class = "subnormal"
		if fraction == 0 {
			class = "zero"
		}
	}
	return map[string]any{"type": w[0], "raw_hex": hex.EncodeToString(w), "value_hex": hex.EncodeToString(w[1:]),
		"float_width": width, "float_class": class, "negative": negative, "value": value}
}

// data consumes exactly one bounded A-XDR value. Its shared node counter spans
// every result and selection parameter in this APDU, including empty containers.
// Root Data maps have projection depth4; each elements slice and child map add2.
func (c *wrapperListCursor) data(limit, level int) (map[string]any, error) {
	if c.nodes >= min(limit, wrapperDataNodes) || level >= wrapperDataLevels || 4+2*level > c.maxDepth {
		return nil, wrapperError(ErrResourceExceeded, "aggregate Data node/depth budget exceeded")
	}
	c.nodes++
	if c.at == len(c.wire) || c.wire[c.at] != 1 && c.wire[c.at] != 2 {
		return c.scalar(limit)
	}
	start := c.at
	tag := c.wire[c.at]
	c.at++
	n, enc, err := c.count()
	if err != nil {
		return nil, err
	}
	// Check unsigned counts before conversion/allocation. Children may themselves
	// contain descendants, whose cost is checked against the same remaining pool.
	if n > uint64(min(limit, wrapperDataNodes)-c.nodes) {
		return nil, wrapperError(ErrResourceExceeded, "announced child count exceeds remaining Data budget")
	}
	children := make([]map[string]any, 0, int(n))
	for i := uint64(0); i < n; i++ {
		child, err := c.data(limit, level+1)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	return map[string]any{"type": tag, "length": n, "length_encoding_hex": enc,
		"elements": children, "raw_hex": hex.EncodeToString(c.wire[start:c.at])}, nil
}

func decodeWrapperList(m *wrapperMessage, p []byte, limit, depth int) error {
	c := wrapperListCursor{wire: p, at: 3, maxDepth: depth}
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
				parameter, err := c.data(limit, 0)
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
				data, err := c.data(limit, 0)
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
	if wrapperIsBlock(w) {
		return "dlms-wrapper-v1-get-block"
	}
	if wrapperIsList(w) {
		return "dlms-wrapper-v1-get-list"
	}
	return "dlms-wrapper-v1-get-normal"
}
func wrapperProbeVersion(w []byte) string {
	if wrapperIsBlock(w) {
		return "v1-get-block"
	}
	if wrapperIsList(w) {
		return "v1-get-list"
	}
	return "v1-get-normal"
}
func wrapperProjection(w []byte, n int) int64 {
	if wrapperIsBlock(w) {
		return 32768 + 512*int64(n+wrapperBlockBytes) + 2048*wrapperDataNodes
	}
	if !wrapperIsList(w) {
		return wrapperProjectionBytes + 128*int64(n)
	}
	// A null result needs at least two wire bytes but creates a map. Reserve the
	// worst selected list graph and all owned public/native/string projections
	// before raw bytes, item slices, fields or association state are allocated.
	items := min(wrapperListItems, max(0, n-12)/2)
	// The additional pool covers all owned descendant map/slice projections.
	// A null child consumes at least one byte; no APDU retains more than256 nodes.
	nodes := min(wrapperDataNodes, max(0, n-12))
	return 32768 + 512*int64(n) + 8192*int64(items) + 2048*int64(nodes)
}
