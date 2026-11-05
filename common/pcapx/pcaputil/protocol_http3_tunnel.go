package pcaputil

import "bytes"

// Once CONNECT succeeds, a known non-DATA type is enough to prove the
// connection violation. Neither a complete length/payload nor QPACK context is
// required. Cache only the first such type per direction while context is late.
type h3TunnelTypeObservation struct {
	pdu, stream, offset, typ uint64
	direction                int
	header                   []byte
	source                   *h3ResponseObservation
	orderUnverified          bool
}

func (o *h3TunnelTypeObservation) memory() int64 {
	return 128 + int64(cap(o.header)) + o.source.memory()
}

func h3KnownNonDATA(typ uint64) bool {
	switch typ {
	case 1, 2, 3, 4, 5, 6, 7, 8, 9, 13:
		return true // RFC9114 core types and explicitly forbidden HTTP/2 types
	}
	return false // unknown extensions are not certified or guessed from payload
}

func h3TunnelFrameName(typ uint64) string {
	switch typ {
	case 1:
		return "HEADERS"
	case 3:
		return "CANCEL_PUSH"
	case 4:
		return "SETTINGS"
	case 5:
		return "PUSH_PROMISE"
	case 7:
		return "GOAWAY"
	case 13:
		return "MAX_PUSH_ID"
	}
	return "reserved HTTP/2 frame type"
}

func (q *binQUIC) h3TunnelEligible(st *quicStream, sid uint64) bool {
	return q.native && q.alpn == "h3" && !q.closed && !q.retired && q.eventID != 0 && sid&3 == 0 && !st.reset && !st.resetDirection[0] && !st.resetDirection[1]
}

func (q *binQUIC) h3TunnelEstablished(st *quicStream, sid uint64) bool {
	h := st.h3
	return q.h3TunnelEligible(st, sid) && h.requestMethod == h3MethodCONNECT && h.reqHeaders > 0 && h.fieldFailure[0] == "" && !h.framingFailure[0] && h.body[1].header && h.body[1].status >= 200 && h.body[1].status < 300
}

func (q *binQUIC) observeH3TunnelType(st *quicStream, dir int, sid, typ uint64, prefix []byte) {
	h := st.h3
	if !q.h3TunnelEligible(st, sid) || !h3KnownNonDATA(typ) || h.headers[dir] == 0 || h.tunnelTypes[dir] != nil || h.tunnelFailure[dir] || h.requestMethod != h3MethodUnknown && h.requestMethod != h3MethodCONNECT {
		return
	}
	if h.body[1].header && (h.body[1].status < 200 || h.body[1].status >= 300) {
		return
	}
	start := uint64(h.parsed[dir])
	source := h3HeaderObservation(q.eventID, map[string]any{"Byte Source": st.sources[dir].describe(sid, dir, start, start+uint64(len(prefix)))})
	for i, ref := range source.refs {
		source.refs[i] = h3OwnedReference(ref)
	}
	h.tunnelTypes[dir] = &h3TunnelTypeObservation{pdu: q.eventID, stream: sid, offset: start, typ: typ, direction: dir, header: bytes.Clone(prefix), source: source}
}

func (o *h3TunnelTypeObservation) snapshot(observed uint64) map[string]any {
	return map[string]any{
		"Observation Verdict": "violation-observed", "Error Name": "H3_FRAME_UNEXPECTED", "Error Code": uint64(0x105), "Error Scope": "connection",
		"Error": "known non-DATA frame after successful CONNECT", "First PDU ID": observed,
		"Frame PDU ID": o.pdu, "Observed In PDU ID": observed, "Stream ID": o.stream, "Direction": uint64(o.direction),
		"Frame Type": h3TunnelFrameName(o.typ), "Frame Type Value": o.typ, "Frame Offset": o.offset,
		"Frame Header Bytes": bytes.Clone(o.header), "Frame Header Ranges": map[string]any{"Frame Type": []any{uint64(0), uint64(len(o.header))}, "Frame Type Value": []any{uint64(0), uint64(len(o.header))}},
		"Header Scope": "forbidden type varint only; length and payload are not required or decoded",
		"Byte Source":  map[string]any{"Kind": "quic-authenticated-stream-reassembly", "Stream ID": o.stream, "Direction": uint64(o.direction), "Offset": o.offset, "Length": uint64(len(o.header)), "Parent PDUs": append([]uint64(nil), o.source.origins...), "Packet References": quicSourceReferences(o.source.refs), "Byte Coverage Complete": o.source.originsFull, "Packet Coverage Complete": o.source.refsFull},
		"Scope":        "authenticated contiguous known frame type after completed CONNECT; connection violation observed; peer closure and delivery are not inferred",
	}
}

