package pcaputil

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DTLS uses datagram boundaries, not TLS stream framing. We observe DTLS
// 1.0/1.2 plaintext handshake envelopes; record authentication, decryption,
// DTLS 1.3 unified headers and connection IDs require separate contexts.
const dtlsReplayWindow = 128
const dtlsIdleTTL = time.Minute

type dtlsRecord struct {
	typ            byte
	version, epoch uint16
	seq            uint64
	raw, body      []byte
}
type dtlsMessageKey struct {
	dir        int
	epoch, seq uint16
}
type dtlsRecordKey struct {
	dir   int
	epoch uint16
	seq   uint64
}
type dtlsAssembly struct {
	typ        byte
	body, seen []byte
	filled     int
	refs       []PacketReference
}
type dtlsCompleted struct {
	typ    byte
	length int
	hash   [32]byte
}
type binDTLS struct {
	fragments      map[dtlsMessageKey]*dtlsAssembly
	completed      map[dtlsMessageKey]dtlsCompleted
	completedOrder []dtlsMessageKey
	records        map[dtlsRecordKey][32]byte
	recordOrder    []dtlsRecordKey
	client         int
	suites         []byte
	negotiated     uint16
}

func dtlsRecords(w []byte, maxRecords int) ([]dtlsRecord, error) {
	records := []dtlsRecord{}
	for pos := 0; pos < len(w); {
		b := w[pos:]
		typ := b[0]
		if typ < 20 || typ > 24 {
			return nil, protocolError(ErrUnsupportedFeature, "DTLS content type / CID")
		}
		// CID and unified records need different headers, which may be shorter
		// than thirteen bytes. Do not diagnose them as truncated legacy headers.
		if len(b) >= 3 && binary.BigEndian.Uint16(b[1:]) != 0xfeff && binary.BigEndian.Uint16(b[1:]) != 0xfefd {
			return nil, protocolError(ErrUnsupportedVersion, "DTLS legacy record version")
		}
		if len(b) < 13 {
			return nil, protocolError(ErrMalformedMessage, "DTLS record header truncated")
		}
		version := binary.BigEndian.Uint16(b[1:])
		epoch := binary.BigEndian.Uint16(b[3:])
		n := int(binary.BigEndian.Uint16(b[11:]))
		if n > 18432 || n > len(b)-13 {
			return nil, protocolError(ErrMalformedMessage, "DTLS record payload length")
		}
		if epoch == 0 && n > 16384 {
			return nil, protocolError(ErrMalformedMessage, "DTLS plaintext record exceeds 16384 bytes")
		}
		if len(records) >= maxRecords {
			return nil, protocolError(ErrResourceExceeded, "DTLS records per datagram")
		}
		seq := uint64(0)
		for _, c := range b[5:11] {
			seq = seq<<8 | uint64(c)
		}
		records = append(records, dtlsRecord{typ, version, epoch, seq, b[:13+n], b[13 : 13+n]})
		pos += 13 + n
	}
	if len(records) == 0 {
		return nil, protocolError(ErrMalformedMessage, "DTLS empty datagram")
	}
	return records, nil
}

func dtlsWireCandidate(w []byte) bool {
	return len(w) >= 3 && w[0] >= 20 && w[0] <= 25 && w[1] == 0xfe && (w[2] == 0xff || w[2] == 0xfd || w[2] == 0xfc)
}

func (s *binDTLS) storage() int64 {
	n := int64(512 + cap(s.suites) + (len(s.completed)+cap(s.completedOrder))*80 + (len(s.records)+cap(s.recordOrder))*80)
	for _, m := range s.fragments {
		n += int64(128 + cap(m.body) + cap(m.seen) + cap(m.refs)*96)
	}
	return n
}

// Grow replay-order slices explicitly so their retained capacity is reserved
// before allocation. Full windows rotate in place instead of losing capacity
// by slicing off the first entry on every record.
func dtlsReplayCapacity(length, capacity, window int) int {
	if length < capacity || length == window {
		return capacity
	}
	return min(window, max(1, capacity*2))
}

