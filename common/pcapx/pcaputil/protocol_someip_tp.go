package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
	"unsafe"
)

const someIPTPProfile = "someip-v1-tp-observation"
const someIPTPBytes = 65536
const someIPTPFrames = 64
const someIPTPBase = int64(unsafe.Sizeof(binFlow{})) + 2048
const someIPTPTTL = 30 * time.Second // local observation, not inferred ECU configuration

type someIPPacket struct {
	message, request, length   uint32
	version, iface, kind, code byte
	tp                         uint32
	payload                    []byte
}
type someIPKey struct{ message, request uint32 }

func (p *someIPPacket) key() someIPKey  { return someIPKey{p.message, p.request} }
func (p *someIPPacket) header() [4]byte { return [4]byte{p.version, p.iface, p.kind, p.code} }
func someIPHeader(w []byte) *someIPPacket {
	if len(w) < 16 {
		return nil
	}
	return &someIPPacket{message: binary.BigEndian.Uint32(w[:4]), length: binary.BigEndian.Uint32(w[4:8]), request: binary.BigEndian.Uint32(w[8:12]), version: w[12], iface: w[13], kind: w[14], code: w[15]}
}
func inspectSomeIPTP(w []byte, b ParserBudget) (*someIPPacket, error) {
	if b.MaxRecursionDepth < 3 {
		return someIPHeader(w), protocolError(ErrResourceExceeded, "SOME/IP field nesting budget")
	}
	p := someIPHeader(w)
	if p == nil {
		return nil, protocolError(ErrMalformedMessage, "short SOME/IP header")
	}
	if len(w) > min(b.MaxFrameBytes, b.MaxMessageBytes) {
		return p, protocolError(ErrResourceExceeded, "SOME/IP datagram exceeds wire budget")
	}
	if p.version != 1 {
		return p, protocolError(ErrUnsupportedVersion, "selected SOME/IP version1 only")
	}
	switch p.kind &^ byte(0x20) {
	case 0, 1, 2, 0x80, 0x81:
	default:
		return p, protocolError(ErrUnsupportedFeature, "unselected SOME/IP message type")
	}
	if p.length < 8 || uint64(p.length)+8 != uint64(len(w)) {
		return p, protocolError(ErrMalformedMessage, "SOME/IP length must describe exactly one complete datagram PDU")
	}
	if p.kind&0x20 == 0 {
		p.payload = w[16:]
		return p, nil
	}
	if p.length < 12 || len(w) < 20 {
		return p, protocolError(ErrMalformedMessage, "short SOME/IP-TP header")
	}
	p.tp = binary.BigEndian.Uint32(w[16:20])
	p.payload = w[20:]
	// R22-11 SWS_SomeIpTp_00013: receiver ignores the three reserved bits.
	if p.tp&1 != 0 && len(p.payload) == 0 {
		return p, protocolError(ErrUnsupportedFeature, "empty in-progress TP fragment outside selected observation")
	}
	if p.tp&1 != 0 && len(p.payload)%16 != 0 {
		return p, protocolError(ErrMalformedMessage, "nonfinal SOME/IP-TP payload must be divisible by16")
	}
	limit := min(someIPTPBytes, max(0, b.MaxMessageBytes-16))
	if uint64(p.tp&0xfffffff0)+uint64(len(p.payload)) > uint64(limit) {
		return p, protocolError(ErrResourceExceeded, "SOME/IP-TP declared payload range exceeds assembly budget")
	}
	return p, nil
}
func someIPFields(p *someIPPacket, w []byte) map[string]any {
	f := map[string]any{"message_id": p.message, "service_id": uint16(p.message >> 16), "method_id": uint16(p.message), "length": p.length, "request_id": p.request, "client_id": uint16(p.request >> 16), "session_id": uint16(p.request), "protocol_version": p.version, "interface_version": p.iface, "message_type": p.kind, "base_message_type": p.kind &^ byte(0x20), "return_code": p.code, "header_hex": hex.EncodeToString(w[:16]), "tp_present": p.kind&0x20 != 0, "payload_hex": hex.EncodeToString(p.payload), "application_schema_verified": false, "endpoint_authenticated": false, "request_response_verified": false}
	if p.kind&0x20 != 0 {
		f["tp"] = map[string]any{"offset_units": p.tp >> 4, "offset_bytes": p.tp & 0xfffffff0, "reserved_bits": (p.tp >> 1) & 7, "reserved_bits_ignored": true, "more_segments": p.tp&1 != 0, "header_hex": hex.EncodeToString(w[16:20])}
	}
	return f
}

