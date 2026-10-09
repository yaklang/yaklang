package pcaputil

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"time"
)

// SH(NA)-080956ENG-M: connected-station binary 3E/4E Self Test 0619/0000.
// MELSOFT's private UDP 5560 messages are a different wire format. UDP replies
// have no command on success; they require observed request context, never a
// payload-length guess. TCP uses the separate stream profile; no ASCII, other
// commands or device identity claim.
func slmpStart(w []byte) bool {
	return len(w) >= 2 && w[1] == 0 && (w[0] == 0x50 || w[0] == 0x54 || w[0] == 0xd0 || w[0] == 0xd4)
}
func slmpError(kind ProtocolErrorKind, s string) error { return protocolError(kind, "SLMP %s", s) }

type slmpMessage struct {
	request bool
	frame   int
	serial  uint16
	route   [5]byte
	body    []byte
	data    []byte
	fields  map[string]any
}

func decodeSLMPMessage(w []byte) (*slmpMessage, error) {
	bad := func(s string) (*slmpMessage, error) { return nil, slmpError(ErrMalformedMessage, s) }
	unsupported := func(s string) (*slmpMessage, error) { return nil, slmpError(ErrUnsupportedFeature, s) }
	if len(w) < 2 {
		return bad("subheader is truncated")
	}
	if !slmpStart(w) {
		if w[0] == 0x50 || w[0] == 0x54 || w[0] == 0xd0 || w[0] == 0xd4 {
			return bad("binary subheader reserved byte is nonzero")
		}
		return unsupported("wire format is outside binary 3E/4E")
	}
	m := &slmpMessage{request: w[0] == 0x50 || w[0] == 0x54, frame: 3}
	at := 2
	if w[0] == 0x54 || w[0] == 0xd4 {
		m.frame = 4
		at = 6
		if len(w) < at {
			return bad("4E serial/reserved fields are truncated")
		}
		m.serial = binary.LittleEndian.Uint16(w[2:4])
		if w[4] != 0 || w[5] != 0 {
			return bad("4E reserved fields are nonzero")
		}
	}
	if len(w) < at+7 {
		return bad("route/data length is truncated")
	}
	copy(m.route[:], w[at:at+5])
	n := int(binary.LittleEndian.Uint16(w[at+5 : at+7]))
	if n != len(w)-at-7 {
		return bad("data length differs from complete frame")
	}
	m.body = w[at+7:]
	f := map[string]any{"Observation": "unverified-slmp-self-test", "Frame Type": "3E", "Encoding": "binary", "Role": "response", "Network Number": m.route[0], "Station Number": m.route[1], "Module IO Number": binary.LittleEndian.Uint16(m.route[2:4]), "Multidrop Station Number": m.route[4], "Data Length": n, "Command": uint16(0x0619), "Subcommand": uint16(0)}
	if m.frame == 4 {
		f["Frame Type"] = "4E"
		f["Serial Number"] = m.serial
	}
	m.fields = f
	b := m.body
	if m.request {
		f["Role"] = "request"
		if len(b) < 8 {
			return bad("Self Test timer/command/count is truncated")
		}
		cmd, sub := binary.LittleEndian.Uint16(b[2:4]), binary.LittleEndian.Uint16(b[4:6])
		if cmd != 0x0619 || sub != 0 {
			return unsupported("command/subcommand is outside Self Test")
		}
		if m.route[0] != 0 || m.route[1] != 0xff {
			return unsupported("Self Test is limited to the connected station")
		}
		f["Monitoring Timer"] = binary.LittleEndian.Uint16(b[:2])
		b = b[6:]
	} else {
		if len(b) < 2 {
			return bad("end code is truncated")
		}
		end := binary.LittleEndian.Uint16(b[:2])
		f["End Code"] = end
		b = b[2:]
		if end != 0 {
			if len(b) < 9 {
				return bad("error station/command is truncated")
			}
			if binary.LittleEndian.Uint16(b[5:7]) != 0x0619 || binary.LittleEndian.Uint16(b[7:9]) != 0 {
				return nil, slmpError(ErrContextRequired, "error response names a different command")
			}
			f["Error Station"] = map[string]any{"Network Number": b[0], "Station Number": b[1], "Module IO Number": binary.LittleEndian.Uint16(b[2:4]), "Multidrop Station Number": b[4]}
			f["Error Data"] = append([]byte{}, b[9:]...)
			return m, nil
		}
	}
	if len(b) < 2 {
		return bad("loopback count is truncated")
	}
	n = int(binary.LittleEndian.Uint16(b[:2]))
	if n < 1 || n > 960 || n != len(b)-2 {
		return bad("loopback count must match 1..960 octets")
	}
	for _, c := range b[2:] {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return bad("loopback data is outside 0..9/A..F")
		}
	}
	m.data = b[2:]
	f["Loopback Length"] = n
	f["Loopback Data"] = append([]byte(nil), m.data...)
	return m, nil
}
func slmpRequestEvidence(w []byte) bool {
	m, err := decodeSLMPMessage(w)
	return err == nil && m.request
}

