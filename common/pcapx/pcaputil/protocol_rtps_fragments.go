package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// DATA_FRAG geometry uses byte offsets, not UDP concatenation. Only SPDP
// serialized DATA is selected; keyed/nonstandard payloads remain unsupported.
type rtpsDataFragment struct {
	fullData                  bool
	qosElements               int
	position                  int
	reader, writer            string
	sequence                  uint64
	start, count, size, total uint32
	payload, qosWire          []byte
	qos                       []rtpsParameter
	context, timestamp        map[string]any
	hasContext                bool
}

func rtpsParseFragment(body []byte, flags byte, order binary.ByteOrder, remaining *int) (rtpsDataFragment, error) {
	var f rtpsDataFragment
	if flags&12 != 0 {
		return f, rtpsUnsupported("DATA_FRAG key or nonstandard payload is outside SPDP DATA")
	}
	if len(body) < 32 {
		return f, rtpsMalformed("DATA_FRAG fixed header is truncated")
	}
	if !bytes.Equal(body[8:12], []byte{0, 1, 0, 0xc2}) {
		return f, rtpsUnsupported("DATA_FRAG writer is outside SPDP discovery")
	}
	hi, lo := int32(order.Uint32(body[12:16])), order.Uint32(body[16:20])
	if hi < 0 || hi == 0 && lo == 0 {
		return f, rtpsMalformed("DATA_FRAG writer sequence is not positive")
	}
	f.reader, f.writer = hex.EncodeToString(body[4:8]), hex.EncodeToString(body[8:12])
	f.sequence = uint64(uint32(hi))<<32 | uint64(lo)
	f.start = order.Uint32(body[20:24])
	f.count = uint32(order.Uint16(body[24:26]))
	f.size = uint32(order.Uint16(body[26:28]))
	f.total = order.Uint32(body[28:32])
	if f.start == 0 || f.count == 0 || f.size == 0 || f.total == 0 || f.size > f.total {
		return f, rtpsMalformed("DATA_FRAG fragment geometry contains zero or oversized fields")
	}
	n := (uint64(f.total) + uint64(f.size) - 1) / uint64(f.size)
	if uint64(f.start)+uint64(f.count)-1 > n {
		return f, rtpsMalformed("DATA_FRAG fragment range exceeds sample")
	}
	at := 4 + int(order.Uint16(body[2:4]))
	if at < 32 || at > len(body) || at%4 != 0 {
		return f, rtpsMalformed("DATA_FRAG inline QoS offset is invalid")
	}
	if flags&2 != 0 {
		beforeQoS := *remaining
		params, used, err := rtpsParameters(body[at:], order, remaining)
		if err != nil {
			return f, err
		}
		if _, err = rtpsInlineFields(params); err != nil {
			return f, err
		}
		f.qosElements = beforeQoS - *remaining
		f.qos = params
		f.qosWire = body[at : at+used]
		at += used
	}
	if uint64(f.count) > uint64(max(*remaining, 0)) {
		return f, protocolError(ErrResourceExceeded, "RTPS fragment collection exceeds limit")
	}
	*remaining -= int(f.count)
	offset := (uint64(f.start) - 1) * uint64(f.size)
	end := min(uint64(f.total), offset+uint64(f.count)*uint64(f.size))
	want := int(end - offset)
	if len(body)-at < want || len(body)-at > want+3 {
		return f, rtpsMalformed("DATA_FRAG serialized bytes do not contain the declared complete fragments")
	}
	// CDR trailing alignment octets carry no sample bytes and need not be zero.
	f.payload = body[at : at+want]
	return f, nil
}
func rtpsSerializedDiscovery(payload []byte, remaining *int) (map[string]any, error) {
	if len(payload) < 4 {
		return nil, rtpsMalformed("serialized payload header is truncated")
	}
	var order binary.ByteOrder
	switch binary.BigEndian.Uint16(payload[:2]) {
	case 2:
		order = binary.BigEndian
	case 3:
		order = binary.LittleEndian
	default:
		return nil, rtpsUnsupported("SPDP requires PL_CDR_BE or PL_CDR_LE")
	}
	if payload[2] != 0 || payload[3] != 0 {
		return nil, rtpsUnsupported("serialized payload options require unsupported representation semantics")
	}
	params, used, err := rtpsParameters(payload[4:], order, remaining)
	if err != nil {
		return nil, err
	}
	if used != len(payload)-4 {
		return nil, rtpsMalformed("bytes follow SPDP parameter sentinel")
	}
	return rtpsDiscoveryFields(params, order)
}
func (f rtpsDataFragment) key() string {
	return fmt.Sprintf("%s/%s/%s/%s/%d", f.context["Source GUID Prefix"], f.context["Destination GUID Prefix"], f.reader, f.writer, f.sequence)
}

type rtpsSample struct {
	first    rtpsDataFragment
	data     []byte
	seen     []bool
	complete bool
}

