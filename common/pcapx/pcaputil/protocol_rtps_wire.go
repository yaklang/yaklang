package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// rtpsWireData keeps the concrete UDP DATA layout independent of application
// schema selection. Payload and key octets are owned observations; decoding a
// known type will require explicit context, never a guess from entity IDs.
func rtpsWireData(body []byte, flags byte, order binary.ByteOrder, remaining *int, depth int) (map[string]any, error) {
	return rtpsWireDataDetails(body, flags, order, remaining, depth, nil)
}

func rtpsWireDataDetails(body []byte, flags byte, order binary.ByteOrder, remaining *int, depth int, fragment *rtpsDataFragment) (map[string]any, error) {
	if flags&12 == 12 {
		return nil, rtpsMalformed("DATA and KEY flags are both set")
	}
	if flags&16 != 0 {
		return nil, rtpsUnsupported("nonstandard DATA payload requires a selected representation")
	}
	if len(body) < 20 {
		return nil, rtpsMalformed("DATA fixed body is truncated")
	}
	at := 4 + int(order.Uint16(body[2:4]))
	if at < 20 || at > len(body) || at%4 != 0 {
		return nil, rtpsMalformed("DATA inline QoS offset is invalid")
	}
	sequence, err := rtpsControlSequence(body, 12, order, false)
	if err != nil {
		return nil, err
	}
	d := map[string]any{"Submessage": "DATA", "Flags": flags, "Extra Flags": order.Uint16(body), "Octets To Inline QoS": order.Uint16(body[2:4]), "Reader ID": hex.EncodeToString(body[4:8]), "Writer ID": hex.EncodeToString(body[8:12]), "Sender Role": "reported-writer"}
	rtpsControlWords(d, "Writer Sequence", sequence)
	if at > 20 {
		d["Fixed Extension Bytes"] = bytes.Clone(body[20:at])
	}
	if flags&2 != 0 {
		if depth < 5 {
			return nil, protocolError(ErrResourceExceeded, "RTPS DATA QoS projection exceeds depth budget")
		}
		before := *remaining
		params, used, err := rtpsParameters(body[at:], order, remaining)
		if err != nil {
			return nil, err
		}
		if fragment != nil {
			fragment.qosElements = before - *remaining
			fragment.qos = params
			fragment.qosWire = body[at : at+used]
		}
		var qos []map[string]any
		for _, p := range params {
			qos = append(qos, map[string]any{"ID": p.id, "Value": bytes.Clone(p.value)})
		}
		d["Inline QoS"] = qos
		at += used
	}
	d["Data Present"], d["Key Present"] = flags&4 != 0, flags&8 != 0
	if flags&12 == 0 {
		if at != len(body) {
			return nil, rtpsMalformed("DATA has trailing bytes without payload flags")
		}
		return d, nil
	}
	if len(body)-at < 4 {
		return nil, rtpsMalformed("DATA serialized payload header is truncated")
	}
	d["Representation ID"] = binary.BigEndian.Uint16(body[at:])
	d["Representation Options"] = binary.BigEndian.Uint16(body[at+2:])
	d["Serialized Payload"] = bytes.Clone(body[at:])
	d["Serialization"] = "opaque-application-data"
	if fragment != nil {
		fragment.fullData, fragment.keyOnly = true, flags&8 != 0
		fragment.reader, fragment.writer = hex.EncodeToString(body[4:8]), hex.EncodeToString(body[8:12])
		fragment.sequence = sequence
		fragment.start, fragment.count = 1, 1
		fragment.payload = body[at:]
		fragment.size, fragment.total = uint32(len(fragment.payload)), uint32(len(fragment.payload))
	}
	return d, nil
}

func decodeRTPSWire(w []byte, elements int) (map[string]any, error) {
	return decodeRTPSMessageBudget(w, elements, DefaultParserBudget().MaxRecursionDepth, true)
}
func decodeRTPSWireBudget(w []byte, elements, depth int) (map[string]any, error) {
	return decodeRTPSMessageBudget(w, elements, depth, true)
}

// WithSessionRTPSWire selects one-datagram DATA/control observations. Endpoint
// ports and UDP transport are supplied separately; no application type is
// inferred from the port, source prefix, DATA entity or serialized payload.
func WithSessionRTPSWire(port uint16) ProtocolSessionOption {
	return func(s *captureSession) error {
		if port == 0 {
			return fmt.Errorf("RTPS wire port must be nonzero")
		}
		old := s.f.a.datagramDecodeAs[port]
		if old != "" && old != "rtps-wire" {
			return fmt.Errorf("conflicting RTPS wire profile")
		}
		s.f.a.datagramDecodeAs[port] = "rtps-wire"
		return nil
	}
}

func (s *captureSession) probeRTPSWire(data []byte) (ProbeResult, bool) {
	a := s.f.a
	p := a.datagramDecodeAs[s.f.ports[1]]
	if p == "" {
		p = a.datagramDecodeAs[s.f.ports[0]]
	}
	if p != "rtps-wire" {
		return ProbeResult{}, false
	}
	reject := func(reason string) (ProbeResult, bool) {
		return ProbeResult{Verdict: ProbeReject, Protocol: "rtps", Reason: reason}, true
	}
	if len(data) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		return reject("RTPS wire byte budget")
	}
	if _, err := a.decodeRTPSWireSelected(data, s.f.ports[0], s.f.ports[1]); err != nil {
		return reject(err.Error())
	}
	return probeAccept("rtps", "rtps-wire-native", 95), true
}