func (q *binQUIC) reportH3TunnelType(st *quicStream, dir int, info map[string]any) map[string]any {
	o := st.h3.tunnelTypes[dir]
	if o == nil || o.orderUnverified {
		return nil
	}
	fact := o.snapshot(q.eventID)
	if dir == 0 && o.pdu != q.eventID {
		// Same-direction server offsets prove HEADERS follows its 2xx.
		// Client offsets do not order the opposite-direction response.
		for _, key := range []string{"Error Name", "Error Code", "Error Scope", "Error", "First PDU ID"} {
			delete(fact, key)
		}
		fact["Observation Verdict"] = "order-unverified"
		fact["Header Scope"] = "known type varint only; cross-direction order, length and payload semantics are unverified"
		fact["Scope"] = "client type observed before success context; stream offsets do not prove order relative to the server response; no connection error asserted"
		rows, _ := info["HTTP3 Tunnel Observations"].([]map[string]any)
		info["HTTP3 Tunnel Observations"] = append(rows, fact)
		o.orderUnverified = true
		if uint64(st.h3.parsed[dir]) > o.offset {
			st.h3.tunnelTypes[dir] = nil
		}
		return fact
	}
	fault := &h3ConnectionFault{pdu: q.eventID, stream: o.stream, offset: o.offset, direction: dir, tunnel: o}
	if q.h3Fault == nil {
		q.h3Fault = fault
	}
	rows, _ := info["HTTP3 Tunnel Observations"].([]map[string]any)
	info["HTTP3 Tunnel Observations"] = append(rows, fact)
	st.h3.tunnelTypes[dir] = nil
	st.h3.tunnelFailure[dir], st.h3.blocked[dir] = true, false
	q.annotateH3ConnectionFault(info)
	return fact
}

func (q *binQUIC) recheckH3Tunnel(st *quicStream, sid uint64, info map[string]any) {
	if !q.h3TunnelEligible(st, sid) {
		return
	}
	h := st.h3
	if q.h3TunnelEstablished(st, sid) {
		// Keep observation order deterministic if both directions await context.
		dirs := [2]int{0, 1}
		if a, b := h.tunnelTypes[0], h.tunnelTypes[1]; a != nil && b != nil && b.pdu < a.pdu {
			dirs = [2]int{1, 0}
		}
		for _, dir := range dirs {
			q.reportH3TunnelType(st, dir, info)
		}
	} else if h.requestMethod != h3MethodUnknown && h.requestMethod != h3MethodCONNECT || h.body[1].header && (h.body[1].status < 200 || h.body[1].status >= 300) {
		h.tunnelTypes = [2]*h3TunnelTypeObservation{}
	}
}

func (q *binQUIC) rejectH3TunnelType(st *quicStream, dir int, sid uint64, fr, info map[string]any, frames []map[string]any) error {
	fact := q.reportH3TunnelType(st, dir, info)
	if fact == nil || fact["Observation Verdict"] != "violation-observed" {
		return nil
	}
	for _, m := range []map[string]any{fr, info} {
		m["HTTP3"], m["HTTP3 Stream ID"] = true, sid
		m["HTTP3 Frame Error"], m["HTTP3 Stream State"] = cloneSession(fact), "unexpected-tunnel-frame; later bytes quarantined"
		m["HTTP3 Frames"] = append(frames, cloneSession(fact))
	}
	info["Protocol Transition"] = "quic->http3"
	st.sources[dir].prune(^uint64(0))
	return &h3ConnectionObservationError{protocolError(ErrMalformedMessage, "HTTP3 H3_FRAME_UNEXPECTED: known non-DATA frame after successful CONNECT")}
}
