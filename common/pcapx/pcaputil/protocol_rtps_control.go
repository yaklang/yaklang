package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// Control fields are reported wire claims, not evidence that a reader received
// a sample. Sequence words/offsets remain exact through JSON projections.
func decodeRTPSControl(w []byte, elements int) (map[string]any, error) {
	return decodeRTPSControlBudget(w, elements, DefaultParserBudget().MaxRecursionDepth)
}

func decodeRTPSControlBudget(w []byte, elements, depth int) (map[string]any, error) {
	if !rtpsDatagramHeader(w) {
		return nil, rtpsMalformed("control message header is truncated or invalid")
	}
	if w[5] > 5 {
		return nil, rtpsUnsupported("control version is beyond supported RTPS2.0-2.5 layouts")
	}
	if depth < 4 {
		return nil, protocolError(ErrResourceExceeded, "RTPS control projection exceeds depth budget")
	}
	version := fmt.Sprintf("2.%d", w[5])
	source, vendor, destination := hex.EncodeToString(w[8:20]), hex.EncodeToString(w[6:8]), "000000000000000000000000"
	out := map[string]any{"Protocol Version": version, "Vendor ID": vendor, "Source GUID Prefix": source, "Observation": "unverified-control-report"}
	var controls []map[string]any
	var timestamp map[string]any
	ordinal := uint32(0)
	for at := 20; at < len(w); {
		if len(w)-at < 4 {
			return nil, rtpsMalformed("control submessage header is truncated")
		}
		if elements <= 0 {
			return nil, protocolError(ErrResourceExceeded, "RTPS control submessage limit exceeded")
		}
		elements--
		ordinal++
		kind, flags := w[at], w[at+1]
		order := rtpsOrder(flags)
		n := int(order.Uint16(w[at+2:]))
		at += 4
		if n == 0 && kind != 1 && kind != 9 {
			n = len(w) - at
		}
		if n > len(w)-at {
			return nil, rtpsMalformed("control submessage exceeds datagram boundary")
		}
		body := w[at : at+n]
		at += n
		if at < len(w) && at%4 != 0 {
			return nil, rtpsMalformed("next control submessage is unaligned")
		}
		switch kind {
		case 1:
		case 9:
			if flags&2 != 0 {
				timestamp = nil
			} else {
				if len(body) < 8 {
					return nil, rtpsMalformed("control timestamp is truncated")
				}
				timestamp = map[string]any{"Seconds": int32(order.Uint32(body)), "Fraction": order.Uint32(body[4:])}
			}
		case 0x0c:
			if len(body) < 20 {
				return nil, rtpsMalformed("control INFO_SRC is truncated")
			}
			if body[4] != 2 || body[5] > 5 {
				return nil, rtpsUnsupported("control logical source version has no pinned layout")
			}
			source, vendor, version = hex.EncodeToString(body[8:20]), hex.EncodeToString(body[6:8]), fmt.Sprintf("2.%d", body[5])
			timestamp = nil
		case 0x0e:
			if len(body) < 12 {
				return nil, rtpsMalformed("control INFO_DST is truncated")
			}
			destination = hex.EncodeToString(body[:12])
		case 6, 7, 8, 0x12, 0x13:
			// OMG document2.3 specifies wire version2.4. Older minor versions
			// retain the common layout and treat group bits as unknown flags.
			d, err := rtpsControlFields(kind, flags, body, order, &elements, version == "2.4" || version == "2.5", version == "2.5")
			if err != nil {
				return nil, err
			}
			d["Submessage Index"] = ordinal
			d["Receiver Context"] = map[string]any{"Source GUID Prefix": source, "Protocol Version": version, "Vendor ID": vendor, "Destination GUID Prefix": destination, "Destination Known": destination != "000000000000000000000000"}
			reader, writer := destination, source
			if kind == 6 || kind == 0x12 {
				reader, writer = source, destination
			}
			d["Reported Reader GUID Prefix"], d["Reported Writer GUID Prefix"] = reader, writer
			d["Reader Prefix Known"], d["Writer Prefix Known"] = reader != "000000000000000000000000", writer != "000000000000000000000000"
			if timestamp != nil {
				d["Timestamp"] = cloneSession(timestamp)
			}
			controls = append(controls, d)
		default:
			// Unimplemented state-changing/security or DATA messages cannot be
			// silently skipped and relabeled a complete control observation.
			return nil, rtpsUnsupported("submessage needs DATA, security or other receiver-state support")
		}
	}
	if len(controls) == 0 {
		return nil, rtpsUnsupported("no reliability control observed")
	}
	out["Controls"] = controls
	return out, nil
}

