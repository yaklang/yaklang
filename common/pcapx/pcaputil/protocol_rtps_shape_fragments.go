package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const rtpsShapeFragmentProfile = "rtps-shape-fragment-session"

// WithProtocolRTPSShapeFragments explicitly selects bounded UDP reconstruction
// for the same caller-associated final ShapeType as WithProtocolRTPSShapeWriter.
// Capture-lifetime retired identities are not reopened. This is observation,
// not an authenticated DDS reader or a reliable endpoint implementation.
func WithProtocolRTPSShapeFragments(port uint16, binding RTPSShapeWriter) CaptureOption {
	return func(c *CaptureConfig) error {
		if err := validateRTPSShapeWriter(binding); err != nil {
			return err
		}
		if err := WithProtocolDecodeAs("udp", port, "rtps-shape-fragments")(c); err != nil {
			return err
		}
		if c.rtpsShapeWriters == nil {
			c.rtpsShapeWriters = map[uint16]RTPSShapeWriter{}
		}
		if old, ok := c.rtpsShapeWriters[port]; ok && old != binding {
			return fmt.Errorf("conflicting RTPS ShapeType writer context")
		}
		c.rtpsShapeWriters[port] = binding
		return nil
	}
}

// WithSessionRTPSShapeFragments supplies an owned exact schema association.
// UDP transport and endpoint ports are separate explicit session options.
func WithSessionRTPSShapeFragments(port uint16, binding RTPSShapeWriter) ProtocolSessionOption {
	return func(s *captureSession) error {
		if err := validateRTPSShapeWriter(binding); err != nil {
			return err
		}
		if port == 0 {
			return fmt.Errorf("RTPS ShapeType fragment port must be nonzero")
		}
		if old := s.f.a.datagramDecodeAs[port]; old != "" && old != "rtps-shape-fragments" {
			return fmt.Errorf("conflicting RTPS fragment profile")
		}
		if s.f.a.rtpsShapeWriters == nil {
			s.f.a.rtpsShapeWriters = map[uint16]RTPSShapeWriter{}
		}
		if old, ok := s.f.a.rtpsShapeWriters[port]; ok && old != binding {
			return fmt.Errorf("conflicting RTPS ShapeType writer context")
		}
		s.f.a.datagramDecodeAs[port] = "rtps-shape-fragments"
		s.f.a.rtpsShapeWriters[port] = binding
		return nil
	}
}

func (a *binParser) rtpsShapeFragmentBinding(src, dst uint16) (RTPSShapeWriter, bool) {
	port := dst
	if a.datagramDecodeAs[port] == "" {
		port = src
	}
	b, ok := a.rtpsShapeWriters[port]
	return b, ok && a.datagramDecodeAs[port] == "rtps-shape-fragments"
}

