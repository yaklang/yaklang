package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"time"
	"unsafe"
)

// CANISOTransport identifies an explicitly configured physical CAN pair.
// IDs use Linux CAN_EFF_FLAG (0x80000000) for extended identifiers; RTR/error
// flags are forbidden. CANFD selects the carrier, independently of CAN ID width.
// Address extension bytes are wire context, not authenticated ECU identities.
type CANISOTransport struct {
	RequestID, ResponseID           uint32
	CANFD, ExtendedAddressing       bool
	RequestAddress, ResponseAddress byte
	CheckPadding                    bool
	PaddingByte                     byte
	MaxWaitFrames                   byte
	DynamicFlowControl              bool          // fixed Linux v6.12 CAN_ISOTP_DYN_FC_PARMS semantics
	DecodeUDS                       bool          // selected DSC and single-DID RDBI, no arbitrary DID schema
	UDSResponseTimeout              time.Duration // capture policy; zero selects thirty seconds
}

func WithCANISOTransport(captureInterface int, options CANISOTransport) CaptureOption {
	if options.UDSResponseTimeout == 0 {
		options.UDSResponseTimeout = 30 * time.Second
	}
	return func(c *CaptureConfig) error {
		validID := func(id uint32) bool { return id&0x60000000 == 0 && (id&0x80000000 != 0 || id <= 0x7ff) }
		if captureInterface < 0 || !validID(options.RequestID) || !validID(options.ResponseID) || options.RequestID == options.ResponseID || options.UDSResponseTimeout < 0 || options.UDSResponseTimeout > 24*time.Hour {
			return fmt.Errorf("CAN ISO-TP requires an interface, distinct valid CAN IDs and bounded timeout")
		}
		if old := c.canDecodeAs[captureInterface]; old != "" && old != "can-isotp" {
			return fmt.Errorf("conflicting CAN DecodeAs profile")
		}
		for _, old := range c.canISOTransports[captureInterface] {
			if old == options {
				return nil
			}
			for d := 0; d < 2; d++ {
				for other := 0; other < 2; other++ {
					if canISOSelectorsOverlap(options, d, old, other) {
						return fmt.Errorf("overlapping ISO-TP CAN IDs require an unambiguous pair/address/carrier")
					}
				}
			}
		}
		if c.canDecodeAs == nil {
			c.canDecodeAs = make(map[int]string)
		}
		if c.canISOTransports == nil {
			c.canISOTransports = make(map[int][]CANISOTransport)
		}
		c.canDecodeAs[captureInterface] = "can-isotp"
		c.canISOTransports[captureInterface] = append(c.canISOTransports[captureInterface], options)
		return nil
	}
}

const canISOProfile = "can-iso15765-2-physical"

type canISOKey struct {
	domain CaptureDomain
	pair   int
}
type canISOReceiver struct {
	active, blocked, waiting                bool
	size, offset, ll, block, inBlock, waits int
	next                                    byte
	firstFC                                 bool
	stmin                                   time.Duration
	stminCode                               byte
	payloadCost                             int64
	body                                    []byte
	refs                                    []PacketReference
	at, lastCF                              time.Time
}
type canUDSIdentity struct {
	service    byte
	descriptor uint16
}
type canUDSPending struct {
	ref        PacketReference
	key        canUDSIdentity
	id         uint64
	at         time.Time
	suppressed bool
}
type canISOConnection struct {
	cost            int64
	options         CANISOTransport
	clock           time.Time
	directions      [2]canISOReceiver
	used            map[canUDSIdentity]uint64
	serviceSeen     [256]byte
	blockedServices [256]bool
	pending         *canUDSPending
	udsDisabled     bool
}
type canISOStore struct {
	entries map[canISOKey]*canISOConnection
	refused map[CaptureDomain]int64
	cost    int64
}

