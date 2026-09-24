package pcaputil

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
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
	e.Protocol = "tftp"
	if s == nil {
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		a.udpSessions = s
	}
	if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	if el != nil && request && el.Value.(*binUDPEntry).flow.tftp.done {
		el.Value.(*binUDPEntry).flow.closeSession()
		s.lru.Remove(el)
		delete(s.entries, key)
		el = nil
	}
	var err error
	if el == nil {
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		} else {
			f := &binFlow{a: a, id: a.flows.Add(1), protocol: "tftp", tftp: &binTFTP{}, endpoints: [2]string{e.Source, e.Destination}}
			if err = f.reserveSession(512 + int64(len(w))*3); err == nil {
				el = s.lru.PushBack(&binUDPEntry{key, f, s.clock})
				s.entries[key] = el
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
		if dir == 1 && t.server != "" && t.server != e.Source || dir == 0 && !request && t.server != "" && t.server != e.Destination {
			err = protocolError(ErrContextRequired, "TFTP packet from unexpected transfer ID")
		} else {
			e.Session, err = t.consume(dir, w, sessionCollectionLimit(a.budget.MaxCollectionElements))
			if err == nil && dir == 1 && t.server == "" {
				t.server = e.Source
			}
		}
		if e.Session != nil {
			e.Session["Observation Scope"] = "conversation"
			e.Session["Server Transfer ID"] = t.server
		}
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
	if err := validateSIPDatagramLength(wire); err != nil {
		e.Protocol, e.Profile, e.Admission, e.Completeness = "sip", "sip-udp-observed", "wire-signature", "message"
		if explicit {
			e.Admission = "explicit-decode-as"
		}
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
	} else if e.Timestamp.After(store.clock) {
		store.clock = e.Timestamp
	}
	for el := store.lru.Front(); el != nil; {
		next := el.Next()
		entry := el.Value.(*binUDPEntry)
		if store.clock.Before(entry.touched) || store.clock.Sub(entry.touched) < sipDialogStateTTL {
			break
		}
		if strings.HasPrefix(entry.key.a, "sip/") {
			entry.flow.closeSession()
			delete(store.entries, entry.key)
			store.lru.Remove(el)
		}
		el = next
	}
	el := store.entries[key]
	var err error
	if el == nil {
		if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		} else {
			endpoints := []string{e.Source, e.Destination}
			sort.Strings(endpoints)
			flow := &binFlow{a: a, id: a.flows.Add(1), protocol: "sip", sip: &binSIP{}, endpoints: [2]string{endpoints[0], endpoints[1]}}
			if err = flow.reserveSession(512); err == nil {
				el = store.lru.PushBack(&binUDPEntry{key: key, flow: flow, touched: store.clock})
				store.entries[key] = el
			}
		}
	}
	if el != nil {
		entry := el.Value.(*binUDPEntry)
		entry.touched = store.clock
		store.lru.MoveToBack(el)
		flow := entry.flow
		e.FlowID = flow.id
		if e.Source != flow.endpoints[0] {
			e.Direction = 1
		}
		stateEntries := len(flow.sip.pending) + len(flow.sip.seen) + len(flow.sip.inviteStatus) + len(flow.sip.inviteComplete) + len(flow.sip.dialogs) + len(flow.sip.dialogTxns) + len(flow.sip.pendingAt) + len(flow.sip.completedAt) + len(flow.sip.seenAt) + len(flow.sip.finalResponses) + len(flow.sip.responseAt) + len(flow.sip.responseDir) + len(flow.sip.dialogAt)
		if err = flow.reserveSession(512 + int64(stateEntries)*128 + int64(len(wire))*3); err == nil {
			e.Session, err = flow.sip.consumeAt(wire, a.budget.MaxCollectionElements, e.Timestamp, e.Direction)
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
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		a.udpSessions = s
	} else if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	for el := s.lru.Front(); el != nil; {
		next := el.Next()
		v := el.Value.(*binUDPEntry)
		if s.clock.Sub(v.touched) >= 10*time.Minute {
			v.flow.closeSession()
			delete(s.entries, v.key)
			s.lru.Remove(el)
		}
		el = next
	}
	el := s.entries[key]
	var err error
	if el == nil {
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		} else {
			first, second := e.Source, e.Destination
			if first > second {
				first, second = second, first
			}
			f := &binFlow{a: a, id: a.flows.Add(1), protocol: "snmp", snmp: &binSNMP{pending: map[snmpPendingKey]snmpPendingRequest{}}, endpoints: [2]string{first, second}}
			if err = f.reserveSession(512); err == nil {
				el = s.lru.PushBack(&binUDPEntry{key: key, flow: f, touched: s.clock})
				s.entries[key] = el
			}
		}
	}
	if el != nil {
		entry := el.Value.(*binUDPEntry)
		entry.touched = s.clock
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
			f.snmp.expirePending(s.clock)
			err = f.reserveSession(256 + int64(len(f.snmp.pending)+1)*128 + int64(len(w))*3)
			if err == nil {
				e.Session, err = f.snmp.consumeAt(w, sessionCollectionLimit(a.budget.MaxCollectionElements), dir, s.clock)
			}
		}
		if e.Session != nil {
			e.Session["Observation Scope"] = "conversation"
		}
	}
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
	if s != nil {
		if e.Timestamp.After(s.clock) {
			s.clock = e.Timestamp
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
	}
	var el *list.Element
	if s != nil {
		el = s.entries[key]
	}
	stun := probeSTUN(w, len(w)).Verdict == ProbeAccept
	channelHeader := len(w) >= 4 && w[0]&0xc0 == 0x40
	channel := channelHeader && el != nil
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
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		a.udpSessions = s
	}
	if el == nil {
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "UDP conversation budget")
		} else {
			f := &binFlow{a: a, id: a.flows.Add(1), endpoints: [2]string{e.Source, e.Destination}, protocol: "stun", stun: &binSTUN{}}
			if err = f.reserveSession(1024); err == nil {
				el = s.lru.PushBack(&binUDPEntry{key, f, s.clock})
				s.entries[key] = el
			}
		}
	}
	if el != nil {
		v := el.Value.(*binUDPEntry)
		v.touched = s.clock
		s.lru.MoveToBack(el)
		f := v.flow
		e.FlowID = f.id
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
		if err = f.reserveSession(f.stun.bytes() + int64(len(w))*8 + 512); err == nil {
			e.Session, err = f.stun.consume(e.Direction, e.Timestamp, w, a.budget.MaxCollectionElements, false)
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
	if err == nil && !a.config.Deferred {
		e.Structured, err = e.Decode()
	}
	if err != nil {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error = err.Error()
		if e.Status == "context-required" {
			a.contextRequired.Add(1)
		} else if e.Status == "limited" {
			a.limited.Add(uint64(len(w)))
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
	a.dnsMu.Lock()
	for _, p := range a.dns.pending {
		a.buffered.Add(-p.cost)
	}
	a.dns.pending = nil
	a.dnsMu.Unlock()
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	if s := a.udpSessions; s != nil {
		for _, el := range s.entries {
			el.Value.(*binUDPEntry).flow.closeSession()
		}
		a.udpSessions = nil
	}
}
