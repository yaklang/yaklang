package pcaputil

import (
	"container/list"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"strings"
	"time"
)

type controlDatagramState struct {
	indexes  map[uint64]bool
	sessions map[uint64]string
}
type controlDatagramProfile struct {
	name, entry string
	ports       []uint16
}

var controlDatagramProfiles = []controlDatagramProfile{
	{"ldp", "LDPDiscoveryFields", []uint16{646}},
	{"ike", "IKEMessage", []uint16{500}}, {"ike-natt", "IKENATTMessage", []uint16{4500}},
	{"wireguard", "WireGuardMessage", []uint16{51820}}, {"rip", "RIPMessage", []uint16{520}},
	{"nbt-dg", "NBTDGMessage", []uint16{138}}, {"onc-rpc", "ONCRPCMessage", []uint16{111}},
	{"l2tp", "L2TPMessage", []uint16{1701}}, {"openvpn", "OpenVPNMessage", []uint16{1194}},
	{"ipmi", "IPMIMessage", []uint16{623}},
	{"hsrp", "HSRPMessageFields", []uint16{1985, 2029}},
	{"glbp", "GLBPMessageFields", []uint16{3222}},
	{"bfd", "BFDMessageFields", []uint16{3784, 4784}},
}

func controlPort(ports []uint16, p uint16) bool {
	for _, v := range ports {
		if p == v {
			return true
		}
	}
	return false
}
func (a *binParser) decodeControlDatagram(e *ProtocolEvent, w []byte, src, dst uint16, explicit string) bool {
	if a.decodeNetFlowV5Datagram(e, w, src, dst, explicit) {
		return true
	}
	if a.decodeNetFlowV9Datagram(e, w, src, dst, explicit) {
		return true
	}
	if a.decodeRIPngDatagram(e, w, src, dst, explicit) {
		return true
	}
	for _, p := range controlDatagramProfiles {
		selected := explicit == p.name
		if !selected && (explicit != "" || !controlPort(p.ports, src) && !controlPort(p.ports, dst)) {
			continue
		}
		var fields map[string]any
		var err error
		routing := p.name == "hsrp" || p.name == "glbp" || p.name == "bfd"
		if p.name == "ldp" {
			fields, err = stream_parser.DecodeLDPDiscovery(w, a.budget.MaxCollectionElements)
			err = observedWireError(err)
		} else if routing {
			fields, err = stream_parser.DecodeRoutingControl(w, p.name, a.budget.MaxCollectionElements)
			err = observedWireError(err)
		} else {
			fields, err = stream_parser.DecodeNetworkControl(w, p.name)
		}
		if err != nil && !selected {
			continue
		}
		if err == nil && (p.name == "wireguard" || p.name == "openvpn") {
			if !a.controlConversation(e, fields, p.name) && !selected {
				continue
			}
		}
		if routing && err == nil && !a.routingConversation(e, fields, p.name, selected) {
			continue
		}
		e.Protocol, e.Rule, e.Entry = p.name, "network_control_fields", p.entry
		if p.name == "ldp" {
			e.Rule, e.nativeSessionMessage = "ldp_discovery", true
		}
		if routing {
			e.Rule = "routing_control"
			e.nativeSessionMessage = true
		}
		if p.name == "ike-natt" {
			e.Protocol = "ike"
		}
		e.Profile, e.Admission, e.Completeness = "bounded-"+p.name+"-wire", "validated-wire-and-service-hint", "message"
		if selected {
			e.Admission = "explicit-decode-as"
		}
		if err == nil {
			e.Session = fields
			e.semanticFields = cloneSession(fields)
			if routing || p.name == "ldp" {
				e.semanticSession = cloneSession(fields)
			}
			e.Summary = fields["Packet Name"].(string)
		}
		a.finishProtocolDatagram(e, w, nil, err)
		if err == nil && p.name == "ldp" {
			e.Summary = fields["Packet Name"].(string)
		}
		return true
	}
	return false
}

