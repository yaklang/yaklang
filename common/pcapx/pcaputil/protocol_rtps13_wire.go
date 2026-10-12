package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// This explicit legacy wire observer follows the1.2/1.3 layout used by the
// fixed Wireshark4.2.5 dissector and received1.3 trace. It does not infer DDS
// types, participant identity, discovery success or reliable endpoint state.
func decodeRTPS13Wire(w []byte, budget ParserBudget) (map[string]any, error) {
	return decodeRTPS13Mode(w, budget, false)
}
func decodeRTPS13Mode(w []byte, budget ParserBudget, parameters bool) (map[string]any, error) {
	if len(w) < 16 || !bytes.Equal(w[:4], []byte("RTPS")) {
		return nil, rtpsMalformed("legacy RTPS header is truncated or invalid")
	}
	if w[4] != 1 || w[5] != 3 {
		return nil, rtpsUnsupported("legacy observer requires the selected1.3 layout")
	}
	if budget.MaxRecursionDepth < 4 {
		return nil, protocolError(ErrResourceExceeded, "legacy RTPS projection depth")
	}
	source, vendor, destination := hex.EncodeToString(w[8:16]), hex.EncodeToString(w[6:8]), "0000000000000000"
	out := map[string]any{"Protocol Version": "1.3", "Vendor ID": vendor, "Source GUID Prefix": source, "Observation": "unverified-legacy-wire-message"}
	if parameters {
		out["Observation"] = "unverified-legacy-parameter-message"
	}
	sourceVersion := "1.3"
	var messages []map[string]any
	var ts map[string]any
	remaining := budget.MaxCollectionElements
	ordinal := uint32(0)
	for at := 16; at < len(w); {
		if len(w)-at < 4 {
			return nil, rtpsMalformed("legacy submessage header is truncated")
		}
		if remaining <= 0 {
			return nil, protocolError(ErrResourceExceeded, "legacy submessage collection budget")
		}
		remaining--
		ordinal++
		kind, flags := w[at], w[at+1]
		order := rtpsOrder(flags)
		n := int(order.Uint16(w[at+2:]))
		at += 4
		// Legacy zero means zero body, unlike2.x's selected rest-of-message rule.
		if n > len(w)-at {
			return nil, rtpsMalformed("legacy submessage exceeds datagram")
		}
		b := w[at : at+n]
		at += n
		// The fixed legacy reader advances by the literal body length. Do not
		// import2.x global alignment rejection into this byte-observation profile.
		var f map[string]any
		var err error
		switch kind {
		case 1:
			continue
		case 9:
			if flags&2 != 0 {
				ts = nil
			} else {
				if len(b) < 8 {
					return nil, rtpsMalformed("legacy timestamp is truncated")
				}
				ts = map[string]any{"Seconds": int32(order.Uint32(b)), "Fraction": order.Uint32(b[4:])}
			}
			f = map[string]any{"Submessage": "INFO_TS", "Flags": flags, "Timestamp Invalidated": flags&2 != 0}
			if flags&2 != 0 && len(b) > 0 {
				f["Extension Bytes"] = bytes.Clone(b)
			} else if len(b) > 8 {
				f["Extension Bytes"] = bytes.Clone(b[8:])
			}
		case 12:
			if len(b) != 16 {
				return nil, rtpsMalformed("legacy INFO_SRC body must contain16 bytes")
			}
			if b[4] != 1 || b[5] > 3 {
				return nil, rtpsUnsupported("legacy logical source has an unselected layout")
			}
			source, vendor = hex.EncodeToString(b[8:]), hex.EncodeToString(b[6:8])
			sourceVersion = fmt.Sprintf("1.%d", b[5])
			ts = nil
			f = map[string]any{"Submessage": "INFO_SRC", "Flags": flags, "Source Header Bytes": bytes.Clone(b[:4]), "Reported Source Protocol Version": sourceVersion}
		case 14:
			if len(b) != 8 {
				return nil, rtpsMalformed("legacy INFO_DST body must contain8 bytes")
			}
			destination = hex.EncodeToString(b)
			f = map[string]any{"Submessage": "INFO_DST", "Flags": flags}
		case 2, 3:
			if sourceVersion != "1.3" {
				return nil, protocolError(ErrContextRequired, "legacy DATA logical source layout is not selected")
			}
			fixed := 20
			if kind == 3 {
				fixed = 16
			} else if flags&8 != 0 {
				fixed += 8
			}
			if len(b) < fixed {
				return nil, rtpsMalformed("legacy DATA fixed body is truncated")
			}
			f = map[string]any{"Flags": flags, "Reader ID": hex.EncodeToString(b[:4]), "Writer ID": hex.EncodeToString(b[4:8]), "Sender Role": "reported-writer"}
			pos := 8
			if kind == 2 {
				f["Submessage"], f["Data Present"], f["Alive Flag"], f["Unregister Flag"], f["Key Hash Prefix Present"] = "DATA", flags&2 != 0, flags&4 != 0, flags&32 != 0, flags&8 != 0
				if flags&8 != 0 {
					f["Key Hash Prefix"] = hex.EncodeToString(b[pos : pos+8])
					pos += 8
				}
				f["Key Hash Suffix"] = hex.EncodeToString(b[pos : pos+4])
				pos += 4
			} else {
				f["Submessage"] = "NOKEY_DATA"
			}
			rtps13SequenceWords(f, "Writer Sequence", b[pos:pos+8], order)
			pos += 8
			qmask := byte(16)
			if kind == 3 {
				qmask = 2
			}
			if flags&qmask != 0 {
				if budget.MaxRecursionDepth < 5 {
					return nil, protocolError(ErrResourceExceeded, "legacy inline parameter depth")
				}
				params, used, e := rtps13Parameters(b[pos:], order, &remaining)
				if e != nil {
					return nil, e
				}
				if parameters {
					for _, q := range params {
						typed, e := rtps13ParameterFields(q["ID"].(uint16), q["Value"].([]byte), order, budget, &remaining)
						if e != nil {
							return nil, e
						}
						if typed != nil {
							q["Parameter Fields"] = typed
						}
					}
				}
				f["Inline QoS"] = params
				pos += used
			}
			if kind == 3 {
				// The fixed legacy reference treats reserved bit2 differently on this
				// path. Preserve bytes and flags without inventing DataPresent semantics.
				f["Payload Bytes"] = bytes.Clone(b[pos:])
				f["Serialization"] = "opaque-legacy-nokey-data"
			} else if flags&2 != 0 {
				if len(b)-pos < 4 {
					return nil, rtpsMalformed("legacy serialized DATA is truncated")
				}
				f["Serialized Payload"] = bytes.Clone(b[pos:])
				f["Serialization"] = "opaque-legacy-data"
			} else if pos < len(b) {
				f["Extension Bytes"] = bytes.Clone(b[pos:])
			}
		case 6, 7, 8:
			if sourceVersion != "1.3" {
				return nil, protocolError(ErrContextRequired, "legacy control logical source layout is not selected")
			}
			f, err = rtps13ControlFields(kind, flags, b, order, &remaining)
			if err != nil {
				return nil, err
			}
		default:
			return nil, rtpsUnsupported("legacy submessage has no selected wire layout")
		}
		f["Submessage Index"] = ordinal
		f["Receiver Context"] = map[string]any{"Source GUID Prefix": source, "Protocol Version": sourceVersion, "Vendor ID": vendor, "Destination GUID Prefix": destination, "Destination Known": destination != "0000000000000000"}
		if kind == 2 || kind == 3 || kind == 6 || kind == 7 || kind == 8 {
			reader, writer := destination, source
			if kind == 6 {
				reader, writer = source, destination
			}
			f["Reported Reader GUID Prefix"], f["Reported Writer GUID Prefix"] = reader, writer
			f["Reader Prefix Known"], f["Writer Prefix Known"] = reader != "0000000000000000", writer != "0000000000000000"
		}
		if ts != nil {
			f["Timestamp"] = cloneSession(ts)
		}
		messages = append(messages, f)
	}
	if len(messages) == 0 {
		return nil, rtpsUnsupported("no selected legacy DATA or control observation")
	}
	out["Messages"] = messages
	return out, nil
}