func (a *binParser) decodeDTLSDatagram(e *ProtocolEvent, w []byte, explicit bool) bool {
	if !explicit && !dtlsWireCandidate(w) && (len(w) == 0 || !(w[0] == 25 || w[0]&0xe0 == 0x20)) {
		return false
	}
	key := binUDPKey{"dtls/" + e.Source, "dtls/" + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	store := a.udpSessions
	var el *list.Element
	if store != nil {
		if e.Timestamp.After(store.clock) {
			store.clock = e.Timestamp
		}
		for cur := store.lru.Front(); cur != nil; {
			next := cur.Next()
			entry := cur.Value.(*binUDPEntry)
			if store.clock.Sub(entry.touched) < dtlsIdleTTL {
				break
			}
			if strings.HasPrefix(entry.key.a, "dtls/") {
				entry.flow.closeSession()
				delete(store.entries, entry.key)
				store.lru.Remove(cur)
			}
			cur = next
		}
		el = store.entries[key]
	}
	if !explicit && !dtlsWireCandidate(w) {
		// A negotiated DTLS conversation can report unsupported modern/CID
		// records, but unrelated traffic never gains admission from a port alone.
		if el == nil || len(w) == 0 || !(w[0] == 25 || w[0]&0xe0 == 0x20) {
			return false
		}
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "dtls", "dtls-legacy-udp", "wire-signature", "datagram"
	if explicit {
		e.Admission = "explicit-decode-as"
	}
	records, err := dtlsRecords(w, a.budget.MaxCollectionElements)
	if err != nil {
		a.finishProtocolDatagram(e, w, nil, err)
		return true
	}
	if store == nil {
		store = &binUDPStore{entries: map[binUDPKey]*list.Element{}, clock: e.Timestamp}
		a.udpSessions = store
	}
	if el == nil {
		if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = protocolError(ErrResourceExceeded, "DTLS conversation budget")
		} else {
			endpoints := []string{e.Source, e.Destination}
			sort.Strings(endpoints)
			state := &binDTLS{client: -1, fragments: map[dtlsMessageKey]*dtlsAssembly{}, completed: map[dtlsMessageKey]dtlsCompleted{}, records: map[dtlsRecordKey][32]byte{}}
			flow := &binFlow{a: a, id: a.flows.Add(1), protocol: "dtls", dtls: state, endpoints: [2]string{endpoints[0], endpoints[1]}}
			if err = flow.reserveSession(state.storage()); err == nil {
				el = store.lru.PushBack(&binUDPEntry{key, flow, store.clock})
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
		e.Session, err = f.dtls.consume(f, e, records)
		if reserveErr := f.reserveSession(f.dtls.storage()); err == nil {
			err = reserveErr
		}
		if err != nil {
			// A failed message may have partially changed fragment/hello state.
			// Release that context rather than admitting later records into it.
			f.closeSession()
			delete(store.entries, key)
			store.lru.Remove(el)
		}
	}
	if e.Session != nil {
		e.semanticFields = cloneSession(e.Session)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}

func (s *binDTLS) consume(f *binFlow, e *ProtocolEvent, records []dtlsRecord) (map[string]any, error) {
	out := map[string]any{"Records": []map[string]any{}, "Authentication Verified": false, "Payload Decrypted": false, "Replay Tracking Window": min(dtlsReplayWindow, f.a.budget.MaxCollectionElements), "Observation Scope": "conversation"}
	rows := make([]map[string]any, 0, len(records))
	for _, r := range records {
		row := map[string]any{"Content Type": r.typ, "Version": r.version, "Epoch": r.epoch, "Sequence": r.seq, "Length": len(r.body), "Protected": r.epoch != 0, "Content Decoded": false}
		rows = append(rows, row)
		out["Records"] = rows
		key := dtlsRecordKey{e.Direction, r.epoch, r.seq}
		hash := sha256.Sum256(r.raw)
		if old, seen := s.records[key]; seen {
			if old != hash {
				return out, protocolError(ErrDesynchronized, "DTLS record identity has conflicting captured bytes")
			}
			row["Retransmission"] = true
			continue
		}
		if r.epoch != 0 {
			row["Semantic Status"] = "encrypted"
			row["Protected Bytes"] = len(r.body)
		} else {
			switch r.typ {
			case 20:
				if !bytes.Equal(r.body, []byte{1}) {
					return out, protocolError(ErrMalformedMessage, "DTLS ChangeCipherSpec")
				}
				row["Change Cipher Spec"] = true
				row["Content Decoded"] = true
			case 21:
				if len(r.body) == 0 || len(r.body)%2 != 0 {
					return out, protocolError(ErrMalformedMessage, "DTLS alert length")
				}
				if len(r.body)/2 > f.a.budget.MaxCollectionElements {
					return out, protocolError(ErrResourceExceeded, "DTLS alert count")
				}
				alerts := []map[string]any{}
				for i := 0; i < len(r.body); i += 2 {
					if r.body[i] != 1 && r.body[i] != 2 {
						return out, protocolError(ErrMalformedMessage, "DTLS alert level")
					}
					alerts = append(alerts, map[string]any{"Level": r.body[i], "Description": r.body[i+1]})
				}
				row["Alerts"] = alerts
				row["Content Decoded"] = true
			case 22:
				messages, err := s.handshakes(f, e, r)
				row["Handshakes"] = messages
				if err != nil {
					return out, err
				}
				row["Content Decoded"] = true
			default:
				return out, protocolError(ErrUnsupportedFeature, "DTLS epoch-zero application/heartbeat payload")
			}
		}
		window := min(dtlsReplayWindow, f.a.budget.MaxCollectionElements)
		capacity := dtlsReplayCapacity(len(s.recordOrder), cap(s.recordOrder), window)
		growth := (capacity - cap(s.recordOrder)) * 80
		if len(s.recordOrder) < window {
			growth += 80
		}
		if err := f.reserveSession(s.storage() + int64(growth)); err != nil {
			return out, err
		}
		if len(s.recordOrder) >= window {
			delete(s.records, s.recordOrder[0])
			copy(s.recordOrder, s.recordOrder[1:])
			s.recordOrder[len(s.recordOrder)-1] = key
		} else {
			if capacity != cap(s.recordOrder) {
				order := make([]dtlsRecordKey, len(s.recordOrder), capacity)
				copy(order, s.recordOrder)
				s.recordOrder = order
			}
			s.recordOrder = append(s.recordOrder, key)
		}
		s.records[key] = hash
	}
	out["Negotiated Version"] = s.negotiated
	if s.negotiated == 0 {
		out["Context Level"] = "partial"
	}
	out["Buffered Handshake Messages"] = len(s.fragments)
	return out, nil
}

func (s *binDTLS) handshakes(f *binFlow, e *ProtocolEvent, r dtlsRecord) ([]map[string]any, error) {
	out := []map[string]any{}
	if len(r.body) == 0 {
		return out, protocolError(ErrMalformedMessage, "DTLS empty handshake record")
	}
	for pos := 0; pos < len(r.body); {
		if len(r.body)-pos < 12 {
			return out, protocolError(ErrMalformedMessage, "DTLS handshake fragment header")
		}
		b := r.body[pos:]
		typ := b[0]
		total := tlsU24(b[1:])
		seq := binary.BigEndian.Uint16(b[4:])
		offset, n := tlsU24(b[6:]), tlsU24(b[9:])
		if n > len(b)-12 || offset > total || n > total-offset || total > 0 && n == 0 {
			return out, protocolError(ErrMalformedMessage, "DTLS fragment range")
		}
		if total > f.a.budget.MaxMessageBytes {
			return out, protocolError(ErrResourceExceeded, "DTLS handshake size")
		}
		if len(out) >= f.a.budget.MaxCollectionElements {
			return out, protocolError(ErrResourceExceeded, "DTLS handshake fragment count")
		}
		data := b[12 : 12+n]
		pos += 12 + n
		m := map[string]any{"Type": typ, "Message Seq": seq, "Length": total, "Fragment Offset": offset, "Fragment Length": n, "Complete": false, "Authenticated": false}
		out = append(out, m)
		key := dtlsMessageKey{e.Direction, r.epoch, seq}
		assembly := s.fragments[key]
		if assembly == nil {
			if len(s.fragments) >= f.a.budget.MaxCollectionElements {
				return out, protocolError(ErrResourceExceeded, "DTLS pending fragments")
			}
			if err := f.reserveSession(s.storage() + int64(2*total+256+len(e.SourceBytes.PacketRefs)*192)); err != nil {
				return out, err
			}
			assembly = &dtlsAssembly{typ: typ, body: make([]byte, total), seen: make([]byte, total)}
			s.fragments[key] = assembly
		}
		if assembly.typ != typ || len(assembly.body) != total {
			return out, protocolError(ErrDesynchronized, "DTLS fragment message identity")
		}
		for i, c := range data {
			at := offset + i
			if assembly.seen[at] != 0 && assembly.body[at] != c {
				return out, protocolError(ErrDesynchronized, "DTLS overlapping fragments disagree")
			}
		}
		newRefs := []PacketReference{}
		for _, ref := range e.SourceBytes.PacketRefs {
			if !dtlsHasRef(assembly.refs, ref) && !dtlsHasRef(newRefs, ref) {
				newRefs = append(newRefs, ref)
			}
		}
		if len(assembly.refs)+len(newRefs) > f.a.budget.MaxCollectionElements {
			return out, protocolError(ErrResourceExceeded, "DTLS fragment packet references")
		}
		if err := f.reserveSession(s.storage() + int64(len(newRefs)*192)); err != nil {
			return out, err
		}
		assembly.refs = append(assembly.refs, newRefs...)
		for i, c := range data {
			at := offset + i
			if assembly.seen[at] == 0 {
				assembly.seen[at] = 1
				assembly.filled++
			}
			assembly.body[at] = c
		}
		if assembly.filled != total {
			m["Received Bytes"] = assembly.filled
			continue
		}
		hash := sha256.Sum256(assembly.body)
		if old, seen := s.completed[key]; seen {
			if old.typ != typ || old.length != total || old.hash != hash {
				return out, protocolError(ErrDesynchronized, "DTLS retransmitted handshake differs")
			}
			m["Retransmission"] = true
		} else {
			fields, err := s.message(f, e.Direction, typ, assembly.body)
			if err != nil {
				delete(s.fragments, key)
				return out, err
			}
			for k, v := range fields {
				m[k] = v
			}
			window := min(dtlsReplayWindow, f.a.budget.MaxCollectionElements)
			capacity := dtlsReplayCapacity(len(s.completedOrder), cap(s.completedOrder), window)
			growth := (capacity - cap(s.completedOrder)) * 80
			if len(s.completedOrder) < window {
				growth += 80
			}
			if err := f.reserveSession(s.storage() + int64(growth)); err != nil {
				return out, err
			}
			if len(s.completedOrder) >= window {
				delete(s.completed, s.completedOrder[0])
				copy(s.completedOrder, s.completedOrder[1:])
				s.completedOrder[len(s.completedOrder)-1] = key
			} else {
				if capacity != cap(s.completedOrder) {
					order := make([]dtlsMessageKey, len(s.completedOrder), capacity)
					copy(order, s.completedOrder)
					s.completedOrder = order
				}
				s.completedOrder = append(s.completedOrder, key)
			}
			s.completed[key] = dtlsCompleted{typ, total, hash}
		}
		m["Complete"] = true
		m["Packet Refs"] = append([]PacketReference(nil), assembly.refs...)
		delete(s.fragments, key)
	}
	return out, nil
}

func (s *binDTLS) message(f *binFlow, dir int, typ byte, b []byte) (map[string]any, error) {
	out := map[string]any{"Body SHA256": fmt.Sprintf("%x", sha256.Sum256(b)), "Semantic Status": "observed"}
	switch typ {
	case 1, 2:
		client := typ == 1
		wire := b
		if len(b) < 35 {
			return nil, protocolError(ErrMalformedMessage, "DTLS hello size")
		}
		version := binary.BigEndian.Uint16(b)
		if version != 0xfeff && version != 0xfefd {
			return nil, protocolError(ErrUnsupportedVersion, "DTLS hello version")
		}
		if client {
			p := 35 + int(b[34])
			if p >= len(b) || b[34] > 32 {
				return nil, protocolError(ErrMalformedMessage, "DTLS client session ID")
			}
			n := int(b[p])
			if n > len(b)-p-1 {
				return nil, protocolError(ErrMalformedMessage, "DTLS cookie length")
			}
			out["Cookie Bytes"] = n
			wire = append(bytes.Clone(b[:p]), b[p+1+n:]...)
		}
		hello, err := tlsHello(wire, client)
		if err != nil {
			return nil, err
		}
		if len(hello.order) > f.a.budget.MaxCollectionElements || len(hello.suites)/2 > f.a.budget.MaxCollectionElements {
			return nil, protocolError(ErrResourceExceeded, "DTLS hello collection")
		}
		out["Hello Version"], out["Random"], out["Session ID"] = version, hex.EncodeToString(b[2:34]), hex.EncodeToString(hello.session)
		// Extension vectors reuse the TLS syntax, not its handshake state machine.
		t := &binTLS{}
		if len(wire) > len(hello.prefix) {
			if err := t.extensions(wire[len(hello.prefix):], client); err != nil {
				return nil, err
			}
		}
		if !client && t.version != 0 && t.version != version {
			return nil, protocolError(ErrUnsupportedVersion, "DTLS selected modern version")
		}
		out["SNI"], out["ALPN"] = t.sni, t.alpn
		if versions := hello.ext[43]; client && versions != nil {
			if len(versions) < 3 || int(versions[0]) != len(versions)-1 || versions[0]%2 != 0 {
				return nil, protocolError(ErrMalformedMessage, "DTLS supported versions vector")
			}
			offered := []uint16{}
			for p := 1; p < len(versions); p += 2 {
				offered = append(offered, binary.BigEndian.Uint16(versions[p:]))
			}
			out["Offered Versions"] = offered
		}
		if client {
			if alpn := hello.ext[16]; len(alpn) >= 2 {
				offered := []string{}
				for p := 2; p < len(alpn); {
					n := int(alpn[p])
					p++
					offered = append(offered, string(alpn[p:p+n]))
					p += n
				}
				if len(offered) > f.a.budget.MaxCollectionElements {
					return nil, protocolError(ErrResourceExceeded, "DTLS offered ALPN")
				}
				out["Offered ALPN"] = offered
			}
		}
		extensions := []map[string]any{}
		for _, kind := range hello.order {
			extensions = append(extensions, map[string]any{"Type": kind, "Value": bytes.Clone(hello.ext[kind])})
		}
		out["Extensions"] = extensions
		if client {
			if s.client >= 0 && s.client != dir {
				return nil, protocolError(ErrDesynchronized, "DTLS ClientHello direction changed")
			}
			if err := f.reserveSession(s.storage() + int64(len(hello.suites))); err != nil {
				return nil, err
			}
			s.client = dir
			s.suites = bytes.Clone(hello.suites)
			suites := []uint16{}
			for i := 0; i < len(hello.suites); i += 2 {
				suites = append(suites, binary.BigEndian.Uint16(hello.suites[i:]))
			}
			out["Cipher Suites"] = suites
		} else {
			if s.client == dir || s.client >= 0 && !tlsContains16(s.suites, hello.suite) {
				return nil, protocolError(ErrMalformedMessage, "DTLS server selection contradicts observed client")
			}
			if wire[len(hello.prefix)-1] != 0 {
				return nil, protocolError(ErrUnsupportedFeature, "DTLS compression")
			}
			s.negotiated = version
			out["Cipher Suite"] = hello.suite
			if s.client < 0 {
				out["Context Level"] = "partial"
			}
		}
	case 3:
		if len(b) < 3 || int(b[2]) != len(b)-3 {
			return nil, protocolError(ErrMalformedMessage, "DTLS HelloVerifyRequest cookie")
		}
		version := binary.BigEndian.Uint16(b)
		if version != 0xfeff && version != 0xfefd {
			return nil, protocolError(ErrUnsupportedVersion, "DTLS cookie version")
		}
		if s.client == dir {
			return nil, protocolError(ErrMalformedMessage, "DTLS HelloVerifyRequest direction")
		}
		out["Hello Version"], out["Cookie Bytes"] = version, len(b)-3
	case 11:
		certificates, err := tlsCertificates(b, false)
		if err != nil {
			return nil, err
		}
		if len(certificates) > f.a.budget.MaxCollectionElements {
			return nil, protocolError(ErrResourceExceeded, "DTLS certificate chain")
		}
		out["Certificates"] = certificates
		out["Certificate Verified"] = false
	case 0, 14:
		if len(b) != 0 {
			return nil, protocolError(ErrMalformedMessage, "DTLS empty handshake body required")
		}
	case 4, 12, 13, 15, 16:
		out["Semantic Status"] = "opaque-handshake-body"
	case 20:
		return nil, protocolError(ErrMalformedMessage, "DTLS Finished cannot be trusted in epoch zero")
	default:
		out["Semantic Status"] = "unsupported-handshake-type"
	}
	return out, nil
}

func dtlsHasRef(refs []PacketReference, ref PacketReference) bool {
	for _, old := range refs {
		if old == ref {
			return true
		}
	}
	return false
}
