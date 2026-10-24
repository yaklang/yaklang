package pcaputil

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strconv"
	"time"
)

type binUDPKey struct {
	a, b   string
	domain CaptureDomain
}
type binUDPEntry struct {
	key     binUDPKey
	flow    *binFlow
	touched time.Time
}
type binUDPStore struct {
	entries map[binUDPKey]*list.Element
	lru     list.List
	clock   time.Time
}

func (f *binFlow) retireSTUNUDP(ts time.Time) {
	f.stun.expire(ts)
	retained := f.stun.bytes()
	if retained < f.sessionBytes {
		f.a.buffered.Add(retained - f.sessionBytes)
		f.sessionBytes = retained
	}
}

// Reclaim only already inactive STUN observations under slot pressure. A foreign
// capture timestamp cannot declare a live request/allocation/relay idle. Losing
// retired identities still quarantines new keys; retained active peers survive.
func (a *binParser) reclaimInactiveSTUNSlot(s *binUDPStore) {
	if len(s.entries) < sessionCollectionLimit(a.budget.MaxCollectionElements) {
		return
	}
	for el := s.lru.Front(); el != nil; el = el.Next() {
		v := el.Value.(*binUDPEntry)
		if v.flow.stun == nil || len(v.flow.stun.pending) != 0 || len(v.flow.stun.channels) != 0 || len(v.flow.stun.permissions) != 0 || !v.flow.stun.allocations[0].IsZero() || !v.flow.stun.allocations[1].IsZero() {
			continue
		}
		a.stunUntrackedRefusal = true
		v.flow.closeSession()
		delete(s.entries, v.key)
		s.lru.Remove(el)
		return
	}
}

func (f *binFlow) retireSNMPUDP() {
	f.snmp.retirePending()
	retained := f.snmp.storage()
	if retained < f.sessionBytes {
		f.a.buffered.Add(retained - f.sessionBytes)
		f.sessionBytes = retained
	}
}

// Idle time is not a wire generation boundary for UDP. Keep a bounded,
// capture-domain/endpoint-scoped identity history while releasing pending wire,
// fragments and Data. A fully completed exchange may accept a distinct identity
// using its retained sequence/context history; idle alone must not revoke that
// history or manufacture a fresh generation. An unfinished exchange still needs
// quarantine because its peer progression was not completely observed.
func (f *binFlow) retireDLMSUDP() {
	if s := f.dlms; s != nil && (s.pending != nil || s.transfer != nil || s.setTransfer != nil || s.actionTransfer != nil || s.fragments != nil || s.hasActiveSNBlocks() || s.hasActiveSNGBT()) {
		f.dlms.invalidate()
	}
	retained := int64(512)
	if f.dlms != nil {
		retained = f.dlms.storage()
	}
	if f.wrapper != nil {
		s := f.wrapper
		complete := !s.blocked && s.outstanding() == 0
		for _, p := range s.seen {
			complete = complete && !p.blockActive
		}
		if complete {
			for _, p := range s.associations {
				p.pending = false
				p.ambiguous = true
			}
			// Keep every used logical-endpoint/invoke token charged and unusable.
			// A late reply for one token cannot poison another pending token.
			for _, p := range s.seen {
				p.ambiguous, p.idleRetired = true, true
			}
		} else {
			s.invalidate()
			s.seen = nil
		}
		retained = s.storage()
	}
	if f.sessionBytes > retained {
		f.a.buffered.Add(retained - f.sessionBytes)
		f.sessionBytes = retained
	}
}

// Idle capture time releases the body, not the on-wire 3E generation or used
// 4E serials. Incomplete exchanges cannot establish fresh context; completed
// 4E exchanges may still use distinct never-observed serials within the budget.
func (f *binFlow) retireSLMPUDP() {
	s := f.slmp
	if s.pending != nil {
		s.pending = nil
		s.ambiguous = true
	}
	retained := int64(512 + 64*len(s.seen))
	if f.sessionBytes > retained {
		f.a.buffered.Add(retained - f.sessionBytes)
		f.sessionBytes = retained
	}
}

