package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
)

// DDSI-RTPS formal/2008-04-09, 8.3.7.2 and 9.4.5.5. Version 2.0 DATA
// has no extraFlags/octetsToInlineQos and always includes a keyHashSuffix.
// Its application serialization remains explicitly opaque; a key hint or
// status flag does not establish a participant, device or reliable session.
func decodeRTPS20Datagram(w []byte, remaining int) (map[string]any, error) {
	return decodeRTPS20DatagramBudget(w, remaining, DefaultParserBudget().MaxRecursionDepth)
}

func decodeRTPS20DatagramBudget(w []byte, remaining, depth int) (map[string]any, error) {
	return decodeRTPS20Mode(w, remaining, depth, false)
}
func decodeRTPS20Mode(w []byte, remaining, depth int, rtiFilter bool) (map[string]any, error) {
	if !rtpsDatagramHeader(w) || w[5] != 0 {
		return nil, rtpsUnsupported("legacy decoder requires version 2.0")
	}
	if depth < 3 {
		return nil, protocolError(ErrResourceExceeded, "RTPS legacy change projection exceeds depth budget")
	}
	out := map[string]any{
		"Protocol Version": "2.0", "Vendor ID": hex.EncodeToString(w[6:8]),
		"Source GUID Prefix": hex.EncodeToString(w[8:20]), "Observation": "unverified-data-object",
	}
	if rtiFilter {
		out["Observation"] = "unverified-rti-filter-parameter-message"
	}
	var changes []map[string]any
	var timestamp map[string]any
	source, destination := out["Source GUID Prefix"], "000000000000000000000000"
	sourceVendor := out["Vendor ID"]
	contextSeen := false
	for at := 20; at < len(w); {
		if len(w)-at < 4 {
			return nil, rtpsMalformed("legacy submessage header is truncated")
		}
		if remaining <= 0 {
			return nil, protocolError(ErrResourceExceeded, "RTPS legacy submessage limit exceeded")
		}
		remaining--
		kind, flags := w[at], w[at+1]
		order := rtpsOrder(flags)
		n := int(order.Uint16(w[at+2:]))
		at += 4
		if n == 0 && kind != 1 && kind != 9 {
			n = len(w) - at
		}
		if n > len(w)-at {
			return nil, rtpsMalformed("legacy submessage exceeds datagram boundary")
		}
		body := w[at : at+n]
		at += n
		if at < len(w) && at%4 != 0 {
			return nil, rtpsMalformed("legacy next submessage is unaligned")
		}
		switch kind {
		case 1: // PAD, including the zero-length form.
		case 9:
			if flags&2 != 0 {
				if len(body) != 0 {
					return nil, rtpsMalformed("legacy invalidated timestamp has a body")
				}
				timestamp = nil
			} else {
				if len(body) != 8 {
					return nil, rtpsMalformed("legacy timestamp length is not eight")
				}
				if depth < 4 {
					return nil, protocolError(ErrResourceExceeded, "RTPS legacy timestamp projection exceeds depth budget")
				}
				timestamp = map[string]any{"Seconds": int32(order.Uint32(body)), "Fraction": order.Uint32(body[4:])}
			}
		case 0x0c:
			if len(body) < 20 {
				return nil, rtpsMalformed("legacy INFO_SRC is truncated")
			}
			if body[4] != 2 || body[5] != 0 {
				return nil, rtpsUnsupported("legacy logical source requires version 2.0")
			}
			source, timestamp, contextSeen = hex.EncodeToString(body[8:20]), nil, true
			// Logical vendor information belongs to the relayed source, not the
			// outer header. Keep both projections independent.
			sourceVendor = hex.EncodeToString(body[6:8])
		case 0x0e:
			if len(body) < 12 {
				return nil, rtpsMalformed("legacy INFO_DST is truncated")
			}
			destination, contextSeen = hex.EncodeToString(body[:12]), true
		case 2, 3:
			if contextSeen && depth < 4 {
				return nil, protocolError(ErrResourceExceeded, "RTPS legacy receiver context exceeds depth budget")
			}
			change, err := rtps20DataMode(body, kind, flags, order, &remaining, depth, rtiFilter, sourceVendor.(string))
			if err != nil {
				return nil, err
			}
			if timestamp != nil {
				change["Timestamp"] = cloneSession(timestamp)
			}
			if contextSeen {
				change["Receiver Context"] = map[string]any{"Source GUID Prefix": source, "Protocol Version": "2.0", "Vendor ID": sourceVendor, "Destination GUID Prefix": destination, "Destination Known": destination != "000000000000000000000000"}
			}
			changes = append(changes, change)
		default:
			return nil, rtpsUnsupported("legacy submessage is outside DATA/NOKEY_DATA profile")
		}
	}
	if len(changes) == 0 {
		return nil, rtpsUnsupported("no legacy DATA observed")
	}
	out["Changes"] = changes
	return out, nil
}

