package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// This receiver observes TLS wire records, not authenticated RADIUS sessions.
// Missing NAS identity remains an outer RADIUS conformance error. State echo,
// request/response identifiers and a captured PEAP/TTLS Start delimit this separate
// wire observation; they do not verify a Message-Authenticator or peer identity.
type eapTLSStore struct {
	entries map[binUDPKey]*eapTLSConversation
	charge  int64
}
type eapTLSConversation struct {
	pending  []*eapTLSMessage
	requests map[byte]*eapTLSRequest
	epochs   []*eapTLSEpoch
	seen     map[[32]byte]bool
	blocked  bool
}
type eapTLSRequest struct {
	message  *eapTLSMessage
	epoch    *eapTLSEpoch
	answered bool
}
type eapTLSMessage struct {
	parent                                        *ProtocolEvent
	radiusCode, radiusID, code, id, method, flags byte
	state, body                                   []byte
	declared                                      uint32
	digest                                        [32]byte
}
type eapTLSFlight struct {
	pieces   []rdpUDPChunk
	size     int
	declared uint32
}
type eapTLSEpoch struct {
	id                          uint64
	client, server              string
	method, version, expectedID byte
	state                       []byte
	tls                         *carrierTLS
	flights                     [2]eapTLSFlight
	closed                      bool
}

func parseEAPTLSCarrier(parent *ProtocolEvent, w []byte, max int) (*eapTLSMessage, error) {
	if len(w) < 20 || int(binary.BigEndian.Uint16(w[2:4])) < 20 || int(binary.BigEndian.Uint16(w[2:4])) > len(w) || (w[0] != 1 && w[0] != 2 && w[0] != 3 && w[0] != 11) {
		return nil, fmt.Errorf("RADIUS envelope")
	}
	w = w[:binary.BigEndian.Uint16(w[2:4])]
	m := &eapTLSMessage{parent: carrierParent(parent), radiusCode: w[0], radiusID: w[1], digest: sha256.Sum256(w)}
	var eap []byte
	ma, states, attrs := 0, 0, 0
	endedEAP := false
	for at := 20; at < len(w); {
		if len(w)-at < 2 || w[at+1] < 2 || int(w[at+1]) > len(w)-at {
			return nil, fmt.Errorf("RADIUS attribute boundary")
		}
		typ, n := w[at], int(w[at+1])
		v := w[at+2 : at+n]
		at += n
		attrs++
		if attrs > max {
			return nil, fmt.Errorf("RADIUS attribute budget")
		}
		switch typ {
		case 79:
			if endedEAP {
				return nil, fmt.Errorf("nonconsecutive EAP-Message attributes")
			}
			eap = append(eap, v...)
		case 24:
			states++
			m.state = bytes.Clone(v)
		case 80:
			ma++
			if len(v) != 16 {
				return nil, fmt.Errorf("Message-Authenticator length")
			}
		}
		if typ != 79 && len(eap) > 0 {
			endedEAP = true
		}
	}
	if ma != 1 || states > 1 || len(eap) < 4 || int(binary.BigEndian.Uint16(eap[2:4])) != len(eap) {
		return nil, fmt.Errorf("EAP envelope/Message-Authenticator")
	}
	m.code, m.id = eap[0], eap[1]
	if (m.radiusCode == 1 && m.code != 2) || (m.radiusCode == 11 && m.code != 1) || (m.radiusCode == 2 && m.code != 3) || (m.radiusCode == 3 && m.code != 4) {
		return nil, fmt.Errorf("RADIUS/EAP code mismatch")
	}
	if m.code == 3 || m.code == 4 {
		if len(eap) != 4 {
			return nil, fmt.Errorf("EAP terminal length")
		}
		return m, nil
	}
	if len(eap) < 5 {
		return nil, fmt.Errorf("EAP method absent")
	}
	m.method = eap[4]
	if m.method != 13 && m.method != 21 && m.method != 25 {
		return m, nil
	}
	if len(eap) < 6 {
		return nil, fmt.Errorf("EAP TLS flags absent")
	}
	m.flags = eap[5]
	// PEAP and TTLS version zero have the established TLS tunnel grammar.
	if m.flags&0x1f != 0 {
		return nil, fmt.Errorf("unselected EAP TLS reserved bits/version")
	}
	m.body = eap[6:]
	if m.flags&0x80 != 0 {
		if len(m.body) < 4 {
			return nil, fmt.Errorf("EAP TLS declared length absent")
		}
		m.declared = binary.BigEndian.Uint32(m.body)
		m.body = m.body[4:]
	}
	if m.flags&0x20 != 0 && (m.code != 1 || m.flags != 0x20 || len(m.body) != 0) {
		return nil, fmt.Errorf("EAP TLS Start profile")
	}
	if m.flags&0x80 != 0 && uint64(m.declared) < uint64(len(m.body)) {
		return nil, fmt.Errorf("EAP TLS declared length underrun")
	}
	if m.flags&0x80 != 0 && m.declared == 0 || m.flags&0x40 != 0 && len(m.body) == 0 {
		return nil, fmt.Errorf("empty EAP TLS declared/continued flight")
	}
	m.body = bytes.Clone(m.body)
	return m, nil
}