func (a *binParser) decodeTFTPDatagram(e *ProtocolEvent, w []byte) bool {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	srcIP, _, srcErr := net.SplitHostPort(e.Source)
	dstIP, _, dstErr := net.SplitHostPort(e.Destination)
	if srcErr != nil || dstErr != nil {
		return false
	}
	request := probeTFTP(w, sessionCollectionLimit(a.budget.MaxCollectionElements))
	key := binUDPKey{"tftp/" + e.Source, dstIP, e.Domain}
	s := a.udpSessions
	var el *list.Element
	dir := 0
	if s != nil {
		el = s.entries[key]
		if el == nil && !request {
			key = binUDPKey{"tftp/" + e.Destination, srcIP, e.Domain}
			el = s.entries[key]
			dir = 1
		}
	}
	if !request && el == nil {
		return false
	}
	e.nativeTFTPObservation = true
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	e.Protocol = "tftp"
	if s == nil {
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		a.udpSessions = s
	}
	if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	var err error
	if el != nil && request && el.Value.(*binUDPEntry).flow.tftp.done {
		f := el.Value.(*binUDPEntry).flow
		t := f.tftp
		count := len(t.retired)
		if t.server != "" && !t.retiredTID(t.server) {
			count++
		}
		target := int64(512 + len(w)*3 + count*128)
		if a.newTFTPFile(e.Destination) != nil {
			target += 96
		}
		if count > min(128, sessionCollectionLimit(a.budget.MaxCollectionElements)) {
			err = protocolError(ErrResourceExceeded, "TFTP retired transfer ID budget")
		} else {
			// Reserve only the delta on the existing charged slot before moving
			// ownership. A completed transfer must not need two live reservations.
			err = f.reserveSession(target)
		}
		if err != nil {
			t.ambiguous = true
		} else {
			history := t.retired
			if history == nil && t.server != "" {
				history = make(map[string]struct{}, count)
			}
			if t.server != "" {
				history[t.server] = struct{}{}
			}
			f.tftp = &binTFTP{retired: history, file: a.newTFTPFile(e.Destination), ambiguous: t.futureAmbiguous, futureAmbiguous: t.futureAmbiguous}
			f.id = a.flows.Add(1)
			f.endpoints = [2]string{e.Source, e.Destination}
			if target < f.sessionBytes {
				a.buffered.Add(target - f.sessionBytes)
				f.sessionBytes = target
			}
		}
	}
	if el == nil {
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			a.tftpUntrackedRefusal = true
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		} else {
			f := &binFlow{a: a, id: a.flows.Add(1), protocol: "tftp", tftp: &binTFTP{file: a.newTFTPFile(e.Destination), ambiguous: a.tftpUntrackedRefusal, unverified: a.tftpUntrackedRefusal, futureAmbiguous: a.tftpUntrackedRefusal}, endpoints: [2]string{e.Source, e.Destination}}
			target := int64(512 + len(w)*3)
			if f.tftp.file != nil {
				target += 96
			}
			if err = f.reserveSession(target); err == nil {
				el = s.lru.PushBack(&binUDPEntry{key, f, s.clock})
				s.entries[key] = el
			} else {
				a.tftpUntrackedRefusal = true
			}
		}
	}
	if el != nil {
		v := el.Value.(*binUDPEntry)
		v.touched = s.clock
		s.lru.MoveToBack(el)
		f := v.flow
		t := f.tftp
		e.FlowID = f.id
		e.Direction = dir
		peer := e.Source
		if dir == 0 {
			peer = e.Destination
		}
		// Only consume after admission succeeds: a budget refusal must preserve
		// the completed generation and cannot quietly replace its history.
		if err == nil {
			if request && e.Destination != f.endpoints[1] {
				t.ambiguous, t.unverified, t.futureAmbiguous = true, true, true
				if t.file != nil {
					t.file.abort()
				}
				err = protocolError(ErrContextRequired, "TFTP request reused a client TID on another initial service")
			} else if !request && t.retiredTID(peer) {
				err = protocolError(ErrContextRequired, "TFTP packet uses a retired transfer ID")
			} else if dir == 1 && t.server != "" && t.server != e.Source || dir == 0 && !request && t.server != "" && t.server != e.Destination {
				err = protocolError(ErrContextRequired, "TFTP packet from unexpected transfer ID")
				if tftpCandidatePrefix(w) {
					if historyErr := f.rememberTFTPCandidate(peer); historyErr != nil {
						err = historyErr
					}
				}
			} else {
				if t.file != nil {
					base := f.sessionBytes - t.file.allocated()
					t.file.reserve = func(n int64) error {
						target := base + n
						if target < f.sessionBytes {
							a.buffered.Add(target - f.sessionBytes)
							f.sessionBytes = target
							return nil
						}
						return f.reserveSession(target)
					}
				}
				e.Session, err = t.consume(dir, w, sessionCollectionLimit(a.budget.MaxCollectionElements), tftpFileEvidence{ID: e.ID, Refs: e.SourceBytes.PacketRefs})
				if err != nil && t.file != nil {
					t.file.abort()
					t.ambiguous = true
					t.futureAmbiguous = true
				}
				if err == nil && dir == 1 && t.server == "" {
					t.server = e.Source
				}
			}
		}
		if e.Session != nil {
			e.Session["Observation Scope"] = "conversation"
			e.Session["Server Transfer ID"] = t.server
		}
	}
	if el != nil && el.Value.(*binUDPEntry).flow.tftp.file != nil {
		e.Profile = "tftp-octet-file"
	}
	a.finishProtocolDatagram(e, w, a.specs["tftp/TFTP"], err)
	return true
}

