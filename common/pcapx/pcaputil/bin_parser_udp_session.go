package pcaputil

import (
	"container/list"
	"net"
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

// Capture-owned UDP state is serialized independently of callbacks. The same
// endpoint pair may multiplex STUN/TURN and media; unknown datagrams never feed
// a TCP byte buffer or destroy the observed relay state.
func (a *binParser) decodeSTUNDatagram(e *ProtocolEvent, w []byte) bool {
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
	channel := len(w) >= 4 && w[0]&0xc0 == 0x40 && el != nil
	if !stun && !channel {
		return false
	}
	e.Protocol = "stun"
	if channel {
		e.Protocol = "turn"
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
	entry := "STUN"
	if channel {
		entry = "ChannelData"
	}
	spec := a.specs["stun_session/"+entry]
	a.finishProtocolDatagram(e, w, spec, err)
	return true
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