func rtpsControlSequence(b []byte, at int, order binary.ByteOrder, zero bool) (uint64, error) {
	if at < 0 || len(b)-at < 8 {
		return 0, rtpsMalformed("control sequence words are truncated")
	}
	hi, lo := int32(order.Uint32(b[at:])), order.Uint32(b[at+4:])
	if hi < 0 || !zero && hi == 0 && lo == 0 {
		return 0, rtpsMalformed("control sequence is outside its positive range")
	}
	return uint64(uint32(hi))<<32 | uint64(lo), nil
}

func rtpsControlWords(d map[string]any, key string, value uint64) {
	d[key+" High"], d[key+" Low"] = int32(value>>32), uint32(value)
}

// Bitmap words are ordered by the submessage's E flag, but bit zero is the
// word's MSB. Unused bits in the last word are retained, not requested offsets.
func rtpsControlBitmap(b []byte, order binary.ByteOrder, fragment bool, remaining *int, d map[string]any) (int, uint64, error) {
	baseBytes := 8
	if fragment {
		baseBytes = 4
	}
	if len(b) < baseBytes+4 {
		return 0, 0, rtpsMalformed("control bitmap header is truncated")
	}
	var base uint64
	if fragment {
		base = uint64(order.Uint32(b))
		if base == 0 {
			return 0, 0, rtpsMalformed("fragment bitmap base is zero")
		}
		d["Fragment Base"] = uint32(base)
	} else {
		var err error
		base, err = rtpsControlSequence(b, 0, order, false)
		if err != nil {
			return 0, 0, err
		}
		rtpsControlWords(d, "Bitmap Base", base)
	}
	bits := order.Uint32(b[baseBytes:])
	if bits > 256 {
		return 0, 0, rtpsMalformed("control bitmap exceeds 256 bits")
	}
	words := int((bits + 31) / 32)
	used := baseBytes + 4 + words*4
	if used > len(b) {
		return 0, 0, rtpsMalformed("control bitmap words are truncated")
	}
	max := uint64(math.MaxInt64)
	if fragment {
		max = math.MaxUint32
	}
	selected := 0
	for i := uint32(0); i < bits; i++ {
		if order.Uint32(b[baseBytes+4+int(i/32)*4:])&(uint32(1)<<(31-i%32)) != 0 {
			if base > max-uint64(i) {
				return 0, 0, rtpsMalformed("control selected sequence overflows")
			}
			selected++
		}
	}
	if words+selected > *remaining {
		return 0, 0, protocolError(ErrResourceExceeded, "RTPS control bitmap projection exceeds element budget")
	}
	*remaining -= words + selected
	bitmap := make([]uint32, words)
	offsets := make([]uint32, 0, selected)
	for i := range bitmap {
		bitmap[i] = order.Uint32(b[baseBytes+4+i*4:])
	}
	for i := uint32(0); i < bits; i++ {
		if bitmap[i/32]&(uint32(1)<<(31-i%32)) != 0 {
			offsets = append(offsets, i)
		}
	}
	d["Num Bits"], d["Bitmap Words"], d["Selected Offsets"] = bits, bitmap, offsets
	return used, base, nil
}