func rtps20Data(body []byte, kind, flags byte, order binary.ByteOrder, remaining *int, depth int) (map[string]any, error) {
	return rtps20DataMode(body, kind, flags, order, remaining, depth, false, "")
}
func rtps20DataMode(body []byte, kind, flags byte, order binary.ByteOrder, remaining *int, depth int, rtiFilter bool, vendor string) (map[string]any, error) {
	if len(body) < 16 {
		return nil, rtpsMalformed("legacy DATA fixed body is truncated")
	}
	hi, lo := int32(order.Uint32(body[8:])), order.Uint32(body[12:])
	if hi < 0 || hi == 0 && lo == 0 {
		return nil, rtpsMalformed("legacy writer sequence is not positive")
	}
	d := map[string]any{
		"Submessage": "DATA", "Flags": flags, "Reader ID": hex.EncodeToString(body[:4]),
		"Writer ID": hex.EncodeToString(body[4:8]), "Writer Sequence High": hi, "Writer Sequence Low": lo,
	}
	at := 16
	if kind == 2 {
		need := 4 // Suffix is mandatory even when the prefix is absent.
		if flags&8 != 0 {
			need += 12
		}
		if flags&16 != 0 {
			need += 4
		}
		if need > len(body)-at {
			return nil, rtpsMalformed("legacy key/status fields are truncated")
		}
		d["Key Hash Prefix Known"] = flags&8 != 0
		if flags&8 != 0 {
			d["Key Hash Prefix"] = hex.EncodeToString(body[at : at+12])
			at += 12
		}
		d["Key Hash Suffix"] = hex.EncodeToString(body[at : at+4])
		at += 4
		var status uint32
		if flags&16 != 0 {
			status = order.Uint32(body[at:])
			at += 4
		}
		d["Status Info Present"], d["Status Info"] = flags&16 != 0, status
		d["Disposed"], d["Unregistered"] = status&1 != 0, status&2 != 0
	} else {
		d["Submessage"] = "NOKEY_DATA"
	}
	if flags&2 != 0 {
		params, used, err := rtps20Parameters(body[at:], order, remaining, depth, true)
		if err != nil {
			return nil, err
		}
		if rtiFilter {
			for _, p := range params {
				if p["ID"].(uint16) != 0x8006 {
					continue
				}
				if vendor != "0101" {
					return nil, protocolError(ErrContextRequired, "RTI filter typing requires selected reported logical vendor0101")
				}
				f, e := rtps20RTIFilterFields(p["Value"].([]byte), order, remaining, depth)
				if e != nil {
					return nil, e
				}
				p["Parameter Fields"] = f
			}
		}
		d["Inline QoS"] = params
		at += used
	}
	if flags&4 != 0 {
		// Interpretation depends on the application's type/schema. Preserve the
		// exact complete bytes without pretending to decode arbitrary CDR.
		if len(body)-at < 4 {
			return nil, rtpsMalformed("legacy serialized-data encapsulation is truncated")
		}
		representation := binary.BigEndian.Uint16(body[at:])
		if representation > 3 {
			return nil, rtpsUnsupported("legacy serialized-data representation is outside CDR/PL_CDR")
		}
		if representation >= 2 {
			payloadOrder := rtpsOrder(byte(representation & 1))
			_, used, err := rtps20Parameters(body[at+4:], payloadOrder, remaining, depth, false)
			if err != nil {
				return nil, err
			}
			if len(body)-at-4-used > 3 {
				return nil, rtpsMalformed("unexpected bytes follow legacy serialized parameter list")
			}
		}
		d["Representation ID"] = representation
		d["Representation Options"] = binary.BigEndian.Uint16(body[at+2:])
		d["Serialization"] = "opaque-application-data"
		d["Serialized Payload"] = bytes.Clone(body[at:])
	} else if at != len(body) {
		return nil, rtpsMalformed("bytes follow legacy DATA without serialized-data flag")
	}
	return d, nil
}

func rtps20Parameters(w []byte, order binary.ByteOrder, remaining *int, depth int, project bool) ([]map[string]any, int, error) {
	var out []map[string]any
	for at := 0; ; {
		if len(w)-at < 4 {
			return nil, 0, rtpsMalformed("legacy QoS has no complete sentinel")
		}
		id, n := order.Uint16(w[at:]), int(int16(order.Uint16(w[at+2:])))
		at += 4
		if id == 1 {
			return out, at, nil
		}
		if n < 0 || n%4 != 0 || n > len(w)-at {
			return nil, 0, rtpsMalformed("legacy parameter length is negative, unaligned or truncated")
		}
		if *remaining <= 0 {
			return nil, 0, protocolError(ErrResourceExceeded, "RTPS legacy QoS parameter limit exceeded")
		}
		*remaining -= 1
		if id != 0 && project {
			if depth < 5 {
				return nil, 0, protocolError(ErrResourceExceeded, "RTPS legacy QoS projection exceeds depth budget")
			}
			out = append(out, map[string]any{"ID": id, "Length": n, "Value": bytes.Clone(w[at : at+n])})
		}
		at += n
	}
}

func (s *captureSession) probeRTPS20(data []byte) (ProbeResult, bool) {
	if !rtpsDatagramHeader(data) || data[5] != 0 {
		return ProbeResult{}, false
	}
	a := s.f.a
	selection := a.datagramDecodeAs[s.f.ports[1]]
	if selection == "" {
		selection = a.datagramDecodeAs[s.f.ports[0]]
	}
	if selection != "" && selection != "rtps" {
		return ProbeResult{}, false
	}
	if len(data) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		return ProbeResult{Verdict: ProbeReject, Protocol: "rtps", Reason: "RTPS legacy byte budget"}, true
	}
	if _, err := decodeRTPS20DatagramBudget(data, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth); err != nil {
		return ProbeResult{Verdict: ProbeReject, Protocol: "rtps", Reason: err.Error()}, true
	}
	return probeAccept("rtps", "", 90), true
}
