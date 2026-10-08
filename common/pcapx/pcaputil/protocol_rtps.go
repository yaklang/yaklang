package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
)

// UDP RTPS 2.x SPDP discovery and inline lifecycle observations (OMG DDSI-RTPS 2.5, 9.4/9.6/10.3).
// Participant names, GUIDs and locators are observed, never authenticated. User
// topic CDR, DATA_FRAG, DDS Security and reliable-writer state are separate profiles.
func rtpsDatagramHeader(w []byte) bool {
	return len(w) >= 20 && bytes.Equal(w[:4], []byte("RTPS")) && w[4] == 2
}

func rtpsMalformed(s string) error   { return protocolError(ErrMalformedMessage, "RTPS %s", s) }
func rtpsUnsupported(s string) error { return protocolError(ErrUnsupportedFeature, "RTPS %s", s) }
func rtpsOrder(flags byte) binary.ByteOrder {
	if flags&1 != 0 {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

type rtpsParameter struct {
	id    uint16
	value []byte
}

// Sentinel's length is ignored by the specification. Other lengths include
// padding and are multiples of four. Count PAD too, so zero-length entries
// cannot bypass the collection budget. Nothing returned aliases caller bytes.
func rtpsParameters(w []byte, order binary.ByteOrder, remaining *int) ([]rtpsParameter, int, error) {
	var out []rtpsParameter
	for at := 0; ; {
		if len(w)-at < 4 {
			return nil, 0, rtpsMalformed("parameter list has no complete sentinel")
		}
		id, n := order.Uint16(w[at:]), int(order.Uint16(w[at+2:]))
		at += 4
		if id == 1 {
			return out, at, nil
		}
		if *remaining <= 0 {
			return nil, 0, protocolError(ErrResourceExceeded, "RTPS parameter limit exceeded")
		}
		*remaining -= 1
		if n%4 != 0 || n > len(w)-at {
			return nil, 0, rtpsMalformed("parameter length is unaligned or truncated")
		}
		if id&0x4000 != 0 {
			return nil, 0, rtpsUnsupported("unrecognized must-understand parameter")
		}
		if id != 0 {
			out = append(out, rtpsParameter{id, bytes.Clone(w[at : at+n])})
		}
		at += n
	}
}

func decodeRTPSDatagram(w []byte, maxElements int) (map[string]any, error) {
	if !rtpsDatagramHeader(w) {
		return nil, rtpsMalformed("header is truncated or has wrong magic/version")
	}
	if w[5] < 1 || w[5] > 5 {
		return nil, rtpsUnsupported("minor version is outside the checked 2.1-2.5 profile")
	}
	remaining := maxElements
	out := map[string]any{"Protocol Version": fmt.Sprintf("%d.%d", w[4], w[5]), "Vendor ID": hex.EncodeToString(w[6:8]), "Source GUID Prefix": hex.EncodeToString(w[8:20]), "Observation": "unverified-participant"}
	var changes []map[string]any
	var timestamp map[string]any
	for at := 20; at < len(w); {
		if len(w)-at < 4 {
			return nil, rtpsMalformed("submessage header is truncated")
		}
		if remaining <= 0 {
			return nil, protocolError(ErrResourceExceeded, "RTPS submessage limit exceeded")
		}
		remaining--
		kind, flags := w[at], w[at+1]
		order := rtpsOrder(flags)
		n := int(order.Uint16(w[at+2 : at+4]))
		at += 4
		if n == 0 && kind != 1 && kind != 9 {
			n = len(w) - at
		}
		if n > len(w)-at {
			return nil, rtpsMalformed("submessage exceeds datagram boundary")
		}
		body := w[at : at+n]
		at += n
		if at < len(w) && at%4 != 0 {
			return nil, rtpsMalformed("next submessage is unaligned")
		}
		switch kind {
		case 1: // PAD is opaque, including the zero-length form.
		case 9:
			if flags&2 != 0 {
				if len(body) != 0 {
					return nil, rtpsMalformed("invalidated timestamp has a body")
				}
				timestamp = nil
			} else {
				if len(body) != 8 {
					return nil, rtpsMalformed("timestamp length is not eight")
				}
				timestamp = map[string]any{"Seconds": int32(order.Uint32(body)), "Fraction": order.Uint32(body[4:])}
			}
		case 0x15:
			if flags&0x0c == 0x0c {
				return nil, rtpsMalformed("DATA and KEY flags are both set")
			}
			if flags&0x18 != 0 {
				return nil, rtpsUnsupported("only standard SPDP data or inline lifecycle changes are implemented")
			}
			if len(body) < 20 {
				return nil, rtpsMalformed("DATA fixed header is truncated")
			}
			if !bytes.Equal(body[8:12], []byte{0, 1, 0, 0xc2}) {
				return nil, rtpsUnsupported("DATA writer is outside SPDP discovery")
			}
			inline := 4 + int(order.Uint16(body[2:4]))
			if inline < 20 || inline > len(body) || inline%4 != 0 {
				return nil, rtpsMalformed("DATA inline QoS offset is invalid")
			}
			hi, lo := int32(order.Uint32(body[12:16])), order.Uint32(body[16:20])
			if hi < 0 || hi == 0 && lo == 0 {
				return nil, rtpsMalformed("writer sequence is not positive")
			}
			data := map[string]any{"Reader Entity ID": hex.EncodeToString(body[4:8]), "Writer Entity ID": hex.EncodeToString(body[8:12]), "Writer Sequence": uint64(uint32(hi))<<32 | uint64(lo)}
			payload := body[inline:]
			if flags&2 != 0 {
				params, used, err := rtpsParameters(payload, order, &remaining)
				if err != nil {
					return nil, err
				}
				var qos []map[string]any
				for _, p := range params {
					qos = append(qos, map[string]any{"ID": p.id, "Value": hex.EncodeToString(p.value)})
				}
				data["Inline QoS"] = qos
				inlineFields, err := rtpsInlineFields(params)
				if err != nil {
					return nil, err
				}
				for key, value := range inlineFields {
					data[key] = value
				}
				payload = payload[used:]
			}
			if flags&4 == 0 {
				if len(payload) != 0 {
					return nil, rtpsMalformed("bytes follow DATA without serialized payload flags")
				}
				if _, ok := data["Inline QoS"]; !ok {
					return nil, protocolError(ErrContextRequired, "RTPS lifecycle change needs observed key hash and status info")
				}
				inlinePresentKey, inlinePresentStatus := false, false
				for _, value := range data["Inline QoS"].([]map[string]any) {
					id := value["ID"].(uint16)
					inlinePresentKey = inlinePresentKey || id == 0x70
					inlinePresentStatus = inlinePresentStatus || id == 0x71
				}
				if !inlinePresentKey || !inlinePresentStatus {
					return nil, protocolError(ErrContextRequired, "RTPS lifecycle change needs observed key hash and status info")
				}
				data["Kind"] = "lifecycle"
				data["Timestamp"] = cloneSession(timestamp)
				changes = append(changes, data)
				continue
			}
			if len(payload) < 4 {
				return nil, rtpsMalformed("serialized payload header is truncated")
			}
			representation := binary.BigEndian.Uint16(payload[:2])
			var payloadOrder binary.ByteOrder
			switch representation {
			case 2:
				payloadOrder = binary.BigEndian
			case 3:
				payloadOrder = binary.LittleEndian
			default:
				return nil, rtpsUnsupported("SPDP requires PL_CDR_BE or PL_CDR_LE")
			}
			if payload[2] != 0 || payload[3] != 0 {
				return nil, rtpsUnsupported("serialized payload options require unsupported representation semantics")
			}
			params, used, err := rtpsParameters(payload[4:], payloadOrder, &remaining)
			if err != nil {
				return nil, err
			}
			if used != len(payload)-4 {
				return nil, rtpsMalformed("bytes follow SPDP parameter sentinel")
			}
			fields, err := rtpsDiscoveryFields(params, payloadOrder)
			if err != nil {
				return nil, err
			}
			for k, v := range fields {
				data[k] = v
			}
			data["Timestamp"] = cloneSession(timestamp)
			data["Kind"] = "discovery"
			changes = append(changes, data)
		default:
			return nil, rtpsUnsupported("submessage is outside the checked SPDP profile")
		}
	}
	if len(changes) == 0 {
		return nil, rtpsUnsupported("no SPDP discovery or lifecycle DATA observed")
	}
	out["Changes"] = changes
	return out, nil
}

func rtpsDiscoveryFields(params []rtpsParameter, order binary.ByteOrder) (map[string]any, error) {
	out := map[string]any{}
	var values []map[string]any
	var locators []map[string]any
	ambiguous := map[string]bool{}
	var ambiguousFields []string
	guidPresent := false
	scalar := func(key string, value any) {
		if ambiguous[key] {
			return
		}
		if prior, exists := out[key]; exists && !reflect.DeepEqual(prior, value) {
			delete(out, key)
			ambiguous[key] = true
			ambiguousFields = append(ambiguousFields, key)
			return
		}
		out[key] = value
	}
	for _, p := range params {
		// Repeated ParameterIds are permitted by RTPS; preserve their full order.
		// Expose a scalar only when its value is unambiguous.
		values = append(values, map[string]any{"ID": p.id, "Value": hex.EncodeToString(p.value)})
		switch p.id {
		case 0x50:
			if len(p.value) != 16 {
				return nil, rtpsMalformed("participant GUID length is not sixteen")
			}
			guidPresent = true
			scalar("Participant GUID", hex.EncodeToString(p.value))
		case 0x15, 0x16, 0x58, 0x0f:
			if len(p.value) != 4 {
				return nil, rtpsMalformed("fixed discovery parameter has wrong length")
			}
			key, value := "", any(nil)
			switch p.id {
			case 0x15:
				key, value = "Participant Protocol Version", fmt.Sprintf("%d.%d", p.value[0], p.value[1])
			case 0x16:
				key, value = "Participant Vendor ID", hex.EncodeToString(p.value[:2])
			case 0x58:
				key, value = "Builtin Endpoint Set", order.Uint32(p.value)
			case 0x0f:
				key, value = "Domain ID", order.Uint32(p.value)
			}
			scalar(key, value)
		case 0x2:
			if len(p.value) != 8 {
				return nil, rtpsMalformed("lease duration length is not eight")
			}
			scalar("Lease Duration", map[string]any{"Seconds": int32(order.Uint32(p.value)), "Fraction": order.Uint32(p.value[4:])})
		case 0x31, 0x32, 0x33, 0x48:
			if len(p.value) != 24 {
				return nil, rtpsMalformed("locator length is not twenty-four")
			}
			kind := int32(order.Uint32(p.value))
			port := order.Uint32(p.value[4:])
			address := ""
			switch kind {
			case 1:
				address = net.IP(p.value[20:24]).String()
			case 2:
				address = net.IP(p.value[8:24]).String()
			}
			locators = append(locators, map[string]any{"ID": p.id, "Kind": kind, "Port": port, "Address": address, "Raw Address": hex.EncodeToString(p.value[8:24])})
		}
	}
	if !guidPresent {
		return nil, rtpsMalformed("SPDP participant key is absent")
	}
	out["Parameters"] = values
	out["Locators"] = locators
	if len(ambiguousFields) > 0 {
		out["Ambiguous Fields"] = ambiguousFields
	}
	return out, nil
}

// KeyHash and StatusInfo are octet arrays, independent of submessage endian.
// Repeated claims are preserved; conflicting values cannot yield a chosen key
// or lifecycle state. This is observation, not an authenticated participant DB.
func rtpsInlineFields(params []rtpsParameter) (map[string]any, error) {
	out := map[string]any{}
	ambiguous := map[string]bool{}
	var keys []string
	for _, p := range params {
		key := ""
		var value any
		switch p.id {
		case 0x70:
			if len(p.value) != 16 {
				return nil, rtpsMalformed("inline key hash length is not sixteen")
			}
			key, value = "Key Hash", hex.EncodeToString(p.value)
		case 0x71:
			if len(p.value) != 4 {
				return nil, rtpsMalformed("inline status info length is not four")
			}
			key, value = "Status Info", binary.BigEndian.Uint32(p.value)
		default:
			continue
		}
		if ambiguous[key] {
			continue
		}
		if prior, exists := out[key]; exists && !reflect.DeepEqual(prior, value) {
			delete(out, key)
			ambiguous[key] = true
			keys = append(keys, key)
		} else {
			out[key] = value
		}
	}
	if status, ok := out["Status Info"].(uint32); ok {
		out["Disposed"] = status&1 != 0
		out["Unregistered"] = status&2 != 0
		out["Filtered"] = status&4 != 0
	}
	if len(keys) > 0 {
		out["Ambiguous Inline Fields"] = keys
	}
	return out, nil
}
