package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
)

const dcpServicesProfile = "profinet-dcp-get-set-hello-observed-properties"

// Selected wire observations follow p-net public/v1.0.2 (459e043) and
// Wireshark 4.2.5 (4aa814ac). They neither execute configuration nor prove
// operational acceptance. Identify retains its separate discovery profile.
func dcpServicesError(kind ProtocolErrorKind, why string) error {
	return protocolError(kind, "DCP Get/Set/Hello: %s", why)
}

func dcpControlProperty(sub byte, body []byte, request bool) (map[string]any, error) {
	out := map[string]any{"option": byte(5), "suboption": sub, "length": len(body), "body_hex": hex.EncodeToString(body), "block_info": nil}
	n := -1
	if !request && sub == 4 {
		n = 3
	}
	if request && (sub == 1 || sub == 2 || sub == 5 || sub == 6) {
		n = 2
	}
	if request && sub == 3 {
		n = 4
	}
	if n < 0 {
		return nil, dcpServicesError(ErrUnsupportedFeature, "control direction/suboption outside selected layout")
	}
	if len(body) < n {
		return nil, dcpServicesError(ErrMalformedMessage, "control fields incomplete")
	}
	if len(body) > n {
		return nil, dcpServicesError(ErrUnsupportedFeature, "unverified additional control bytes")
	}
	if !request {
		names := []string{"no-error-indicated", "option-not-supported", "suboption-not-supported-or-no-dataset", "suboption-not-set", "resource-error", "set-not-possible-locally", "set-not-possible-in-operation"}
		label := "uninterpreted-reserved-error"
		if int(body[2]) < len(names) {
			label = names[body[2]]
		}
		out["kind"], out["target_option"], out["target_suboption"], out["block_error"], out["block_error_name"] = "ControlResponse", body[0], body[1], body[2], label
		out["operation_claim"] = "observed block status; no deviceoperation proof"
	} else {
		out["kind"] = map[byte]string{1: "StartTransaction", 2: "EndTransaction", 3: "Signal", 5: "FactoryReset", 6: "ResetToFactory"}[sub]
		out["block_qualifier"] = binary.BigEndian.Uint16(body)
		if sub == 3 {
			out["signal_value"] = binary.BigEndian.Uint16(body[2:])
		}
		out["operation_claim"] = "observed control request; no deviceoperation proof"
	}
	out["handling"] = "decoded selected control"
	return out, nil
}

func decodeDCPServices(w []byte, limit int) (map[string]any, error) {
	if len(w) < 12 {
		return nil, dcpServicesError(ErrMalformedMessage, "FrameID and header incomplete")
	}
	service, typ := w[2], w[3]
	if !(service == 3 || service == 4 || service == 6) || typ > 1 || service == 6 && typ != 0 {
		return nil, dcpServicesError(ErrUnsupportedFeature, "selected Get/Set request/successresponse and Hello request only")
	}
	fid := binary.BigEndian.Uint16(w)
	want := uint16(0xfefd)
	if service == 6 {
		want = 0xfefc
	}
	if fid != want {
		return nil, dcpServicesError(ErrMalformedMessage, "FrameID/service disagreement")
	}
	end := 12 + int(binary.BigEndian.Uint16(w[10:]))
	if end > len(w) {
		return nil, dcpServicesError(ErrMalformedMessage, "DCPDataLength exceeds frame")
	}
	if end == 12 {
		return nil, dcpServicesError(ErrUnsupportedFeature, "empty service data outside selected observed layout")
	}
	if limit < 18 {
		return nil, dcpServicesError(ErrResourceExceeded, "field map exceeds collection limit")
	}
	name := map[byte]string{3: "Get", 4: "Set", 6: "Hello"}[service]
	kind := name + "Request"
	var claim any
	if typ == 1 {
		kind = name + "Response"
		claim = "observed successresponse header; inspect each block status; no deviceoperation proof"
	}
	out := map[string]any{"frame_id": fid, "service_id": int(service), "service_type": typ, "kind": kind, "transaction_id": binary.BigEndian.Uint32(w[4:]), "response_delay_factor": nil, "reserved_raw": binary.BigEndian.Uint16(w[8:]), "data_length": end - 12, "declared_message_hex": hex.EncodeToString(w[:end]), "link_trailer_hex": hex.EncodeToString(w[end:]), "response_claim": claim, "association_semantics": "observed service only; XID alone is not authenticated transaction evidence"}
	if service == 3 && typ == 0 {
		if (end-12)%2 != 0 {
			return nil, dcpServicesError(ErrMalformedMessage, "Get selector pair incomplete")
		}
		if (end-12)/2 > limit {
			return nil, dcpServicesError(ErrResourceExceeded, "Get selector collection limit")
		}
		selectors := []map[string]any{}
		for at := 12; at < end; at += 2 {
			selectors = append(selectors, map[string]any{"option": w[at], "suboption": w[at+1], "wire_offset": at, "wire_length": 2, "selector_wire_hex": hex.EncodeToString(w[at : at+2])})
		}
		out["selectors"] = selectors
		return out, nil
	}
	blocks := []map[string]any{}
	used := 0
	for at := 12; at < end; {
		if end-at < 4 {
			return nil, dcpServicesError(ErrMalformedMessage, "block header exceeds DCPDataLength")
		}
		start := at
		opt, sub, n := w[at], w[at+1], int(binary.BigEndian.Uint16(w[at+2:]))
		at += 4
		if n+(n&1) > end-at {
			return nil, dcpServicesError(ErrMalformedMessage, "value or odd padding crosses DCPDataLength")
		}
		// Charge both outer blocks and nested DeviceOptions pairs before allocation.
		charge := 1
		if opt == 2 && sub == 5 && n >= 2 {
			charge += (n - 2) / 2
		}
		if charge > limit-used {
			return nil, dcpServicesError(ErrResourceExceeded, "aggregate block/property collection limit")
		}
		used += charge
		v := w[at : at+n]
		at += n
		var pad any
		if n&1 != 0 {
			pad = w[at]
			at++
		}
		var b map[string]any
		var err error
		if opt == 5 {
			if service == 6 {
				return nil, dcpServicesError(ErrUnsupportedFeature, "Hello control layout unverified")
			}
			b, err = dcpControlProperty(sub, v, service == 4 && typ == 0)
		} else if service == 4 && typ == 1 {
			return nil, dcpServicesError(ErrUnsupportedFeature, "Set response requires selected Control/Response status blocks")
		} else if service == 4 && typ == 0 {
			if n < 2 {
				return nil, dcpServicesError(ErrMalformedMessage, "Set property lacks BlockQualifier")
			}
			b, err = dcpProperty(opt, sub, v[2:], false, limit)
			if err == nil {
				b["length"], b["body_hex"], b["block_qualifier"] = n, hex.EncodeToString(v), binary.BigEndian.Uint16(v)
				b["operation_claim"] = "observed property request; no deviceoperation proof"
			}
		} else {
			b, err = dcpProperty(opt, sub, v, true, limit)
		}
		if err != nil {
			return nil, err
		}
		b["padding_raw"], b["wire_offset"], b["wire_length"], b["block_wire_hex"] = pad, start, at-start, hex.EncodeToString(w[start:at])
		blocks = append(blocks, b)
	}
	out["blocks"] = blocks
	return out, nil
}
