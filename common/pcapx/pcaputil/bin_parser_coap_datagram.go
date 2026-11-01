package pcaputil

import (
	"container/list"
	"time"
)

// CoAP has only a four-byte header and no magic. Random protected QUIC bytes
// can satisfy its type/code/token and option lengths. Automatic UDP discovery
// therefore starts with an observed request; orphan replies/ACKs cannot create
// an application identity. The explicit ProtocolSession API still supports
// inspecting a caller-selected CoAP response without a captured request.
func (a *binParser) decodeCoAPDatagram(e *ProtocolEvent, wire []byte) bool {
	if probeCoAP(wire, len(wire)).Verdict != ProbeAccept {
		return false
	}
	request := wire[1] > 0 && wire[1] < 32 && coapRequestDiscoveryEvidence(wire)
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	s := a.udpSessions
	key := binUDPKey{"coap/" + e.Source, "coap/" + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	if s == nil {
		if !request {
			return false
		}
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
		a.udpSessions = s
	}
	for el := s.lru.Front(); el != nil; {
		next := el.Next()
		entry := el.Value.(*binUDPEntry)
		// This store also owns QUIC, DNS, DTLS and other protocols' retained
		// identities. CoAP expiry cannot erase their keys or refusal history.
		if entry.flow.coap != nil && entry.key.domain == e.Domain && !e.Timestamp.IsZero() && !entry.touched.IsZero() && e.Timestamp.Sub(entry.touched) >= 10*time.Minute {
			entry.flow.closeSession()
			delete(s.entries, entry.key)
			s.lru.Remove(el)
		}
		el = next
	}
	el := s.entries[key]
	if el == nil && !request {
		return false
	}
	var err error
	if el == nil {
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		} else {
			f := &binFlow{a: a, id: a.flows.Add(1), protocol: "coap", coap: &binCoAP{maxPending: a.budget.MaxCollectionElements}, endpoints: [2]string{e.Source, e.Destination}}
			el = s.lru.PushBack(&binUDPEntry{key, f, e.Timestamp})
			s.entries[key] = el
		}
	}
	if el != nil {
		v := el.Value.(*binUDPEntry)
		if e.Timestamp.After(v.touched) {
			v.touched = e.Timestamp
		}
		s.lru.MoveToBack(el)
		f := v.flow
		e.FlowID = f.id
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
		if err = f.reserveSession(512 + int64(len(f.coap.pending)+1)*256); err == nil {
			e.Session, err = f.coap.consume(e.Direction, wire)
		}
		if e.Session != nil {
			e.Session["Observation Scope"] = "UDP endpoint tuple and capture domain"
		}
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "coap", "rfc7252", "observed-request-conversation", "message"
	a.finishProtocolDatagram(e, wire, a.specs["application-layer.extended_protocols/CoAP"], err)
	return true
}

// A code plus token is still weak evidence in arbitrary UDP ciphertext. Require
// an observed URI option for a new automatic conversation. A valid root request
// without URI options remains available through explicit CoAP session decoding.
func coapRequestDiscoveryEvidence(w []byte) bool {
	option := 0
	evidence := false
	for at := 4 + int(w[0]&15); at < len(w); {
		if w[at] == 0xff {
			break
		}
		hdr := w[at]
		at++
		delta, next, ok := coapExt(w, at, int(hdr>>4))
		if !ok || delta < 0 {
			return false
		}
		length, next, ok := coapExt(w, next, int(hdr&15))
		if !ok || length < 0 || length > len(w)-next {
			return false
		}
		option += delta
		switch option {
		case 3, 7, 11, 15, 35, 39:
			evidence = true
		case 1, 4, 5, 8, 12, 14, 17, 20, 60:
		default:
			if option&1 != 0 {
				return false
			} // unknown critical option
		}
		at = next + length
	}
	return evidence
}
