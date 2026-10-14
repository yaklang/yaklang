package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
)

const someIPSDProfile = "someip-v1-sd-wire-observation"

// A finite wire observer, not an ECU service-discovery/reboot/subscription engine.
type someIPSDView struct {
	packet        *someIPPacket
	entriesEnd    int
	optionCount   int
	optionOffsets [64]int
	elements      int
}

func inspectSomeIPSD(w []byte, b ParserBudget) (*someIPSDView, error) {
	p, err := inspectSomeIPTP(w, b)
	if err != nil {
		return nil, err
	}
	bad := func(why string) (*someIPSDView, error) { return nil, protocolError(ErrMalformedMessage, why) }
	if b.MaxRecursionDepth < 5 {
		return nil, protocolError(ErrResourceExceeded, "SD field nesting budget")
	}
	if p.message != 0xffff8100 || p.request>>16 != 0 || p.request&65535 == 0 || p.kind != 2 || p.code != 0 {
		return bad("SD fixed header/client/nonzero session/notification/return code")
	}
	if p.iface != 1 {
		return nil, protocolError(ErrUnsupportedVersion, "selected SD interface1 only")
	}
	if len(w) < 28 {
		return bad("short SD header/array lengths")
	}
	n := binary.BigEndian.Uint32(w[20:24])
	if n%16 != 0 || uint64(n) > uint64(len(w)-28) {
		return bad("SD entry array length")
	}
	count := int(n) / 16
	if count > min(64, b.MaxCollectionElements) {
		return nil, protocolError(ErrResourceExceeded, "SD entry count")
	}
	v := &someIPSDView{packet: p, entriesEnd: 24 + int(n), elements: count}
	opts := binary.BigEndian.Uint32(w[v.entriesEnd : v.entriesEnd+4])
	if uint64(opts) != uint64(len(w)-v.entriesEnd-4) {
		return bad("SD option array length/trailing bytes")
	}
	for at := v.entriesEnd + 4; at < len(w); {
		if len(w)-at < 4 {
			return bad("short SD option header")
		}
		size := int(binary.BigEndian.Uint16(w[at : at+2]))
		if size < 1 || size > len(w)-at-3 {
			return bad("SD option range")
		}
		if v.optionCount >= min(64, b.MaxCollectionElements) || v.elements >= b.MaxCollectionElements {
			return nil, protocolError(ErrResourceExceeded, "SD shared entry/option count")
		}
		v.optionOffsets[v.optionCount] = at
		v.optionCount++
		v.elements++
		switch w[at+2] {
		case 4, 0x14:
			if size != 9 {
				return bad("SD IPv4 option exact length")
			}
		case 6, 0x16:
			if size != 21 {
				return bad("SD IPv6 option exact length")
			}
		}
		at += size + 3
	}
	for at := 24; at < v.entriesEnd; at += 16 {
		e := w[at : at+16]
		switch e[0] {
		case 0, 1:
		case 6, 7:
			if binary.BigEndian.Uint16(e[4:6]) == 65535 || binary.BigEndian.Uint16(e[6:8]) == 65535 || e[8] == 255 || binary.BigEndian.Uint16(e[14:16]) == 65535 {
				return bad("SD eventgroup wildcard is invalid")
			}
		default:
			return nil, protocolError(ErrUnsupportedFeature, "unselected SD entry type")
		}
		for run := 0; run < 2; run++ {
			index := int(e[1+run])
			size := int(e[3] >> 4)
			if run == 1 {
				size = int(e[3] & 15)
			}
			// PRS00834: a zero-length run ignores even a nonzero index.
			if size == 0 {
				continue
			}
			if index+size > v.optionCount {
				return bad("SD referenced option range")
			}
			if size > b.MaxCollectionElements-v.elements {
				return nil, protocolError(ErrResourceExceeded, "SD shared option-reference count")
			}
			v.elements += size
			for i := index; i < index+size; i++ {
				off := v.optionOffsets[i]
				kind := w[off+2]
				switch kind {
				case 4, 6, 0x14, 0x16:
					end := off + 3 + int(binary.BigEndian.Uint16(w[off:off+2]))
					proto := w[end-3]
					if proto != 6 && proto != 17 {
						return nil, protocolError(ErrUnsupportedFeature, "SD selected endpoint transport TCP/UDP only")
					}
					ip := net.IP(w[off+4 : end-4])
					if (kind == 0x14 || kind == 0x16) != ip.IsMulticast() {
						return bad("SD referenced endpoint unicast/multicast class")
					}
				default:
					if w[off+3]&0x80 == 0 {
						return nil, protocolError(ErrUnsupportedFeature, "unselected referenced nondiscardable SD option")
					}
				}
			}
		}
	}
	return v, nil
}
func someIPSDFields(v *someIPSDView, w []byte) map[string]any {
	f := someIPFields(v.packet, w)
	entries := make([]any, 0, (v.entriesEnd-24)/16)
	options := make([]any, 0, v.optionCount)
	for i := 0; i < v.optionCount; i++ {
		at := v.optionOffsets[i]
		n := int(binary.BigEndian.Uint16(w[at : at+2]))
		o := w[at : at+n+3]
		kind := o[2]
		known := kind == 4 || kind == 6 || kind == 0x14 || kind == 0x16
		m := map[string]any{"index": i, "length": uint16(n), "type": kind, "flag_octet": o[3], "discardable": o[3]&0x80 != 0, "reserved_bits": o[3] & 0x7f, "raw_hex": hex.EncodeToString(o), "selected_endpoint": known, "endpoint_available_verified": false}
		if known {
			m["address_hex"] = hex.EncodeToString(o[4 : len(o)-4])
			m["address"] = net.IP(o[4 : len(o)-4]).String()
			m["reserved_octet"] = o[len(o)-4]
			m["l4_protocol"] = o[len(o)-3]
			m["port"] = binary.BigEndian.Uint16(o[len(o)-2:])
			m["multicast_option"] = kind == 0x14 || kind == 0x16
		} else {
			m["opaque_value_hex"] = hex.EncodeToString(o[4:])
			m["option_meaning_verified"] = false
		}
		options = append(options, m)
	}
	for at := 24; at < v.entriesEnd; at += 16 {
		e := w[at : at+16]
		ttl := uint32(e[9])<<16 | uint32(e[10])<<8 | uint32(e[11])
		name := "find_service"
		switch e[0] {
		case 1:
			name = "offer_service"
			if ttl == 0 {
				name = "stop_offer_service"
			}
		case 6:
			name = "subscribe_eventgroup"
			if ttl == 0 {
				name = "stop_subscribe_eventgroup"
			}
		case 7:
			name = "subscribe_eventgroup_ack"
			if ttl == 0 {
				name = "subscribe_eventgroup_nack"
			}
		}
		indices := make([]any, 0, int(e[3]>>4)+int(e[3]&15))
		for run := 0; run < 2; run++ {
			n := int(e[3] >> 4)
			if run == 1 {
				n = int(e[3] & 15)
			}
			for j := 0; j < n; j++ {
				indices = append(indices, int(e[1+run])+j)
			}
		}
		m := map[string]any{"type": e[0], "wire_action": name, "index_first": e[1], "index_second": e[2], "count_first": e[3] >> 4, "count_second": e[3] & 15, "option_indices": indices, "service_id": binary.BigEndian.Uint16(e[4:6]), "instance_id": binary.BigEndian.Uint16(e[6:8]), "major_version": e[8], "ttl": ttl, "raw_hex": hex.EncodeToString(e), "service_identity_verified": false, "receiver_action_verified": false}
		if e[0] < 2 {
			m["minor_version"] = binary.BigEndian.Uint32(e[12:16])
			m["ttl_ignored_by_find_receiver"] = e[0] == 0
		} else {
			m["reserved_bits"] = binary.BigEndian.Uint16(e[12:14]) >> 4
			m["counter"] = e[13] & 15
			m["eventgroup_id"] = binary.BigEndian.Uint16(e[14:16])
		}
		entries = append(entries, m)
	}
	f["sd"] = map[string]any{"flags": w[16], "reboot_flag": w[16]&128 != 0, "unicast_flag": w[16]&64 != 0, "undefined_flags": w[16] & 63, "undefined_flags_ignored": true, "reserved_hex": hex.EncodeToString(w[17:20]), "entries_length": binary.BigEndian.Uint32(w[20:24]), "options_length": binary.BigEndian.Uint32(w[v.entriesEnd : v.entriesEnd+4]), "entries": entries, "options": options, "receiver_policy_verified": false, "subscription_success_verified": false, "reboot_detection_verified": false}
	return f
}