func canISOSelector(o CANISOTransport, dir int) (uint32, byte) {
	if dir == 0 {
		return o.RequestID, o.RequestAddress
	}
	return o.ResponseID, o.ResponseAddress
}
func canISOSelectorsOverlap(a CANISOTransport, ad int, b CANISOTransport, bd int) bool {
	ai, aa := canISOSelector(a, ad)
	bi, ba := canISOSelector(b, bd)
	if ai != bi || a.CANFD != b.CANFD {
		return false
	}
	if a.ExtendedAddressing != b.ExtendedAddressing {
		return true
	}
	return !a.ExtendedAddressing || aa == ba
}
func canISODirection(o CANISOTransport, id uint32) (int, bool) {
	if id == o.RequestID {
		return 0, true
	}
	if id == o.ResponseID {
		return 1, true
	}
	return 0, false
}
func canISOLength(n int) bool {
	if n <= 8 {
		return n >= 0
	}
	switch n {
	case 12, 16, 20, 24, 32, 48, 64:
		return true
	}
	return false
}

func (a *binParser) releaseCANISOPayload(s *canISOConnection, r *canISOReceiver) {
	clear(r.body)
	clear(r.refs)
	r.body = nil
	r.refs = nil
	a.buffered.Add(-r.payloadCost)
	s.cost -= r.payloadCost
	r.payloadCost = 0
	r.active = false
	r.waiting = false
}
func (a *binParser) disableCANISOLocked() {
	if a.canISO != nil {
		for _, s := range a.canISO.entries {
			for i := range s.directions {
				clear(s.directions[i].body)
				clear(s.directions[i].refs)
			}
			a.buffered.Add(-s.cost)
		}
		for _, cost := range a.canISO.refused {
			a.buffered.Add(-cost)
		}
		a.buffered.Add(-a.canISO.cost)
		a.canISO = nil
	}
	a.canISODisabled = true
}
func (a *binParser) refuseCANISODomainLocked(domain CaptureDomain) {
	if a.canISODisabled {
		return
	}
	if a.canISO == nil {
		cost := int64(unsafe.Sizeof(canISOStore{})) + 512
		if !a.reserveEvidence(cost) {
			a.canISODisabled = true
			return
		}
		a.canISO = &canISOStore{entries: make(map[canISOKey]*canISOConnection), refused: make(map[CaptureDomain]int64), cost: cost}
	}
	if _, ok := a.canISO.refused[domain]; ok {
		return
	}
	cost := int64(unsafe.Sizeof(domain)) + 128 + int64(len(domain.Encapsulation))
	if len(a.canISO.refused) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.reserveEvidence(cost) {
		a.disableCANISOLocked()
		return
	}
	a.canISO.refused[domain] = cost
	for k, s := range a.canISO.entries {
		if k.domain == domain {
			for i := range s.directions {
				a.releaseCANISOPayload(s, &s.directions[i])
				s.directions[i].blocked = true
			}
			s.pending = nil
			s.udsDisabled = true
		}
	}
}
func (a *binParser) refuseCANISORecord(domain CaptureDomain) {
	a.canISOMu.Lock()
	defer a.canISOMu.Unlock()
	a.refuseCANISODomainLocked(domain)
}

