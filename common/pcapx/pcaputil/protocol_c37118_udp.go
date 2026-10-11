package pcaputil

import (
	"container/list"
	"encoding/binary"
	"time"
)

const c37118IdleTTL = 30 * time.Second

// Configuration context is bounded and scoped by transport pair, publisher
// direction and capture domain. UDP payloads are never concatenated.
func (a *binParser) decodeC37118Datagram(e *ProtocolEvent, w []byte, explicit bool) bool {
	if !explicit && !c37118DatagramEvidence(w) {
		return false
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "c37118", "c37118-v1-v2-cfg2-data", "wire-signature", "message"
	if explicit {
		e.Admission = "explicit-decode-as"
	}
	key := binUDPKey{"c37118/" + e.Source, "c37118/" + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	store := a.udpSessions
	if store == nil {
		store = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		a.udpSessions = store
	}
	if e.Timestamp.After(store.clock) {
		store.clock = e.Timestamp
	}
	// The 30-second capture idle policy belongs to this conversation. A
	// different interface, tunnel, or publisher may have an unrelated clock.
	// Lazy expiry keeps their configurations intact while collection and byte
	// budgets still bound retained state. Backdated packets never extend TTL.
	el := store.entries[key]
	if el != nil {
		v := el.Value.(*binUDPEntry)
		if e.Timestamp.Sub(v.touched) >= c37118IdleTTL {
			v.flow.closeSession()
			delete(store.entries, v.key)
			store.lru.Remove(el)
			el = nil
		}
	}
	var f *binFlow
	if el != nil {
		v := el.Value.(*binUDPEntry)
		f = v.flow
		if e.Timestamp.After(v.touched) {
			v.touched = e.Timestamp
		}
		store.lru.MoveToBack(el)
		e.FlowID = f.id
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
	} else {
		f = &binFlow{a: a, protocol: "c37118", c37118: &binC37118{}, endpoints: [2]string{e.Source, e.Destination}}
	}
	e.Profile = f.c37118Profile(e.Direction, w)
	var err error
	oversized := len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	if oversized {
		err = protocolError(ErrResourceExceeded, "C37.118 UDP frame exceeds byte budget")
		if len(w) > 1 && (w[1]>>4 == 3 || w[1]>>4 == 5) {
			f.c37118.configuration[e.Direction] = nil
		}
	} else {
		e.Session, err = f.consumeC37118(e.Direction, w)
		if err == nil && el == nil && len(w) > 1 && (w[1]>>4 == 3 || w[1]>>4 == 5) {
			if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
				err = protocolError(ErrResourceExceeded, "C37.118 UDP configuration conversation budget")
				e.Session = nil
			} else {
				f.id = a.flows.Add(1)
				e.FlowID = f.id
				el = store.lru.PushBack(&binUDPEntry{key, f, e.Timestamp})
				store.entries[key] = el
			}
		}
	}
	if el == nil {
		f.closeSession()
	}
	a.udpMu.Unlock()
	if err == nil {
		e.semanticFields = cloneSession(e.Session)
	}
	if oversized {
		w = nil
	}
	a.finishProtocolDatagram(e, w, nil, err)
	if err != nil {
		e.Completeness = e.Status
	}
	return true
}

func c37118DatagramEvidence(w []byte) bool {
	return len(w) >= 16 && probeC37118(w, len(w)).Verdict == ProbeAccept && (w[1]>>4 < 5 || w[1] == 0x52 && len(w) >= 24 && binary.BigEndian.Uint16(w[14:16]) == 0) && int(binary.BigEndian.Uint16(w[2:4])) == len(w) && c37118CRC(w[:len(w)-2]) == binary.BigEndian.Uint16(w[len(w)-2:])
}
