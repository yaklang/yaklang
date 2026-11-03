package pcaputil

import (
	"bytes"
	"encoding/binary"
)

const (
	quicEarlyAwaiting = "QUIC early application data awaits observed server acceptance"
	quicEarlyRejected = "QUIC early application data was rejected by the observed server handshake"
)

// These are private observations in one capture-domain/CID owner. A usable
// traffic secret alone says nothing about the server's acceptance of 0-RTT.
type quicEarlyContext struct {
	offerKnown, offered, decisionKnown, accepted, retryRejected bool
}

func (s *quicEarlyContext) observe(t *binTLS, typ byte, body []byte, max int) error {
	switch typ {
	case 1:
		h, err := tlsHello(body, true, max)
		if err != nil {
			return err
		}
		_, offered := h.ext[42]
		if s.decisionKnown && s.accepted && !offered {
			return protocolError(ErrMalformedMessage, "QUIC accepted early data was not offered")
		}
		s.offerKnown, s.offered = true, offered
	case 2:
		if t.retrySeen {
			s.retryRejected = true // TLS HRR; a QUIC Retry is not a rejection.
		}
	case 8:
		if s.decisionKnown {
			return protocolError(ErrMalformedMessage, "QUIC duplicate EncryptedExtensions early-data decision")
		}
		accepted := false
		// The shared TLS admission/decoder has already checked this complete
		// vector, extension uniqueness and the empty early_data value.
		for rest := body[2:]; len(rest) > 0; {
			kind, n := binary.BigEndian.Uint16(rest), int(binary.BigEndian.Uint16(rest[2:]))
			accepted = accepted || kind == 42
			rest = rest[4+n:]
		}
		if accepted {
			if s.retryRejected || s.offerKnown && !s.offered {
				return protocolError(ErrMalformedMessage, "QUIC accepted early data contradicts ClientHello or TLS retry")
			}
			selection, _ := t.resumption.context["PSK Selection"].(map[string]any)
			if t.finalHello && (t.resumption.context["PSK Selected"] != true || selection["Selected Identity"] != uint16(0)) {
				return protocolError(ErrMalformedMessage, "QUIC early data requires selected PSK identity zero")
			}
		}
		s.decisionKnown, s.accepted = true, accepted
	}
	return nil
}

func (q *binQUIC) earlyRejected() bool {
	return q.earlyContext.retryRejected || q.earlyContext.decisionKnown && !q.earlyContext.accepted
}

func (q *binQUIC) earlyAccepted() bool {
	s := q.earlyContext
	if !s.offerKnown || !s.offered || !s.decisionKnown || !s.accepted || s.retryRejected || q.tls == nil || !q.tls.finalHello {
		return false
	}
	selection, _ := q.tls.resumption.context["PSK Selection"].(map[string]any)
	return q.tls.resumption.context["PSK Selected"] == true && selection["Selected Identity"] == uint16(0)
}

func recoverableQUICEarlyError(err error) bool {
	_, typed := classifySessionError(err)
	return typed != nil && typed.Kind == ErrContextRequired && (typed.Message == quicEarlyAwaiting || typed.Message == quicEarlyRejected)
}

func (q *binQUIC) awaitEarlyApplication(h quicHdr, frames []map[string]any) bool {
	if !q.native || q.provider == nil || h.typeName != "0-RTT" || q.earlyAccepted() {
		return false
	}
	for _, frame := range frames {
		if name, _ := frame["Frame Type"].(string); name != "PADDING" && name != "PING" {
			return true
		}
	}
	return false
}

type quicEarlyPending struct {
	meta ProtocolEvent
	wire []byte
	from uint64
	off  int
	cost int64
}

// Count bounded wire envelopes without decrypting, delivering frames or
// allocating state. An invalid tail still needs one diagnostic observation.
func (q *binQUIC) remainingWireEvents(dir int, wire []byte, limit int) int {
	count := 0
	for len(wire) > 0 {
		count++
		if count >= limit {
			return count
		}
		h, err := q.wireHeader(wire, dir)
		if err != nil || h.size <= 0 || h.size > len(wire) {
			return count
		}
		wire = wire[h.size:]
	}
	return count
}

