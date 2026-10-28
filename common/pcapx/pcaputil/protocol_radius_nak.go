package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// This observes a NAK's advertised alternatives only. The packet does not prove
// a corresponding inner Request, capability negotiation or authentication result.
func radiusNAKFields(inner map[string]any, code, typ byte, data []byte, max int, note func(string, bool)) (bool, error) {
	if typ == 3 {
		note("eap-nak-context-not-verified", false)
		inner["Type Name"] = "Legacy Nak"
		if code != 2 {
			note("eap-nak-code", true)
		}
		if len(data) == 0 {
			note("eap-nak-empty", true)
		}
		inner["Method Count"] = len(data)
		if len(data) > max {
			return true, protocolError(ErrResourceExceeded, "EAP NAK method count exceeds collection budget")
		}
		inner["Desired Types"] = bytes.Clone(data)
		zero, other := false, false
		for _, v := range data {
			if v == 0 {
				zero = true
			} else {
				other = true
				if v < 4 {
					note("eap-nak-invalid-method", true)
				}
			}
		}
		inner["Method Count"], inner["No Alternative"] = len(data), zero && !other
		if zero && other {
			note("eap-nak-zero-mixed", false)
		}
		return true, nil
	}
	if typ != 254 {
		return false, nil
	}
	if len(data) < 7 {
		return false, protocolError(ErrMalformedMessage, "Expanded EAP missing Vendor-Id/Type")
	}
	vendor := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
	method := binary.BigEndian.Uint32(data[3:7])
	inner["Vendor-Id"], inner["Vendor-Type"] = vendor, method
	inner["Vendor Data"] = bytes.Clone(data[7:])
	if vendor != 0 || method != 3 {
		note("eap-method-unimplemented", false)
		return true, protocolError(ErrUnsupportedFeature, "Expanded EAP vendor %d method %d requires a separate profile", vendor, method)
	}
	inner["Type Name"] = "Expanded Nak"
	note("eap-nak-context-not-verified", false)
	if code != 2 {
		note("eap-nak-code", true)
	}
	list := data[7:]
	if len(list) == 0 {
		note("eap-expanded-nak-empty", true)
	}
	if len(list)%8 != 0 {
		note("eap-expanded-nak-tuple-length", true)
	}
	inner["Method Count"] = len(list) / 8
	if len(list)/8 > max {
		return true, protocolError(ErrResourceExceeded, "Expanded EAP NAK method count exceeds collection budget")
	}
	alternatives := make([]map[string]any, 0, len(list)/8)
	zero, other := false, false
	for at := 0; at+8 <= len(list); at += 8 {
		row := list[at : at+8]
		if row[0] != 254 {
			note("eap-expanded-nak-marker", true)
			continue
		}
		v := uint32(row[1])<<16 | uint32(row[2])<<8 | uint32(row[3])
		m := binary.BigEndian.Uint32(row[4:8])
		alternatives = append(alternatives, map[string]any{"Vendor-Id": v, "Vendor-Type": m})
		if v == 0 && m == 0 {
			zero = true
		} else {
			other = true
			if v == 0 && m < 4 {
				note("eap-nak-invalid-method", true)
			}
		}
	}
	inner["Desired Expanded Types"], inner["Method Count"], inner["No Alternative"] = alternatives, len(list)/8, zero && !other && len(list)%8 == 0 && len(alternatives) == len(list)/8
	if zero && other {
		note("eap-nak-zero-mixed", false)
	}
	return true, nil
}