// Fixed legacy reference advances by the declared length, without applying
// a2.x must-understand mask or requiring that length to be a multiple of4.
// Extended-ID encodings need their separately selected source/layout contract.
func rtps13Parameters(w []byte, o binary.ByteOrder, remaining *int) ([]map[string]any, int, error) {
	var out []map[string]any
	for at := 0; ; {
		if len(w)-at < 4 {
			return nil, 0, rtpsMalformed("legacy parameter list has no complete sentinel")
		}
		id, n := o.Uint16(w[at:]), int(o.Uint16(w[at+2:]))
		at += 4
		if id == 1 {
			return out, at, nil
		}
		if id&0x3f01 == 0x3f01 {
			return nil, 0, rtpsUnsupported("legacy extended parameter ID layout is not selected")
		}
		if *remaining <= 0 {
			return nil, 0, protocolError(ErrResourceExceeded, "legacy parameter collection budget")
		}
		*remaining--
		if n > len(w)-at {
			return nil, 0, rtpsMalformed("legacy parameter value exceeds bounded body")
		}
		out = append(out, map[string]any{"ID": id, "Value": bytes.Clone(w[at : at+n])})
		at += n
	}
}

// Literal legacy words are preserved even for zero/UNKNOWN encodings. The
// 2.x PIM positive-range/acknowledged-through interpretation is not borrowed.
func rtps13SequenceWords(f map[string]any, key string, b []byte, o binary.ByteOrder) {
	f[key+" High"], f[key+" Low"] = int32(o.Uint32(b)), o.Uint32(b[4:])
}
func rtps13ControlFields(kind, flags byte, b []byte, o binary.ByteOrder, remaining *int) (map[string]any, error) {
	minimum := 24
	if kind == 7 || kind == 8 {
		minimum = 28
	}
	if len(b) < minimum {
		return nil, rtpsMalformed("legacy control fixed fields are truncated")
	}
	f := map[string]any{"Flags": flags, "Reader ID": hex.EncodeToString(b[:4]), "Writer ID": hex.EncodeToString(b[4:8]), "Sender Role": "reported-writer"}
	at := 8
	if kind == 7 {
		f["Submessage"], f["Final"], f["Count"] = "HEARTBEAT", flags&2 != 0, int32(o.Uint32(b[24:]))
		rtps13SequenceWords(f, "First Sequence", b[8:16], o)
		rtps13SequenceWords(f, "Last Sequence", b[16:24], o)
		at = 28
	} else {
		if kind == 6 {
			f["Submessage"], f["Sender Role"], f["Final"] = "ACKNACK", "reported-reader", flags&2 != 0
		} else {
			f["Submessage"] = "GAP"
			rtps13SequenceWords(f, "Gap Start", b[at:at+8], o)
			at += 8
		}
		rtps13SequenceWords(f, "Bitmap Base", b[at:at+8], o)
		bits := o.Uint32(b[at+8:])
		at += 12
		// Bound the selected observer without labelling wider legacy encodings
		// invalid by importing a later protocol's PIM validity rule.
		if bits > 256 {
			return nil, protocolError(ErrResourceExceeded, "legacy bitmap exceeds selected256-bit observer capacity")
		}
		n := int((bits + 31) / 32)
		if n > (len(b)-at)/4 {
			return nil, rtpsMalformed("legacy bitmap words are truncated")
		}
		if n > *remaining {
			return nil, protocolError(ErrResourceExceeded, "legacy bitmap collection budget")
		}
		*remaining -= n
		words := make([]uint32, n)
		var offsets []uint32
		for i := range words {
			words[i] = o.Uint32(b[at+4*i:])
			for j := uint32(0); j < 32 && uint32(i)*32+j < bits; j++ {
				if words[i]&(1<<(31-j)) != 0 {
					if *remaining <= 0 {
						return nil, protocolError(ErrResourceExceeded, "legacy selected offset budget")
					}
					*remaining--
					offsets = append(offsets, uint32(i)*32+j)
				}
			}
		}
		at += 4 * n
		f["Num Bits"], f["Bitmap Words"], f["Selected Offsets"] = bits, words, offsets
		if kind == 6 {
			// The fixed legacy reader selects a Count only when exactly four
			// octets remain. Longer tails are not given a guessed counter.
			if len(b)-at < 4 {
				return nil, rtpsMalformed("selected legacy ACKNACK count is truncated")
			}
			if len(b)-at == 4 {
				f["Count"] = int32(o.Uint32(b[at:]))
				at += 4
			} else {
				f["Uninterpreted Counter Tail Bytes"] = bytes.Clone(b[at:])
				at = len(b)
			}
		}
	}
	if at < len(b) {
		f["Extension Bytes"] = bytes.Clone(b[at:])
	}
	return f, nil
}