func (q *binQUIC) retainEarly(e *ProtocolEvent, h quicHdr, raw []byte, off, max int, err error) error {
	_, typed := classifySessionError(err)
	if !q.native || q.provider == nil || q.retired || q.closed || e.Direction != 0 || h.typeName != "0-RTT" ||
		typed == nil || !(typed.Kind == ErrEncrypted || typed.Kind == ErrContextRequired && typed.Message == quicEarlyAwaiting) {
		return nil
	}
	limit := sessionCollectionLimit(max)
	if limit > 64 {
		limit = 64
	}
	// Charge the owned event, raw allocation and all variable provenance before
	// allocating. Pending ciphertext is also included in the flow-wide budget.
	cost := int64(2048 + len(raw) + len(e.Source) + len(e.Destination) + len(e.Domain.Encapsulation))
	cost += int64(len(e.SourceBytes.PacketRefs))*64 + int64(len(e.SourceBytes.ParentPDUs))*8
	for _, layer := range e.CarrierLayers {
		cost += int64(24 + len(layer))
	}
	for _, ref := range e.SourceBytes.PacketRefs {
		cost += int64(len(ref.Domain.Encapsulation))
	}
	if len(q.earlyPending) >= limit || cost > int64(q.byteLimit)-q.earlyPendingBytes {
		return protocolError(ErrResourceExceeded, "QUIC pending early packet byte/count budget")
	}
	meta := ProtocolEvent{Timestamp: e.Timestamp, Transport: e.Transport, Source: e.Source, Destination: e.Destination,
		Domain: e.Domain, SourceBytes: e.SourceBytes, CarrierLayers: e.CarrierLayers, Direction: e.Direction}
	cloneEvidence(&meta)
	p := &quicEarlyPending{meta: meta, wire: bytes.Clone(raw), from: e.ID, off: off, cost: cost}
	q.earlyPending = append(q.earlyPending, p)
	q.earlyPendingBytes += cost
	return nil
}

// Optional early secrets can arrive after all four regular traffic secrets.
// Loading just this slot must not replace established application-key epochs.
func (q *binQUIC) loadLateEarlyKey() error {
	if q.keys == nil || q.keys.early[0] != nil || q.provider == nil || q.tls == nil || len(q.tls.random) != 32 {
		return nil
	}
	secret, ok := q.provider.LookupTLSSecret("CLIENT_EARLY_TRAFFIC_SECRET", q.tls.random)
	if !ok {
		return nil
	}
	algorithm := ""
	switch q.tls.suite {
	case 0x1301:
		algorithm = quicAEADAES128GCM
	case 0x1302:
		algorithm = quicAEADAES256GCM
	case 0x1303:
		algorithm = quicAEADChaCha20Poly1305
	default:
		return nil
	}
	ring, err := quicLoadKeys(QUICKeyMaterial{Secrets: map[string][]byte{"CLIENT_EARLY_TRAFFIC_SECRET": secret}, AEAD: algorithm, Version: q.effectiveVersion()})
	if err == nil {
		q.keys.early[0] = ring.early[0]
	}
	return err
}

func (a *binParser) resumeEarlyQUIC(f *binFlow, trigger *ProtocolEvent, max int) []*ProtocolEvent {
	q := f.quic
	if q == nil || q.retired || q.closed || len(q.earlyPending) == 0 || max <= 0 || !q.earlyRejected() && !q.earlyAccepted() {
		return nil
	}
	keyError := error(nil)
	if !q.earlyRejected() {
		keyError = q.loadLateEarlyKey()
		if keyError == nil && (q.keys == nil || q.keys.early[0] == nil) {
			return nil
		}
	}
	var events []*ProtocolEvent
	for len(q.earlyPending) > 0 && len(events) < max {
		p := q.earlyPending[0]
		// A caller cannot use mutable published provenance to retarget an owner.
		if p.meta.Domain != trigger.Domain {
			break
		}
		q.earlyPending[0] = nil
		q.earlyPending = q.earlyPending[1:]
		q.earlyPendingBytes -= p.cost
		if len(q.earlyPending) == 0 {
			q.earlyPending = nil
		}
		e := p.meta
		e.ID, e.FlowID, e.Protocol, e.Profile = a.ids.Add(1), f.id, "quic", quicNativeProfile(q.effectiveVersion())
		e.nativeQUICObservation, e.Length = true, len(p.wire)
		q.eventID, q.eventRefs = e.ID, e.SourceBytes.PacketRefs
		h, err := q.wireHeader(p.wire, e.Direction)
		if err == nil {
			if keyError != nil {
				err = keyError
			} else {
				e.Session, err = q.consume(e.Direction, p.wire, a.budget.MaxCollectionElements)
			}
		}
		q.eventRefs = nil
		if err == nil || isRecoverableH3ObservationError(err) {
			if budgetError := f.reserveSession(q.initialMemory()); budgetError != nil {
				err = budgetError
			}
		}
		if _, typed := classifySessionError(err); typed != nil && typed.Kind == ErrResourceExceeded {
			// Match ordinary native dispatch: a resource refusal cannot publish
			// a successful frame/application projection from a partial attempt.
			e.Session = nil
			e.ResponseTo, e.TransactionID = 0, 0
		}
		if e.Session == nil {
			e.Session = map[string]any{}
		}
		e.Session["Reprocessed From PDU"] = p.from
		var contextRef *PacketReference
		for _, ref := range trigger.SourceBytes.PacketRefs {
			if ref.Number != 0 && (contextRef == nil || ref.Number > contextRef.Number) {
				owned := ref
				contextRef = &owned
			}
		}
		if contextRef != nil {
			e.Session["Reprocessed In Packet"] = *contextRef
		}
		a.projectNativeQUICSession(&e, q, p.off)
		e.Admission = "observed-CID-unverified-payload"
		if e.Session["Authentication Verified"] == true {
			e.Admission = "authenticated-wire-and-observed-CID"
		} else if e.Session["Encrypted"] == true {
			e.Admission = "observed-CID-protected-payload"
		}
		a.finishNativeQUICWire(&e, h, p.wire, err, true)
		events = append(events, &e)
		if e.sessionError != nil && e.sessionError.Kind == ErrResourceExceeded {
			f.retireQUICUDP()
			break
		}
		if q.closed {
			q.earlyPending, q.earlyPendingBytes = nil, 0
			break
		}
	}
	return events
}