func (s *eapTLSStore) storage() int64 {
	n := int64(512)
	for _, c := range s.entries {
		n += 1024 + int64(len(c.seen)*64)
		for _, m := range c.pending {
			n += 512 + int64(cap(m.body)+cap(m.state)+len(m.parent.SourceBytes.PacketRefs)*64)
		}
		for _, r := range c.requests {
			n += 512 + int64(cap(r.message.body)+cap(r.message.state)+len(r.message.parent.SourceBytes.PacketRefs)*64)
		}
		for _, e := range c.epochs {
			n += 4096 + int64(cap(e.state))
			for _, f := range e.flights {
				n += int64(cap(f.pieces) * 32)
				for _, p := range f.pieces {
					n += 256 + int64(cap(p.data)+len(p.parent.SourceBytes.PacketRefs)*64)
				}
			}
		}
	}
	return n
}
func (s *eapTLSStore) reserve(a *binParser, extra int64) bool {
	n := s.storage() + extra
	if n > s.charge {
		if !a.reserveEvidence(n - s.charge) {
			return false
		}
	} else {
		a.buffered.Add(n - s.charge)
	}
	s.charge = n
	return true
}

func (a *binParser) decodeEAPTLSCarrier(parent *ProtocolEvent, w []byte) (events []*ProtocolEvent) {
	a.carrierMu.Lock()
	defer a.carrierMu.Unlock()
	// Reserve before cloning untrusted EAP collections.
	if !a.reserveEvidence(int64(len(w))*3 + 2048) {
		return nil
	}
	defer a.buffered.Add(-int64(len(w))*3 - 2048)
	m, err := parseEAPTLSCarrier(parent, w, a.budget.MaxCollectionElements)
	if err != nil {
		return nil
	}
	// Type 13 already has a stricter, independent native RADIUS chain. Do not
	// duplicate its PDUs or change its association/resource-limit contract.
	if m.method != 21 && m.method != 25 && m.code != 3 && m.code != 4 {
		return nil
	}
	key := rdpUDPKey(parent)
	key.a = "eap/" + key.a
	if (m.code == 3 || m.code == 4) && (a.eapTLS == nil || a.eapTLS.entries[key] == nil) {
		return nil
	}
	if a.eapTLS == nil {
		a.eapTLS = &eapTLSStore{entries: make(map[binUDPKey]*eapTLSConversation)}
	}
	s := a.eapTLS
	c := s.entries[key]
	if c == nil {
		if len(s.entries) >= min(256, a.budget.MaxCollectionElements) || !s.reserve(a, 8192) {
			return nil
		}
		c = &eapTLSConversation{requests: make(map[byte]*eapTLSRequest), seen: make(map[[32]byte]bool)}
		s.entries[key] = c
	}
	if c.blocked || c.seen[m.digest] {
		return nil
	}
	for _, p := range c.pending {
		if p.digest == m.digest {
			return nil
		}
	}
	if len(c.pending) >= min(256, a.budget.MaxCollectionElements) || len(c.seen) >= min(4096, a.budget.MaxCollectionElements) || !s.reserve(a, int64(len(w))*3+4096) {
		return c.stop("EAP carrier retention budget")
	}
	c.pending = append(c.pending, m)
	for progress := true; progress; {
		progress = false
		for i := 0; i < len(c.pending); {
			p := c.pending[i]
			done, es := c.advance(a, p)
			events = append(events, es...)
			if done {
				if c.blocked {
					break
				}
				c.seen[p.digest] = true
				c.pending = append(c.pending[:i], c.pending[i+1:]...)
				progress = true
			} else {
				i++
			}
			if c.blocked {
				break
			}
		}
		if c.blocked {
			break
		}
	}
	if !s.reserve(a, 0) {
		events = append(events, c.stop("EAP carrier retained state budget")...)
		s.reserve(a, 0)
	}
	return events
}

