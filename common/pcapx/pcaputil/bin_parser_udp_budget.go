package pcaputil

import (
	"container/list"
	"net"
	"time"
)

// The shared UDP byte limit precedes semantic decoding. It must still retire
// association state for an admitted conversation: the denied on-wire request or
// reply did not disappear. Admission here examines only bounded headers/ports or
// an already observed exact endpoint/capture-domain key, never an oversized body.
func (a *binParser) refuseUDPOversizeAssociation(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	if a.refuseSIPOversizeAssociation(e, w) {
		return true
	}
	if a.refuseTFTPOversizeAssociation(e, w) {
		return true
	}
	explicit := a.datagramDecodeAs[dst]
	if explicit == "" {
		explicit = a.datagramDecodeAs[src]
	}
	prefix, protocol, profile := "", "", ""
	ttl := slmpIdleTTL
	switch {
	case explicit == "stun" || explicit == "turn" || explicit == "" && probeSTUN(w, len(w)).Verdict == ProbeAccept:
		protocol, profile = "stun", "stun-rfc8489-udp"
		ttl = 10 * time.Minute
	case explicit == "snmp" || explicit == "" && probeSNMP(w, len(w)).Verdict == ProbeAccept:
		prefix, protocol, profile = "snmp/", "snmp", "snmp-v"+probeSNMP(w, len(w)).Version+"-native"
		ttl = snmpPendingTTL
	case explicit == "slmp" || explicit == "" && slmpStart(w):
		prefix, protocol, profile = "slmp/", "slmp", "slmp-binary-self-test"
	case explicit == "dlms-wrapper" || explicit == "" && len(w) >= 8 && w[0] == 0 && w[1] == 1:
		prefix, protocol, profile = "wrapper/", "dlms-wrapper", wrapperProfile(w)
		ttl = wrapperIdleTTL
	case explicit == "dlms" || explicit == "" && len(w) > 0 && w[0] == 0x7e:
		prefix, protocol, profile = "dlms/", "dlms", dlmsProfile(w)
		ttl = dlmsIdleTTL
	default:
		return false
	}
	key := binUDPKey{prefix + e.Source, prefix + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	store := a.udpSessions
	// A denied SNMP datagram retires only its exact admitted key below.
	// Its timestamp cannot expire other endpoint/domain observations.
	if store != nil && protocol != "snmp" && protocol != "stun" {
		if e.Timestamp.After(store.clock) {
			store.clock = e.Timestamp
		}
		for el := store.lru.Front(); el != nil; {
			next := el.Next()
			v := el.Value.(*binUDPEntry)
			applicable := protocol == "stun" && v.flow.stun != nil || protocol == "snmp" && v.flow.snmp != nil || protocol == "slmp" && v.flow.slmp != nil || protocol == "dlms-wrapper" && v.flow.wrapper != nil || protocol == "dlms" && v.flow.dlms != nil
			if applicable && store.clock.Sub(v.touched) >= ttl {
				if v.flow.stun != nil {
					v.flow.retireSTUNUDP(store.clock)
				} else if v.flow.snmp != nil {
					v.flow.retireSNMPUDP()
				} else if v.flow.dlms != nil || v.flow.wrapper != nil {
					v.flow.retireDLMSUDP()
				} else if v.flow.slmp != nil {
					v.flow.retireSLMPUDP()
				} else {
					v.flow.closeSession()
					delete(store.entries, v.key)
					store.lru.Remove(el)
				}
			}
			el = next
		}
	}
	existing := store != nil && store.entries[key] != nil
	if explicit == "" && !existing && protocol != "stun" && !(protocol == "snmp" && (src == 161 || dst == 161 || src == 162 || dst == 162)) && !(protocol == "slmp" && slmpUDPRefusalHint(w)) && !(protocol == "dlms-wrapper" && (src == 4059 || dst == 4059)) && !(protocol == "dlms" && (src == 4059 || dst == 4059) && dlmsUDPRefusalHint(w)) {
		a.udpMu.Unlock()
		return false
	}
	if protocol == "slmp" && !existing {
		a.slmpUntrackedRefusal = true
	}
	if protocol == "dlms" && !existing {
		// Explicit admission or a bounded port/header hint identified HDLC,
		// but its identities cannot be stored within the rejected budget.
		a.dlmsUntrackedRefusal = true
	}
	if protocol == "dlms-wrapper" && !existing && (explicit != "" || wrapperUDPRefusalHint(w)) {
		a.wrapperUntrackedRefusal = true
	}
	if protocol == "snmp" && !existing {
		if store == nil {
			store = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
			a.udpSessions = store
		}
		// A denied first message is a real observation even without a parsed
		// request ID. Retain a charged exact-key quarantine, not a fresh owner.
		first, second := e.Source, e.Destination
		if first > second {
			first, second = second, first
		}
		f := &binFlow{a: a, id: a.flows.Add(1), endpoints: [2]string{first, second}, protocol: "snmp"}
		if len(store.entries) < sessionCollectionLimit(a.budget.MaxCollectionElements) && f.reserveSession(512) == nil {
			f.snmp = &binSNMP{blocked: true}
			el := store.lru.PushBack(&binUDPEntry{key, f, e.Timestamp})
			store.entries[key] = el
			existing = true
		} else {
			// Existing contexts remain usable. A future unseen key cannot claim
			// a completely observed history after an unretained refusal.
			a.snmpUntrackedRefusal = true
		}
	}
	if protocol == "stun" && !existing {
		if store == nil {
			store = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
			a.udpSessions = store
		}
		a.reclaimInactiveSTUNSlot(store)
		f := &binFlow{a: a, id: a.flows.Add(1), endpoints: [2]string{e.Source, e.Destination}, protocol: "stun", stun: &binSTUN{blocked: true}}
		if len(store.entries) < sessionCollectionLimit(a.budget.MaxCollectionElements) && f.reserveSession(f.stun.bytes()) == nil {
			el := store.lru.PushBack(&binUDPEntry{key, f, e.Timestamp})
			store.entries[key] = el
			existing = true
		} else {
			// The denied wire is real even if no endpoint history slot fits.
			// Existing verified keys are preserved; new keys cannot claim a
			// fully observed generation after an untracked refusal.
			a.stunUntrackedRefusal = true
		}
	}
	if existing {
		el := store.entries[key]
		v := el.Value.(*binUDPEntry)
		f := v.flow
		e.FlowID = f.id
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
		var retained int64
		if f.stun != nil {
			f.stun.invalidate()
			retained = f.stun.bytes()
		} else if f.snmp != nil {
			f.snmp.invalidate()
			retained = f.snmp.storage()
		} else if f.slmp != nil {
			if !f.slmp.retiredWire(w) {
				f.slmp.pending = nil
				f.slmp.ambiguous = true
			}
			retained = 512 + 64*int64(len(f.slmp.seen))
			if f.slmp.pending != nil {
				retained += int64(len(f.slmp.pending.wire)) * 3
			}
		} else if f.wrapper != nil {
			f.wrapper.invalidate()
			retained = f.wrapper.storage()
		} else if f.dlms != nil {
			f.dlms.invalidate()
			retained = f.dlms.storage()
		}
		// reserveSession is a high-water allocator. Removed wire/projection graphs
		// must release their ledger charge while bounded ambiguity markers remain.
		if retained < f.sessionBytes {
			a.buffered.Add(retained - f.sessionBytes)
			f.sessionBytes = retained
		}
		if protocol == "snmp" || protocol == "stun" {
			if e.Timestamp.After(v.touched) {
				v.touched = e.Timestamp
			}
		} else {
			v.touched = store.clock
		}
		store.lru.MoveToBack(el)
	}
	a.udpMu.Unlock()
	if protocol == "snmp" {
		e.nativeSNMPObservation = true
	}
	if protocol == "stun" {
		e.nativeSTUNObservation = true
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = protocol, profile, "observed-conversation", "message"
	if explicit != "" {
		e.Admission = "explicit-decode-as"
	} else if !existing {
		e.Admission = "wire-and-port-hint"
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	a.finishProtocolDatagram(e, nil, nil, protocolError(ErrResourceExceeded, "%s UDP datagram exceeds message byte budget", protocol))
	e.Completeness = e.Status
	a.limited.Add(uint64(len(w)))
	return true
}

// TFTP uses the observed server TID rather than a fixed symmetric port pair.
// A denied block/negotiation has unknown semantics: preserve the charged
// bounded transfer, but do not infer progression from later changed bytes.
func (a *binParser) refuseTFTPOversizeAssociation(e *ProtocolEvent, w []byte) bool {
	srcIP, _, srcErr := net.SplitHostPort(e.Source)
	dstIP, _, dstErr := net.SplitHostPort(e.Destination)
	if srcErr != nil || dstErr != nil {
		return false
	}
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	s := a.udpSessions
	if s == nil {
		return false
	}
	el := s.entries[binUDPKey{"tftp/" + e.Source, dstIP, e.Domain}]
	dir := 0
	if el == nil {
		el = s.entries[binUDPKey{"tftp/" + e.Destination, srcIP, e.Domain}]
		dir = 1
	}
	if el == nil {
		return false
	}
	v := el.Value.(*binUDPEntry)
	f := v.flow
	t := f.tftp
	if t == nil || t.server != "" && (dir == 1 && t.server != e.Source || dir == 0 && t.server != e.Destination) {
		return false
	}
	peer := e.Source
	if dir == 0 {
		peer = e.Destination
	}
	// Oversized packets on an already retired TID still hit the byte limit,
	// but their old generation cannot poison a fresh observed candidate.
	if !t.retiredTID(peer) {
		t.ambiguous = true
	}
	e.Protocol, e.FlowID, e.Direction = "tftp", f.id, dir
	e.Admission, e.Completeness = "observed-conversation", "message"
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	a.finishProtocolDatagram(e, nil, nil, protocolError(ErrResourceExceeded, "TFTP UDP datagram exceeds message byte budget"))
	e.Completeness = e.Status
	a.limited.Add(uint64(len(w)))
	return true
}
