package pcaputil

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// D301401X012 Jan-2022: serial-carried local RDB ReadByName, not native BSAP-IP.
// Serial belongs to the link layer; the application sequence associates replies.
// Value-only responses lack a type. Analog numeric interpretation/Q-bit semantics
// and other field selections require a separate profile, never a length guess.
func bsapCRC(w []byte) uint16 {
	c := uint16(0xffff)
	for _, b := range w {
		c ^= uint16(b)
		for i := 0; i < 8; i++ {
			if c&1 != 0 {
				c = c>>1 ^ 0x8408
			} else {
				c >>= 1
			}
		}
	}
	return c
}

func bsapMalformed(s string) error   { return protocolError(ErrMalformedMessage, "BSAP %s", s) }
func bsapUnsupported(s string) error { return protocolError(ErrUnsupportedFeature, "BSAP %s", s) }
func bsapContext(s string) error     { return protocolError(ErrContextRequired, "BSAP %s", s) }
func bsapStart(w []byte) bool        { return len(w) >= 2 && w[0] == 0x10 && w[1] == 2 }

func bsapBody(w []byte) ([]byte, error) {
	if !bsapStart(w) {
		return nil, bsapMalformed("missing DLE STX")
	}
	out := make([]byte, 0, len(w))
	for i := 2; i < len(w); i++ {
		if w[i] != 0x10 {
			out = append(out, w[i])
			continue
		}
		i++
		if i >= len(w) {
			return nil, bsapMalformed("truncated DLE escape")
		}
		switch w[i] {
		case 0x10:
			out = append(out, 0x10)
		case 3:
			if i+3 != len(w) {
				return nil, bsapMalformed("incomplete CRC or bytes after frame")
			}
			// ETX is included, the first DLE of escapes/terminator is excluded.
			out = append(out, 3)
			if bsapCRC(out) != binary.LittleEndian.Uint16(w[i+1:]) {
				return nil, bsapMalformed("CRC mismatch")
			}
			return out[:len(out)-1], nil
		default:
			return nil, bsapMalformed("invalid DLE escape")
		}
	}
	return nil, bsapMalformed("missing DLE ETX")
}

type bsapMessage struct {
	body     []byte
	fields   map[string]any
	request  bool
	serial   byte
	sequence uint16
	selector byte
	names    []string
}