// DATA_FRAG has K at bit2 and N at bit3, unlike DATA's D/K/N flags.
// The wire geometry is kept separately from the selected application schema.
func rtpsShapeFragmentWire(body []byte, flags byte, order binary.ByteOrder, remaining *int, depth int) (rtpsDataFragment, map[string]any, error) {
	var f rtpsDataFragment
	if flags&8 != 0 {
		return f, nil, rtpsUnsupported("nonstandard DATA_FRAG payload is not selected")
	}
	if len(body) < 32 {
		return f, nil, rtpsMalformed("DATA_FRAG fixed body is truncated")
	}
	seq, err := rtpsControlSequence(body, 12, order, false)
	if err != nil {
		return f, nil, err
	}
	f.reader, f.writer, f.sequence, f.keyOnly = hex.EncodeToString(body[4:8]), hex.EncodeToString(body[8:12]), seq, flags&4 != 0
	f.start, f.count, f.size, f.total = order.Uint32(body[20:]), uint32(order.Uint16(body[24:])), uint32(order.Uint16(body[26:])), order.Uint32(body[28:])
	if f.start == 0 || f.count == 0 || f.size == 0 || f.total == 0 || f.size > f.total {
		return f, nil, rtpsMalformed("DATA_FRAG has zero or oversized geometry")
	}
	n := (uint64(f.total) + uint64(f.size) - 1) / uint64(f.size)
	if uint64(f.start)+uint64(f.count)-1 > n {
		return f, nil, rtpsMalformed("DATA_FRAG range exceeds sample")
	}
	at := 4 + int(order.Uint16(body[2:]))
	if at < 32 || at > len(body) || at%4 != 0 {
		return f, nil, rtpsMalformed("DATA_FRAG inline QoS offset is invalid")
	}
	d := map[string]any{"Submessage": "DATA_FRAG", "Flags": flags, "Extra Flags": order.Uint16(body), "Octets To Inline QoS": order.Uint16(body[2:]), "Reader ID": f.reader, "Writer ID": f.writer, "Sender Role": "reported-writer", "Data Present": !f.keyOnly, "Key Present": f.keyOnly, "Fragment Starting Number": f.start, "Fragments In Submessage": uint16(f.count), "Fragment Size": uint16(f.size), "Sample Size": f.total}
	rtpsControlWords(d, "Writer Sequence", seq)
	if at > 32 {
		d["Fixed Extension Bytes"] = bytes.Clone(body[32:at])
	}
	if flags&2 != 0 {
		if depth < 5 {
			return f, nil, protocolError(ErrResourceExceeded, "DATA_FRAG QoS projection exceeds depth budget")
		}
		before := *remaining
		qos, used, err := rtpsParameters(body[at:], order, remaining)
		if err != nil {
			return f, nil, err
		}
		f.qosElements, f.qos, f.qosWire = before-*remaining, qos, body[at:at+used]
		d["Inline QoS"] = rtpsShapeQoS(qos)
		at += used
	}
	if uint64(f.count) > uint64(max(*remaining, 0)) {
		return f, nil, protocolError(ErrResourceExceeded, "DATA_FRAG range projection exceeds element budget")
	}
	*remaining -= int(f.count)
	offset := (uint64(f.start) - 1) * uint64(f.size)
	end := min(uint64(f.total), offset+uint64(f.count)*uint64(f.size))
	want := int(end - offset)
	if len(body)-at < want || len(body)-at > want+3 {
		return f, nil, rtpsMalformed("DATA_FRAG does not contain the declared complete fragment range")
	}
	f.payload = body[at : at+want]
	d["Fragment Bytes"] = bytes.Clone(f.payload)
	if at+want < len(body) {
		d["Fragment Padding"] = bytes.Clone(body[at+want:])
	}
	return f, d, nil
}

func rtpsShapeQoS(params []rtpsParameter) []map[string]any {
	var result []map[string]any
	for _, p := range params {
		result = append(result, map[string]any{"ID": p.id, "Value": bytes.Clone(p.value)})
	}
	return result
}

func rtpsShapeFragmentMatches(f rtpsDataFragment, b RTPSShapeWriter) bool {
	return f.context["Source GUID Prefix"] == hex.EncodeToString(b.SourceGUIDPrefix[:]) && f.writer == hex.EncodeToString(b.WriterID[:])
}

func decodeRTPSShapeFragments(w []byte, budget ParserBudget, binding RTPSShapeWriter) (map[string]any, []rtpsDataFragment, int, error) {
	remaining := budget.MaxCollectionElements
	var fragments []rtpsDataFragment
	fields, err := decodeRTPSMessageFragments(w, remaining, budget.MaxRecursionDepth, true, &remaining, &fragments)
	if err != nil {
		return nil, nil, remaining, err
	}
	for _, d := range fields["Messages"].([]map[string]any) {
		if d["Serialized Payload"] == nil {
			continue
		}
		ctx := d["Receiver Context"].(map[string]any)
		if ctx["Source GUID Prefix"] == hex.EncodeToString(binding.SourceGUIDPrefix[:]) && d["Writer ID"] == hex.EncodeToString(binding.WriterID[:]) && ctx["Protocol Version"] != "2.4" && ctx["Protocol Version"] != "2.5" {
			return nil, nil, remaining, protocolError(ErrContextRequired, "selected ShapeType fragment alignment needs wire2.4/2.5")
		}
	}
	selected := fragments[:0]
	for _, f := range fragments {
		if !rtpsShapeFragmentMatches(f, binding) {
			continue
		}
		if f.context["Protocol Version"] != "2.4" && f.context["Protocol Version"] != "2.5" {
			return nil, nil, remaining, protocolError(ErrContextRequired, "selected ShapeType fragment alignment needs wire2.4/2.5")
		}
		maximum := uint32(88)
		if f.keyOnly {
			maximum = 76
		}
		// Bound allocation before the first fragment/encapsulation is known.
		// A size outside this selected representation is a capacity refusal,
		// not a claim that every possible DDS encoding is malformed.
		if f.total > maximum {
			return nil, nil, remaining, protocolError(ErrResourceExceeded, "selected ShapeType sample exceeds its bounded CDR1 observer capacity")
		}
		selected = append(selected, f)
	}
	return fields, selected, remaining, nil
}

