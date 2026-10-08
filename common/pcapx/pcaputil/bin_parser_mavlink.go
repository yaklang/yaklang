package pcaputil

// MAVLink messages may be co-packed in one datagram. Validate the complete
// datagram before exposing any decoded prefix; no state joins adjacent packets.
func (a *binParser) decodeMAVLinkDatagram(base *ProtocolEvent, w []byte, src, dst uint16) ([]*ProtocolEvent, bool) {
	explicit := a.datagramDecodeAs[dst]
	if explicit == "" {
		explicit = a.datagramDecodeAs[src]
	}
	if explicit != "" && explicit != "mavlink" {
		return nil, false
	}
	hint := src == 14550 || dst == 14550 || src == 14551 || dst == 14551
	magic := len(w) > 0 && (w[0] == 0xfe || w[0] == 0xfd)
	if !magic || explicit == "" && !hint && probeMAVLink(w, a.budget.MaxFrameBytes).Verdict != ProbeAccept {
		return nil, false
	}
	base.Protocol, base.Profile, base.Completeness = "mavlink", "mavlink-common-three-messages", "message"
	base.Admission = "wire-signature"
	if hint {
		base.Admission = "wire-and-port-hint"
	}
	if explicit != "" {
		base.Admission = "explicit-decode-as"
	}
	messages, err := decodeMAVLinkDatagram(w, a.budget.MaxFrameBytes, a.budget.MaxCollectionElements)
	if err != nil {
		a.finishProtocolDatagram(base, w, nil, err)
		return []*ProtocolEvent{base}, true
	}
	events := make([]*ProtocolEvent, 0, len(messages))
	for _, message := range messages {
		e := *base
		cloneEvidence(&e)
		e.Offset, e.Length = uint64(message.Offset), len(message.Raw)
		e.Session, e.semanticFields = message.Fields, cloneSession(message.Fields)
		a.finishProtocolDatagram(&e, message.Raw, nil, nil)
		events = append(events, &e)
	}
	return events, true
}
