package pcaputil

import (
	"container/list"
	"encoding/binary"
	"time"
)

// A bounded UDP entrypoint for QUIC v1 long headers. Only authenticated packet
// payloads can reach the existing frame engine. This does not claim CID-based
// migration, multi-connection keylog selection or complete HTTP/3 wire support.
func (a *binParser) decodeQUICDatagram(base *ProtocolEvent, wire []byte) ([]*ProtocolEvent, bool) {
	if len(wire) < 7 || wire[0]&0xc0 != 0xc0 || binary.BigEndian.Uint32(wire[1:5]) != 1 {
		return nil, false
	}
	if probeQUIC(wire, len(wire)).Verdict != ProbeAccept {
		return nil, false
	}
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	key := binUDPKey{base.Source, base.Destination, base.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	key.a = "quic/" + key.a
	s := a.udpSessions
	if s == nil {
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: base.Timestamp}
		a.udpSessions = s
	}
	if base.Timestamp.After(s.clock) {
		s.clock = base.Timestamp
	}
	for el := s.lru.Front(); el != nil; el = s.lru.Front() {
		v := el.Value.(*binUDPEntry)
		if s.clock.Sub(v.touched) < 10*time.Minute {
			break
		}
		v.flow.closeSession()
		delete(s.entries, v.key)
		s.lru.Remove(el)
	}
	el := s.entries[key]
	var f *binFlow
	if el != nil {
		f = el.Value.(*binUDPEntry).flow
	} else {
		f = &binFlow{a: a, protocol: "quic", quic: &binQUIC{}, endpoints: [2]string{base.Source, base.Destination}}
	}
	dir := 0
	if base.Source != f.endpoints[0] {
		dir = 1
	}
	var events []*ProtocolEvent
	for off := 0; off < len(wire); {
		e := *base
		e.Protocol = "quic"
		e.Direction = dir
		h, err := quicParseHeader(wire[off:], true)
		size := len(wire) - off
		if err == nil && (h.size <= 0 || h.size > size || h.version != 1 || !h.long) {
			err = protocolError(ErrMalformedMessage, "invalid coalesced QUIC long packet")
		}
		if err == nil {
			size = h.size
		}
		raw := wire[off : off+size]
		e.Length = size
		if err == nil && len(events) >= a.budget.MaxCollectionElements {
			err = protocolError(ErrResourceExceeded, "QUIC packets per datagram budget")
		}
		if err == nil && el == nil && len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		}
		if err == nil {
			err = f.reserveSession(f.quic.initialMemory() + int64(len(raw))*32)
		}
		if err == nil {
			e.Session, err = f.quic.consume(dir, raw, a.budget.MaxCollectionElements)
		}
		if err == nil && el == nil && e.Session["Authentication Verified"] == true {
			f.id = a.flows.Add(1)
			el = s.lru.PushBack(&binUDPEntry{key, f, s.clock})
			s.entries[key] = el
		}
		if el != nil {
			v := el.Value.(*binUDPEntry)
			v.touched = s.clock
			s.lru.MoveToBack(el)
			e.FlowID = f.id
		}
		if e.Session != nil {
			e.Session["Observation Scope"] = "UDP endpoint pair"
			e.Session["Datagram Offset"] = off
			e.Session["Native Carrier"] = true
		}
		a.finishProtocolDatagram(&e, raw, a.specs["application-layer.quic/QUIC"], err)
		events = append(events, &e)
		off += size
		if err != nil {
			break
		}
	}
	if el == nil {
		f.closeSession()
	}
	return events, true
}