func bsapASCII(w []byte) bool {
	for _, b := range w {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

func decodeBSAPMessage(w []byte, limit int) (*bsapMessage, error) {
	b, err := bsapBody(w)
	if err != nil {
		return nil, err
	}
	if len(b) < 8 {
		return nil, bsapMalformed("local RDB header/payload is truncated")
	}
	if b[0]&0x80 != 0 {
		return nil, bsapUnsupported("global routing is outside the local profile")
	}
	if b[1] == 0 {
		return nil, bsapMalformed("zero link serial is reserved")
	}
	request := b[2] == 0xa0 && b[5] == 3
	if !request && !(b[2] == 3 && b[5] == 0xa0) {
		return nil, bsapUnsupported("function pair is outside RDB/PEI")
	}
	m := &bsapMessage{body: b, request: request, serial: b[1], sequence: binary.LittleEndian.Uint16(b[3:5])}
	m.fields = map[string]any{"Local Address": b[0], "Serial": b[1], "Destination Function": b[2], "Sequence": m.sequence, "Source Function": b[5], "Node Status": b[6], "CRC": binary.LittleEndian.Uint16(w[len(w)-2:]), "CRC Valid": true, "Observation": "unverified-serial-rdb", "Transport": "UDP"}
	if request {
		if b[0] == 0 {
			return nil, bsapMalformed("local request has no slave address")
		}
		if b[7] != 4 {
			return nil, bsapUnsupported("only Read Signal By Name is implemented")
		}
		if len(b) < 12 {
			return nil, bsapMalformed("ReadByName selector/security/count is truncated")
		}
		if b[8] != 1 || b[9] != 0x40 && b[9] != 0x80 && b[9] != 0xc0 {
			return nil, bsapUnsupported("only FS1 type/value selections are implemented")
		}
		if b[10] != 0 && b[10] != 1 && b[10] != 3 && b[10] != 7 && b[10] != 15 {
			return nil, bsapMalformed("invalid reported security level")
		}
		n := int(b[11])
		if n == 0 {
			return nil, bsapMalformed("empty ReadByName request")
		}
		if n > limit {
			return nil, protocolError(ErrResourceExceeded, "BSAP name count exceeds collection budget")
		}
		at := 12
		for i := 0; i < n; i++ {
			end := bytes.IndexByte(b[at:], 0)
			if end <= 0 {
				return nil, bsapMalformed("missing or empty terminated signal name")
			}
			name := b[at : at+end]
			if !bsapASCII(name) {
				return nil, bsapMalformed("signal name is not printable ASCII")
			}
			m.names = append(m.names, string(name))
			at += end + 1
		}
		if at != len(b) {
			return nil, bsapMalformed("bytes follow requested names")
		}
		m.selector = b[9]
		m.fields["Role"], m.fields["Function"], m.fields["Field Select Selector"], m.fields["Field Select Byte 1"] = "request", byte(4), b[8], b[9]
		m.fields["Security Level"], m.fields["Element Count"], m.fields["Names"] = b[10], n, append([]string(nil), m.names...)
	} else {
		if b[0] != 0 {
			return nil, bsapMalformed("local reply is not addressed to master")
		}
		if len(b) < 9 {
			return nil, bsapMalformed("response status/count is truncated")
		}
		m.fields["Role"], m.fields["Request Status"], m.fields["Element Count"] = "response", b[7], int(b[8])
	}
	return m, nil
}

func bsapEvidence(w []byte) bool {
	b, err := bsapBody(w)
	return err == nil && len(b) >= 9 && b[0]&0x80 == 0 && b[1] != 0 && (b[2] == 0xa0 && b[5] == 3 || b[2] == 3 && b[5] == 0xa0)
}

type bsapRequest struct {
	sequence uint16
	selector byte
	names    []string
	digest   [32]byte
	eventID  uint64
}
type binBSAP struct {
	pending   *bsapRequest
	ambiguous bool
	seen      map[uint16]bool
}

func (s *binBSAP) consume(m *bsapMessage, dir int, eventID uint64, limit int) (map[string]any, uint64, error) {
	if s.ambiguous {
		return nil, 0, bsapContext("overlapping requests require a new idle conversation")
	}
	if m.request {
		if dir != 0 {
			return nil, 0, bsapContext("request direction differs from observed requester")
		}
		digest := sha256.Sum256(append([]byte{m.body[0]}, m.body[2:]...)) // link serial may change on retry
		if s.pending != nil {
			if s.pending.digest != digest {
				s.pending = nil
				s.ambiguous = true
				return nil, 0, bsapContext("overlapping requests exceed one unambiguous slot")
			}
			m.fields["Association"] = "retransmitted-request"
			return m.fields, 0, nil
		}
		if s.seen[m.sequence] {
			s.ambiguous = true
			return nil, 0, bsapContext("application sequence reused within the observed conversation")
		}
		if len(s.seen) >= limit {
			return nil, 0, protocolError(ErrResourceExceeded, "BSAP retained sequence limit")
		}
		if s.seen == nil {
			s.seen = make(map[uint16]bool)
		}
		s.seen[m.sequence] = true
		s.pending = &bsapRequest{m.sequence, m.selector, append([]string(nil), m.names...), digest, eventID}
		m.fields["Association"] = "request"
		return m.fields, 0, nil
	}
	r := s.pending
	if dir != 1 || r == nil || r.sequence != m.sequence {
		return nil, 0, bsapContext("reply has no matching endpoint/direction/application sequence")
	}
	status, n := m.body[7], int(m.body[8])
	p := m.body[9:]
	if n > limit {
		return nil, 0, protocolError(ErrResourceExceeded, "BSAP response count exceeds collection budget")
	}
	if status&0x7e != 0 && status&0x80 == 0 {
		return nil, 0, bsapMalformed("error bits without error indicator")
	}
	if status&1 != 0 {
		return nil, 0, bsapUnsupported("continued response requires a separate continuation profile")
	}
	var elements []map[string]any
	if status&0x7e != 0 {
		if n != 0 || len(p) != 0 {
			return nil, 0, bsapMalformed("request-level error has response elements")
		}
	} else {
		if n != len(r.names) {
			return nil, 0, bsapMalformed("response count differs from requested names")
		}
		at := 0
		for i := 0; i < n; i++ {
			v := map[string]any{"Name": r.names[i]}
			if status&0x80 != 0 {
				if at >= len(p) {
					return nil, 0, bsapMalformed("element error byte is truncated")
				}
				v["Element Error"] = p[at]
				at++
				if p[at-1] != 0 {
					elements = append(elements, v)
					continue
				}
			}
			if r.selector&0x80 == 0 {
				// The matched response was observed, but its value type was not.
				// Do not retain it as an outstanding request or guess a type.
				s.pending = nil
				return nil, 0, bsapContext("value-only reply has no observed signal type")
			}
			if at >= len(p) {
				return nil, 0, bsapMalformed("type byte is truncated")
			}
			t := p[at]
			at++
			if t&3 == 1 {
				return nil, 0, bsapMalformed("reserved signal type")
			}
			v["Type And Inhibits"] = t
			v["Alarm Signal"], v["Constant Signal"], v["Manual Inhibited"], v["Control Inhibited"], v["Alarm Inhibited"] = t&4 != 0, t&8 != 0, t&16 != 0, t&32 != 0, t&64 != 0
			v["Type"] = map[byte]string{0: "logical", 2: "analog", 3: "string"}[t&3]
			if t&3 == 2 {
				v["Questionable Data"] = t&0x80 != 0
			}
			if r.selector&0x40 != 0 {
				switch t & 3 {
				case 0:
					if at >= len(p) || p[at] > 1 {
						return nil, 0, bsapMalformed("logical value is missing or not zero/one")
					}
					v["Value"] = p[at] != 0
					at++
				case 3:
					end := bytes.IndexByte(p[at:], 0)
					if end < 0 {
						return nil, 0, bsapMalformed("string value has no terminator")
					}
					if end > 65 {
						return nil, 0, bsapMalformed("string value exceeds 65 characters")
					}
					if !bsapASCII(p[at : at+end]) {
						return nil, 0, bsapMalformed("string value is not ASCII")
					}
					v["Value"] = string(p[at : at+end])
					at += end + 1
				case 2:
					s.pending = nil
					return nil, 0, bsapUnsupported("analog value/Q-bit interpretation is outside this profile")
				}
			}
			elements = append(elements, v)
		}
		if at != len(p) {
			return nil, 0, bsapMalformed("bytes follow response elements")
		}
	}
	m.fields["Association"], m.fields["Function"], m.fields["Names"], m.fields["Elements"] = "matched", byte(4), append([]string(nil), r.names...), elements
	m.fields["Field Select Byte 1"] = r.selector
	s.pending = nil
	return m.fields, r.eventID, nil
}

const bsapIdleTTL = 30 * time.Second

func (a *binParser) decodeBSAPDatagram(e *ProtocolEvent, w []byte, explicit bool) bool {
	key := binUDPKey{"bsap/" + e.Source, "bsap/" + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	store := a.udpSessions
	var existing bool
	if store != nil {
		existing = store.entries[key] != nil
	}
	if !explicit && !(bsapStart(w) && (existing || bsapEvidence(w))) {
		a.udpMu.Unlock()
		return false
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "bsap", "serial-local-rdb-name-2022", "wire-signature", "message"
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
		if entry.flow.bsap != nil && store.clock.Sub(entry.touched) >= bsapIdleTTL {
			entry.flow.closeSession()
			delete(store.entries, entry.key)
			store.lru.Remove(el)
		}
		el = next
	}
	var m *bsapMessage
	var err error
	if len(w) > a.budget.MaxFrameBytes {
		m, err = nil, protocolError(ErrResourceExceeded, "BSAP frame exceeds byte budget")
	} else {
		m, err = decodeBSAPMessage(w, a.budget.MaxCollectionElements)
	}
	el := store.entries[key]
	if err != nil && len(w) <= a.budget.MaxFrameBytes {
		// A CRC-valid but unsupported/malformed new request can also receive an
		// indistinguishable reply. Never leave the earlier slot eligible.
		b, frameErr := bsapBody(w)
		if frameErr == nil && len(b) >= 7 && b[0] > 0 && b[0] < 128 && b[1] != 0 && b[2] == 0xa0 && b[5] == 3 {
			if el == nil {
				if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
					err = protocolError(ErrResourceExceeded, "BSAP UDP conversation limit")
				} else {
					f := &binFlow{a: a, id: a.flows.Add(1), protocol: "bsap", bsap: &binBSAP{}, endpoints: [2]string{e.Source, e.Destination}}
					if reserveErr := f.reserveSession(576 + int64(len(w))*3); reserveErr != nil {
						err = reserveErr
					} else {
						el = store.lru.PushBack(&binUDPEntry{key, f, store.clock})
						store.entries[key] = el
					}
				}
			}
			if el != nil {
				f := el.Value.(*binUDPEntry).flow
				if e.Source == f.endpoints[0] {
					seq := binary.LittleEndian.Uint16(b[3:5])
					if f.bsap.pending != nil {
						f.bsap.pending = nil
						f.bsap.ambiguous = true
					}
					if !f.bsap.seen[seq] {
						if len(f.bsap.seen) >= a.budget.MaxCollectionElements {
							err = protocolError(ErrResourceExceeded, "BSAP retained sequence limit")
						} else if reserveErr := f.reserveSession(512 + int64(len(f.bsap.seen)+1)*64 + int64(len(w))*3); reserveErr != nil {
							err = reserveErr
						} else {
							if f.bsap.seen == nil {
								f.bsap.seen = make(map[uint16]bool)
							}
							f.bsap.seen[seq] = true
						}
					}
				}
			}
		}
	}
	if err == nil && el == nil {
		if !m.request {
			err = bsapContext("reply requires an observed ReadByName request")
		} else if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "BSAP UDP conversation limit")
		} else {
			f := &binFlow{a: a, id: a.flows.Add(1), protocol: "bsap", bsap: &binBSAP{}, endpoints: [2]string{e.Source, e.Destination}}
			if err = f.reserveSession(576 + int64(len(w))*3); err == nil {
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
		e.FlowID = f.id
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		}
		if err == nil {
			n := len(f.bsap.seen)
			if m.request && f.bsap.pending == nil && !f.bsap.seen[m.sequence] {
				if n >= a.budget.MaxCollectionElements {
					err = protocolError(ErrResourceExceeded, "BSAP retained sequence limit")
				} else {
					n++
				}
			}
			if err == nil {
				if err = f.reserveSession(512 + int64(n)*64 + int64(len(w))*3); err == nil {
					e.Session, e.ResponseTo, err = f.bsap.consume(m, e.Direction, e.ID, a.budget.MaxCollectionElements)
				}
			}
		}
	}
	a.udpMu.Unlock()
	if err == nil {
		e.semanticFields = cloneSession(e.Session)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}