func (a *binParser) projectNativeQUICSession(e *ProtocolEvent, q *binQUIC, off int) {
	if e.Session != nil {
		e.Session["Observation Scope"] = "UDP capture domain and observed CID"
		e.Session["Datagram Offset"] = off
		e.Session["Native Carrier"] = true
		e.Session["ALPN"] = q.alpn
		// Duplicate/protected packets can return before finishFrames. Their
		// connection context still includes an earlier observed HTTP3 fault.
		q.annotateH3ConnectionFault(e.Session)
		if links, ok := e.Session["HTTP3 Late Associations"].([]map[string]any); ok {
			for _, link := range links {
				link["Flow ID"], link["Observed In PDU ID"] = e.FlowID, e.ID
				link["Capture Domain"] = map[string]any{"Section": e.Domain.Section, "Interface": e.Domain.Interface, "Encapsulation": e.Domain.Encapsulation}
			}
		}
	}
	if e.Session != nil {
		var transaction uint64
		multiple := false
		frames, _ := e.Session["Frames"].([]map[string]any)
		for _, fr := range frames {
			if id, ok := fr["Request PDU ID"].(uint64); ok && id != 0 {
				if transaction != 0 && transaction != id {
					multiple = true
				}
				transaction = id
			}
		}
		if !multiple && transaction != 0 {
			e.TransactionID = transaction
			if e.Direction == 1 {
				e.ResponseTo = transaction
			}
		}
		if e.Session["Encrypted"] == true {
			e.Completeness = "encrypted"
			e.Session["Content Visibility"] = "encrypted"
			e.Admission = "observed-CID-protected-payload"
		}
	}
	if e.Session["HTTP3"] == true {
		e.Protocol = "http3"
		e.Profile = "http3-native"
	}
	if e.Session["DoQ"] == true {
		e.Protocol = "doq"
		e.Profile = "doq-native"
		if d, ok := e.Session["DoQ Message"].(map[string]any); ok {
			e.Session["DNS"] = d["DNS"]
		}
	}
}

func (a *binParser) finishNativeQUICWire(e *ProtocolEvent, h quicHdr, raw []byte, err error, bounded bool) {
	// The authenticated session already parsed this header. Preserve the wire
	// field view without executing a YAML operator for every ciphertext octet.
	e.nativeQUICWire = err == nil
	if bounded && len(raw) == h.size && e.Session != nil {
		observed := h
		if !h.long {
			// The session resolves a short header's CID size from observed
			// receiver context; the stateless outer header has no size byte.
			observed.dcid, _ = e.Session["DCID"].([]byte)
		}
		e.Session["Wire Field Ranges"] = quicHeaderWireRanges(observed, raw)
	}
	// Query identity and passive context are private even on a refused
	// native event; mutable public labels/caches cannot grant success.
	e.semanticSession = cloneSession(e.Session)
	if e.Protocol == "http3" {
		e.nativeQUICApplication = 1
	} else if e.Protocol == "doq" {
		e.nativeQUICApplication = 2
	}
	a.finishProtocolDatagram(e, raw, a.specs["application-layer.quic/QUIC"], err)
}
