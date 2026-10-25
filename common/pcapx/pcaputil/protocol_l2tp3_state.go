package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// This is a bounded passive observation of base control exchanges. Cookies
// distinguish observed receiver proposals; they do not authenticate a peer.
// History lives until capture Close: eviction must not make reused IDs fresh.
type l2tp3Endpoint struct {
	host string
	port uint16
}

func l2tp3EndpointOf(s string) l2tp3Endpoint {
	h, p, err := net.SplitHostPort(s)
	if err != nil {
		return l2tp3Endpoint{host: s}
	}
	n, _ := strconv.ParseUint(p, 10, 16)
	return l2tp3Endpoint{h, uint16(n)}
}
func (p l2tp3Endpoint) String() string {
	if p.port != 0 {
		return net.JoinHostPort(p.host, strconv.Itoa(int(p.port)))
	}
	return p.host
}

type l2tp3ReceiverKey struct {
	domain   CaptureDomain
	receiver l2tp3Endpoint
	sid      uint32
	udp      bool
}
type l2tp3Proof struct {
	pdu  uint64
	hash [32]byte
	refs []PacketReference
}
type l2tp3Control struct {
	wire        []byte
	src, dst    l2tp3Endpoint
	domain      CaptureDomain
	udp         bool
	mt, ns, nr  uint16
	cid         uint32
	fingerprint [32]byte
	proof       l2tp3Proof
	cost        int64
	conflict    bool
}
type l2tp3ControlDirection struct {
	next    uint16
	pending map[uint16]*l2tp3Control
	history map[uint16][32]byte
	order   []uint16
}
type l2tp3Connection struct {
	id          uint64
	domain      CaptureDomain
	udp         bool
	peers       [2]l2tp3Endpoint
	initialPort uint16
	cid         [2]uint32
	rqHash      [32]byte
	phase       int
	closed      bool
	refusal     string
	caps        [2]map[uint16]bool
	directions  [2]l2tp3ControlDirection
	proofs      []l2tp3Proof
	sessions    []*l2tp3Session
	last        time.Time
}
type l2tp3DataSequence struct {
	known bool
	high  uint32
	seen  map[uint32][32]byte
}
type l2tp3Session struct {
	id        uint64
	c         *l2tp3Connection
	requester int
	outgoing  bool
	ids       [2]uint32
	props     [2]stream_parser.L2TPv3DataContext
	proposal  [2]bool
	phase     int
	closed    bool
	refusal   string
	proofs    []l2tp3Proof
	sequence  [2]l2tp3DataSequence // indexed by receiving peer
	circuit   [2]uint16
}
type l2tp3Binding struct {
	session       *l2tp3Session
	receiver      int
	unknownCookie bool
}
type l2tp3Store struct {
	connections  []*l2tp3Connection
	orphans      []*l2tp3Control
	bindings     map[l2tp3ReceiverKey][]l2tp3Binding
	sessions     int
	bindingCount int
	charge       int64
	disabled     bool
}
type l2tp3Parameters struct {
	assigned, local, remote           uint32
	pw, sublayer, sequencing, circuit uint16
	cookie                            []byte
	caps                              []uint16
	present                           [76]bool
	err                               error
}

func l2tp3Params(m map[string]any) l2tp3Parameters {
	var p l2tp3Parameters
	if m["Ambiguous Parameters"] == true || m["Unparsed Message Type"] == true {
		p.err = sessionContext("ambiguous or unselected L2TPv3 control parameters")
	}
	for _, a := range m["AVPs"].([]map[string]any) {
		t := routingNumber(a, "Attribute Type")
		vendor := routingNumber(a, "Vendor ID")
		if vendor != 0 || a["Value Decoded"] == false {
			critical := vendor == 0 && (t == 60 || t == 61 || t == 62 || t >= 63 && t <= 71)
			if a["Mandatory"] == true || critical {
				p.err = sessionContext("unparsed mandatory or hidden L2TPv3 receiver parameter")
			}
			continue
		}
		if t < uint64(len(p.present)) {
			p.present[t] = true
		}
		switch t {
		case 61:
			p.assigned = uint32(routingNumber(a, "Assigned Control Connection ID"))
		case 62:
			for _, v := range a["Pseudowire Capabilities"].([]map[string]any) {
				p.caps = append(p.caps, uint16(routingNumber(v, "Pseudowire Type")))
			}
		case 63:
			p.local = uint32(routingNumber(a, "Local Session ID"))
		case 64:
			p.remote = uint32(routingNumber(a, "Remote Session ID"))
		case 65:
			p.cookie = bytes.Clone(a["Assigned Cookie"].([]byte))
		case 68:
			p.pw = uint16(routingNumber(a, "Pseudowire Type"))
		case 69:
			p.sublayer = uint16(routingNumber(a, "Sublayer Type"))
		case 70:
			p.sequencing = uint16(routingNumber(a, "Data Sequencing"))
		case 71:
			p.circuit = uint16(routingNumber(a, "Circuit Status"))
		}
	}
	return p
}