func (c *eapTLSConversation) stop(reason string) (events []*ProtocolEvent) {
	c.blocked = true
	for _, e := range c.epochs {
		for dir, f := range e.flights {
			if len(f.pieces) > 0 {
				first := f.pieces[0].parent
				v := *first
				cloneEvidence(&v)
				v.ID = 0
				v.FlowID = e.id
				v.Protocol, v.Status, v.Error = "tls", "incomplete", reason+": EAP TLS fragment flight incomplete"
				v.Direction, v.CarrierLayers = dir, append([]string(nil), e.tls.layers...)
				v.SourceBytes.Kind = "reassembled"
				v.SourceBytes.PacketRefs = nil
				v.SourceBytes.ParentPDUs = nil
				for _, p := range f.pieces {
					v.Raw = append(v.Raw, p.data...)
					v.SourceBytes.PacketRefs = append(v.SourceBytes.PacketRefs, p.parent.SourceBytes.PacketRefs...)
					v.SourceBytes.ParentPDUs = append(v.SourceBytes.ParentPDUs, p.parent.ID)
				}
				v.SourceBytes.ParentPDU = first.ID
				v.Length = len(v.Raw)
				v.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: v.Error}
				v.Profile, v.Completeness = "tls-observed-carrier", "incomplete"
				events = append(events, &v)
				e.tls.f.a.incomplete.Add(1)
			}
		}
		events = append(events, e.tls.close(reason)...)
		e.flights = [2]eapTLSFlight{}
		e.closed = true
	}
	c.pending = nil
	c.requests = nil
	c.seen = nil
	return events
}

func (c *eapTLSConversation) advance(a *binParser, m *eapTLSMessage) (bool, []*ProtocolEvent) {
	if m.radiusCode == 11 && m.flags&0x20 != 0 {
		if len(m.state) == 0 || len(c.epochs) >= min(128, a.budget.MaxCollectionElements) {
			return true, c.stop("EAP Start generation budget/context")
		}
		for _, prior := range c.epochs {
			if !prior.closed && prior.client == m.parent.Destination && prior.server == m.parent.Source && prior.method == m.method && prior.expectedID == m.id && bytes.Equal(prior.state, m.state) {
				return true, c.stop("EAP competing Start reuses a live State/Identifier")
			}
		}
		name := map[byte]string{21: "EAP-TTLS", 25: "PEAP"}[m.method]
		e := &eapTLSEpoch{id: a.flows.Add(1), client: m.parent.Destination, server: m.parent.Source, method: m.method, state: bytes.Clone(m.state), expectedID: m.id}
		e.tls = newCarrierTLS(a, e.id, [2]string{e.client, e.server}, m.parent.Domain, "RADIUS", name)
		c.epochs = append(c.epochs, e)
		// The Start plus matching State/Identifier echo establishes a wire
		// generation. It does not infer an unseen initial RADIUS transaction.
		return true, nil
	}
	if m.radiusCode == 1 {
		if prior := c.requests[m.radiusID]; prior != nil && !prior.answered {
			if prior.message.digest == m.digest {
				return true, nil
			}
			return false, nil
		}
		var epoch *eapTLSEpoch
		if m.method == 13 || m.method == 21 || m.method == 25 {
			for _, e := range c.epochs {
				if !e.closed && e.client == m.parent.Source && e.server == m.parent.Destination && e.method == m.method && e.expectedID == m.id && bytes.Equal(e.state, m.state) {
					if epoch != nil {
						return true, c.stop("EAP State/Identifier has multiple observed owners")
					}
					epoch = e
				}
			}
			if epoch == nil {
				return false, nil
			}
		} else if m.method != 1 && m.method != 3 {
			return true, nil
		}
		c.requests[m.radiusID] = &eapTLSRequest{message: m, epoch: epoch}
		if epoch != nil {
			return true, epoch.accept(a, 0, m)
		}
		return true, nil
	}
	r := c.requests[m.radiusID]
	if r == nil || r.answered || r.message.parent.Source != m.parent.Destination || r.message.parent.Destination != m.parent.Source {
		return false, nil
	}
	if m.code == 3 || m.code == 4 {
		if r.epoch == nil || m.id != r.message.id {
			return false, nil
		}
		r.answered = true
		r.epoch.closed = true
		return true, r.epoch.tls.close("EAP observed terminal result")
	}
	if m.method != 13 && m.method != 21 && m.method != 25 {
		r.answered = true
		return true, nil
	}
	if m.id == r.message.id || len(m.state) == 0 {
		return false, nil
	}
	e := r.epoch
	if e == nil || e.closed || e.method != m.method {
		return false, nil
	}
	r.answered = true
	e.state = bytes.Clone(m.state)
	e.expectedID = m.id
	return true, e.accept(a, 1, m)
}