func rtpsControlFields(kind, flags byte, b []byte, order binary.ByteOrder, remaining *int, groups, filtered bool) (map[string]any, error) {
	if len(b) < 8 {
		return nil, rtpsMalformed("control entity IDs are truncated")
	}
	d := map[string]any{"Flags": flags, "Reader ID": hex.EncodeToString(b[:4]), "Writer ID": hex.EncodeToString(b[4:8])}
	used := 0
	seq := func(at int, zero bool, key string) (uint64, error) {
		n, err := rtpsControlSequence(b, at, order, zero)
		if err == nil {
			rtpsControlWords(d, key, n)
		}
		return n, err
	}
	count := func(at int) error {
		if len(b)-at < 4 {
			return rtpsMalformed("control count is truncated")
		}
		d["Count"] = int32(order.Uint32(b[at:])) // Signed Count_t wraps; negative is valid.
		return nil
	}
	switch kind {
	case 6:
		d["Submessage"], d["Final"], d["Sender Role"] = "ACKNACK", flags&2 != 0, "reported-reader"
		n, base, err := rtpsControlBitmap(b[8:], order, false, remaining, d)
		if err != nil {
			return nil, err
		}
		rtpsControlWords(d, "Reported Acknowledged Through", base-1)
		used = 8 + n
		if err := count(used); err != nil {
			return nil, err
		}
		used += 4
	case 7:
		d["Submessage"], d["Final"], d["Liveliness"], d["Sender Role"] = "HEARTBEAT", flags&2 != 0, flags&4 != 0, "reported-writer"
		first, err := seq(8, false, "First Sequence")
		if err != nil {
			return nil, err
		}
		last, err := seq(16, true, "Last Sequence")
		if err != nil || last < first-1 {
			return nil, rtpsMalformed("heartbeat range is invalid")
		}
		d["Empty Range"] = last == first-1
		if err := count(24); err != nil {
			return nil, err
		}
		used = 28
		if groups && flags&8 != 0 {
			current, err := seq(28, false, "Current Group Sequence")
			if err != nil {
				return nil, err
			}
			first, err := seq(36, false, "First Group Sequence")
			if err != nil {
				return nil, err
			}
			last, err := seq(44, true, "Last Group Sequence")
			if err != nil || last < first-1 || current < first || current > last {
				return nil, rtpsMalformed("heartbeat group range is invalid")
			}
			// PSM9.4.5.7 maps only the three GSNs. The PIM's abstract
			// writer-set digests have no wire members here. Do not invent
			// their meanings from extension octets, even when eight exist.
			used = 52
		}
	case 8:
		d["Submessage"], d["Sender Role"] = "GAP", "reported-writer"
		start, err := seq(8, false, "Gap Start")
		if err != nil {
			return nil, err
		}
		n, base, err := rtpsControlBitmap(b[16:], order, false, remaining, d)
		if err != nil {
			return nil, err
		}
		d["Contiguous Gap Empty"] = base <= start
		used = 16 + n
		if groups && flags&2 != 0 {
			first, err := seq(used, false, "Gap Start Group Sequence")
			if err != nil {
				return nil, err
			}
			last, err := seq(used+8, false, "Gap End Group Sequence")
			if err != nil || last < first-1 {
				return nil, rtpsMalformed("gap group range is invalid")
			}
			used += 16
		}
		if filtered && flags&4 != 0 {
			if len(b)-used < 8 {
				return nil, rtpsMalformed("gap filtered count is truncated")
			}
			d["Filtered Count High"], d["Filtered Count Low"] = int32(order.Uint32(b[used:])), order.Uint32(b[used+4:])
			used += 8
		}
	case 0x12:
		d["Submessage"], d["Sender Role"] = "NACK_FRAG", "reported-reader"
		if _, err := seq(8, false, "Writer Sequence"); err != nil {
			return nil, err
		}
		n, _, err := rtpsControlBitmap(b[16:], order, true, remaining, d)
		if err != nil {
			return nil, err
		}
		used = 16 + n
		if err := count(used); err != nil {
			return nil, err
		}
		used += 4
	case 0x13:
		d["Submessage"], d["Sender Role"] = "HEARTBEAT_FRAG", "reported-writer"
		if _, err := seq(8, false, "Writer Sequence"); err != nil {
			return nil, err
		}
		if len(b) < 24 || order.Uint32(b[16:]) == 0 {
			return nil, rtpsMalformed("heartbeat fragment last number/count is invalid")
		}
		d["Last Fragment Number"] = order.Uint32(b[16:])
		if err := count(20); err != nil {
			return nil, err
		}
		used = 24
	default:
		return nil, rtpsUnsupported("unimplemented reliability control")
	}
	if used < len(b) {
		d["Extension Bytes"] = bytes.Clone(b[used:])
	}
	return d, nil
}

// WithSessionRTPSControl selects wire control observations at explicit UDP
// endpoint ports, without inferring reliable state or DATA application types.
func WithSessionRTPSControl(port uint16) ProtocolSessionOption {
	return func(s *captureSession) error {
		if port == 0 {
			return fmt.Errorf("RTPS control port must be nonzero")
		}
		old := s.f.a.datagramDecodeAs[port]
		if old != "" && old != "rtps-control" {
			return fmt.Errorf("conflicting RTPS control profile")
		}
		s.f.a.datagramDecodeAs[port] = "rtps-control"
		return nil
	}
}
func (s *captureSession) probeRTPSControl(data []byte) (ProbeResult, bool) {
	a := s.f.a
	p := a.datagramDecodeAs[s.f.ports[1]]
	if p == "" {
		p = a.datagramDecodeAs[s.f.ports[0]]
	}
	if p != "rtps-control" {
		return ProbeResult{}, false
	}
	reject := func(reason string) (ProbeResult, bool) {
		return ProbeResult{Verdict: ProbeReject, Protocol: "rtps", Reason: reason}, true
	}
	if len(data) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		return reject("RTPS control byte budget")
	}
	if _, err := decodeRTPSControlBudget(data, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth); err != nil {
		return reject(err.Error())
	}
	return probeAccept("rtps", "rtps-control-native", 95), true
}