// Nr can advance on a retransmission, and its digest changes with it. The
// sequence's semantic identity excludes Nr and Message Digest AVPs, not the
// actual parameters. Reserved flag bits are ignored on receipt (RFC3931).
func l2tp3ControlHash(w []byte, udp bool) [32]byte {
	start := 0
	if !udp {
		start = 4
	}
	h := sha256.New()
	h.Write(w[start+4 : start+10]) // receiver CID and Ns
	for at := start + 12; at+6 <= len(w); {
		n := int(binary.BigEndian.Uint16(w[at:]) & 1023)
		if n < 6 || at+n > len(w) {
			break
		}
		if binary.BigEndian.Uint16(w[at+2:]) != 0 || binary.BigEndian.Uint16(w[at+4:]) != 59 {
			flags := binary.BigEndian.Uint16(w[at:]) & 0xc3ff
			h.Write([]byte{byte(flags >> 8), byte(flags)})
			h.Write(w[at+2 : at+n])
		}
		at += n
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
func (a *binParser) l2tp3State() *l2tp3Store {
	if a.l2tp3 == nil {
		a.l2tp3 = &l2tp3Store{bindings: map[l2tp3ReceiverKey][]l2tp3Binding{}}
	}
	return a.l2tp3
}
func (a *binParser) l2tp3Reserve(s *l2tp3Store, n int64) bool {
	if s.disabled || n < 0 || !a.reserveEvidence(n) {
		s.disabled = true
		return false
	}
	s.charge += n
	return true
}
func (a *binParser) l2tp3Release(s *l2tp3Store, n int64) { s.charge -= n; a.buffered.Add(-n) }
func (a *binParser) l2tp3Limit() int {
	return min(64, sessionCollectionLimit(a.budget.MaxCollectionElements))
}
func l2tp3Direction(c *l2tp3Connection, src, dst l2tp3Endpoint, cid uint32, mt uint16) []int {
	var found []int
	for d := 0; d < 2; d++ {
		if src.host != c.peers[d].host || dst.host != c.peers[1-d].host || cid == 0 || cid != c.cid[1-d] {
			continue
		}
		if c.udp {
			if dst.port != c.peers[1-d].port {
				continue
			}
			if src.port != c.peers[d].port && !(d == 1 && c.cid[1] == 0 && mt == 2) {
				continue
			}
		}
		found = append(found, d)
	}
	return found
}
func (s *l2tp3Store) connection(domain CaptureDomain, udp bool, src, dst l2tp3Endpoint, cid uint32, mt uint16) (*l2tp3Connection, int) {
	var got *l2tp3Connection
	direction := 0
	for _, c := range s.connections {
		if c.domain != domain || c.udp != udp {
			continue
		}
		for _, d := range l2tp3Direction(c, src, dst, cid, mt) {
			if got != nil {
				return nil, 0
			}
			got, direction = c, d
		}
	}
	return got, direction
}
func (a *binParser) l2tp3NewControl(s *l2tp3Store, e *ProtocolEvent, m map[string]any, w []byte, udp bool) *l2tp3Control {
	refs := e.SourceBytes.PacketRefs
	if len(refs) > a.l2tp3Limit() {
		s.disabled = true
		return nil
	}
	cost := int64(len(w) + len(refs)*96 + 512 + len(e.Source) + len(e.Destination))
	if !a.l2tp3Reserve(s, cost) {
		return nil
	}
	return &l2tp3Control{wire: bytes.Clone(w), src: l2tp3EndpointOf(e.Source), dst: l2tp3EndpointOf(e.Destination), domain: e.Domain, udp: udp, mt: uint16(routingNumber(m, "Message Type")), ns: uint16(routingNumber(m, "Ns")), nr: uint16(routingNumber(m, "Nr")), cid: uint32(routingNumber(m, "Control Connection ID")), fingerprint: l2tp3ControlHash(w, udp), proof: l2tp3Proof{e.ID, sha256.Sum256(w), append([]PacketReference(nil), refs...)}, cost: cost}
}
func (a *binParser) l2tp3Proof(s *l2tp3Store, to *[]l2tp3Proof, p l2tp3Proof) bool {
	if len(*to) >= 3 || !a.l2tp3Reserve(s, int64(len(p.refs)*96+128)) {
		return false
	}
	p.refs = append([]PacketReference(nil), p.refs...)
	*to = append(*to, p)
	return true
}
func (a *binParser) l2tp3Bind(s *l2tp3Store, session *l2tp3Session, receiver int, unknown bool) bool {
	c := session.c
	k := l2tp3ReceiverKey{c.domain, c.peers[receiver], session.ids[receiver], c.udp}
	if k.sid == 0 {
		return false
	}
	for _, b := range s.bindings[k] {
		if b.session == session && b.receiver == receiver {
			return true
		}
	}
	if len(s.bindings[k]) >= 4 || s.bindingCount >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.l2tp3Reserve(s, 256) {
		s.disabled = true
		return false
	}
	s.bindings[k] = append(s.bindings[k], l2tp3Binding{session, receiver, unknown})
	s.bindingCount++
	return true
}
func l2tp3FindSession(c *l2tp3Connection, dir int, p l2tp3Parameters) *l2tp3Session {
	var got *l2tp3Session
	for _, v := range c.sessions {
		match := v.ids[dir] == p.local && (p.remote == 0 || v.ids[1-dir] == p.remote)
		if !match && p.remote != 0 {
			match = v.ids[1-dir] == p.remote && v.ids[dir] == 0
		}
		if match {
			// A closed session remains in the data binding history. Prefer an
			// active exchange for later control messages; cookies, rather than
			// this preference, distinguish any reused receiver ID in data.
			if got != nil && got.closed != v.closed {
				if got.closed {
					got = v
				}
				continue
			}
			if got != nil {
				return nil
			}
			got = v
		}
	}
	return got
}

// apply returns false only while an observed counterpart is still missing.
// A structural/semantic contradiction quarantines the known connection rather
// than publishing partially matched receiver parameters.
func (a *binParser) l2tp3Apply(s *l2tp3Store, c *l2tp3Connection, dir int, msg *l2tp3Control, m map[string]any) bool {
	p := l2tp3Params(m)
	refuse := func(why string) bool { c.refusal = why; return true }
	if msg.conflict {
		return refuse("conflicting L2TPv3 control sequence")
	}
	if p.err != nil {
		return refuse(p.err.Error())
	}
	if len(p.caps) > 128 {
		return refuse("L2TPv3 pseudowire capability observation limit")
	}
	if c.closed || c.refusal != "" {
		return true
	}
	mt := msg.mt
	switch mt {
	case 1:
		if dir != 0 || c.phase != 0 || msg.cid != 0 || msg.ns != 0 {
			return refuse("unexpected repeated SCCRQ")
		}
		c.caps[0] = map[uint16]bool{}
		for _, v := range p.caps {
			c.caps[0][v] = true
		}
		c.phase = 1
		a.l2tp3Proof(s, &c.proofs, msg.proof)
	case 2:
		if c.phase < 1 {
			return false
		}
		if dir != 1 || c.phase != 1 || msg.ns != 0 || msg.nr == 0 || p.assigned == 0 {
			return refuse("SCCRP does not match observed SCCRQ")
		}
		c.cid[1] = p.assigned
		c.peers[1].port = msg.src.port
		c.caps[1] = map[uint16]bool{}
		for _, v := range p.caps {
			c.caps[1][v] = true
		}
		c.phase = 2
		a.l2tp3Proof(s, &c.proofs, msg.proof)
	case 3:
		if c.phase < 2 {
			return false
		}
		if dir != 0 || c.phase != 2 || msg.nr == 0 {
			return refuse("SCCCN does not match observed control reply")
		}
		c.phase = 3
		a.l2tp3Proof(s, &c.proofs, msg.proof)
	case 4:
		if c.cid[dir] != 0 && (!p.present[61] || p.assigned != c.cid[dir]) {
			return refuse("StopCCN sender ID missing or changed")
		}
		c.closed = true
		for _, v := range c.sessions {
			v.closed = true
		}
	case 6:
		if c.phase < 3 {
			return false
		}
	case 7, 10:
		if c.phase < 3 {
			return false
		}
		if p.remote != 0 || p.local == 0 || !c.caps[0][p.pw] || !c.caps[1][p.pw] {
			return refuse("new session IDs or pseudowire are inconsistent with observed capabilities")
		}
		for _, v := range c.sessions {
			if v.ids[dir] == p.local && !v.closed {
				return refuse("sender reused an observed L2TPv3 session ID")
			}
		}
		if (p.pw == 4 || p.pw == 5) && p.circuit&2 == 0 {
			m["Conformance Warning"] = "Ethernet request lacks new-circuit status (RFC4719)"
		}
		if s.sessions >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.l2tp3Reserve(s, 8192) {
			s.disabled = true
			return true
		}
		v := &l2tp3Session{id: a.flows.Add(1), c: c, requester: dir, outgoing: mt == 7, phase: 1}
		v.ids[dir] = p.local
		v.props[dir] = stream_parser.L2TPv3DataContext{Cookie: bytes.Clone(p.cookie), Sublayer: p.sublayer, Sequencing: p.sequencing, Pseudowire: p.pw}
		v.proposal[dir] = true
		v.circuit[dir] = p.circuit
		c.sessions = append(c.sessions, v)
		s.sessions++
		a.l2tp3Proof(s, &v.proofs, msg.proof)
		a.l2tp3Bind(s, v, dir, false)
	case 8, 11:
		if c.phase < 3 {
			return false
		}
		v := l2tp3FindSession(c, dir, p)
		if v == nil {
			return false
		}
		if dir == v.requester || v.phase != 1 || v.outgoing != (mt == 8) || p.remote != v.ids[1-dir] || p.local == 0 {
			return refuse("session reply does not match the observed request")
		}
		if p.present[68] && p.pw != v.props[1-dir].Pseudowire {
			return refuse("session reply changed the requested pseudowire")
		}
		pw := v.props[1-dir].Pseudowire
		if (pw == 4 || pw == 5) && p.circuit&2 == 0 {
			m["Conformance Warning"] = "Ethernet reply lacks new-circuit status (RFC4719)"
		}
		v.ids[dir] = p.local
		v.props[dir] = stream_parser.L2TPv3DataContext{Cookie: bytes.Clone(p.cookie), Sublayer: p.sublayer, Sequencing: p.sequencing, Pseudowire: pw}
		v.proposal[dir] = true
		v.circuit[dir] = p.circuit
		v.phase = 2
		a.l2tp3Proof(s, &v.proofs, msg.proof)
		a.l2tp3Bind(s, v, dir, false)
	case 9, 12:
		if c.phase < 3 {
			return false
		}
		v := l2tp3FindSession(c, dir, p)
		if v == nil || v.phase < 2 {
			return false
		}
		wantDir := v.requester
		if v.outgoing {
			wantDir = 1 - v.requester
		}
		if v.phase != 2 || dir != wantDir || v.outgoing != (mt == 9) || p.remote != v.ids[1-dir] || p.local != v.ids[dir] || p.present[65] {
			return refuse("session connected message does not match both observed receiver IDs")
		}
		if p.present[69] {
			v.props[dir].Sublayer = p.sublayer
		}
		if p.present[70] {
			v.props[dir].Sequencing = p.sequencing
		}
		if p.present[71] {
			if (v.props[dir].Pseudowire == 4 || v.props[dir].Pseudowire == 5) && p.circuit&2 != 0 {
				// An incorrect status flag does not change cookie/sublayer byte
				// boundaries. Keep its observed value and expose a warning;
				// passive analysis must not discard subsequent intact traffic.
				m["Conformance Warning"] = "connected Ethernet circuit is marked new (RFC4719)"
			}
			v.circuit[dir] = p.circuit
		}
		v.phase = 3
		a.l2tp3Proof(s, &v.proofs, msg.proof)
	case 14, 15, 16:
		v := l2tp3FindSession(c, dir, p)
		if v == nil {
			return false
		}
		if p.present[65] || p.present[69] || p.present[70] {
			return refuse("late session status cannot replace cookie or sublayer requirements")
		}
		if v.ids[dir] == 0 {
			v.ids[dir] = p.local
			a.l2tp3Bind(s, v, dir, true)
		}
		if mt == 14 {
			v.closed = true
		} else if mt == 16 {
			if !p.present[71] {
				return refuse("SLI lacks circuit status")
			}
			v.circuit[dir] = p.circuit
		}
	default:
		return refuse("unselected L2TPv3 control state transition")
	}
	return true
}

func (a *binParser) l2tp3Queue(s *l2tp3Store, c *l2tp3Connection, dir int, msg *l2tp3Control) string {
	d := &c.directions[dir]
	if d.pending == nil {
		d.pending = map[uint16]*l2tp3Control{}
		d.history = map[uint16][32]byte{}
	}
	delta := uint16(msg.ns - d.next)
	if delta >= 0x8000 {
		if old, ok := d.history[msg.ns]; ok {
			if old != msg.fingerprint {
				c.refusal = "conflicting retained L2TPv3 control retransmission"
			}
			a.l2tp3Release(s, msg.cost)
			return "retransmission"
		}
		a.l2tp3Release(s, msg.cost)
		return "sequence older than retained observation window"
	}
	if int(delta) >= a.l2tp3Limit() {
		c.refusal = "L2TPv3 control sequence gap exceeds bounded window"
		a.l2tp3Release(s, msg.cost)
		return c.refusal
	}
	if old := d.pending[msg.ns]; old != nil {
		if old.fingerprint != msg.fingerprint {
			c.refusal = "conflicting pending L2TPv3 control sequence"
		}
		a.l2tp3Release(s, msg.cost)
		return "pending retransmission"
	}
	d.pending[msg.ns] = msg
	return "pending ordered control"
}
func (a *binParser) l2tp3Adopt(s *l2tp3Store, c *l2tp3Connection) bool {
	changed := false
	for i := 0; i < len(s.orphans); {
		msg := s.orphans[i]
		if msg.domain == c.domain && msg.udp == c.udp {
			owner, dir := s.connection(msg.domain, msg.udp, msg.src, msg.dst, msg.cid, msg.mt)
			if owner == c {
				s.orphans = append(s.orphans[:i], s.orphans[i+1:]...)
				a.l2tp3Queue(s, c, dir, msg)
				changed = true
				continue
			}
		}
		i++
	}
	return changed
}
func (a *binParser) l2tp3Drain(s *l2tp3Store, c *l2tp3Connection) {
	for round := 0; round < 2*a.l2tp3Limit()+2; round++ {
		changed := a.l2tp3Adopt(s, c)
		for dir := 0; dir < 2; dir++ {
			d := &c.directions[dir]
			msg := d.pending[d.next]
			if msg == nil {
				continue
			}
			m, err := stream_parser.DecodeL2TPv3Wire(msg.wire, msg.udp, a.budget.MaxCollectionElements)
			if err != nil {
				c.refusal = "retained control wire could not be revalidated"
			} else if !a.l2tp3Apply(s, c, dir, msg, m) {
				continue
			}
			delete(d.pending, d.next)
			d.history[d.next] = msg.fingerprint
			d.order = append(d.order, d.next)
			if len(d.order) > a.l2tp3Limit() {
				delete(d.history, d.order[0])
				d.order = d.order[1:]
			}
			d.next++
			a.l2tp3Release(s, msg.cost)
			changed = true
		}
		if !changed {
			return
		}
	}
}
func l2tp3ProofValues(proofs ...[]l2tp3Proof) []map[string]any {
	var out []map[string]any
	for _, group := range proofs {
		for _, p := range group {
			out = append(out, map[string]any{"PDU": p.pdu, "Wire SHA256": fmt.Sprintf("%x", p.hash), "Packet Refs": append([]PacketReference(nil), p.refs...)})
		}
	}
	return out
}
func l2tp3ConnectionValues(m map[string]any, c *l2tp3Connection) {
	m["Observed Control Connection"] = c.id
	m["Control Setup Complete"] = c.phase == 3
	m["Control Retired"] = c.closed
	m["Control Receiver IDs"] = []any{uint64(c.cid[0]), uint64(c.cid[1])}
	m["Association Scope"] = "observed base exchanges; peer identity and control authentication not verified"
	if c.refusal != "" {
		m["Association Diagnostic"] = c.refusal
	}
}

func l2tp3SessionValues(m map[string]any, v *l2tp3Session) {
	l2tp3ConnectionValues(m, v.c)
	m["Observed Session"] = v.id
	m["Session Receiver IDs"] = []any{uint64(v.ids[0]), uint64(v.ids[1])}
	m["Session Setup Complete"] = v.phase == 3
	m["Session Retired"] = v.closed
	m["Context Proof"] = l2tp3ProofValues(v.c.proofs, v.proofs)
	m["Observed Circuit Status"] = []any{uint64(v.circuit[0]), uint64(v.circuit[1])}
	m["Connection Context"] = "partly observed control/session exchange; receiver admission unavailable"
	if v.c.phase == 3 && v.phase == 3 {
		m["Connection Context"] = "complete observed control and session exchanges; unverified peer"
	}
}

func (a *binParser) l2tp3ControlObserve(e *ProtocolEvent, m map[string]any, w []byte, udp bool) {
	s := a.l2tp3State()
	src, dst := l2tp3EndpointOf(e.Source), l2tp3EndpointOf(e.Destination)
	mt := uint16(routingNumber(m, "Message Type"))
	cid := uint32(routingNumber(m, "Control Connection ID"))
	if s.disabled {
		m["Association Diagnostic"] = "L2TPv3 observation history exhausted"
		return
	}
	var c *l2tp3Connection
	dir := 0
	if mt == 1 {
		p := l2tp3Params(m)
		if p.err != nil || cid != 0 || routingNumber(m, "Ns") != 0 || p.assigned == 0 || len(p.caps) > 128 {
			m["Association Diagnostic"] = "SCCRQ cannot establish selected receiver context"
			return
		}
		hash := l2tp3ControlHash(w, udp)
		for _, old := range s.connections {
			if old.domain == e.Domain && old.udp == udp && old.peers[0] == src && old.peers[1].host == dst.host && old.initialPort == dst.port && old.cid[0] == p.assigned {
				if old.rqHash != hash {
					old.refusal = "reused control receiver ID without a distinguishable generation"
				}
				c = old
				break
			}
		}
		if c == nil {
			if len(s.connections) >= min(256, sessionCollectionLimit(a.budget.MaxCollectionElements)) || !a.l2tp3Reserve(s, int64(16384+len(src.host)+len(dst.host)+len(e.Domain.Encapsulation))) {
				s.disabled = true
				m["Association Diagnostic"] = "control connection history budget"
				return
			}
			c = &l2tp3Connection{id: a.flows.Add(1), domain: e.Domain, udp: udp, peers: [2]l2tp3Endpoint{src, dst}, initialPort: dst.port, cid: [2]uint32{p.assigned, 0}, rqHash: hash}
			s.connections = append(s.connections, c)
		}
	} else {
		c, dir = s.connection(e.Domain, udp, src, dst, cid, mt)
	}
	if c == nil {
		// Only setup/session messages are retained for counterpart recovery.
		// Midstream HELLO/ACK never creates a connection or an unbounded cache.
		if mt != 2 && mt != 3 && mt != 7 && mt != 8 && mt != 9 && mt != 10 && mt != 11 && mt != 12 {
			return
		}
		fingerprint := l2tp3ControlHash(w, udp)
		for _, old := range s.orphans {
			if old.domain == e.Domain && old.udp == udp && old.src == src && old.dst == dst && old.cid == cid && old.ns == uint16(routingNumber(m, "Ns")) {
				if old.fingerprint != fingerprint {
					old.conflict = true
				}
				m["Control Observation"] = "unbound retransmission"
				return
			}
		}
		if len(s.orphans) >= a.l2tp3Limit() {
			s.disabled = true
			m["Association Diagnostic"] = "orphan control history budget"
			return
		}
		if msg := a.l2tp3NewControl(s, e, m, w, udp); msg != nil {
			s.orphans = append(s.orphans, msg)
		}
		m["Control Observation"] = "counterpart setup not yet observed"
		return
	}
	e.FlowID, e.Direction = c.id, dir
	if e.Timestamp.After(c.last) {
		c.last = e.Timestamp
	}
	if m["Zero Length Body"] == true || mt == 20 {
		m["Control Observation"] = "acknowledgment; no sequence advancement"
		l2tp3ConnectionValues(m, c)
		return
	}
	if c.closed || c.refusal != "" {
		l2tp3ConnectionValues(m, c)
		return
	}
	if msg := a.l2tp3NewControl(s, e, m, w, udp); msg != nil {
		m["Control Observation"] = a.l2tp3Queue(s, c, dir, msg)
		a.l2tp3Drain(s, c)
	}
	// The retained control is decoded again while draining. Project any
	// conformance warning onto this original immutable event as well.
	p := l2tp3Params(m)
	if p.present[63] {
		if v := l2tp3FindSession(c, dir, p); v != nil {
			l2tp3SessionValues(m, v)
		}
	}
	if (mt == 7 || mt == 10) && (p.pw == 4 || p.pw == 5) && p.circuit&2 == 0 {
		m["Conformance Warning"] = "Ethernet request lacks new-circuit status (RFC4719)"
	}
	if mt == 9 || mt == 12 || mt == 8 || mt == 11 {
		if v := l2tp3FindSession(c, dir, p); v != nil {
			pw := v.props[v.requester].Pseudowire
			if (pw == 4 || pw == 5) && p.present[71] && ((mt == 9 || mt == 12) == (p.circuit&2 != 0)) {
				m["Conformance Warning"] = "Ethernet circuit status new/existing flag is inconsistent with this message (RFC4719)"
			}
		}
	}
	l2tp3ConnectionValues(m, c)
	if s.disabled {
		m["Association Diagnostic"] = "L2TPv3 observation history exhausted; receiver association unavailable"
	}
	m["Control Context Proof"] = l2tp3ProofValues(c.proofs)
}

func (a *binParser) l2tp3DataResolve(e *ProtocolEvent, m map[string]any, w []byte, udp bool) (map[string]any, []byte, string, error) {
	s := a.l2tp3
	if s == nil {
		return m, nil, "", nil
	}
	if s.disabled {
		return m, nil, "", sessionContext("L2TPv3 retained history exhausted; no receiver context inherited")
	}
	src, dst := l2tp3EndpointOf(e.Source), l2tp3EndpointOf(e.Destination)
	sid := uint32(routingNumber(m, "Session ID"))
	key := l2tp3ReceiverKey{e.Domain, dst, sid, udp}
	var matches []l2tp3Binding
	at := int(routingNumber(m, "Payload Offset"))
	for _, b := range s.bindings[key] {
		v := b.session
		if v.c.peers[1-b.receiver] != src {
			continue
		}
		cookie := v.props[b.receiver].Cookie
		if b.unknownCookie || len(w)-at >= len(cookie) && bytes.Equal(w[at:at+len(cookie)], cookie) {
			matches = append(matches, b)
		}
	}
	if len(matches) == 0 {
		if len(s.bindings[key]) > 0 {
			var candidates []map[string]any
			for _, b := range s.bindings[key] {
				if b.session.c.peers[1-b.receiver] != src {
					continue
				}
				cookie := b.session.props[b.receiver].Cookie
				item := map[string]any{"Observed Session": b.session.id, "Expected Receiver Cookie": fmt.Sprintf("%x", cookie), "Cookie Length": uint64(len(cookie)), "Retired": b.session.closed, "Cookie Proposal Unknown": b.unknownCookie}
				if len(w)-at >= len(cookie) {
					item["Candidate Wire Prefix"] = fmt.Sprintf("%x", w[at:at+len(cookie)])
				}
				candidates = append(candidates, item)
			}
			m["Receiver Candidates"] = candidates
			return m, nil, "", sessionContext("receiver cookie or peer does not match an observed L2TPv3 proposal")
		}
		return m, nil, "", nil
	}
	if len(matches) != 1 {
		return m, nil, "", sessionContext("reused receiver Session ID has indistinguishable cookie contexts")
	}
	b := matches[0]
	v := b.session
	c := v.c
	e.FlowID, e.Direction = v.id, 1-b.receiver
	context := func(x map[string]any) {
		l2tp3SessionValues(x, v)
		x["Receiver Proposal Side"] = uint64(b.receiver)
	}
	context(m)
	if b.unknownCookie || v.phase != 3 || !v.proposal[0] || !v.proposal[1] {
		return m, nil, "", sessionContext("L2TPv3 session setup was not fully observed")
	}
	if c.closed || v.closed || c.refusal != "" || v.refusal != "" {
		return m, nil, "", sessionContext("L2TPv3 receiver context is retired or quarantined")
	}
	decoded, err := stream_parser.DecodeL2TPv3Data(w, udp, v.props[b.receiver], a.budget.MaxCollectionElements)
	if decoded == nil {
		decoded = m
	}
	context(decoded)
	if err != nil {
		return decoded, nil, "", observedWireError(err)
	}
	if decoded["Sequence Meaningful"] == true {
		seq := uint32(routingNumber(decoded, "Sequence Number"))
		d := &v.sequence[b.receiver]
		hash := sha256.Sum256(decoded["Ethernet Bytes"].([]byte))
		if d.seen == nil {
			d.seen = map[uint32][32]byte{}
		}
		if old, ok := d.seen[seq]; ok {
			if old != hash {
				v.refusal = "conflicting sequenced L2TPv3 payload"
				decoded["Payload Admission"] = "quarantined"
				return decoded, nil, "", sessionContext(v.refusal)
			}
			decoded["Sequence Observation"] = "identical repeated wire frame"
		} else if !d.known {
			d.known = true
			d.high = seq
			decoded["Sequence Observation"] = "first observed sequence"
		} else {
			delta := (seq - d.high) & 0xffffff
			if delta > 0 && delta < 0x800000 {
				decoded["Sequence Observation"] = "forward"
				decoded["Unobserved Sequence Count"] = uint64(delta - 1)
				d.high = seq
			} else if (d.high-seq)&0xffffff < uint32(a.l2tp3Limit()) {
				decoded["Sequence Observation"] = "reordered whole frame"
			} else {
				return decoded, nil, "", sessionContext("L2TPv3 data sequence outside retained observation window")
			}
		}
		d.seen[seq] = hash
		for n := range d.seen {
			if (d.high-n)&0xffffff >= uint32(a.l2tp3Limit()) {
				delete(d.seen, n)
			}
		}
	}
	decoded["Payload Admission"] = "selected wire frame; no endpoint delivery or authentication claim"
	suffix := fmt.Sprintf("/l2tp3:%s~%s:c%d~%d:s%d~%d:g%d", c.peers[0].String(), c.peers[1].String(), c.cid[0], c.cid[1], v.ids[0], v.ids[1], v.id)
	return decoded, bytes.Clone(decoded["Ethernet Bytes"].([]byte)), suffix, nil
}

// Called with l2tp3Mu held. This selector never identifies arbitrary UDP data
// from a port/header alone; it requires a retained receiver ID or Decode-As.
func (a *binParser) l2tp3UDPAdmission(domain CaptureDomain, src, dst l2tp3Endpoint, w []byte) string {
	if len(w) < 2 || w[1]&15 != 3 {
		return ""
	}
	if a.datagramDecodeAs[src.port] == "l2tp" || a.datagramDecodeAs[dst.port] == "l2tp" {
		return "explicit-decode-as"
	}
	flags := binary.BigEndian.Uint16(w)
	if flags&0x8000 != 0 {
		if src.port == 1701 || dst.port == 1701 {
			return "complete-control-grammar-and-service-hint"
		}
		if len(w) < 12 {
			return ""
		}
		cid := binary.BigEndian.Uint32(w[4:])
		if a.l2tp3 != nil {
			if c, _ := a.l2tp3.connection(domain, true, src, dst, cid, 2); c != nil {
				return "observed-control-receiver"
			}
		}
		// A complete setup has multiple required identity/capability AVPs;
		// a short ACK/HELLO on an arbitrary port is insufficient evidence.
		if len(w) >= 52 {
			if m, err := stream_parser.DecodeL2TPv3Wire(w, true, a.budget.MaxCollectionElements); err == nil && (routingNumber(m, "Message Type") == 1 || routingNumber(m, "Message Type") == 2) {
				p := l2tp3Params(m)
				if p.err == nil {
					return "complete-setup-grammar"
				}
			}
		}
		return ""
	}
	if len(w) < 8 || a.l2tp3 == nil {
		return ""
	}
	k := l2tp3ReceiverKey{domain, dst, binary.BigEndian.Uint32(w[4:]), true}
	for _, b := range a.l2tp3.bindings[k] {
		if b.session.c.peers[1-b.receiver] == src {
			return "observed-session-receiver"
		}
	}
	return ""
}
func (a *binParser) closeL2TPv3() []*ProtocolEvent {
	a.l2tp3Mu.Lock()
	defer a.l2tp3Mu.Unlock()
	s := a.l2tp3
	if s == nil {
		return nil
	}
	var out []*ProtocolEvent
	for _, c := range s.connections {
		pending := len(c.directions[0].pending) + len(c.directions[1].pending)
		unfinished := c.phase < 3
		for _, v := range c.sessions {
			unfinished = unfinished || !v.closed && v.phase < 3
		}
		if !c.closed && (pending > 0 || unfinished) {
			transport := "network"
			if c.udp {
				transport = "udp"
			}
			m := map[string]any{"Version": uint64(3), "Pending Controls": uint64(pending), "Context Proof": l2tp3ProofValues(c.proofs)}
			l2tp3ConnectionValues(m, c)
			e := &ProtocolEvent{Protocol: "l2tp", Transport: transport, Profile: "l2tp3-observed-association-close", FlowID: c.id, Domain: c.domain, Timestamp: c.last, Source: c.peers[0].String(), Destination: c.peers[1].String(), Status: "incomplete", Completeness: "incomplete", Error: "observed L2TPv3 control/session exchange unfinished at capture close", Session: m, nativeSessionMessage: true, SourceBytes: ByteSource{Kind: "association"}}
			e.semanticFields, e.semanticSession = cloneSession(m), cloneSession(m)
			e.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: e.Error}
			out = append(out, e)
			a.incomplete.Add(1)
		}
	}
	a.buffered.Add(-s.charge)
	a.l2tp3 = nil
	return out
}