type slmpRequest struct {
	wire, data []byte
	frame      int
	serial     uint16
	route      [5]byte
	id         uint64
}
type binSLMP struct {
	pending           *slmpRequest
	ambiguous, used3E bool
	seen              map[uint16]bool
}

func (s *binSLMP) consume(m *slmpMessage, w []byte, dir int, id uint64, limit int) (map[string]any, uint64, error) {
	context := func(why string) (map[string]any, uint64, error) { return nil, 0, slmpError(ErrContextRequired, why) }
	if s.ambiguous {
		return context("overlap/reuse requires a new idle conversation")
	}
	if m.request {
		if dir != 0 {
			return context("request direction differs from observed requester")
		}
		if s.pending != nil {
			if !bytes.Equal(s.pending.wire, w) {
				s.pending = nil
				s.ambiguous = true
				return context("more than one distinct outstanding request")
			}
			m.fields["Association"] = "retransmitted-request"
			return m.fields, 0, nil
		}
		if m.frame == 3 && s.used3E || m.frame == 4 && s.seen[m.serial] {
			s.ambiguous = true
			return context("unsequenced exchange or serial reused within conversation")
		}
		if m.frame == 4 {
			if len(s.seen) >= limit {
				return nil, 0, slmpError(ErrResourceExceeded, "retained serial limit")
			}
			if s.seen == nil {
				s.seen = make(map[uint16]bool)
			}
			s.seen[m.serial] = true
		} else {
			s.used3E = true
		}
		s.pending = &slmpRequest{bytes.Clone(w), bytes.Clone(m.data), m.frame, m.serial, m.route, id}
		m.fields["Association"] = "request"
		return m.fields, 0, nil
	}
	r := s.pending
	if dir != 1 || r == nil || r.frame != m.frame || r.serial != m.serial || r.route != m.route {
		return context("reply has no matching direction/route/frame/serial request")
	}
	if m.fields["End Code"].(uint16) == 0 && !bytes.Equal(r.data, m.data) {
		return nil, 0, slmpError(ErrMalformedMessage, "loopback response differs from request data")
	}
	s.pending = nil
	m.fields["Association"] = "response"
	return m.fields, r.id, nil
}

const slmpIdleTTL = 30 * time.Second

