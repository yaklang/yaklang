package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

const knxExtendedProfile = "knx-extended-search"

func knxExtendedType(w []byte) bool {
	if len(w) < 4 {
		return false
	}
	t := binary.BigEndian.Uint16(w[2:4])
	return t == 0x20b || t == 0x20c
}

// SRP widths follow the constructors in Calimero Core v2.5.1 at
// 13badcae4466072561ce26d0d0719b8628651c84. Binary parser permissiveness does
// not establish validity of zero length, odd RequestDibs or cross-block reads.
// Duplicate selectors and possible zero padding remain ordered observations.
func decodeKNXSearchParameters(w []byte, limit int) ([]map[string]any, error) {
	out := []map[string]any{}
	for at := 0; at < len(w); {
		if len(w)-at < 2 {
			return nil, discoveryError(ErrMalformedMessage, "short SRP header")
		}
		n, t := int(w[at]), w[at+1]&0x7f
		if n < 2 || n > len(w)-at {
			return nil, discoveryError(ErrMalformedMessage, "SRP length boundary")
		}
		if len(out) >= limit {
			return nil, discoveryError(ErrResourceExceeded, "SRP count exceeds collection budget")
		}
		v := w[at+2 : at+n]
		width := 0
		switch t {
		case 1:
			width = 2
		case 2:
			width = 8
		case 3:
			width = 4
		case 4:
			if len(v) == 0 {
				return nil, discoveryError(ErrMalformedMessage, "RequestDibs needs at least one code")
			}
			if len(v)%2 != 0 {
				return nil, discoveryError(ErrUnsupportedFeature, "RequestDibs outside even constructor-shaped profile")
			}
			if len(v) > limit || limit < 9 {
				return nil, discoveryError(ErrResourceExceeded, "RequestDibs field/byte collection budget")
			}
		default:
			return nil, discoveryError(ErrUnsupportedFeature, "unknown or test-only SRP selection")
		}
		if width != 0 && n != width {
			return nil, discoveryError(ErrMalformedMessage, "SRP typed field width")
		}
		fields := 6
		if t != 1 {
			fields = 5 + int(t)
		}
		if limit < fields {
			return nil, discoveryError(ErrResourceExceeded, "SRP field map exceeds collection budget")
		}
		item := map[string]any{"length": n, "type": t, "mandatory": w[at+1]&0x80 != 0, "raw_hex": hex.EncodeToString(w[at : at+n]), "data_hex": hex.EncodeToString(v)}
		switch t {
		case 1:
			item["selection"] = "programming-mode-enabled"
		case 2:
			item["selection"], item["mac_hex"] = "mac-address", hex.EncodeToString(v)
		case 3:
			item["selection"], item["family_id"], item["minimum_version"] = "service-family-at-least-version", v[0], v[1]
		case 4:
			codes, zeros := make([]int, 0, len(v)), []int{}
			for i, b := range v {
				codes = append(codes, int(b))
				if b == 0 {
					zeros = append(zeros, i)
				}
			}
			item["selection"], item["requested_dib_octets"], item["zero_octet_positions"] = "requested-dibs", codes, zeros
			item["normalization"] = "preserve all wire octets including possible constructor padding; no guessed removal"
		}
		out = append(out, item)
		at += n
	}
	return out, nil
}

// Calculate selected field depth without allocating or reading across a block.
func knxExtendedFieldDepth(w []byte) int {
	depth := 2 // header and HPAI; an empty selector/DIB list adds no deeper node
	if len(w) < 14 {
		return depth
	}
	request := binary.BigEndian.Uint16(w[2:4]) == 0x20b
	for at := 14; at+2 <= len(w); {
		n, t := int(w[at]), w[at+1]
		depth = max(depth, 3)
		if request && t&0x7f == 4 {
			depth = max(depth, 4)
		} else if !request && (t == 2 || t == 6) {
			depth = 5
		}
		if n < 2 || n > len(w)-at {
			break
		}
		at += n
	}
	return depth
}
