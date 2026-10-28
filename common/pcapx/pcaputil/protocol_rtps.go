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
// topic CDR, DDS Security and reliable-writer state remain separate. Complete
// DATA_FRAG samples in one datagram are decoded locally; cross-datagram assembly
// requires the explicit bounded SPDP fragment-session profile.
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
	if rtpsDatagramHeader(w) && w[5] == 0 {
		return decodeRTPS20Datagram(w, maxElements)
	}
	var fragments []rtpsDataFragment
	remaining := maxElements
	out, err := decodeRTPSSelectedDatagram(w, maxElements, &fragments, &remaining)
	hasFragments := false
	for _, f := range fragments {
		hasFragments = hasFragments || !f.fullData
	}
	if err != nil || !hasFragments {
		return out, err
	}
	return rtpsLocalFragments(out, fragments, maxElements, len(w), remaining)
}

func decodeRTPSSelectedDatagram(w []byte, maxElements int, fragments *[]rtpsDataFragment, budgetRemaining *int) (map[string]any, error) {
	if !rtpsDatagramHeader(w) {
		return nil, rtpsMalformed("header is truncated or has wrong magic/version")
	}
	if w[5] < 1 || w[5] > 5 {
		return nil, rtpsUnsupported("minor version is outside the checked 2.1-2.5 profile")
	}
	remaining := maxElements
	if budgetRemaining != nil {
		defer func() { *budgetRemaining = remaining }()
	}
	out := map[string]any{"Protocol Version": fmt.Sprintf("%d.%d", w[4], w[5]), "Vendor ID": hex.EncodeToString(w[6:8]), "Source GUID Prefix": hex.EncodeToString(w[8:20]), "Observation": "unverified-participant"}
	var changes []map[string]any
	var timestamp map[string]any
	// Receiver context is scoped to one RTPS message. The datagram header stays
	// the outer observation; explicit context changes are copied into each DATA.
	sourcePrefix, sourceVersion, sourceVendor := out["Source GUID Prefix"], out["Protocol Version"], out["Vendor ID"]
	destination := "000000000000000000000000"
	contextSeen := false
	contextFields := func(data map[string]any) {
		if contextSeen {
			data["Receiver Context"] = map[string]any{
				"Source GUID Prefix": sourcePrefix, "Protocol Version": sourceVersion,
				"Vendor ID": sourceVendor, "Destination GUID Prefix": destination,
				"Destination Known": destination != "000000000000000000000000",
			}
		}
	}
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
		case 0x0c: // INFO_SRC, DDSI-RTPS 2.5 8.3.8.10 / 9.4.5.11.
			if len(body) < 20 {
				return nil, rtpsMalformed("INFO_SRC fixed body is truncated")
			}
			if body[4] != 2 || body[5] < 1 || body[5] > 5 {
				return nil, rtpsUnsupported("INFO_SRC version is outside the checked 2.1-2.5 profile")
			}
			// The unused long and future trailing extension bytes are opaque.
			sourceVersion = fmt.Sprintf("%d.%d", body[4], body[5])
			sourceVendor = hex.EncodeToString(body[6:8])
			sourcePrefix = hex.EncodeToString(body[8:20])
			timestamp, contextSeen = nil, true
		case 0x0e: // INFO_DST, DDSI-RTPS 2.5 8.3.8.8 / 9.4.5.9.
			if len(body) < 12 {
				return nil, rtpsMalformed("INFO_DST GUID prefix is truncated")
			}
			destination, contextSeen = hex.EncodeToString(body[:12]), true
			// GUIDPREFIX_UNKNOWN selects the receiving participant. A passive
			// capture cannot infer its GUID from an IP address or port.
		case 0x16:
			if fragments == nil {
				return nil, rtpsUnsupported("DATA_FRAG requires fragment support")
			}
			fragment, err := rtpsParseFragment(body, flags, order, &remaining)
			if err != nil {
				return nil, err
			}
			fragment.context = map[string]any{"Source GUID Prefix": sourcePrefix, "Protocol Version": sourceVersion, "Vendor ID": sourceVendor, "Destination GUID Prefix": destination, "Destination Known": destination != "000000000000000000000000"}
			fragment.position = len(changes)
			fragment.hasContext = contextSeen
			fragment.timestamp = cloneSession(timestamp)
			*fragments = append(*fragments, fragment)
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
			var normalQoS []rtpsParameter
			var normalQoSWire []byte
			if flags&2 != 0 {
				params, used, err := rtpsParameters(payload, order, &remaining)
				if err != nil {
					return nil, err
				}
				normalQoS, normalQoSWire = params, payload[:used]
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
				contextFields(data)
				changes = append(changes, data)
				continue
			}
			fields, err := rtpsSerializedDiscovery(payload, &remaining)
			if err != nil {
				return nil, err
			}
			for k, v := range fields {
				data[k] = v
			}
			data["Timestamp"] = cloneSession(timestamp)
			data["Kind"] = "discovery"
			contextFields(data)
			changes = append(changes, data)
			if fragments != nil {
				*fragments = append(*fragments, rtpsDataFragment{fullData: true, position: len(changes) - 1, reader: hex.EncodeToString(body[4:8]), writer: hex.EncodeToString(body[8:12]), sequence: uint64(uint32(hi))<<32 | uint64(lo), start: 1, count: 1, size: uint32(len(payload)), total: uint32(len(payload)), payload: payload, qos: normalQoS, qosWire: normalQoSWire, context: map[string]any{"Source GUID Prefix": sourcePrefix, "Protocol Version": sourceVersion, "Vendor ID": sourceVendor, "Destination GUID Prefix": destination, "Destination Known": destination != "000000000000000000000000"}, timestamp: cloneSession(timestamp), hasContext: contextSeen})
			}
		default:
			return nil, rtpsUnsupported("submessage is outside the checked SPDP profile")
		}
	}
	if len(changes) == 0 && (fragments == nil || len(*fragments) == 0) {
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