// decodeSIPDatagram keeps UDP SIP transactions separate from TCP byte-stream
// framing while using the same bounded Call-ID/Via/CSeq/dialog state machine.
func (a *binParser) decodeSIPDatagram(e *ProtocolEvent, wire []byte, explicit bool) bool {
	probe := probeSIP(wire, len(wire))
	if probe.Verdict != ProbeAccept {
		return false
	}
	e.nativeSIPObservation = true
	if err := validateSIPDatagramLength(wire); err != nil {
		e.Protocol, e.Profile, e.Admission, e.Completeness = "sip", "sip-udp-observed", "wire-signature", "message"
		if explicit {
			e.Admission = "explicit-decode-as"
		}
		a.recordSIPGap(e)
		a.finishProtocolDatagram(e, wire, a.specs["sip/SIP"], err)
		return true
	}
	a.udpMu.Lock()
	key := binUDPKey{a: "sip/" + e.Source, b: "sip/" + e.Destination, domain: e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	store := a.udpSessions
	if store == nil {
		store = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		a.udpSessions = store
	}
	// Foreign endpoints/interfaces cannot expire this owner. The retained
	// per-conversation SIP clock below prunes only its own protocol state.
	el := store.entries[key]
	var err error
	if el == nil {
		if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			a.sipUntrackedRefusal = true
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		} else {
			endpoints := []string{e.Source, e.Destination}
			sort.Strings(endpoints)
			flow := &binFlow{a: a, id: a.flows.Add(1), protocol: "sip", sip: &binSIP{unverified: a.sipUntrackedRefusal}, endpoints: [2]string{endpoints[0], endpoints[1]}}
			if err = flow.reserveSession(512); err == nil {
				el = store.lru.PushBack(&binUDPEntry{key: key, flow: flow, touched: e.Timestamp})
				store.entries[key] = el
			} else {
				a.sipUntrackedRefusal = true
			}
		}
	}
	if el != nil {
		entry := el.Value.(*binUDPEntry)
		if e.Timestamp.After(entry.touched) {
			entry.touched = e.Timestamp
		}
		store.lru.MoveToBack(el)
		flow := entry.flow
		e.FlowID = flow.id
		if e.Source != flow.endpoints[0] {
			e.Direction = 1
		}
		if err = flow.reserveSession(512 + flow.sip.retainedBytes() + flow.sip.projectedStateBytes(wire, e.Direction, a.budget.MaxCollectionElements)); err == nil {
			e.Session, err = flow.sip.consumeAt(wire, a.budget.MaxCollectionElements, e.Timestamp, e.Direction)
		}
		if err != nil {
			flow.sip = &binSIP{unverified: true}
			_ = flow.reserveSession(512)
		}

		if e.Session != nil {
			e.Session["Observation Scope"] = "conversation"
			e.Session["Transport"] = "UDP"
		}
	}
	a.udpMu.Unlock()
	e.Protocol = "sip"
	e.Profile = "sip-udp-observed"
	e.Admission = "wire-signature"
	if explicit {
		e.Admission = "explicit-decode-as"
	}
	e.Completeness = "message"
	if e.Session != nil {
		e.semanticFields = cloneSession(e.Session)
	}
	a.finishProtocolDatagram(e, wire, a.specs["sip/SIP"], err)
	if e.Session != nil {
		e.semanticFields = cloneSession(e.Session)
		e.Summary = fmt.Sprintf("SIP %v %v", e.Session["Packet Name"], e.Session["Call-ID"])
	}
	return true
}

