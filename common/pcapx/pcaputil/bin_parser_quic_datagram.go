package pcaputil

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/hex"
	"time"
)

// Native QUIC uses capture-domain/CID evidence, never a port or a payload guess.
func (a *binParser) decodeQUICDatagram(base *ProtocolEvent, wire []byte) ([]*ProtocolEvent, bool) {
	if len(wire) < 1 || wire[0]&0x40 == 0 {
		return nil, false
	}
	long := wire[0]&0x80 != 0
	var first quicHdr
	if long {
		if len(wire) < 7 || binary.BigEndian.Uint32(wire[1:5]) != 1 {
			return nil, false
		}
		var err error
		first, err = quicParseHeader(wire, true)
		if err != nil {
			return nil, false
		}
	}
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	s := a.udpSessions
	if s == nil {
		if !long || first.typeName != "Initial" {
			return nil, false
		}
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
	var el *list.Element
	dir := 0
	for _, candidate := range s.entries {
		v := candidate.Value.(*binUDPEntry)
		q := v.flow.quic
		if q == nil || v.key.domain != base.Domain {
			continue
		}
		// Zero-length CIDs cannot migrate: retain the observed endpoint tuple.
		for d := 0; d < 2; d++ {
			if q.zeroCID[1-d] && (!long || len(first.dcid) == 0) && base.Source == v.flow.endpoints[d] && base.Destination == v.flow.endpoints[1-d] {
				if el != nil && el != candidate {
					return nil, false
				}
				el = candidate
				dir = d
			}
		}
		for cid, owner := range q.cids {
			match := long && string(first.dcid) == cid || !long && len(wire) >= 1+len(cid) && string(wire[1:1+len(cid)]) == cid
			if match {
				if el != nil && el != candidate {
					return nil, false
				}
				el = candidate
				dir = 1 - owner
			}
		}
	}
	var f *binFlow
	key := binUDPKey{}
	if el != nil {
		f = el.Value.(*binUDPEntry).flow
		key = el.Value.(*binUDPEntry).key
	} else {
		if !long || first.typeName != "Initial" {
			return nil, false
		}
		key = binUDPKey{"quic/" + base.Source + "/" + hex.EncodeToString(first.dcid), base.Destination, base.Domain}
		f = &binFlow{a: a, protocol: "quic", quic: &binQUIC{native: true, byteLimit: a.budget.MaxMessageBytes, provider: a.tlsSecrets}, endpoints: [2]string{base.Source, base.Destination}}
	}
	var events []*ProtocolEvent
	for off := 0; off < len(wire); {
		if off > 0 && bytes.Count(wire[off:], []byte{0}) == len(wire)-off {
			events[len(events)-1].Session["Unauthenticated Datagram Padding Bytes"] = len(wire) - off
			break
		}
		e := *base
		e.Protocol = "quic"
		e.Profile = "quic-v1-native"
		e.Admission = "authenticated-wire-and-observed-CID"
		e.ID = a.ids.Add(1)
		f.quic.eventID = e.ID
		e.Direction = dir
		h, err := f.quic.wireHeader(wire[off:], dir)
		size := len(wire) - off
		if err == nil && (h.size <= 0 || h.size > size) {
			err = protocolError(ErrMalformedMessage, "invalid coalesced QUIC length")
		}
		if err == nil {
			size = h.size
		}
		raw := wire[off : off+size]
		e.Length = size
		if err == nil && (len(events) >= a.budget.MaxCollectionElements || el == nil && len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements)) {
			err = protocolError(ErrResourceExceeded, "QUIC packet/conversation budget")
		}
		if err == nil {
			err = f.reserveSession(f.quic.initialMemory() + int64(len(raw))*32)
		}
		if err == nil {
			e.Session, err = f.quic.consume(dir, raw, a.budget.MaxCollectionElements)
		}
		if err == nil {
			err = f.reserveSession(f.quic.initialMemory())
		}
		if el == nil && err == nil && e.Session["Authentication Verified"] == true {
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
			e.Session["Observation Scope"] = "UDP capture domain and observed CID"
			e.Session["Datagram Offset"] = off
			e.Session["Native Carrier"] = true
			e.Session["ALPN"] = f.quic.alpn
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
				if dir == 1 {
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
		// The authenticated session already parsed this header. Preserve the wire
		// field view without executing a YAML operator for every ciphertext octet.
		e.nativeQUICWire = err == nil
		a.finishProtocolDatagram(&e, raw, a.specs["application-layer.quic/QUIC"], err)
		events = append(events, &e)
		if e.sessionError != nil && e.sessionError.Kind == ErrResourceExceeded && el != nil {
			f.closeSession()
			delete(s.entries, key)
			s.lru.Remove(el)
			el = nil
		}
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

// quicWireEnvelope preserves the existing QUIC YAML field shape and scalar types.
// It is only selected after native wire authentication, never as a plaintext probe.
func quicWireEnvelope(w []byte) (map[string]any, error) {
	h, err := quicParseHeader(w, true)
	if err != nil {
		return nil, err
	}
	if h.size != len(w) {
		return nil, protocolError(ErrMalformedMessage, "QUIC envelope length mismatch")
	}
	fields := map[string]any{"First Byte": w[0]}
	off := 1
	if h.long {
		fields["Version"] = h.version
		fields["DCID Length"], fields["SCID Length"] = uint8(len(h.dcid)), uint8(len(h.scid))
		if len(h.dcid) > 0 {
			fields["DCID"] = string(h.dcid)
		}
		if len(h.scid) > 0 {
			fields["SCID"] = string(h.scid)
		}
		off = 7 + len(h.dcid) + len(h.scid)
		// The legacy envelope exposes QUICToken only for its one-byte length
		// profile. Full varint token semantics remain in the authenticated session.
		if h.typeName == "Initial" && off < len(w) && w[off] < 64 {
			token := map[string]any{"Token Length": w[off]}
			off++
			if len(h.token) > 0 {
				token["Token"] = string(h.token)
				off += len(h.token)
			}
			fields["QUICToken"] = token
		}
	}
	if off < len(w) {
		payload := make([]any, len(w)-off)
		for i, b := range w[off:] {
			payload[i] = b
		}
		fields["Protected Payload"] = payload
	}
	return fields, nil
}