// Observes SF/FF/CF/FC bytes on an explicit physical pair. No frame is emitted
// on the bus, no connection is authenticated, and no TP transaction ID exists.
// Fixed Linux v6.6 PCI, carrier and phase rules supply the transport baseline.
func (a *binParser) decodeCANISO(fields map[string]any, e *ProtocolEvent) (bool, error) {
	id := fields["Identifier"].(uint32)
	if fields["Extended"] == true {
		id |= 0x80000000
	}
	var options CANISOTransport
	pair, dir := -1, 0
	idCandidates := 0
	fallbackPair, fallbackDir := -1, 0
	var fallback CANISOTransport
	for i, o := range a.canISOTransports[e.Domain.Interface] {
		if d, ok := canISODirection(o, id); ok {
			idCandidates++
			fallbackPair, fallbackDir, fallback = i, d, o
			if (fields["Frame Format"] == "fd") != o.CANFD {
				continue
			}
			data := fields["Data"].([]byte)
			if o.ExtendedAddressing {
				_, addr := canISOSelector(o, d)
				if len(data) == 0 || data[0] != addr {
					continue
				}
			}
			pair, dir, options = i, d, o
			break
		}
	}
	if pair < 0 && idCandidates == 1 {
		pair, dir, options = fallbackPair, fallbackDir, fallback
	}
	if pair < 0 && idCandidates > 1 {
		e.ID = a.ids.Add(1)
		e.Protocol, e.Profile, e.Admission = "can-isotp", canISOProfile, "explicit-interface-id-pair"
		if len(fields["Data"].([]byte)) == 0 {
			a.refuseCANISORecord(e.Domain)
			return true, protocolError(ErrMalformedMessage, "CAN ISO-TP address/PCI absent for shared selectors")
		}
		return true, protocolError(ErrContextRequired, "CAN ISO-TP frame does not select one configured address/carrier")
	}
	if pair < 0 {
		return false, nil
	}
	e.ID = a.ids.Add(1)
	e.Protocol, e.Profile, e.Admission, e.Direction = "can-isotp", canISOProfile, "explicit-interface-id-pair", dir
	if options.DecodeUDS {
		e.Profile = "can-iso15765-2-selected-uds"
	}
	a.canISOMu.Lock()
	defer a.canISOMu.Unlock()
	fail := func(kind ProtocolErrorKind, why string) error { return protocolError(kind, "CAN ISO-TP "+why) }
	if a.canISODisabled {
		return true, fail(ErrResourceExceeded, "receiver history exhausted")
	}
	key := canISOKey{e.Domain, pair}
	var s *canISOConnection
	if a.canISO != nil {
		if _, lost := a.canISO.refused[e.Domain]; lost {
			return true, fail(ErrContextRequired, "capture domain retired after unidentifiable refusal")
		}
		s = a.canISO.entries[key]
	}
	if e.Timestamp.IsZero() {
		a.refuseCANISODomainLocked(e.Domain)
		return true, fail(ErrContextRequired, "capture timestamp required for stateful timing")
	}
	if fields["RTR"] == true || fields["Error Frame"] == true || (fields["Frame Format"] == "fd") != options.CANFD {
		a.refuseCANISODomainLocked(e.Domain)
		return true, fail(ErrUnsupportedFeature, "configured carrier requires matching classical/FD data frames")
	}
	data := fields["Data"].([]byte)
	ae := 0
	if options.ExtendedAddressing {
		ae = 1
		expected := options.RequestAddress
		if dir == 1 {
			expected = options.ResponseAddress
		}
		if len(data) == 0 || data[0] != expected {
			return true, fail(ErrContextRequired, "address extension differs from configured direction")
		}
	}
	if len(data) <= ae || !canISOLength(len(data)) {
		a.refuseCANISODomainLocked(e.Domain)
		return true, fail(ErrMalformedMessage, "PCI absent or CAN length is noncanonical")
	}
	typ := data[ae] >> 4
	stateDir := dir
	if typ == 3 {
		stateDir = 1 - dir
	}
	if s == nil {
		if a.canISO == nil {
			cost := int64(unsafe.Sizeof(canISOStore{})) + 512
			if !a.reserveEvidence(cost) {
				a.canISODisabled = true
				return true, fail(ErrResourceExceeded, "receiver store budget")
			}
			a.canISO = &canISOStore{entries: make(map[canISOKey]*canISOConnection), refused: make(map[CaptureDomain]int64), cost: cost}
		}
		cost := int64(unsafe.Sizeof(canISOConnection{})+unsafe.Sizeof(key)) + 256 + int64(len(key.domain.Encapsulation))
		if len(a.canISO.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.reserveEvidence(cost) {
			a.disableCANISOLocked()
			return true, fail(ErrResourceExceeded, "configured pair history budget")
		}
		s = &canISOConnection{options: options, cost: cost}
		a.canISO.entries[key] = s
	}
	r := &s.directions[stateDir]
	refuse := func(kind ProtocolErrorKind, why string) error {
		a.releaseCANISOPayload(s, r)
		r.blocked = true
		s.pending = nil
		return fail(kind, why)
	}
	if !e.Timestamp.IsZero() && !s.clock.IsZero() && e.Timestamp.Before(s.clock) {
		return true, refuse(ErrContextRequired, "backdated contributor cannot change newer context")
	}
	if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	if r.active && !e.Timestamp.IsZero() && !r.at.IsZero() && e.Timestamp.Sub(r.at) > time.Second {
		return true, refuse(ErrContextRequired, "captured transport phase expired")
	}
	padding := func(start int) error {
		if options.CheckPadding {
			for _, b := range data[start:] {
				if b != options.PaddingByte {
					return refuse(ErrMalformedMessage, "padding differs from configured octet")
				}
			}
		}
		return nil
	}
	f := map[string]any{"Observation": "unverified-captured-transport", "Configured Request CAN ID": options.RequestID, "Configured Response CAN ID": options.ResponseID, "Carrier": "classic", "Addressing": "normal", "Direction": dir, "PCI Type": typ, "Address Bytes": ae, "Transfer Complete": false, "Transaction ID Present": false}
	if options.DynamicFlowControl {
		f["Dynamic Flow Control"] = true
	}
	if options.CANFD {
		f["Carrier"] = "fd"
	}
	if ae != 0 {
		f["Addressing"] = "extended"
		if options.RequestAddress == options.ResponseAddress {
			f["Addressing"] = "mixed"
		}
		f["Address Extension"] = data[0]
	}
	var body []byte
	refs := []PacketReference{e.SourceBytes.PacketRefs[0]}
	switch typ {
	case 0:
		if r.active {
			return true, refuse(ErrContextRequired, "single frame overlaps active segmented transfer")
		}
		if r.blocked {
			return true, fail(ErrContextRequired, "direction retired after ambiguous or refused transfer")
		}
		off, n := ae+1, int(data[ae]&15)
		if len(data) > 8 {
			if n != 0 || len(data) <= off {
				return true, refuse(ErrMalformedMessage, "FD long single frame needs escape length")
			}
			n = int(data[off])
			off++
		}
		if n == 0 || n > len(data)-off {
			return true, refuse(ErrMalformedMessage, "single frame declared length exceeds captured payload")
		}
		if n > a.budget.MaxMessageBytes {
			return true, refuse(ErrResourceExceeded, "single frame payload budget")
		}
		if err := padding(off + n); err != nil {
			return true, err
		}
		body = bytes.Clone(data[off : off+n])
		f["Declared Length"], f["PCI Bytes"], f["Payload Offset"], f["Frame Padding"] = n, off-ae, off, bytes.Clone(data[off+n:])
	case 1:
		if r.active {
			return true, refuse(ErrContextRequired, "first frame overlaps active segmented transfer")
		}
		if r.blocked {
			return true, fail(ErrContextRequired, "direction retired after ambiguous or refused transfer")
		}
		if len(data) < 8 || len(data) < ae+2 {
			return true, refuse(ErrMalformedMessage, "first frame must fill canonical link payload")
		}
		n, off := int(data[ae]&15)<<8|int(data[ae+1]), ae+2
		if n == 0 {
			if len(data) < ae+6 {
				return true, refuse(ErrMalformedMessage, "32-bit first-frame length is incomplete")
			}
			wide := binary.BigEndian.Uint32(data[ae+2 : ae+6])
			if wide <= 4095 {
				return true, refuse(ErrMalformedMessage, "32-bit first-frame escape requires larger payload")
			}
			if wide > 1025*1024 || uint64(wide) > uint64(a.budget.MaxMessageBytes) {
				return true, refuse(ErrResourceExceeded, "32-bit declared message budget")
			}
			n = int(wide)
			off = ae + 6
		}
		singleCapacity := len(data) - ae - 1
		if len(data) > 8 {
			singleCapacity--
		}
		if n <= singleCapacity || n <= len(data)-off {
			return true, refuse(ErrMalformedMessage, "first frame length belongs in a single frame")
		}
		if n > a.budget.MaxMessageBytes {
			return true, refuse(ErrResourceExceeded, "declared message budget")
		}
		cfBytes := len(data) - ae - 1
		frames := (n-(len(data)-off)+cfBytes-1)/cfBytes + 1
		capRefs := min(2*frames+int(options.MaxWaitFrames)+8, sessionCollectionLimit(a.budget.MaxCollectionElements))
		if frames+1 > capRefs {
			return true, refuse(ErrResourceExceeded, "declared contributor budget")
		}
		cost := int64(n) + int64(capRefs)*(int64(unsafe.Sizeof(PacketReference{}))+int64(len(e.Domain.Encapsulation)))
		if !a.reserveEvidence(cost) {
			return true, refuse(ErrResourceExceeded, "segmented payload/contributor byte budget")
		}
		r.active, r.waiting, r.firstFC = true, true, false
		r.body = make([]byte, n)
		r.refs = make([]PacketReference, 0, capRefs)
		r.payloadCost = cost
		s.cost += cost
		r.size, r.offset, r.ll = n, len(data)-off, len(data)
		r.next = 1
		r.waits = 0
		r.lastCF = time.Time{}
		r.at = e.Timestamp
		r.refs = append(r.refs, refs[0])
		copy(r.body, data[off:])
		f["Declared Length"], f["PCI Bytes"], f["Payload Offset"], f["Received Length"] = n, off-ae, off, r.offset
	case 2:
		f["PCI Bytes"], f["Payload Offset"] = 1, ae+1
		if !r.active {
			return true, fail(ErrContextRequired, "first frame of active transfer was not captured")
		}
		if r.waiting {
			return true, refuse(ErrContextRequired, "consecutive data before observed CTS")
		}
		if data[ae]&15 != r.next {
			return true, refuse(ErrMalformedMessage, "consecutive frame sequence mismatch")
		}
		if len(data) > r.ll || len(data) < r.ll && r.size-r.offset > r.ll-ae-1 || len(data) <= ae+1 {
			return true, refuse(ErrMalformedMessage, "consecutive frame length differs from declared link payload")
		}
		if !e.Timestamp.IsZero() && !r.lastCF.IsZero() && e.Timestamp.Sub(r.lastCF) < r.stmin {
			return true, refuse(ErrContextRequired, "consecutive frame violates observed STmin")
		}
		n := min(len(data)-ae-1, r.size-r.offset)
		if err := padding(ae + 1 + n); err != nil {
			return true, err
		}
		if len(r.refs) == cap(r.refs) {
			return true, refuse(ErrResourceExceeded, "contributor count budget")
		}
		r.refs = append(r.refs, refs[0])
		copy(r.body[r.offset:], data[ae+1:ae+1+n])
		r.offset += n
		r.next = (r.next + 1) & 15
		r.at = e.Timestamp
		r.lastCF = e.Timestamp
		r.inBlock++
		f["Sequence"], f["Declared Length"], f["Received Length"], f["Frame Padding"] = data[ae]&15, r.size, r.offset, bytes.Clone(data[ae+1+n:])
		refs = append([]PacketReference(nil), r.refs...)
		if r.offset == r.size {
			body = bytes.Clone(r.body)
			a.releaseCANISOPayload(s, r)
		} else if r.block != 0 && r.inBlock >= r.block {
			r.waiting = true
		}
	case 3:
		f["PCI Bytes"], f["Payload Offset"] = 3, ae+3
		if !r.active {
			return true, fail(ErrContextRequired, "flow control lacks reverse active first frame")
		}
		if !r.waiting {
			return true, refuse(ErrContextRequired, "flow control arrives before block boundary")
		}
		if len(data) < ae+3 {
			return true, refuse(ErrMalformedMessage, "flow-control fields incomplete")
		}
		if err := padding(ae + 3); err != nil {
			return true, err
		}
		fs, bs, code := data[ae]&15, int(data[ae+1]), data[ae+2]
		if fs > 2 {
			return true, refuse(ErrMalformedMessage, "flow-control status is reserved")
		}
		stmin := time.Duration(code) * time.Millisecond
		reserved := false
		if code >= 0xf1 && code <= 0xf9 {
			stmin = time.Duration(code-0xf0) * 100 * time.Microsecond
		} else if code > 0x7f {
			reserved = true
			stmin = 127 * time.Millisecond
		}
		if len(r.refs) == cap(r.refs) {
			return true, refuse(ErrResourceExceeded, "contributor count budget")
		}
		r.refs = append(r.refs, refs[0])
		refs = append([]PacketReference(nil), r.refs...)
		r.at = e.Timestamp
		f["Flow Status"], f["Block Size"], f["STmin Code"], f["STmin Reserved"], f["Controlled Direction"], f["Frame Padding"] = fs, bs, code, reserved, stateDir, bytes.Clone(data[ae+3:])
		f["Parameters Applied"] = fs == 0
		if fs == 0 {
			f["Effective STmin Nanoseconds"] = int64(stmin)
		}
		if options.DynamicFlowControl {
			f["Parameters Applied"] = true
			f["Effective STmin Nanoseconds"] = int64(stmin)
			r.block, r.stminCode, r.stmin = bs, code, stmin
		}
		switch fs {
		case 0:
			if r.firstFC && !options.DynamicFlowControl && (bs != r.block || code != r.stminCode) {
				return true, refuse(ErrUnsupportedFeature, "changed FC parameters need a dynamic-parameter profile")
			}
			r.firstFC = true
			r.block, r.stminCode, r.stmin = bs, code, stmin
			r.waiting = false
			r.inBlock = 0
		case 1:
			r.waits++
			if r.waits > int(options.MaxWaitFrames) {
				return true, refuse(ErrResourceExceeded, "flow-control wait count budget")
			}
			f["Wait Count"] = r.waits
		case 2:
			f["Receiver Overflow"] = true
			a.releaseCANISOPayload(s, r)
			r.blocked = true
			s.pending = nil
		}
	default:
		return true, refuse(ErrUnsupportedFeature, "PCI type outside ISO-TP SF/FF/CF/FC")
	}
	if typ == 1 {
		refs = append([]PacketReference(nil), r.refs...)
	}
	if body != nil {
		f["Transfer Complete"] = true
		f["Payload"] = body
		if len(refs) > 1 {
			e.SourceBytes = ByteSource{Kind: "reassembled", PacketRefs: refs}
		}
		if options.DecodeUDS {
			uds, err := a.decodeCANUDS(s, dir, body, e)
			if err != nil {
				return true, err
			}
			fields["UDS"] = uds
		}
	}
	fields["ISO-TP"] = f
	if len(refs) > 1 {
		e.SourceBytes = ByteSource{Kind: "reassembled", PacketRefs: refs}
	}
	return true, nil
}

func (a *binParser) closeCANISO() {
	a.canISOMu.Lock()
	var events []*ProtocolEvent
	if a.canISO != nil {
		for key, s := range a.canISO.entries {
			if s.pending != nil && !s.pending.suppressed {
				p := s.pending
				err := protocolError(ErrNeedMore, "CAN UDS capture ended before terminal response")
				status, typed := classifySessionError(err)
				events = append(events, &ProtocolEvent{nativeCANRecord: true, Timestamp: p.at, Transport: "can", Protocol: "can-isotp", Profile: "can-iso15765-2-selected-uds", Admission: "explicit-interface-id-pair", Direction: 0, Domain: key.domain, Status: status, Completeness: status, Error: err.Error(), sessionError: typed, SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{p.ref}}})
			}
			for dir := range s.directions {
				r := &s.directions[dir]
				if r.active {
					err := protocolError(ErrNeedMore, "CAN ISO-TP capture ended before complete transfer")
					status, typed := classifySessionError(err)
					profile := canISOProfile
					if s.options.DecodeUDS {
						profile = "can-iso15765-2-selected-uds"
					}
					events = append(events, &ProtocolEvent{nativeCANRecord: true, Timestamp: r.at, Transport: "can", Protocol: "can-isotp", Profile: profile, Admission: "explicit-interface-id-pair", Direction: dir, Domain: key.domain, Status: status, Completeness: status, Error: err.Error(), sessionError: typed, SourceBytes: ByteSource{Kind: "reassembled", PacketRefs: append([]PacketReference(nil), r.refs...)}})
				}
			}
		}
	}
	a.disableCANISOLocked()
	a.canISOMu.Unlock()
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
		return a.Direction < b.Direction
	})
	for _, e := range events {
		a.incomplete.Add(1)
		a.emit(e)
	}
}