func rtpsShapeCompletion(sample *rtpsSample, entry map[string]any, remaining *int) (map[string]any, error) {
	app, err := decodeRTPSShapePayload(sample.data, sample.first.keyOnly, remaining)
	if err != nil {
		return nil, err
	}
	d := cloneSession(entry)
	d["Application Type"], d["Application Context"], d["Application Fields"] = "ShapeType", "caller-selected-unverified-writer", app
	d["Assembly State"], d["Reconstructed Payload"] = "complete", bytes.Clone(sample.data)
	if sample.first.timestamp != nil {
		d["Sample Timestamp"] = cloneSession(sample.first.timestamp)
	}
	if len(sample.first.qosWire) > 0 {
		d["Sample Inline QoS"] = rtpsShapeQoS(sample.first.qos)
	}
	return d, nil
}

func (s *captureSession) probeRTPSShapeFragments(data []byte) (ProbeResult, bool) {
	a := s.f.a
	p := a.datagramDecodeAs[s.f.ports[1]]
	if p == "" {
		p = a.datagramDecodeAs[s.f.ports[0]]
	}
	if p != "rtps-shape-fragments" {
		return ProbeResult{}, false
	}
	reject := func(err error) (ProbeResult, bool) {
		return ProbeResult{Verdict: ProbeReject, Protocol: "rtps", Reason: err.Error()}, true
	}
	b, ok := a.rtpsShapeFragmentBinding(s.f.ports[0], s.f.ports[1])
	if !ok {
		return reject(protocolError(ErrContextRequired, "ShapeType fragment writer context is missing"))
	}
	if len(data) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) || a.budget.MaxRecursionDepth < 5 {
		return reject(protocolError(ErrResourceExceeded, "ShapeType fragment byte/depth budget"))
	}
	// The selected observer charges this bounded transient projection window
	// before decoding. Probe must also reject it before copying untrusted QoS.
	transient := rtpsConversationBase + 16384 + 64*int64(len(data))
	if transient > int64(a.config.MaxBufferedBytes) {
		return reject(protocolError(ErrResourceExceeded, "ShapeType fragment transient projection budget"))
	}
	_, fragments, remaining, err := decodeRTPSShapeFragments(data, a.budget, b)
	if err != nil {
		return reject(err)
	}
	samples := map[string]*rtpsSample{}
	for _, f := range fragments {
		key := f.key()
		sample := samples[key]
		if sample == nil {
			sample, err = rtpsNewSample(f, min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes), a.budget.MaxCollectionElements)
			if err != nil {
				return reject(err)
			}
			samples[key] = sample
		}
		if _, err = sample.add(f); err != nil {
			return reject(err)
		}
		if sample.finished() {
			if _, err = decodeRTPSShapePayload(sample.data, sample.first.keyOnly, &remaining); err != nil {
				return reject(err)
			}
			sample.retire()
		}
	}
	return probeAccept("rtps", rtpsShapeFragmentProfile, 98), true
}

func (a *binParser) decodeRTPSShapeFragmentDatagram(e *ProtocolEvent, w []byte, inputErr error, src, dst uint16) bool {
	b, ok := a.rtpsShapeFragmentBinding(src, dst)
	if !ok {
		e.Protocol, e.Profile, e.Admission = "rtps", rtpsShapeFragmentProfile, "explicit-decode-as"
		err := inputErr
		if err == nil && len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
			err = protocolError(ErrResourceExceeded, "RTPS wire exceeds frame/message budget")
		}
		raw := w
		if err != nil {
			raw = nil
		} else {
			err = protocolError(ErrContextRequired, "ShapeType fragment writer context is missing")
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
	return a.decodeRTPSFragmentDatagramMode(e, w, inputErr, &b)
}

func (a *binParser) decodeRTPSSelectedFragmentDatagram(e *ProtocolEvent, w []byte, inputErr error, src, dst uint16) bool {
	p := a.datagramDecodeAs[dst]
	if p == "" {
		p = a.datagramDecodeAs[src]
	}
	if p == "rtps-shape-fragments" {
		return a.decodeRTPSShapeFragmentDatagram(e, w, inputErr, src, dst)
	}
	return a.decodeRTPSFragmentDatagram(e, w, inputErr)
}