func rtpsNewSample(f rtpsDataFragment, maxBytes, maxElements int) (*rtpsSample, error) {
	n := (uint64(f.total) + uint64(f.size) - 1) / uint64(f.size)
	if uint64(f.total) > uint64(max(maxBytes, 0)) || n > uint64(max(maxElements, 0)) {
		return nil, protocolError(ErrResourceExceeded, "RTPS sample byte or fragment count exceeds budget")
	}
	first := f
	first.payload = nil
	first.qosWire = bytes.Clone(f.qosWire)
	return &rtpsSample{first: first, data: make([]byte, int(f.total)), seen: make([]bool, int(n))}, nil
}
func (s *rtpsSample) add(f rtpsDataFragment) (bool, error) {
	if s.complete || s.data == nil {
		return false, protocolError(ErrContextRequired, "RTPS sample identity is retired")
	}
	if !f.fullData && s.first.size != f.size || s.first.total != f.total || s.first.context["Protocol Version"] != f.context["Protocol Version"] || s.first.context["Vendor ID"] != f.context["Vendor ID"] {
		s.retire()
		return false, protocolError(ErrContextRequired, "RTPS fragment metadata conflicts with observed sample")
	}
	if len(f.qosWire) > 0 {
		if len(s.first.qosWire) > 0 && !bytes.Equal(s.first.qosWire, f.qosWire) {
			s.retire()
			return false, protocolError(ErrContextRequired, "RTPS fragment inline QoS conflicts")
		}
		if len(s.first.qosWire) == 0 {
			s.first.qosElements = f.qosElements
			s.first.qos = f.qos
			s.first.qosWire = bytes.Clone(f.qosWire)
		}
	}
	if f.start == 1 && !s.seen[0] {
		s.first.timestamp = cloneSession(f.timestamp)
		s.first.hasContext = f.hasContext
	}
	if f.fullData {
		for i, seen := range s.seen {
			if seen {
				at := i * int(s.first.size)
				end := min(len(s.data), at+int(s.first.size))
				if !bytes.Equal(s.data[at:end], f.payload[at:end]) {
					s.retire()
					return false, protocolError(ErrContextRequired, "RTPS regular DATA conflicts with observed fragments")
				}
			}
		}
		duplicate := s.finished()
		copy(s.data, f.payload)
		for i := range s.seen {
			s.seen[i] = true
		}
		return duplicate, nil
	}
	duplicate := true
	for n := uint32(0); n < f.count; n++ {
		index := int(f.start - 1 + n)
		offset := index * int(f.size)
		length := min(int(f.size), len(s.data)-offset)
		part := f.payload[int(n*f.size) : int(n*f.size)+length]
		if s.seen[index] {
			if !bytes.Equal(s.data[offset:offset+length], part) {
				s.retire()
				return false, protocolError(ErrContextRequired, "RTPS overlapping fragment bytes conflict")
			}
			continue
		}
		duplicate = false
		copy(s.data[offset:offset+length], part)
		s.seen[index] = true
	}
	return duplicate, nil
}
func (s *rtpsSample) finished() bool {
	for _, seen := range s.seen {
		if !seen {
			return false
		}
	}
	return len(s.seen) > 0
}
func (s *rtpsSample) retire() {
	s.data = nil
	s.seen = nil
	s.first.qos = nil
	s.first.qosWire = nil
	s.complete = true
}
func (s *rtpsSample) change(remaining *int) (map[string]any, error) {
	fields, err := rtpsSerializedDiscovery(s.data, remaining)
	if err != nil {
		return nil, err
	}
	return s.decorate(fields)
}
func (s *rtpsSample) decorate(fields map[string]any) (map[string]any, error) {
	fields["Reader Entity ID"], fields["Writer Entity ID"], fields["Writer Sequence"] = s.first.reader, s.first.writer, s.first.sequence
	fields["Kind"], fields["Timestamp"] = "discovery", cloneSession(s.first.timestamp)
	if s.first.hasContext {
		fields["Receiver Context"] = cloneSession(s.first.context)
	}
	if len(s.first.qosWire) > 0 {
		var qos []map[string]any
		for _, p := range s.first.qos {
			qos = append(qos, map[string]any{"ID": p.id, "Value": hex.EncodeToString(p.value)})
		}
		fields["Inline QoS"] = qos
		q, err := rtpsInlineFields(s.first.qos)
		if err != nil {
			return nil, err
		}
		for k, v := range q {
			fields[k] = v
		}
	}
	return fields, nil
}

// A stateless decoder may assemble a complete sample carried by this one
// datagram. It never claims partial samples or carries bytes into a later call.
func rtpsLocalFragments(out map[string]any, fragments []rtpsDataFragment, maxElements, maxBytes, remaining int) (map[string]any, error) {
	samples := map[string]*rtpsSample{}
	total := 0
	completed := map[string]bool{}
	var completions []rtpsCompletedChange
	for _, f := range fragments {
		key := f.key()
		s := samples[key]
		if f.fullData && s == nil {
			continue
		}
		if s == nil {
			if uint64(f.total) > uint64(max(maxBytes-total, 0)) {
				return nil, protocolError(ErrContextRequired, "RTPS incomplete sample requires selected cross-datagram profile")
			}
			var err error
			s, err = rtpsNewSample(f, maxBytes-total, maxElements)
			if err != nil {
				return nil, err
			}
			total += int(f.total)
			samples[key] = s
		}
		if _, err := s.add(f); err != nil {
			return nil, err
		}
		if s.finished() && !completed[key] {
			completed[key] = true
			if !f.fullData {
				change, err := s.change(&remaining)
				if err != nil {
					return nil, err
				}
				completions = append(completions, rtpsCompletedChange{f.position, change})
			}
		}
	}
	for _, s := range samples {
		if !s.finished() {
			return nil, protocolError(ErrContextRequired, "RTPS sample has missing fragments")
		}
	}
	out["Changes"] = rtpsMergeChanges(out["Changes"].([]map[string]any), completions)
	return out, nil
}

type rtpsCompletedChange struct {
	position int
	fields   map[string]any
}

func rtpsMergeChanges(plain []map[string]any, complete []rtpsCompletedChange) []map[string]any {
	var result []map[string]any
	for i := 0; i <= len(plain); i++ {
		for _, c := range complete {
			if c.position == i {
				result = append(result, c.fields)
			}
		}
		if i < len(plain) {
			result = append(result, plain[i])
		}
	}
	return result
}