func WithSessionRTPS13Wire(port uint16) ProtocolSessionOption {
	return withSessionRTPS13Profile(port, "rtps13-wire")
}
func withSessionRTPS13Profile(port uint16, profile string) ProtocolSessionOption {
	return func(s *captureSession) error {
		if port == 0 {
			return fmt.Errorf("legacy RTPS wire port must be nonzero")
		}
		old := s.f.a.datagramDecodeAs[port]
		if old != "" && old != profile {
			return fmt.Errorf("conflicting legacy RTPS profile")
		}
		s.f.a.datagramDecodeAs[port] = profile
		return nil
	}
}
func (s *captureSession) probeRTPS13Wire(data []byte) (ProbeResult, bool) {
	a := s.f.a
	p := a.datagramDecodeAs[s.f.ports[1]]
	if p == "" {
		p = a.datagramDecodeAs[s.f.ports[0]]
	}
	if p != "rtps13-wire" && p != "rtps13-parameters" {
		return ProbeResult{}, false
	}
	reject := func(err error) (ProbeResult, bool) {
		return ProbeResult{Verdict: ProbeReject, Protocol: "rtps", Reason: err.Error()}, true
	}
	if len(data) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) || 32768+512*int64(len(data)) > int64(a.config.MaxBufferedBytes)-a.buffered.Load() {
		return reject(protocolError(ErrResourceExceeded, "legacy wire byte/projection budget"))
	}
	if _, err := decodeRTPS13Mode(data, a.budget, p == "rtps13-parameters"); err != nil {
		return reject(err)
	}
	return probeAccept("rtps", p+"-native", 95), true
}
func (a *binParser) decodeRTPS13WireDatagram(e *ProtocolEvent, w []byte) bool {
	return a.decodeRTPS13Input(e, w, nil, false)
}
func (a *binParser) rtps13Selected(src, dst uint16) bool {
	p := a.datagramDecodeAs[dst]
	if p == "" {
		p = a.datagramDecodeAs[src]
	}
	return p == "rtps13-wire" || p == "rtps13-parameters"
}
func (a *binParser) decodeRTPS13SelectedInput(e *ProtocolEvent, w []byte, inputErr error, src, dst uint16) bool {
	p := a.datagramDecodeAs[dst]
	if p == "" {
		p = a.datagramDecodeAs[src]
	}
	return a.decodeRTPS13Input(e, w, inputErr, p == "rtps13-parameters")
}
func (a *binParser) decodeRTPS13Input(e *ProtocolEvent, w []byte, inputErr error, parameters bool) bool {
	e.Protocol, e.Profile, e.Admission, e.Completeness = "rtps", "rtps13-wire-native", "explicit-decode-as", "message"
	if parameters {
		e.Profile = "rtps13-parameters-native"
	}
	raw := w
	var fields map[string]any
	var err error
	transient := 32768 + 512*int64(len(w))
	if inputErr != nil {
		err = inputErr
		raw = nil
	} else if len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		err = protocolError(ErrResourceExceeded, "legacy wire byte budget")
		raw = nil
	} else if !a.reserveEvidence(transient) {
		err = protocolError(ErrResourceExceeded, "legacy wire projection budget")
		raw = nil
	} else {
		defer a.buffered.Add(-transient)
		fields, err = decodeRTPS13Mode(w, a.budget, parameters)
	}
	if err == nil {
		e.semanticFields = cloneSession(fields)
		e.semanticSession = cloneSession(fields)
		e.Session = cloneSession(fields)
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if raw == nil {
		a.messageBytes.Add(uint64(len(w)))
		if e.Status == "limited" {
			a.limited.Add(uint64(len(w)))
		}
	}
	return true
}