// WithSessionSomeIPSD selects a finite SD UDP wire observer; carrier and ports
// remain caller facts. Observed notifications never establish receiver success.
func WithSessionSomeIPSD(port uint16) ProtocolSessionOption {
	return func(s *captureSession) error {
		if port == 0 {
			return fmt.Errorf("SD needs a nonzero port")
		}
		if s.f.a.datagramDecodeAs == nil {
			s.f.a.datagramDecodeAs = map[uint16]string{}
		}
		old := s.f.a.datagramDecodeAs[port]
		if old != "" && old != "someip-sd" {
			return fmt.Errorf("conflicting SD profile")
		}
		s.f.a.datagramDecodeAs[port] = "someip-sd"
		return nil
	}
}
func (a *binParser) decodeSomeIPSDDatagram(e *ProtocolEvent, w []byte, inputErr error) bool {
	e.Protocol, e.Profile, e.Admission, e.Completeness = "someip", someIPSDProfile, "explicit-decode-as", "message"
	v, err := inspectSomeIPSD(w, a.budget)
	if inputErr != nil {
		err = inputErr
	}
	if err == nil {
		f := &binFlow{a: a}
		cost := int64(32768 + 24*len(w) + 1024*v.elements)
		err = f.reserveSession(cost)
		if err == nil {
			fields := someIPSDFields(v, w)
			e.semanticFields = cloneSession(fields)
			e.semanticSession = cloneSession(fields)
			e.Session = cloneSession(fields)
			f.closeSession()
		}
	}
	raw := w
	if inputErr != nil || len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		raw = nil
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if raw == nil {
		a.messageBytes.Add(uint64(len(w)))
		if e.Status == "limited" {
			a.limited.Add(uint64(len(w)))
		}
	}
	if err != nil {
		e.Completeness = e.Status
	}
	return true
}

func (a *binParser) someIPSDSelected(src, dst uint16) bool {
	p := a.datagramDecodeAs[dst]
	if p == "" {
		p = a.datagramDecodeAs[src]
	}
	return p == "someip-sd"
}
func (s *captureSession) probeSomeIPSD(w []byte) (ProbeResult, bool) {
	if !s.f.a.someIPSDSelected(s.f.ports[0], s.f.ports[1]) {
		return ProbeResult{}, false
	}
	if _, err := inspectSomeIPSD(w, s.f.a.budget); err != nil {
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}, true
	}
	return probeAccept("someip", someIPSDProfile, 98), true
}
