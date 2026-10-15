package pcaputil

import (
	"container/list"
	"encoding/binary"
	"errors"
)

func (f *binFlow) consumeRADIUS(dir int, e *ProtocolEvent, wire []byte, legacy ...map[string]any) error {
	e.nativeRADIUSObservation = true
	e.Profile, e.Completeness = radiusProfile(wire[0]), "message"
	s := f.radius
	if s == nil {
		return protocolError(ErrContextRequired, "RADIUS observation state missing")
	}
	// Reserve before parsing/copying option graphs; retained maps have a fixed
	// cost per identity, regardless of caller mutations and response lifetime.
	err := f.reserveSession(s.storage() + 192 + int64(len(wire))*4)
	if err != nil {
		s.retire(dir, wire, false)
		return err
	}
	fields, err := radiusFields(wire, f.a.budget.MaxCollectionElements)
	if err != nil {
		s.retire(dir, wire, len(s.pending) < f.a.budget.MaxCollectionElements)
		var typed *ProtocolError
		if !errors.As(err, &typed) {
			err = protocolError(ErrMalformedMessage, "%s", err)
		}
		return err
	}
	key := radiusIdentity(dir, wire)
	if _, known := s.pending[key]; !known && len(s.pending) >= f.a.budget.MaxCollectionElements {
		s.untracked = true
		return protocolError(ErrResourceExceeded, "RADIUS identity budget")
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	e.ResponseTo, e.TransactionID = s.observe(dir, e.Timestamp, wire, e.ID, fields, f.a.budget.MaxCollectionElements)
	e.Session = fields
	e.semanticFields = cloneSession(fields)
	e.semanticSession = cloneSession(fields)
	if len(legacy) > 0 && legacy[0] != nil {
		e.retainRuleEnvelope(legacy[0])
	}
	retained := s.storage()
	if f.sessionBytes > retained {
		f.a.buffered.Add(retained - f.sessionBytes)
		f.sessionBytes = retained
	}
	return nil
}
func radiusUDPKey(e *ProtocolEvent) binUDPKey {
	key := binUDPKey{a: "radius/" + e.Source, b: "radius/" + e.Destination, domain: e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	return key
}
func radiusUDPHeader(w []byte) bool {
	return len(w) >= 4 && (w[0] == 1 || w[0] == 2 || w[0] == 3 || w[0] == 4 || w[0] == 5 || w[0] == 11)
}
func (a *binParser) decodeRADIUSDatagram(e *ProtocolEvent, w []byte) bool {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	key := radiusUDPKey(e)
	store := a.udpSessions
	var el *list.Element
	if store != nil {
		el = store.entries[key]
	}
	admitted := radiusDatagramEvidence(w, e.Source, e.Destination)
	if !admitted && !(el != nil && radiusUDPHeader(w)) {
		return false
	}
	if store == nil {
		store = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
		a.udpSessions = store
	}
	var err error
	if el == nil {
		el, err = a.newRADIUSUDPFlow(store, key, e)
	}
	e.Protocol, e.Profile, e.nativeRADIUSObservation, e.Completeness = "radius", radiusProfile(w[0]), true, "message"
	e.Admission = "wire-signature"
	if udpEndpointPort(e.Source, 1812) || udpEndpointPort(e.Destination, 1812) || udpEndpointPort(e.Source, 1813) || udpEndpointPort(e.Destination, 1813) {
		e.Admission = "wire-and-port-hint"
	}
	if el == nil {
		a.radiusUntrackedRefusal = true
	} else {
		v := el.Value.(*binUDPEntry)
		f := v.flow
		dir := 0
		if e.Source != f.endpoints[0] {
			dir = 1
		}
		e.FlowID, e.Direction = f.id, dir
		err = f.consumeRADIUS(dir, e, w)
		if e.Timestamp.After(v.touched) {
			v.touched = e.Timestamp
		}
		store.lru.MoveToBack(el)
		if e.Session != nil {
			e.Session["Observation Scope"] = "conversation"
			e.semanticFields = cloneSession(e.Session)
			e.semanticSession = cloneSession(e.Session)
		}
	}
	if err != nil {
		e.ResponseTo, e.TransactionID = 0, 0
	}
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}

// Byte-budget denial precedes decoding. Read only the fixed header and scope
// invalidation to its exact carrier/domain; denied first histories cannot later
// appear as pristine observed conversations.
func (a *binParser) refuseRADIUSOversize(e *ProtocolEvent, w []byte) bool {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	key := radiusUDPKey(e)
	var f *binFlow
	if a.udpSessions != nil {
		if el := a.udpSessions.entries[key]; el != nil {
			f = el.Value.(*binUDPEntry).flow
		}
	}
	hinted := udpEndpointPort(e.Source, 1812) || udpEndpointPort(e.Destination, 1812) || udpEndpointPort(e.Source, 1813) || udpEndpointPort(e.Destination, 1813)
	if !radiusUDPHeader(w) || f == nil && !hinted {
		return false
	}
	if f == nil {
		// A message-byte refusal does not itself exhaust the bounded session budget.
		// Retain only its known fixed-header identity when that separate budget permits.
		if a.udpSessions == nil {
			a.udpSessions = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
		}
		if el, err := a.newRADIUSUDPFlow(a.udpSessions, key, e); err == nil {
			f = el.Value.(*binUDPEntry).flow
		}
	}
	if f != nil && f.radius != nil {
		dir := 0
		if e.Source != f.endpoints[0] {
			dir = 1
		}
		canAdd := false
		if _, known := f.radius.pending[radiusIdentity(dir, w)]; !known && len(f.radius.pending) < a.budget.MaxCollectionElements {
			canAdd = f.reserveSession(f.radius.storage()+192) == nil
		}
		f.radius.retire(dir, w, canAdd)
		e.FlowID = f.id
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
	} else {
		a.radiusUntrackedRefusal = true
	}
	e.Protocol, e.Profile, e.nativeRADIUSObservation = "radius", radiusProfile(w[0]), true
	preview := w[:min(len(w), a.config.MaxMessageBytes)]
	a.finishProtocolDatagram(e, preview, nil, protocolError(ErrResourceExceeded, "RADIUS UDP datagram exceeds message byte budget"))
	e.ExpertCode = string(ErrResourceExceeded)
	a.messageBytes.Add(uint64(len(w) - len(preview)))
	a.limited.Add(uint64(len(w) - len(preview)))
	return true
}
func radiusAdmittedDatagram(w []byte, src, dst string) bool {
	if !radiusUDPHeader(w) {
		return false
	}
	hinted := udpEndpointPort(src, 1812) || udpEndpointPort(dst, 1812) || udpEndpointPort(src, 1813) || udpEndpointPort(dst, 1813)
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if hinted {
		return true
	}
	if n < 20 || n > 4096 || n > len(w) {
		return false
	}
	// UDP padding follows the declared RADIUS packet, never an extra attribute.
	for at := 20; at < n; {
		if n-at < 2 || w[at+1] < 2 || int(w[at+1]) > n-at {
			return false
		}
		at += int(w[at+1])
	}
	return true
}

// Caller holds udpMu. Conversation count and retained owner bytes are charged
// separately from message bytes, including first-packet fixed-header refusals.
func (a *binParser) newRADIUSUDPFlow(store *binUDPStore, key binUDPKey, e *ProtocolEvent) (*list.Element, error) {
	if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
		return nil, protocolError(ErrResourceExceeded, "RADIUS UDP conversation budget")
	}
	first, second := e.Source, e.Destination
	if first > second {
		first, second = second, first
	}
	f := &binFlow{a: a, id: a.flows.Add(1), protocol: "radius", radius: &binRADIUS{untracked: a.radiusUntrackedRefusal}, endpoints: [2]string{first, second}}
	if err := f.reserveSession(f.radius.storage()); err != nil {
		return nil, err
	}
	el := store.lru.PushBack(&binUDPEntry{key: key, flow: f, touched: e.Timestamp})
	store.entries[key] = el
	return el, nil
}