func (e *eapTLSEpoch) accept(a *binParser, dir int, m *eapTLSMessage) (events []*ProtocolEvent) {
	if e.closed || m.flags&0x20 != 0 {
		return nil
	}
	f := &e.flights[dir]
	if uint64(m.declared) > uint64(a.budget.MaxMessageBytes) {
		e.closed = true
		return []*ProtocolEvent{e.tls.fail(dir, nil, protocolError(ErrResourceExceeded, "EAP TLS declared flight budget"))}
	}
	if len(m.body) == 0 && m.flags == 0 {
		return nil
	} // stop-and-wait acknowledgement, never TLS bytes
	bad := func(reason string) []*ProtocolEvent {
		e.closed = true
		return []*ProtocolEvent{e.tls.fail(dir, nil, protocolError(ErrMalformedMessage, "%s", reason))}
	}
	if m.flags&0x80 != 0 {
		if len(f.pieces) > 0 && f.declared != m.declared {
			return bad("EAP TLS repeated fragment length differs")
		}
		f.declared = m.declared
	}
	if m.flags&0x40 != 0 && len(f.pieces) == 0 && m.flags&0x80 == 0 {
		return bad("EAP TLS first fragment lacks total length")
	}
	if f.size+len(m.body) > a.budget.MaxMessageBytes || len(f.pieces) >= a.budget.MaxCollectionElements {
		e.closed = true
		return []*ProtocolEvent{e.tls.fail(dir, nil, protocolError(ErrResourceExceeded, "EAP TLS fragment flight budget"))}
	}
	f.pieces = append(f.pieces, rdpUDPChunk{data: m.body, parent: m.parent})
	f.size += len(m.body)
	if f.declared > 0 && uint64(f.size) > uint64(f.declared) {
		return bad("EAP TLS fragments exceed declared flight")
	}
	if m.flags&0x40 != 0 {
		return nil
	}
	if f.declared > 0 && uint64(f.size) != uint64(f.declared) {
		return bad("EAP TLS final fragment length mismatch")
	}
	// The complete flight is validated before publishing even its first record.
	if !a.reserveEvidence(int64(f.size)) {
		e.closed = true
		return []*ProtocolEvent{e.tls.fail(dir, nil, protocolError(ErrResourceExceeded, "EAP TLS boundary validation budget"))}
	}
	all := make([]byte, 0, f.size)
	for _, p := range f.pieces {
		all = append(all, p.data...)
	}
	valid := true
	for at := 0; at < len(all); {
		if len(all)-at < 5 {
			valid = false
			break
		}
		n, _, err := e.tls.f.frame(all[at:])
		if err != nil || n > len(all)-at {
			valid = false
			break
		}
		at += n
	}
	a.buffered.Add(-int64(f.size))
	if !valid {
		return bad("EAP TLS completed flight has invalid record boundaries")
	}
	for _, p := range f.pieces {
		events = append(events, e.tls.feed(dir, p.data, p.parent)...)
	}
	*f = eapTLSFlight{}
	return events
}

func (a *binParser) closeCarrierSessions() (events []*ProtocolEvent) {
	a.carrierMu.Lock()
	defer a.carrierMu.Unlock()
	if s := a.rdpUDP; s != nil {
		for _, c := range s.entries {
			if c.active != nil {
				diagnostics := c.active.pendingDiagnostics(a, "capture closed")
				events = append(events, diagnostics...)
				a.incomplete.Add(uint64(len(diagnostics)))
				events = append(events, c.active.tls.close("capture closed")...)
			}
		}
		a.buffered.Add(-s.charge)
		a.rdpUDP = nil
	}
	if s := a.eapTLS; s != nil {
		for _, c := range s.entries {
			events = append(events, c.stop("capture closed")...)
		}
		a.buffered.Add(-s.charge)
		a.eapTLS = nil
	}
	return events
}