type someIPTransfer struct {
	header [4]byte
	data   []byte
	active bool
	at     time.Time
	ids    []uint64
	refs   []PacketReference
	charge int64
}
type someIPConversation struct {
	flow          *binFlow
	direction     int
	clock         time.Time
	seen          map[someIPKey]*someIPTransfer
	charge        int64
	refuseUnknown bool
}
type someIPStore struct {
	entries       map[binUDPKey]*someIPConversation
	refuseUnknown bool
}

func (c *someIPConversation) retire(p *someIPTransfer) {
	c.charge -= p.charge - 256
	p.charge = 256
	p.data = nil
	p.ids = nil
	p.refs = nil
	p.active = false
}

// WithSessionSomeIPTP explicitly selects UDP transport observations on a port.
// Passive header/payload observations never certify PduR routing, application
// schemas or request/response invocation. Carrier and endpoint ports remain
// explicit caller facts; TP restarts/reuse remain ambiguous for this observer.
func WithSessionSomeIPTP(port uint16) ProtocolSessionOption {
	return func(s *captureSession) error {
		if port == 0 {
			return fmt.Errorf("SOME/IP-TP needs a nonzero port")
		}
		if s.f.a.datagramDecodeAs == nil {
			s.f.a.datagramDecodeAs = map[uint16]string{}
		}
		if old := s.f.a.datagramDecodeAs[port]; old != "" && old != "someip-tp" {
			return fmt.Errorf("conflicting SOME/IP-TP profile")
		}
		s.f.a.datagramDecodeAs[port] = "someip-tp"
		return nil
	}
}
func (a *binParser) someIPSelected(src, dst uint16) bool {
	p := a.datagramDecodeAs[dst]
	if p == "" {
		p = a.datagramDecodeAs[src]
	}
	return p == "someip-tp"
}
func (a *binParser) consumeSomeIPTP(e *ProtocolEvent, w []byte, inputErr error) (map[string]any, error) {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	packet, err := inspectSomeIPTP(w, a.budget)
	if inputErr != nil {
		err = inputErr
	}
	if a.someIPTP == nil {
		a.someIPTP = &someIPStore{entries: map[binUDPKey]*someIPConversation{}}
	}
	store := a.someIPTP
	// A complete literal header identifies refusal context without allocating payload.
	if packet == nil {
		if err != nil {
			return nil, err
		}
		return nil, protocolError(ErrMalformedMessage, "missing SOME/IP header")
	}
	key := binUDPKey{e.Source, e.Destination, e.Domain}
	c := store.entries[key]
	if c == nil {
		if store.refuseUnknown || len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			store.refuseUnknown = true
			return nil, protocolError(ErrResourceExceeded, "SOME/IP conversation history budget")
		}
		flow := &binFlow{a: a, id: a.flows.Add(1), protocol: "someip", endpoints: [2]string{e.Source, e.Destination}}
		if problem := flow.reserveSession(someIPTPBase); problem != nil {
			store.refuseUnknown = true
			return nil, problem
		}
		c = &someIPConversation{flow: flow, clock: e.Timestamp, direction: e.Direction, seen: map[someIPKey]*someIPTransfer{}, charge: someIPTPBase}
		store.entries[key] = c
	}
	defer func() {
		if c.flow.sessionBytes > c.charge {
			delta := c.flow.sessionBytes - c.charge
			c.flow.sessionBytes = c.charge
			a.buffered.Add(-delta)
		}
	}()
	e.FlowID = c.flow.id
	backdated := e.Timestamp.Before(c.clock)
	if backdated {
		for _, p := range c.seen {
			if p.active {
				c.retire(p)
			}
		}
	}
	if e.Timestamp.After(c.clock) {
		c.clock = e.Timestamp
	}
	for _, p := range c.seen {
		if p.active && c.clock.Sub(p.at) >= someIPTPTTL {
			c.retire(p)
		}
	}
	mk := packet.key()
	p := c.seen[mk]
	// Ordinary single-PDU wire messages have no TP generation claim. Repeated
	// request IDs remain observable without erasing any retired TP history.
	if !backdated && err == nil && packet.kind&0x20 == 0 && (p == nil || !p.active) {
		if problem := c.flow.reserveSession(c.charge + int64(32768+16*len(w))); problem != nil {
			return nil, problem
		}
		fields := someIPFields(packet, w)
		fields["assembly"] = map[string]any{"state": "standalone-wire-observation", "complete": true, "observed_segments": 1, "observed_payload_bytes": len(packet.payload), "first_event_id": e.ID, "contributor_event_ids": []uint64{e.ID}, "generation_authentication_verified": false, "assembled_payload_hex": hex.EncodeToString(packet.payload)}
		e.SourceBytes.ParentPDUs = []uint64{e.ID}
		return fields, nil
	}
	if p == nil {
		if c.refuseUnknown || len(c.seen) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			c.refuseUnknown = true
			return nil, protocolError(ErrResourceExceeded, "SOME/IP message identity history budget")
		}
		if problem := c.flow.reserveSession(c.charge + 256); problem != nil {
			c.refuseUnknown = true
			return nil, problem
		}
		p = &someIPTransfer{header: packet.header(), charge: 256}
		c.seen[mk] = p
		c.charge += 256
	} else if !p.active {
		if err != nil {
			return nil, err
		}
		return nil, protocolError(ErrContextRequired, "reused/retired SOME/IP message generation")
	}
	fail := func(problem error) (map[string]any, error) { c.retire(p); return nil, problem }
	if backdated {
		return fail(protocolError(ErrContextRequired, "backdated SOME/IP observation cannot establish assembly progression"))
	}
	if err != nil {
		return fail(err)
	}
	if p.active && p.header != packet.header() {
		return fail(protocolError(ErrContextRequired, "SOME/IP transfer header inconsistency"))
	}
	tp := packet.kind&0x20 != 0
	offset := packet.tp & 0xfffffff0
	if p.active && (!tp || offset == 0) {
		return fail(protocolError(ErrContextRequired, "SOME/IP restart cannot certify an unambiguous passive generation"))
	}
	if tp && offset != uint32(len(p.data)) {
		return fail(protocolError(ErrContextRequired, "SOME/IP-TP missing/orphan/overlapping/out-of-order offset"))
	}
	if len(p.ids) >= min(someIPTPFrames, a.budget.MaxCollectionElements) || len(e.SourceBytes.PacketRefs) > a.budget.MaxCollectionElements-len(p.refs) {
		return fail(protocolError(ErrResourceExceeded, "SOME/IP segment/contributor budget"))
	}
	total := len(p.data) + len(packet.payload)
	if total > min(someIPTPBytes, max(0, a.budget.MaxMessageBytes-16)) {
		return fail(protocolError(ErrResourceExceeded, "SOME/IP assembled message budget"))
	}
	// Reserve owned byte/ID/reference slices and full literal/assembled hex projections
	// before retaining data or allocating field maps. This is a logical budget.
	extra := int64(256 + 8*total + 32*(len(p.ids)+1) + 64*(len(p.refs)+len(e.SourceBytes.PacketRefs)))
	projection := int64(32768 + 16*len(w) + 4*total)
	if problem := c.flow.reserveSession(c.charge - p.charge + extra + projection); problem != nil {
		return fail(problem)
	}
	ids := append(append([]uint64(nil), p.ids...), e.ID)
	refs := append(append([]PacketReference(nil), p.refs...), e.SourceBytes.PacketRefs...)
	fields := someIPFields(packet, w)
	complete := !tp || packet.tp&1 == 0
	state := "observed-contiguous-fragment"
	if complete {
		state = "complete-first-observed-transfer"
	}
	if !tp {
		state = "standalone-wire-observation"
	}
	assembly := map[string]any{"state": state, "complete": complete, "observed_segments": len(ids), "observed_payload_bytes": total, "first_event_id": ids[0], "contributor_event_ids": append([]uint64(nil), ids...), "generation_authentication_verified": false}
	if complete {
		data := make([]byte, total)
		copy(data, p.data)
		copy(data[len(p.data):], packet.payload)
		assembly["assembled_payload_hex"] = hex.EncodeToString(data)
		c.retire(p)
	} else {
		joined := make([]byte, total)
		copy(joined, p.data)
		copy(joined[len(p.data):], packet.payload)
		c.charge += extra - p.charge
		p.charge = extra
		p.header = packet.header()
		p.data = joined
		p.ids = ids
		p.refs = refs
		p.at = c.clock
		p.active = true
	}
	fields["assembly"] = assembly
	e.SourceBytes.ParentPDUs = append([]uint64(nil), ids...)
	return fields, nil
}
func (a *binParser) decodeSomeIPTPDatagram(e *ProtocolEvent, w []byte, inputErr error) bool {
	e.Protocol, e.Profile, e.Admission, e.Completeness = "someip", someIPTPProfile, "explicit-decode-as", "fragment"
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	fields, err := a.consumeSomeIPTP(e, w, inputErr)
	if err == nil {
		e.semanticFields = cloneSession(fields)
		e.semanticSession = cloneSession(fields)
		e.Session = cloneSession(fields)
		if fields["assembly"].(map[string]any)["complete"] == true {
			e.Completeness = "message"
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
func (a *binParser) closeSomeIPTPLocked() []*ProtocolEvent {
	var out []*ProtocolEvent
	if a.someIPTP != nil {
		for key, c := range a.someIPTP.entries {
			for _, p := range c.seen {
				if p.active {
					er := &ProtocolError{Kind: ErrNeedMore, Message: "SOME/IP-TP closed with missing final fragment"}
					e := &ProtocolEvent{FlowID: c.flow.id, Direction: c.direction, Timestamp: c.clock, Domain: key.domain, Source: key.a, Destination: key.b, Transport: "udp", Protocol: "someip", Profile: someIPTPProfile, Status: "incomplete", Completeness: "incomplete", Error: er.Message, sessionError: er, SourceBytes: ByteSource{Kind: "reassembled", ParentPDUs: append([]uint64(nil), p.ids...), PacketRefs: append([]PacketReference(nil), p.refs...)}}
					out = append(out, e)
					a.incomplete.Add(1)
				}
				c.retire(p)
			}
			c.flow.closeSession()
		}
	}
	a.someIPTP = nil
	sort.Slice(out, func(i, j int) bool { return out[i].SourceBytes.ParentPDUs[0] < out[j].SourceBytes.ParentPDUs[0] })
	return out
}

func (s *captureSession) probeSomeIPTP(data []byte) (ProbeResult, bool) {
	if !s.f.a.someIPSelected(s.f.ports[0], s.f.ports[1]) {
		return ProbeResult{}, false
	}
	if _, err := inspectSomeIPTP(data, s.f.a.budget); err != nil {
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}, true
	}
	return probeAccept("someip", someIPTPProfile, 98), true
}

// Native Feed supplies caller-relative direction after constructing the UDP
// carrier. Preserve that fact for Close; captures retain their directed flow's
// original direction instead of inventing an orientation from address ordering.
func (a *binParser) retainSomeIPSessionDirection(e *ProtocolEvent) {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	if a.someIPTP != nil {
		if c := a.someIPTP.entries[binUDPKey{e.Source, e.Destination, e.Domain}]; c != nil {
			c.direction = e.Direction
		}
	}
}