// This associates only observed UDP wire exchanges. Index/session identifiers
// are not authenticated peer identities, and ciphertext cannot identify a new
// protocol solely because a previous datagram used the same port.
func (a *binParser) controlConversation(e *ProtocolEvent, m map[string]any, protocol string) bool {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	x, y := e.Source, e.Destination
	if x > y {
		x, y = y, x
	}
	key := binUDPKey{"control/" + protocol + "/" + x, y, e.Domain}
	s := a.udpSessions
	if s == nil {
		s = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
		a.udpSessions = s
	}
	if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	for el := s.lru.Front(); el != nil; {
		next := el.Next()
		v := el.Value.(*binUDPEntry)
		if strings.HasPrefix(v.key.a, "control/") && s.clock.Sub(v.touched) >= 2*time.Minute {
			v.flow.closeSession()
			delete(s.entries, v.key)
			s.lru.Remove(el)
		}
		el = next
	}
	el := s.entries[key]
	initial := false
	if protocol == "wireguard" {
		initial = m["Message Type"] == uint64(1)
	} else {
		op, _ := m["Opcode"].(uint64)
		initial = op == 1 || op == 7 || op == 10
	}
	if el == nil && !initial {
		return false
	}
	if el != nil && initial && protocol == "openvpn" {
		old := el.Value.(*binUDPEntry).flow
		dir := uint64(0)
		if e.Source != old.endpoints[0] {
			dir = 1
		}
		keyID, _ := m["Key ID"].(uint64)
		id, _ := m["Session ID"].([]byte)
		if previous := old.control.sessions[keyID<<1|dir]; previous != "" && previous != string(id) {
			// A new client hard-reset on a reused UDP tuple is a new observed
			// exchange. It cannot inherit the old server's key-ID evidence.
			old.closeSession()
			delete(s.entries, key)
			s.lru.Remove(el)
			el = nil
		}
	}
	if el == nil {
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			return false
		}
		f := &binFlow{a: a, id: a.flows.Add(1), protocol: protocol, control: &controlDatagramState{indexes: map[uint64]bool{}, sessions: map[uint64]string{}}, endpoints: [2]string{e.Source, e.Destination}}
		if f.reserveSession(4096) != nil {
			return false
		}
		el = s.lru.PushBack(&binUDPEntry{key, f, s.clock})
		s.entries[key] = el
	}
	v := el.Value.(*binUDPEntry)
	f := v.flow
	dir := 0
	if e.Source != f.endpoints[0] {
		dir = 1
	}
	if protocol == "wireguard" {
		if receiver, ok := m["Receiver Index"].(uint64); ok && !f.control.indexes[receiver<<1|uint64(1-dir)] {
			return false
		}
		if sender, ok := m["Sender Index"].(uint64); ok {
			if len(f.control.indexes) >= 32 {
				f.control.indexes = map[uint64]bool{}
			}
			f.control.indexes[sender<<1|uint64(dir)] = true
		}
	} else {
		op, _ := m["Opcode"].(uint64)
		keyID, _ := m["Key ID"].(uint64)
		if op == 6 || op == 9 {
			if !f.control.indexes[keyID<<1] || !f.control.indexes[keyID<<1|1] {
				return false
			}
		} else {
			if remote, ok := m["Remote Session ID"].([]byte); ok {
				if peer := f.control.sessions[keyID<<1|uint64(1-dir)]; peer == "" || peer != string(remote) {
					return false
				}
			}
			if id, ok := m["Session ID"].([]byte); ok {
				f.control.sessions[keyID<<1|uint64(dir)] = string(id)
			}
			f.control.indexes[keyID<<1|uint64(dir)] = true
		}
	}
	v.touched = s.clock
	s.lru.MoveToBack(el)
	e.FlowID, e.Direction = f.id, dir
	m["Association"] = "observed UDP exchange; identifiers unverified"
	return true
}