// decodeSNMPDatagram applies the v1/v2c/v3 session parser to the native UDP
// carrier. The endpoint tuple and capture domain scope request-id state; the
// community string is fingerprinted internally and is never copied to events.
func (a *binParser) decodeSNMPDatagram(e *ProtocolEvent, w []byte, explicit string) bool {
	probe := probeSNMP(w, len(w))
	if probe.Verdict != ProbeAccept {
		return false
	}
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	key := binUDPKey{a: "snmp/" + e.Source, b: "snmp/" + e.Destination, domain: e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	s := a.udpSessions
	if s == nil {
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
		a.udpSessions = s
	}
	el := s.entries[key]
	if el != nil {
		entry := el.Value.(*binUDPEntry)
		if entry.flow.snmp != nil && e.Timestamp.Sub(entry.touched) >= snmpPendingTTL {
			entry.flow.retireSNMPUDP()
		}
	}
	var err error
	if el == nil {
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		} else {
			first, second := e.Source, e.Destination
			if first > second {
				first, second = second, first
			}
			f := &binFlow{a: a, id: a.flows.Add(1), protocol: "snmp", snmp: &binSNMP{pending: map[snmpPendingKey]snmpPendingRequest{}, blocked: a.snmpUntrackedRefusal}, endpoints: [2]string{first, second}}
			if err = f.reserveSession(512); err == nil {
				el = s.lru.PushBack(&binUDPEntry{key: key, flow: f, touched: e.Timestamp})
				s.entries[key] = el
			}
		}
	}
	if el == nil && err != nil {
		// Even a structurally valid first request existed on wire. If no
		// charged identity fits, future unseen keys lack a verified history.
		a.snmpUntrackedRefusal = true
	}

	if el != nil {
		entry := el.Value.(*binUDPEntry)
		if e.Timestamp.After(entry.touched) {
			entry.touched = e.Timestamp
		}
		s.lru.MoveToBack(el)
		f := entry.flow
		dir := 0
		if e.Source != f.endpoints[0] {
			dir = 1
		}
		e.FlowID, e.Direction = f.id, dir
		if f.snmp == nil {
			err = protocolError(ErrContextRequired, "SNMP conversation context was closed")
		} else {
			f.snmp.expirePending(e.Timestamp)
			err = f.reserveSession(f.snmp.storage() + 512 + int64(len(w))*3)
			if err == nil {
				e.Session, err = f.snmp.consumeAt(w, sessionCollectionLimit(a.budget.MaxCollectionElements), dir, e.Timestamp)
			} else {
				f.snmp.invalidate()
			}
		}
		if e.Session != nil {
			e.Session["Observation Scope"] = "conversation"
		}
	}
	e.nativeSNMPObservation = true
	e.Protocol = "snmp"
	e.Profile = "snmp-v" + probe.Version + "-native"
	e.Admission = "wire-and-port-hint"
	if explicit == "snmp" {
		e.Admission = "explicit-decode-as"
	}
	e.Completeness = "message"
	e.Summary = "SNMPv" + probe.Version
	if e.Session != nil {
		e.Summary += " " + fmt.Sprint(e.Session["Packet Name"])
		e.semanticFields = cloneSession(e.Session)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}

// Capture-owned UDP state is serialized independently of callbacks. The same
// endpoint pair may multiplex STUN/TURN and media; unknown datagrams never feed
// a TCP byte buffer or destroy the observed relay state.
func (a *binParser) decodeSTUNDatagram(e *ProtocolEvent, w []byte, explicitTurn ...bool) bool {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	key := binUDPKey{e.Source, e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	s := a.udpSessions
	var el *list.Element
	if s != nil {
		el = s.entries[key]
	}
	stun := probeSTUN(w, len(w)).Verdict == ProbeAccept
	channelHeader := len(w) >= 4 && w[0]&0xc0 == 0x40
	channel := false
	if channelHeader && el != nil {
		// An observed STUN transaction is not a TURN channel binding. Only
		// an exact still-observed channel can disambiguate malformed data
		// from a neighboring protocol such as a four-byte CoAP ACK.
		if state := el.Value.(*binUDPEntry).flow.stun; state != nil {
			ch := uint16(w[0])<<8 | uint16(w[1])
			for _, dir := range [2]int{0, 1} {
				if binding, ok := state.channels[turnChannelKey{dir, ch}]; ok && binding.expires.After(e.Timestamp) && binding.expires.After(state.clock) {
					channel = true
					break
				}
			}
		}
	}
	selectedTurn := len(explicitTurn) > 0 && explicitTurn[0]
	knownTURNPort := udpEndpointPort(e.Source, 3478) || udpEndpointPort(e.Destination, 3478)
	coap := probeCoAP(w, len(w)).Verdict == ProbeAccept
	channelAdmitted := channelHeader && (selectedTurn || channel || validTURNChannelDataDatagram(w) && (knownTURNPort || !coap))
	if selectedTurn && channelHeader {
		channelAdmitted = true // explicit DecodeAs selects the envelope; its framing is still validated below.
	}
	if !stun && !channelAdmitted {
		return false
	}
	if el != nil {
		v := el.Value.(*binUDPEntry)
		if v.flow.stun != nil && e.Timestamp.Sub(v.touched) >= 10*time.Minute {
			v.flow.retireSTUNUDP(e.Timestamp)
		}
	}
	e.nativeSTUNObservation = true
	e.Protocol = "stun"
	e.Profile = "stun-rfc8489-udp"
	e.Admission = "wire-signature"
	e.Completeness = "message"
	if channelHeader {
		e.Protocol = "turn"
		e.Profile = "turn-rfc8656-udp"
	}
	if channelHeader && el == nil {
		// ChannelData can be captured after its allocation/ChannelBind exchange.
		// Keep the bounded envelope and raw packet evidence, but do not invent
		// the missing peer mapping needed to interpret its inner media payload.
		f := &binSTUN{}
		info, parseErr := f.consume(0, e.Timestamp, w, a.budget.MaxCollectionElements, false)
		if parseErr != nil {
			a.finishProtocolDatagram(e, w, a.specs["stun_session/ChannelData"], parseErr)
			return true
		}
		e.Session = info
		e.Session["Observation Scope"] = "datagram"
		e.semanticFields = cloneSession(info)
		a.finishProtocolDatagram(e, w, a.specs["stun_session/ChannelData"], protocolError(ErrContextRequired, "TURN ChannelData requires an observed ChannelBind"))
		e.Summary = "TURN ChannelData (channel binding not observed)"
		return true
	}
	var err error
	if s == nil {
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
		a.udpSessions = s
	}
	if el == nil {
		a.reclaimInactiveSTUNSlot(s)
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		} else {
			f := &binFlow{a: a, id: a.flows.Add(1), endpoints: [2]string{e.Source, e.Destination}, protocol: "stun", stun: &binSTUN{blocked: a.stunUntrackedRefusal}}
			if err = f.reserveSession(1024); err == nil {
				el = s.lru.PushBack(&binUDPEntry{key, f, e.Timestamp})
				s.entries[key] = el
			}
		}
	}
	if el == nil && err != nil {
		a.stunUntrackedRefusal = true
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
		if err = f.reserveSession(f.stun.bytes() + int64(len(w))*8 + 512); err == nil {
			e.Session, err = f.stun.consume(e.Direction, e.Timestamp, w, a.budget.MaxCollectionElements, false)
		} else {
			f.stun.invalidate()
		}
		if e.Session != nil {
			e.Session["Observation Scope"] = "conversation"
			if e.Session["TURN"] == true {
				e.Protocol = "turn"
			}
		}
	}
	if channelHeader && err == nil && e.Session != nil && e.Session["Matched"] != true {
		err = protocolError(ErrContextRequired, "TURN ChannelData requires an observed ChannelBind")
	}
	if channelHeader && e.Session != nil {
		e.semanticFields = cloneSession(e.Session)
	}
	if e.Protocol == "turn" {
		e.Profile = "turn-rfc8656-udp"
	}
	entry := "STUN"
	if channelHeader {
		entry = "ChannelData"
	}
	spec := a.specs["stun_session/"+entry]
	a.finishProtocolDatagram(e, w, spec, err)
	return true
}

func udpEndpointPort(endpoint string, want uint16) bool {
	_, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return false
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	return err == nil && uint16(port) == want
}

func validTURNChannelDataDatagram(w []byte) bool {
	if len(w) < 4 || w[0]&0xc0 != 0x40 {
		return false
	}
	channel := binary.BigEndian.Uint16(w[:2])
	if channel < 0x4000 || channel > 0x7fff {
		return false
	}
	n := 4 + int(binary.BigEndian.Uint16(w[2:4]))
	return len(w) == n || len(w) == (n+3)&^3
}
func (a *binParser) finishProtocolDatagram(e *ProtocolEvent, w []byte, spec *binSpec, err error) {
	e.Raw = append([]byte(nil), w...)
	e.Status, e.Summary = "deferred", e.Protocol
	if spec != nil {
		e.Rule, e.Entry, e.plan = spec.rule, spec.entry, spec.plan
	}
	a.messages.Add(1)
	a.messageBytes.Add(uint64(len(w)))
	if err == nil && (e.Protocol == "stun" || e.Protocol == "turn" || e.Protocol == "tftp" || e.nativeQUICWire) {
		// Keep the accepted structural wire fields separate from editable event
		// projections; full/deferred queries share identical owned answers.
		var decoded map[string]any
		decoded, err = e.Decode()
		if err == nil {
			e.semanticFields = cloneSession(protocolFields(decoded))
			if !a.config.Deferred {
				e.Structured = decoded
			}
		}
	} else if err == nil && !a.config.Deferred {
		e.Structured, err = e.Decode()
	}
	if err != nil {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error = err.Error()
		if e.Status == "context-required" {
			a.contextRequired.Add(1)
		} else if e.Status == "limited" {
			a.limited.Add(uint64(len(w)))
		} else if e.Status == "incomplete" {
			a.incomplete.Add(1)
		} else {
			a.malformed.Add(1)
		}
	} else if a.config.Deferred {
		a.deferred.Add(1)
	} else {
		e.Status = "decoded"
		a.decoded.Add(1)
	}
}
func (a *binParser) closeUDPSessions() {
	for _, e := range a.closeCarrierSessions() {
		a.emit(e)
	}
	a.closeMAVLinkSigning()
	// Snapshot incomplete native block and information segmentation observations.
	// Emit after unlocking; user callbacks must not run under udpMu.
	var incomplete []*ProtocolEvent
	defer func() {
		for _, e := range incomplete {
			a.emit(e)
		}
	}()
	a.dnsMu.Lock()
	for _, p := range a.dns.pending {
		a.buffered.Add(-p.cost)
	}
	a.dns.pending = nil
	for _, p := range a.lan.pending {
		a.buffered.Add(-p.cost)
	}
	a.lan.pending = nil
	a.dnsMu.Unlock()
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	incomplete = append(incomplete, a.closeSomeIPTPLocked()...)
	incomplete = append(incomplete, a.closeAISMultipartLocked()...)
	incomplete = append(incomplete, a.closeRTPSFragmentsLocked()...)
	if s := a.semtechSessions; s != nil {
		for _, el := range s.entries {
			el.Value.(*binUDPEntry).flow.closeSession()
		}
		a.semtechSessions = nil
	}
	a.semtechDisabled = false
	if s := a.bacnetReadSessions; s != nil {
		for _, el := range s.entries {
			el.Value.(*binUDPEntry).flow.closeSession()
		}
		a.bacnetReadSessions = nil
	}
	a.bacnetReadDisabled = false
	a.bacnetRoutedReadDisabled = false
	a.bacnetInvokeOrigins = nil
	if s := a.udpSessions; s != nil {
		for _, el := range s.entries {
			v := el.Value.(*binUDPEntry)
			f := v.flow
			if state := f.radius; state != nil {
				groups, buffered := state.tlsRounds.unfinished()
				if state.outstanding() > 0 || groups > 0 {
					summary := "RADIUS UDP unmatched observed Identifiers"
					if groups > 0 {
						summary += " or observed TLS fragments"
					}
					e := &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[0], Destination: f.endpoints[1], Protocol: "radius", nativeRADIUSObservation: true, Profile: state.profile(), Status: "incomplete", Completeness: "incomplete", Summary: summary, Error: summary, Session: map[string]any{"Outstanding": state.outstanding()}, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}}
					if groups > 0 {
						e.Session["TLS Fragment Groups"] = groups
						e.Session["TLS Fragment Buffered Bytes"] = buffered
					}
					e.semanticSession = cloneSession(e.Session)
					incomplete = append(incomplete, e)
					a.incomplete.Add(1)
				}
			}
			if p := f.hart; p != nil && len(p.pending) > 0 {
				dir := max(p.client, 0)
				summary := "HART-IP UDP outstanding observed requests"
				e := &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "hart-ip", nativeHARTIPObservation: true, Profile: hartIPProfile, Status: "incomplete", Completeness: "incomplete", Summary: summary, Error: summary, Session: map[string]any{"Outstanding": len(p.pending)}, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}}
				e.semanticSession = cloneSession(e.Session)
				incomplete = append(incomplete, e)
				a.incomplete.Add(1)
			}
			if state := f.fins; state != nil {
				for dir, count := range state.counts() {
					if count > 0 {
						summary := "FINS UDP unmatched observed requests"
						e := &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "fins", nativeFINSObservation: true, Profile: finsProfile, Status: "incomplete", Completeness: "incomplete", Summary: summary, Error: summary, Session: map[string]any{"Outstanding": count}, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}}
						e.semanticSession = cloneSession(e.Session)
						incomplete = append(incomplete, e)
						a.incomplete.Add(1)
					}
				}
			}
			if w := f.wrapper; w != nil {
				for dir, choices := range w.unfinishedGetCounts() {
					for choice, count := range choices {
						if count == 0 {
							continue
						}
						summary := "DLMS Wrapper exchange ended with unmatched observed requests"
						e := &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms-wrapper", Profile: wrapperPendingGetProfile(byte(choice + 1)), nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: map[string]any{"Outstanding": count}, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}}
						e.semanticSession = cloneSession(e.Session)
						e.Error = e.sessionError.Error()
						incomplete = append(incomplete, e)
						a.incomplete.Add(1)
					}
				}
				for dir, count := range w.unfinishedGBTCounts() {
					if count == 0 {
						continue
					}
					summary := "DLMS Wrapper exchange ended with unmatched observed requests"
					e := &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms-wrapper", Profile: "dlms-wrapper-v1-gbt", nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: map[string]any{"Outstanding": count}, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}}
					e.semanticSession = cloneSession(e.Session)
					e.Error = e.sessionError.Error()
					incomplete = append(incomplete, e)
					a.incomplete.Add(1)
				}
				for _, key := range w.unfinishedSNBlockKeys() {
					p := w.snGroups[key]
					summary := wrapperSNGroupCloseSummary(key)
					e := &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[key.dir], Destination: f.endpoints[1-key.dir], Direction: key.dir, Protocol: "dlms-wrapper", Profile: wrapperSNGroupProfile(key), nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: wrapperSNGroupCloseFields(key, p), sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}}
					e.semanticSession = cloneSession(e.Session)
					e.Error = e.sessionError.Error()
					incomplete = append(incomplete, e)
					a.incomplete.Add(1)
				}
				for _, key := range w.pendingOperations() {
					command := w.seen[key].command
					summary := "DLMS Wrapper " + wrapperOperationName(command, w.seen[key].choice) + " ended with an unmatched request"
					e := &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[key.dir], Destination: f.endpoints[1-key.dir], Direction: key.dir, Protocol: "dlms-wrapper", Profile: wrapperOperationProfile(command, w.seen[key].choice), nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: wrapperOperationCloseFields(key, command, w.seen[key].choice, w.seen[key].count), sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}}
					e.semanticSession = cloneSession(e.Session)
					e.Error = e.sessionError.Error()
					incomplete = append(incomplete, e)
					a.incomplete.Add(1)
				}
				for key, p := range w.associations {
					if p.pending && p.lowAuthentication {
						summary := "DLMS Wrapper LOW association ended with an unmatched request"
						incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[key.dir], Destination: f.endpoints[1-key.dir], Direction: key.dir, Protocol: "dlms-wrapper", Profile: "dlms-wrapper-v1-acse", Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: map[string]any{"Outstanding": 1, "source_wport": key.source, "destination_wport": key.destination}, Error: summary, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}})
						a.incomplete.Add(1)
					}
					if p.releasePending {
						summary := "DLMS Wrapper release ended with an unmatched request"
						incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[key.dir], Destination: f.endpoints[1-key.dir], Direction: key.dir, Protocol: "dlms-wrapper", Profile: "dlms-wrapper-v1-acse-lifecycle", Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: map[string]any{"Outstanding": 1, "source_wport": key.source, "destination_wport": key.destination}, Error: summary, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}})
						a.incomplete.Add(1)
					}
				}
			}

			if d := f.dlms; d != nil {
				for _, key := range d.unfinishedSNGBTKeys() {
					summary := "DLMS HDLC SN GBT observation ended before complete APDU"
					e := &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[key.dir], Destination: f.endpoints[1-key.dir], Direction: key.dir, Protocol: "dlms", Profile: "dlms-hdlc-sn-gbt", nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: d.snGBTCloseFields(key), sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}}
					e.Error = e.sessionError.Error()
					e.semanticSession = cloneSession(e.Session)
					a.incomplete.Add(1)
					incomplete = append(incomplete, e)
				}
				for _, key := range d.unfinishedSNBlockKeys() {
					summary := dlmsSNBlockCloseSummary(key)
					e := &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[key.dir], Destination: f.endpoints[1-key.dir], Direction: key.dir, Protocol: "dlms", Profile: dlmsSNBlockProfile(key), nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: d.snBlockCloseFields(key), sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}}
					e.semanticSession = cloneSession(e.Session)
					e.Error = e.sessionError.Error()
					incomplete = append(incomplete, e)
					a.incomplete.Add(1)
				}
			}
			if d := f.dlms; d != nil && d.fragments != nil {
				dir := d.fragmentCloseDirection()
				profile, summary := "dlms-hdlc-segmented-response", "DLMS HDLC exchange ended with an incomplete segmented response"
				if d.fragments.request {
					profile, summary = "dlms-hdlc-segmented-request", "DLMS HDLC exchange ended with an incomplete segmented request"
				}
				if d.fragments.operation != "" {
					profile, summary = d.fragments.operationSegmentProfile(), d.fragments.operationCloseSummary()
				}
				if d.fragments.acseCommand != 0 {
					profile, summary = "dlms-hdlc-acse-segmented", "DLMS HDLC exchange ended with incomplete ACSE information"
				}
				incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms", Profile: profile, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: d.fragmentCloseFields()})
				if d.fragments.acseCommand != 0 || d.fragments.operation != "" {
					e := incomplete[len(incomplete)-1]
					e.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: summary}
					e.Error = summary
					if d.fragments.operation != "" {
						e.nativeDLMSOrSLMPObservation = true
					}
				}
				a.incomplete.Add(1)
			} else if d != nil && d.transfer != nil && d.transfer.initial.kind == "ACTION" {
				p := &d.transfer.initial
				dir := p.dir
				summary := "DLMS HDLC Action-download ended with an unmatched request"
				incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms", Profile: "dlms-hdlc-action-download", nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: dlmsOperationCloseFields(p), Error: summary, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}})
				a.incomplete.Add(1)
			} else if d != nil && d.transfer != nil {
				t := d.transfer
				dir := t.initial.dir
				incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms", Profile: "dlms-hdlc-get-block", Status: "incomplete", Completeness: "incomplete", Summary: "DLMS HDLC exchange ended with an incomplete data-block transfer", Session: map[string]any{"Outstanding": 1, "ObservedBlocks": t.blocks, "EncodedBytes": len(t.data)}})
				a.incomplete.Add(1)
			} else if d != nil && d.setTransfer != nil {
				dir := d.setTransfer.initial.dir
				summary := "DLMS HDLC " + d.setTransfer.closeName() + " ended with an unmatched request"
				incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms", Profile: d.setTransfer.profile(), nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: d.setTransfer.closeFields(), Error: summary, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}})
				a.incomplete.Add(1)
			} else if d != nil && d.actionTransfer != nil {
				dir := d.actionTransfer.initial.dir
				summary := "DLMS HDLC Action-with-pblock ended with an unmatched request"
				incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms", Profile: "dlms-hdlc-action-upload", nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: d.actionTransfer.closeFields(), Error: summary, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}})
				a.incomplete.Add(1)
			} else if d != nil && d.pending != nil && dlmsOperationKind(d.pending.kind) {
				dir := d.pending.dir
				summary := "DLMS HDLC " + dlmsOperationCloseName(d.pending) + " ended with an unmatched request"
				incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms", Profile: "dlms-hdlc-ln-operations", nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: dlmsOperationCloseFields(d.pending), Error: summary, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}})
				a.incomplete.Add(1)
			} else if d != nil && d.pending != nil && d.pending.kind == "GET" && (d.pending.choice == 1 || d.pending.choice == 3) {
				dir := d.pending.dir
				summary := "DLMS HDLC exchange ended with an unmatched request"
				profile := "dlms-hdlc-get-normal"
				if d.pending.choice == 3 {
					profile = "dlms-hdlc-get-list"
				}
				incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms", Profile: profile, nativeDLMSOrSLMPObservation: true, Status: "incomplete", Completeness: "incomplete", Summary: summary, Session: map[string]any{"Outstanding": 1}, Error: summary, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}})
				a.incomplete.Add(1)
			} else if d != nil && d.pending != nil && d.pending.kind == "ACSE" {
				dir := d.pending.dir
				summary := "DLMS HDLC association ended with an unmatched request"
				profile := "dlms-hdlc-acse"
				if d.pending.acseCommand == 0x62 {
					profile = "dlms-hdlc-acse-lifecycle"
				}
				incomplete = append(incomplete, &ProtocolEvent{FlowID: f.id, Timestamp: v.touched, Domain: v.key.domain, Transport: "udp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Protocol: "dlms", Profile: profile, Status: "incomplete", Completeness: "incomplete", Summary: "DLMS HDLC association ended with an unmatched request", Session: map[string]any{"Outstanding": 1}, Error: summary, sessionError: &ProtocolError{Kind: ErrNeedMore, Message: summary}})
				a.incomplete.Add(1)
			}
			f.closeSession()
		}
		a.udpSessions = nil
	}
}
