package pcaputil

import (
	"bytes"
	"encoding/binary"
	"sort"
	"time"
	"unsafe"
)

const j1939TPProfile = "j1939-classic-transport"

type j1939TPKey struct {
	domain              CaptureDomain
	source, destination byte
}
type j1939TPReceiver struct {
	payloadCost                                             int64
	cost                                                    int64
	active, blocked, bam                                    bool
	pgn                                                     uint32
	size, packets, block, windowFirst, windowLast, received int
	lastCommand                                             byte
	at, clock                                               time.Time
	timeout                                                 time.Duration
	body                                                    []byte // includes literal final-frame padding; logical size is separate
	seen                                                    []bool
	refs                                                    []PacketReference
}
type j1939TPStore struct {
	entries map[j1939TPKey]*j1939TPReceiver
	cost    int64
}

// TP.DT has no PGN or transaction ID. Only an observed CM and its directed
// capture-domain pair can authorize a transfer; ambiguous overlapping starts
// retire that pair. This receiver observes bytes, not ECU identity, physical
// delivery or application PGN semantics. Linux v6.6 transport.c supplies the
// control layouts and phase time budgets; stricter length/count agreement is
// required here rather than reproducing its warning-only inconsistent headers.
func (a *binParser) decodeJ1939TP(id uint32, data []byte, ref PacketReference, at time.Time) (map[string]any, []PacketReference, error) {
	a.j1939TPMu.Lock()
	defer a.j1939TPMu.Unlock()
	fields, err := decodeJ1939RequestName(id, data)
	if err != nil {
		return nil, nil, err
	}
	carrier := fields["PGN"].(uint32)
	if carrier != 0xec00 && carrier != 0xeb00 {
		return fields, nil, nil
	}
	key := j1939TPKey{ref.Domain, byte(id), byte(id >> 8)}
	fail := func(kind ProtocolErrorKind, why string) (map[string]any, []PacketReference, error) {
		return nil, nil, protocolError(kind, "J1939 TP "+why)
	}
	if a.j1939TPDisabled {
		return fail(ErrResourceExceeded, "receiver history exhausted")
	}
	if _, lost := a.j1939TPRefusedDomains[ref.Domain]; lost {
		return fail(ErrContextRequired, "capture domain retired after unidentifiable refusal")
	}
	if len(data) != 8 || byte(id) == 254 || byte(id) == 255 || id>>24&3 != 0 {
		a.refuseJ1939TPDomainLocked(ref.Domain)
		return fail(ErrMalformedMessage, "classical TP needs eight octets, data page zero and a valid source address")
	}
	cmd := byte(0)
	pgn := uint32(0)
	if carrier == 0xec00 {
		cmd = data[0]
		pgn = uint32(data[5]) | uint32(data[6])<<8 | uint32(data[7])<<16
		if pgn > 0x3ffff || byte(pgn>>8) < 240 && byte(pgn) != 0 || pgn == 0xeb00 || pgn == 0xec00 {
			a.refuseJ1939TPDomainLocked(ref.Domain)
			return fail(ErrMalformedMessage, "target PGN is not canonical or recursively names TP")
		}
		if cmd == 0x11 || cmd == 0x13 {
			key.source, key.destination = key.destination, key.source
		}
	}
	var s *j1939TPReceiver
	if a.j1939TP != nil {
		s = a.j1939TP.entries[key]
	}
	// Abort may be emitted by either peer; its PGN must still identify the same
	// running transfer. It cannot retire an unrelated reverse-direction transfer.
	if cmd == 255 && s != nil && s.active && s.pgn == pgn && a.j1939TP != nil {
		reverse := j1939TPKey{ref.Domain, key.destination, key.source}
		r := a.j1939TP.entries[reverse]
		if r != nil && r.active && r.pgn == pgn {
			for _, x := range []*j1939TPReceiver{s, r} {
				x.active = false
				x.blocked = true
				a.releaseJ1939TPPayloadLocked(x)
			}
			return fail(ErrContextRequired, "Abort direction is ambiguous for two active transfers")
		}
	}
	if cmd == 255 && (s == nil || !s.active || s.pgn != pgn) {
		reverse := j1939TPKey{ref.Domain, key.destination, key.source}
		if a.j1939TP != nil {
			r := a.j1939TP.entries[reverse]
			if r != nil && r.active && r.pgn == pgn {
				key, s = reverse, r
			}
		}
	}
	refuse := func(kind ProtocolErrorKind, why string) (map[string]any, []PacketReference, error) {
		if s != nil {
			s.active = false
			s.blocked = true
			a.releaseJ1939TPPayloadLocked(s)
		}
		return fail(kind, why)
	}
	if s != nil {
		if !at.IsZero() && !s.clock.IsZero() && at.Before(s.clock) {
			return refuse(ErrContextRequired, "backdated contributor")
		}
		if s.active && !at.IsZero() && !s.at.IsZero() && at.Sub(s.at) > s.timeout {
			return refuse(ErrContextRequired, "captured transfer phase expired")
		}
	}
	start := cmd == 0x20 || cmd == 0x10
	if start {
		n := int(binary.LittleEndian.Uint16(data[1:3]))
		packets := int(data[3])
		bam := cmd == 0x20
		if n < 9 || n > 1785 || packets != (n+6)/7 || bam && (key.destination != 255 || data[4] != 255) || !bam && (key.destination >= 254 || data[4] == 0) {
			return refuse(ErrMalformedMessage, "start length/count/destination/block is invalid")
		}
		if s != nil && s.active {
			return refuse(ErrContextRequired, "overlapping start is ambiguous")
		}
		if s != nil && s.blocked {
			return fail(ErrContextRequired, "pair retired after ambiguous or refused transfer")
		}
		if n > a.budget.MaxMessageBytes || packets > sessionCollectionLimit(a.budget.MaxCollectionElements) {
			a.disableJ1939TPLocked()
			return fail(ErrResourceExceeded, "declared transfer payload/packet budget")
		}
		if a.j1939TP == nil {
			cost := int64(unsafe.Sizeof(j1939TPStore{})) + 256
			if !a.reserveEvidence(cost) {
				a.j1939TPDisabled = true
				return fail(ErrResourceExceeded, "receiver store byte budget")
			}
			a.j1939TP = &j1939TPStore{entries: make(map[j1939TPKey]*j1939TPReceiver), cost: cost}
		}
		if s == nil {
			cost := int64(unsafe.Sizeof(j1939TPReceiver{})+unsafe.Sizeof(key)) + 128 + int64(len(key.domain.Encapsulation))
			if len(a.j1939TP.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.reserveEvidence(cost) {
				a.disableJ1939TPLocked()
				return fail(ErrResourceExceeded, "receiver identity budget")
			}
			s = &j1939TPReceiver{cost: cost}
			a.j1939TP.entries[key] = s
		}
		// Release the previous completed allocation, keeping the bounded pair marker.
		a.releaseJ1939TPPayloadLocked(s)
		refs := min(3*packets+8, sessionCollectionLimit(a.budget.MaxCollectionElements))
		extra := int64(8*packets) + int64(refs)*(int64(unsafe.Sizeof(PacketReference{}))+int64(len(key.domain.Encapsulation)))
		if !a.reserveEvidence(extra) {
			a.disableJ1939TPLocked()
			return fail(ErrResourceExceeded, "transfer contributor byte budget")
		}
		s.cost += extra
		s.payloadCost = extra
		s.body = make([]byte, 7*packets)
		s.seen = make([]bool, packets)
		s.refs = make([]PacketReference, 0, refs)
		s.active, s.bam = true, bam
		s.pgn, s.size, s.packets = pgn, n, packets
		s.block = int(data[4])
		s.received = 0
		s.windowFirst, s.windowLast = 0, 0
		if bam {
			s.windowFirst, s.windowLast = 1, packets
		}
		s.timeout = 1250 * time.Millisecond
		if bam {
			s.timeout = 750 * time.Millisecond
		}
		s.lastCommand = cmd
	} else {
		if s == nil || !s.active {
			return fail(ErrContextRequired, "start of active transfer was not captured")
		}
		if s.blocked {
			return fail(ErrContextRequired, "pair retired after ambiguous or refused transfer")
		}
		if carrier == 0xec00 && pgn != s.pgn {
			return fail(ErrContextRequired, "control target PGN differs from active transfer")
		}
	}
	if len(s.refs) == cap(s.refs) {
		return refuse(ErrResourceExceeded, "transfer contributor count budget")
	}
	s.refs = append(s.refs, ref)
	s.at = at
	if at.After(s.clock) {
		s.clock = at
	}
	fields["Content"] = "transport-control"
	fields["Observation"] = "unverified-captured-transfer"
	fields["Transfer Mode"] = "RTS-CTS"
	if s.bam {
		fields["Transfer Mode"] = "BAM"
	}
	fields["Transfer Source Address"], fields["Transfer Destination Address"] = key.source, key.destination
	fields["Target PGN"], fields["Transfer Size"], fields["Transfer Packets"] = s.pgn, s.size, s.packets
	complete := false
	switch {
	case start:
		fields["Control"], fields["Maximum Packets Per CTS"] = cmd, int(data[4])
	case carrier == 0xeb00:
		seq := int(data[0])
		fields["Content"], fields["Sequence"] = "transport-data", seq
		if seq < 1 || seq > s.packets || seq < s.windowFirst || seq > s.windowLast {
			return refuse(ErrContextRequired, "data sequence outside observed CTS/BAM window")
		}
		pos := (seq - 1) * 7
		if s.seen[seq-1] {
			if !bytes.Equal(s.body[pos:pos+7], data[1:]) {
				return refuse(ErrMalformedMessage, "conflicting repeated data sequence")
			}
			fields["Repeated Sequence"] = true
		} else {
			copy(s.body[pos:pos+7], data[1:])
			s.seen[seq-1] = true
			s.received++
		}
		s.lastCommand = 255
		s.timeout = 750 * time.Millisecond
		if !s.bam {
			windowComplete := true
			for i := s.windowFirst - 1; i < s.windowLast; i++ {
				if !s.seen[i] {
					windowComplete = false
					break
				}
			}
			if windowComplete {
				s.timeout = 1250 * time.Millisecond
			}
		}
		complete = s.bam && s.received == s.packets
	case cmd == 0x11:
		first, count := int(data[2]), int(data[1])
		if s.bam || first < 1 || first > s.packets || count > s.block || count > 0 && first+count-1 > s.packets || data[3] != 255 || data[4] != 255 {
			return refuse(ErrMalformedMessage, "CTS window/reserved octets are invalid")
		}
		if s.lastCommand == 0x11 && s.windowFirst != 0 {
			return refuse(ErrContextRequired, "consecutive active CTS without captured data")
		}
		// A new grant cannot silently skip missing earlier packet numbers. Explicit
		// backwards grants can retransmit already observed bytes, checked above.
		for i := 0; i < first-1; i++ {
			if !s.seen[i] {
				return refuse(ErrContextRequired, "CTS advances beyond uncaptured data")
			}
		}
		s.windowFirst, s.windowLast = first, first+count-1
		s.lastCommand = cmd
		s.timeout = 1250 * time.Millisecond
		if count == 0 {
			s.windowFirst, s.windowLast = 0, 0
			s.timeout = 550 * time.Millisecond
		}
		fields["Control"], fields["Next Sequence"], fields["Granted Packets"] = cmd, first, count
	case cmd == 0x13:
		if s.bam || s.received != s.packets || int(binary.LittleEndian.Uint16(data[1:3])) != s.size || int(data[3]) != s.packets || data[4] != 255 {
			return refuse(ErrMalformedMessage, "EOM ACK lacks complete matching transfer")
		}
		fields["Control"] = cmd
		complete = true
	case cmd == 255:
		if s.bam || data[2] != 255 || data[3] != 255 || data[4] != 255 {
			return refuse(ErrMalformedMessage, "Abort destination/reserved octets invalid")
		}
		fields["Control"], fields["Abort Reason"], fields["Transfer Aborted"] = cmd, data[1], true
		s.active = false
		clear(s.body)
		clear(s.seen)
	default:
		return refuse(ErrUnsupportedFeature, "control command outside classical TP profile")
	}
	fields["Received Packets"], fields["Transfer Complete"] = s.received, complete
	refs := append([]PacketReference(nil), s.refs...)
	if complete {
		fields["Transfer Payload"] = bytes.Clone(s.body[:s.size])
		fields["Transfer Padding"] = bytes.Clone(s.body[s.size:])
		s.active = false
	}
	if !s.active {
		a.releaseJ1939TPPayloadLocked(s)
	}
	return fields, refs, nil
}

func (a *binParser) releaseJ1939TPPayloadLocked(s *j1939TPReceiver) {
	extra := s.payloadCost
	s.payloadCost = 0
	clear(s.body)
	clear(s.seen)
	clear(s.refs)
	s.body = nil
	s.seen = nil
	s.refs = nil
	a.buffered.Add(-extra)
	s.cost -= extra
}
func (a *binParser) disableJ1939TPLocked() {
	if a.j1939TP != nil {
		for _, s := range a.j1939TP.entries {
			clear(s.body)
			clear(s.seen)
			clear(s.refs)
			a.buffered.Add(-s.cost)
		}
		a.buffered.Add(-a.j1939TP.cost)
		a.j1939TP = nil
	}
	for _, cost := range a.j1939TPRefusedDomains {
		a.buffered.Add(-cost)
	}
	a.j1939TPRefusedDomains = nil
	a.j1939TPDisabled = true
}
func (a *binParser) refuseJ1939TPDomainLocked(domain CaptureDomain) {
	if _, ok := a.j1939TPRefusedDomains[domain]; ok {
		return
	}
	cost := int64(unsafe.Sizeof(domain)) + 128 + int64(len(domain.Encapsulation))
	if len(a.j1939TPRefusedDomains) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.reserveEvidence(cost) {
		a.disableJ1939TPLocked()
		return
	}
	if a.j1939TPRefusedDomains == nil {
		a.j1939TPRefusedDomains = make(map[CaptureDomain]int64)
	}
	a.j1939TPRefusedDomains[domain] = cost
	if a.j1939TP != nil {
		for key, s := range a.j1939TP.entries {
			if key.domain == domain {
				s.active = false
				s.blocked = true
				a.releaseJ1939TPPayloadLocked(s)
			}
		}
	}
}
func (a *binParser) refuseJ1939TPRecord(domain CaptureDomain) {
	a.j1939TPMu.Lock()
	defer a.j1939TPMu.Unlock()
	a.refuseJ1939TPDomainLocked(domain)
}
func (a *binParser) closeJ1939TP() {
	a.j1939TPMu.Lock()
	var events []*ProtocolEvent
	if a.j1939TP != nil {
		for key, s := range a.j1939TP.entries {
			if s.active {
				err := protocolError(ErrNeedMore, "J1939 TP capture ended before complete transfer")
				status, typed := classifySessionError(err)
				events = append(events, &ProtocolEvent{nativeCANRecord: true, Timestamp: s.at, Transport: "can", Protocol: "j1939", Profile: j1939TPProfile, Admission: "explicit-interface-decode-as", Domain: key.domain, Status: status, Completeness: status, Error: err.Error(), sessionError: typed, SourceBytes: ByteSource{Kind: "reassembled", PacketRefs: append([]PacketReference(nil), s.refs...)}})
			}
		}
	}
	a.disableJ1939TPLocked()
	a.j1939TPMu.Unlock()
	sort.Slice(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.SourceBytes.PacketRefs[0].Number != b.SourceBytes.PacketRefs[0].Number {
			return a.SourceBytes.PacketRefs[0].Number < b.SourceBytes.PacketRefs[0].Number
		}
		if a.Domain.Section != b.Domain.Section {
			return a.Domain.Section < b.Domain.Section
		}
		if a.Domain.Interface != b.Domain.Interface {
			return a.Domain.Interface < b.Domain.Interface
		}
		return a.Domain.Encapsulation < b.Domain.Encapsulation
	})
	for _, e := range events {
		a.incomplete.Add(1)
		a.emit(e)
	}
}