func (a *binParser) decodeSLMPDatagram(e *ProtocolEvent, w []byte, explicit bool) bool {
	key := binUDPKey{"slmp/" + e.Source, "slmp/" + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	store := a.udpSessions
	existing := store != nil && store.entries[key] != nil
	if !explicit && !(slmpStart(w) && (existing || slmpRequestEvidence(w))) {
		a.udpMu.Unlock()
		return false
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "slmp", "slmp-binary-self-test", "wire-signature", "message"
	if explicit {
		e.Admission = "explicit-decode-as"
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if store == nil {
		store = &binUDPStore{entries: make(map[binUDPKey]*list.Element), clock: e.Timestamp}
		a.udpSessions = store
	}
	if e.Timestamp.After(store.clock) {
		store.clock = e.Timestamp
	}
	for el := store.lru.Front(); el != nil; {
		next := el.Next()
		entry := el.Value.(*binUDPEntry)
		if entry.flow.slmp != nil && store.clock.Sub(entry.touched) >= slmpIdleTTL {
			entry.flow.closeSession()
			delete(store.entries, entry.key)
			store.lru.Remove(el)
		}
		el = next
	}
	var m *slmpMessage
	var err error
	budgetExceeded := len(w) > a.budget.MaxFrameBytes
	if budgetExceeded {
		err = slmpError(ErrResourceExceeded, "frame exceeds byte budget")
	} else {
		m, err = decodeSLMPMessage(w)
	}
	el := store.entries[key]
	// With no observed flow, the subheader still identifies a response role.
	// An existing flow uses its observed endpoint direction instead.
	if slmpStart(w) && (w[0] == 0xd0 || w[0] == 0xd4) {
		e.Direction = 1
	}
	if err == nil && el == nil {
		if !m.request {
			err = slmpError(ErrContextRequired, "successful reply lacks command context")
		} else if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = slmpError(ErrResourceExceeded, "UDP conversation limit")
		} else {
			f := &binFlow{a: a, id: a.flows.Add(1), protocol: "slmp", slmp: &binSLMP{}, endpoints: [2]string{e.Source, e.Destination}}
			if err = f.reserveSession(512 + int64(len(w))*3 + 64); err == nil {
				el = store.lru.PushBack(&binUDPEntry{key, f, store.clock})
				store.entries[key] = el
			}
		}
	}
	if el != nil {
		entry := el.Value.(*binUDPEntry)
		entry.touched = store.clock
		store.lru.MoveToBack(el)
		f := entry.flow
		if budgetExceeded {
			f.slmp.pending = nil
			f.slmp.ambiguous = true
			retained := 512 + 64*int64(len(f.slmp.seen))
			if retained < f.sessionBytes {
				a.buffered.Add(retained - f.sessionBytes)
				f.sessionBytes = retained
			}
		}
		e.FlowID = f.id
		e.Direction = 0
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
		// Even an unsupported new command can produce an indistinguishable 3E
		// success reply. Retire the earlier slot instead of associating by length.
		if err != nil && e.Direction == 0 && slmpStart(w) && (w[0] == 0x50 || w[0] == 0x54) && f.slmp.pending != nil {
			f.slmp.pending = nil
			f.slmp.ambiguous = true
		}
		if err == nil {
			n := len(f.slmp.seen)
			if m.request && m.frame == 4 && f.slmp.pending == nil && !f.slmp.seen[m.serial] {
				n++
			}
			if n > a.budget.MaxCollectionElements {
				err = slmpError(ErrResourceExceeded, "retained serial limit")
			} else if err = f.reserveSession(512 + int64(n)*64 + int64(len(w))*3); err == nil {
				e.Session, e.ResponseTo, err = f.slmp.consume(m, w, e.Direction, e.ID, a.budget.MaxCollectionElements)
			}
		}
		// Resource denial does not make a distinct on-wire request disappear. A 3E
		// error response has no echo/serial to distinguish it from the old slot.
		// Preserve the resource error, but retire that now-ambiguous association.
		if err != nil && m != nil && m.request && e.Direction == 0 && f.slmp.pending != nil && !bytes.Equal(f.slmp.pending.wire, w) {
			f.slmp.pending = nil
			f.slmp.ambiguous = true
		}
	}
	a.udpMu.Unlock()
	if err == nil {
		e.semanticFields = cloneSession(e.Session)
	}
	if budgetExceeded {
		a.limited.Add(uint64(len(w)))
		w = nil
	}
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}
