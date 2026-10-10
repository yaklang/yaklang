package pcaputil

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Selected unciphered LN Get-normal and HDLC link controls, carried as complete
// HDLC frames through a TCP/UDP tunnel, plus bounded LN Get-with-list.
// ACSE, authentication, ciphering, segmentation and GBT remain
// explicit unsupported boundaries. Values and link identities are unverified.
func dlmsError(k ProtocolErrorKind, why string) error { return protocolError(k, "DLMS HDLC %s", why) }
func dlmsCRC(w []byte) uint16 {
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
	return ^c
}
func dlmsFrameSize(w []byte, limit int) (int, error) {
	if len(w) == 0 {
		return 0, nil
	}
	if w[0] != 0x7e {
		return 0, dlmsError(ErrDesynchronized, "opening flag is missing")
	}
	if len(w) < 3 {
		return 0, nil
	}
	f := binary.BigEndian.Uint16(w[1:3])
	if f>>12 != 0xa {
		return 0, dlmsError(ErrMalformedMessage, "frame format type is not 3")
	}
	n := int(f&0x7ff) + 2
	if n < 9 {
		return 0, dlmsError(ErrMalformedMessage, "declared frame is too short")
	}
	if n > limit {
		return 0, dlmsError(ErrResourceExceeded, "frame exceeds byte budget")
	}
	if len(w) < n {
		return 0, nil
	}
	if w[n-1] != 0x7e {
		return 0, dlmsError(ErrMalformedMessage, "closing flag differs from declared frame boundary")
	}
	return n, nil
}

type dlmsMessage struct {
	fields              map[string]any
	kind                string
	request             bool
	source, destination uint32
	invoke, ns, nr      byte
	flags, choice       byte
	count               int
	block               *wrapperMessage
}

func dlmsAddress(w []byte, at *int) (uint32, int, error) {
	start := *at
	v := uint32(0)
	for i := 0; i < 4; i++ {
		if *at >= len(w) {
			return 0, 0, dlmsError(ErrMalformedMessage, "address is truncated")
		}
		b := w[*at]
		(*at)++
		v = v<<7 | uint32(b>>1)
		if b&1 != 0 {
			n := *at - start
			if n == 3 {
				return 0, 0, dlmsError(ErrMalformedMessage, "address has three octets")
			}
			return v, n, nil
		}
	}
	return 0, 0, dlmsError(ErrMalformedMessage, "address exceeds four octets or lacks terminator")
}
func decodeDLMS(w []byte, maxElements int) (*dlmsMessage, error) {
	return decodeDLMSBudget(w, maxElements, DefaultParserBudget().MaxRecursionDepth)
}
func decodeDLMSBudget(w []byte, maxElements, depth int) (*dlmsMessage, error) {
	bad := func(s string) (*dlmsMessage, error) { return nil, dlmsError(ErrMalformedMessage, s) }
	unsupported := func(s string) (*dlmsMessage, error) { return nil, dlmsError(ErrUnsupportedFeature, s) }
	n, err := dlmsFrameSize(w, 2049)
	if err != nil {
		return nil, err
	}
	if n == 0 || n != len(w) {
		return bad("complete frame length differs from input")
	}
	format := binary.BigEndian.Uint16(w[1:3])
	end := len(w) - 3
	fcs := binary.LittleEndian.Uint16(w[end : len(w)-1])
	if dlmsCRC(w[1:end]) != fcs {
		return bad("FCS is invalid")
	}
	at := 3
	dest, ds, err := dlmsAddress(w[:end], &at)
	if err != nil {
		return nil, err
	}
	src, ss, err := dlmsAddress(w[:end], &at)
	if err != nil {
		return nil, err
	}
	if at >= end {
		return bad("control field is missing")
	}
	cf := w[at]
	at++
	m := &dlmsMessage{source: src, destination: dest, count: 1, choice: 1}
	f := map[string]any{"Observation": "unverified-dlms-hdlc-wire-values", "Frame Format": format, "Frame Length": len(w) - 2, "Destination Address": dest, "Source Address": src, "Destination Address Octets": ds, "Source Address Octets": ss, "Control": cf, "Poll/Final": cf&0x10 != 0, "FCS": fcs}
	m.fields = f
	var info []byte
	if at < end {
		if end-at < 3 {
			return bad("HCS/information is truncated")
		}
		hcs := binary.LittleEndian.Uint16(w[at : at+2])
		if dlmsCRC(w[1:at]) != hcs {
			return bad("HCS is invalid")
		}
		f["HCS"] = hcs
		info = w[at+2 : end]
	}
	if format&0x800 != 0 {
		return unsupported("segmented information is outside this profile")
	}
	if cf&3 == 3 {
		names := map[byte]string{0x83: "SNRM", 0x63: "UA", 0x43: "DISC", 0x0f: "DM"}
		kind, ok := names[cf&0xef]
		if !ok {
			return unsupported("unnumbered control is outside this profile")
		}
		m.kind = kind
		m.request = kind == "SNRM" || kind == "DISC"
		f["Frame Kind"] = kind
		if kind == "SNRM" || kind == "UA" {
			neg, e := dlmsNegotiation(info, maxElements)
			if e != nil {
				return nil, e
			}
			f["Negotiation"] = neg
		} else if len(info) != 0 {
			return unsupported("nonempty disconnect/error control information")
		}
		return m, nil
	}
	if cf&1 != 0 {
		if len(info) != 0 {
			return bad("supervisory frame has information")
		}
		m.kind = "S"
		m.nr = cf >> 5
		f["Frame Kind"] = "supervisory"
		f["Supervisory Function"] = []string{"RR", "RNR", "REJ", "SREJ"}[(cf>>2)&3]
		f["Receive Sequence"] = cf >> 5
		return m, nil
	}
	m.ns = (cf >> 1) & 7
	m.nr = cf >> 5
	f["Send Sequence"] = m.ns
	f["Receive Sequence"] = m.nr
	if len(info) < 4 {
		return bad("information frame LLC/APDU is truncated")
	}
	if info[0] != 0xe6 || info[2] != 0 || info[1] != 0xe6 && info[1] != 0xe7 {
		return bad("LLC header is invalid")
	}
	f["LLC"] = bytes.Clone(info[:3])
	p := info[3:]
	if p[0] != 0xc0 && p[0] != 0xc4 {
		return unsupported("APDU is outside unencrypted LN Get-normal")
	}
	if len(p) < 3 {
		return bad("Get service header is truncated")
	}
	if p[1] != 1 && p[1] != 2 && p[1] != 3 {
		return unsupported("Get service choice is outside this profile")
	}
	if p[2]&0x30 != 0 {
		return bad("invoke-id-and-priority reserved bits are nonzero")
	}
	m.invoke = p[2] & 15
	m.flags, m.choice = p[2], p[1]
	m.request = p[0] == 0xc0
	m.kind = "GET"
	f["Invoke ID and Priority"] = p[2]
	f["Invoke ID"] = m.invoke
	f["High Priority"] = p[2]&0x80 != 0
	f["Confirmed Service"] = p[2]&0x40 != 0
	if p[1] == 2 {
		if m.request && info[1] != 0xe6 || !m.request && info[1] != 0xe7 {
			return bad("Get-block LLC direction differs from command")
		}
		b := &wrapperMessage{fields: make(map[string]any), request: m.request}
		if err := decodeWrapperBlock(b, p, maxElements); err != nil {
			return nil, dlmsBlockError(err)
		}
		m.block = b
		f["Get Block"] = b.fields
		f["Frame Kind"] = "Get Response With Data Block"
		if m.request {
			f["Frame Kind"] = "Get Request Next"
		}
		return m, nil
	}
	if p[1] == 3 {
		if m.request && info[1] != 0xe6 || !m.request && info[1] != 0xe7 {
			return bad("Get-list LLC direction differs from command")
		}
		if err := dlmsListFields(m, p, maxElements, depth); err != nil {
			return nil, err
		}
		return m, nil
	}
	if m.request {
		if info[1] != 0xe6 {
			return bad("Get request uses response LLC")
		}
		if len(p) < 13 {
			return bad("Get-normal descriptor is truncated")
		}
		if p[12] != 0 && p[12] != 1 {
			return unsupported("selected optional access selection0/1")
		}
		if p[12] == 0 && len(p) != 13 {
			return bad("Get-normal descriptor has trailing bytes")
		}
		if p[11] == 0 || p[11] > 127 {
			return unsupported("attribute outside positive signed8 profile")
		}
		f["Frame Kind"] = "Get Request Normal"
		f["Class ID"] = binary.BigEndian.Uint16(p[3:5])
		f["Logical Name"] = fmt.Sprintf("%d.%d.%d.%d.%d.%d", p[5], p[6], p[7], p[8], p[9], p[10])
		f["Attribute ID"] = int8(p[11])
		f["Selective Access"] = p[12] == 1
		if p[12] == 1 {
			if len(p) < 14 {
				return bad("Get-normal access selector is missing")
			}
			parameter, err := dlmsNormalDataAt(p, 14, maxElements, depth)
			if err != nil {
				return nil, err
			}
			// Only observe the selector and its complete Data parameter. Object
			// semantics, permissions and negotiated access are not established.
			f["Access Selection Raw"] = p[12]
			f["Access Selector"] = p[13]
			f["Access Parameters"] = parameter
			f["Selector Semantics Verified"] = false
		}
	} else {
		if info[1] != 0xe7 {
			return bad("Get response uses request LLC")
		}
		if len(p) < 5 {
			return bad("Get-normal result is truncated")
		}
		f["Frame Kind"] = "Get Response Normal"
		if p[3] == 1 {
			if len(p) != 5 {
				return bad("data-access-result has trailing data")
			}
			valid := map[byte]bool{0: true, 1: true, 2: true, 3: true, 4: true, 9: true, 11: true, 12: true, 13: true, 14: true, 15: true, 16: true, 17: true, 18: true, 19: true, 250: true}
			if !valid[p[4]] {
				return bad("data-access-result code is reserved")
			}
			f["Result Choice"] = "data-access-result"
			f["Data Access Result"] = p[4]
		} else if p[3] == 0 {
			var v any
			var e error
			if dlmsNormalExtended(p) {
				v, e = dlmsNormalData(p, maxElements, depth)
			} else {
				// Existing scalar/octet public representations remain unchanged.
				v, e = dlmsScalar(p[4:], maxElements)
			}
			if e != nil {
				return nil, e
			}
			f["Result Choice"] = "data"
			f["Data Type"] = p[4]
			f["Data Value"] = v
		} else {
			return bad("Get-normal result choice is invalid")
		}
	}
	return m, nil
}
func dlmsNegotiation(w []byte, limit int) ([]map[string]any, error) {
	out := make([]map[string]any, 0, 4)
	if len(w) == 0 {
		return out, nil
	}
	if len(w) < 3 || w[0] != 0x81 || w[1] != 0x80 || int(w[2]) != len(w)-3 {
		return nil, dlmsError(ErrMalformedMessage, "negotiation group header/length is invalid")
	}
	seen := byte(0)
	for at := 3; at < len(w); {
		if len(w)-at < 2 {
			return nil, dlmsError(ErrMalformedMessage, "negotiation parameter header is truncated")
		}
		tag, n := w[at], int(w[at+1])
		at += 2
		if tag < 5 || tag > 8 {
			return nil, dlmsError(ErrUnsupportedFeature, "negotiation parameter is outside selected link controls")
		}
		if seen&(1<<(tag-5)) != 0 {
			return nil, dlmsError(ErrMalformedMessage, "negotiation parameter repeats")
		}
		seen |= 1 << (tag - 5)
		if n != 1 && n != 2 && n != 4 || n > len(w)-at {
			return nil, dlmsError(ErrMalformedMessage, "negotiation parameter length is invalid")
		}
		v := uint32(0)
		for _, b := range w[at : at+n] {
			v = v<<8 | uint32(b)
		}
		at += n
		if len(out) >= limit {
			return nil, dlmsError(ErrResourceExceeded, "negotiation parameters exceed collection budget")
		}
		out = append(out, map[string]any{"Parameter": tag, "Octets": n, "Value": v})
	}
	return out, nil
}
func dlmsScalar(w []byte, limit int) (any, error) {
	bad := func(s string) (any, error) { return nil, dlmsError(ErrMalformedMessage, s) }
	if len(w) == 0 {
		return bad("A-XDR scalar is missing")
	}
	tag := w[0]
	b := w[1:]
	sizes := map[byte]int{0: 0, 3: 1, 5: 4, 6: 4, 15: 1, 16: 2, 17: 1, 18: 2, 20: 8, 21: 8, 22: 1}
	if tag == 9 {
		if len(b) == 0 {
			return bad("octet-string length is missing")
		}
		count, at := uint64(b[0]), 1
		if b[0] >= 0x80 {
			k := int(b[0] & 127)
			if k != 1 && k != 2 && k != 4 {
				return nil, dlmsError(ErrUnsupportedFeature, "octet-string length determinant is outside supported definite forms")
			}
			if len(b) < 1+k {
				return bad("octet-string long length is truncated")
			}
			count = 0
			for _, v := range b[1 : 1+k] {
				count = count<<8 | uint64(v)
			}
			at += k
		}
		if count > 1024 || count > uint64(limit) {
			return nil, dlmsError(ErrResourceExceeded, "octet string exceeds selected byte/collection budget")
		}
		if count != uint64(len(b)-at) {
			return bad("octet-string length differs from complete result")
		}
		return append([]byte{}, b[at:]...), nil
	}
	n, ok := sizes[tag]
	if !ok {
		return nil, dlmsError(ErrUnsupportedFeature, "A-XDR data type is outside selected scalars")
	}
	if len(b) != n {
		return bad("scalar length differs from complete result")
	}
	switch tag {
	case 0:
		return nil, nil
	case 3:
		if b[0] > 1 {
			return bad("boolean is outside 0/1")
		}
		return b[0] == 1, nil
	case 5:
		return int32(binary.BigEndian.Uint32(b)), nil
	case 6:
		return binary.BigEndian.Uint32(b), nil
	case 15:
		return int8(b[0]), nil
	case 16:
		return int16(binary.BigEndian.Uint16(b)), nil
	case 17, 22:
		return b[0], nil
	case 18:
		return binary.BigEndian.Uint16(b), nil
	case 20:
		return strconv.FormatInt(int64(binary.BigEndian.Uint64(b)), 10), nil
	case 21:
		return strconv.FormatUint(binary.BigEndian.Uint64(b), 10), nil
	}
	panic("unreachable scalar")
}
func probeDLMS(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 0x7e {
		return ProbeResult{Verdict: ProbeReject}
	}
	if probeMySQL(w, limit).Verdict == ProbeAccept {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, e := dlmsFrameSize(w, limit)
	if e != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 {
		return probeNeed("dlms", "hdlc-get-normal", len(w), max(3, len(w)+1))
	}
	elements := 4096
	list := dlmsListAPDU(w[:n])
	apdu := dlmsAPDU(w[:n])
	if list != nil || dlmsBlockAPDU(apdu) || dlmsNormalExtended(apdu) || dlmsNormalAccess(apdu) {
		// Probe never admits unsolicited responses. Check frame integrity with
		// a zero Data pool; selected requests expand only after reservation too.
		elements = 0
	}
	m, e := decodeDLMS(w[:n], elements)
	var pe *ProtocolError
	if errors.As(e, &pe) && pe.Kind == ErrResourceExceeded {
		if len(list) != 0 && list[0] == 0xc0 {
			return probeAccept("dlms", "hdlc-get-list", 98)
		}
		if dlmsNormalAccess(apdu) {
			return probeAccept("dlms", "hdlc-get-normal", 98)
		}
	}
	if e != nil || !m.request {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("dlms", "hdlc-get-normal", 98)
}

type dlmsPending struct {
	wire                []byte
	kind                string
	dir                 int
	source, destination uint32
	id                  uint64
	invoke, ns          byte
	flags, choice       byte
	count               int
}
type binDLMS struct {
	pending             *dlmsPending
	clientKnown         bool
	clientDir           int
	ambiguous, usedLink bool
	seenRequest         [16]uint8
	seenCount           uint16
	next                [2]byte
	seqKnown            [2]bool
	transfer            *dlmsTransfer
}

func (s *binDLMS) invalidate() {
	// A refused exchange is unobserved even between two completed requests.
	// Keeping only the old sequence history would let its late response acquire
	// a subsequent request's ID. Retire the conversation, not just a live slot.
	s.pending = nil
	s.transfer = nil
	s.ambiguous = true
}
func (s *binDLMS) consume(m *dlmsMessage, w []byte, dir int, id uint64) (map[string]any, uint64, error) {
	ctx := func(why string) (map[string]any, uint64, error) { return nil, 0, dlmsError(ErrContextRequired, why) }
	if m.kind == "DM" {
		// A disconnect-mode observation ends the association, even though its
		// wire fields remain useful. Late replies must not reuse the old slot.
		s.invalidate()
		m.fields["Association"] = "unassociated-link-observation"
		return m.fields, 0, nil
	}
	if s.ambiguous {
		return ctx("ambiguous/reused exchange requires new conversation")
	}
	if m.kind == "S" {
		if s.clientKnown && s.seqKnown[1-dir] {
			advance := (m.nr - s.next[1-dir]) & 7
			if advance > 0 && advance <= 4 {
				// Future (or half-space ambiguous) modulo8 acknowledgements expose
				// a hidden exchange. Old acknowledgements do not reopen or advance
				// request identity; preserve their literal link observation.
				s.invalidate()
			}
		}
		m.fields["Association"] = "unassociated-link-observation"
		return m.fields, 0, nil
	}
	if m.request {
		if s.transfer != nil {
			s.invalidate()
			return ctx("distinct request interrupts observed data-block transfer")
		}
		if s.clientKnown && dir != s.clientDir {
			return ctx("request contradicts observed requester direction")
		}
		if !s.clientKnown {
			s.clientKnown = true
			s.clientDir = dir
		}
		if s.pending != nil {
			if s.pending.dir == dir && bytes.Equal(s.pending.wire, w) {
				m.fields["Association"] = "retransmitted-request"
				return m.fields, 0, nil
			}
			s.invalidate()
			return ctx("more than one distinct outstanding request")
		}
		if m.kind == "GET" {
			if s.seenRequest[m.invoke]&(1<<m.ns) != 0 {
				s.ambiguous = true
				return ctx("invoke ID and HDLC send sequence reused in conversation")
			}
			if s.seqKnown[dir] && m.ns != s.next[dir] || s.seqKnown[1-dir] && m.nr != s.next[1-dir] {
				s.ambiguous = true
				return ctx("request sequence differs from observed progression")
			}
			s.seenRequest[m.invoke] |= 1 << m.ns
			s.seenCount++
			s.next[dir] = (m.ns + 1) & 7
			s.seqKnown[dir] = true
		} else {
			if s.usedLink {
				s.ambiguous = true
				return ctx("unsequenced link request reused")
			}
			s.usedLink = true
		}
		s.pending = &dlmsPending{wire: bytes.Clone(w), kind: m.kind, dir: dir, source: m.source, destination: m.destination, id: id, invoke: m.invoke, ns: m.ns, flags: m.flags, choice: m.choice, count: m.count}
		m.fields["Association"] = "observed-request"
		return m.fields, 0, nil
	}
	p := s.pending
	if p == nil && s.clientKnown && m.kind == "GET" && (!s.seqKnown[dir] || m.ns == s.next[dir]) {
		// An orphan at the next peer sequence can belong to a missed request.
		// Repeating that response after a later request cannot establish a new
		// binding. A prior-sequence duplicate still leaves adjacent traffic usable.
		s.invalidate()
	}
	if p == nil || p.dir == dir || p.source != m.destination || p.destination != m.source {
		return ctx("response lacks reversed observed endpoint/logical-address request")
	}
	if m.kind == "GET" {
		if p.kind != "GET" || p.invoke != m.invoke || p.flags != m.flags || p.choice != m.choice || p.count != m.count || m.nr != (p.ns+1)&7 {
			return ctx("response service/invoke/acknowledgement differs from pending request")
		}
		if s.seqKnown[dir] && m.ns != s.next[dir] {
			return ctx("response send sequence differs from observed progression")
		}
		s.next[dir] = (m.ns + 1) & 7
		s.seqKnown[dir] = true
	} else if m.kind != "UA" || p.kind != "SNRM" && p.kind != "DISC" {
		return ctx("response link control differs from pending request")
	}
	s.pending = nil
	if p.kind == "DISC" {
		s.ambiguous = true
	}
	m.fields["Association"] = "observed-response"
	return m.fields, p.id, nil
}

func dlmsRequestEvidence(w []byte) bool {
	return probeDLMS(w, 2049).Verdict == ProbeAccept
}

// Fixed direction/sequence/ID state plus an owned pending wire copy. Parsing
// fields and byte projections is reserved before it allocates either copy.
func (s *binDLMS) storage() int64 {
	n := int64(512)
	if s.pending != nil {
		n += int64(len(s.pending.wire)) * 2
	}
	if s.transfer != nil {
		n += 128 + 2*int64(len(s.transfer.data))
	}
	return n
}
func (f *binFlow) consumeDLMS(dir int, w []byte, id uint64) (map[string]any, uint64, error) {
	s := f.dlms
	if err := f.reserveSession(s.storage() + dlmsProjection(w)); err != nil {
		// Refusing memory cannot hide an on-wire exchange from the association
		// state. Retire the old slot before any later response can use it.
		s.invalidate()
		return nil, 0, err
	}
	m, err := decodeDLMSBudget(w, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth)
	if err != nil {
		s.invalidate()
		return nil, 0, err
	}
	return f.consumeDecodedDLMS(dir, w, id, m)
}
func (f *binFlow) consumeDecodedDLMS(dir int, w []byte, id uint64, m *dlmsMessage) (map[string]any, uint64, error) {
	s := f.dlms
	if m.kind == "GET" && m.choice == 2 {
		return f.consumeDLMSBlock(dir, w, id, m)
	}
	if m.request && m.kind == "GET" && s.pending == nil && s.seenRequest[m.invoke]&(1<<m.ns) == 0 && int(s.seenCount) >= f.a.budget.MaxCollectionElements {
		s.ambiguous = true
		return nil, 0, dlmsError(ErrResourceExceeded, "retained invoke/sequence token limit")
	}
	return s.consume(m, w, dir, id)
}

func dlmsRecoverableMessageError(err error) bool {
	pe, ok := err.(*ProtocolError)
	return ok && (pe.Kind == ErrMalformedMessage || pe.Kind == ErrContextRequired || pe.Kind == ErrUnsupportedFeature)
}

const dlmsIdleTTL = 30 * time.Second

func (a *binParser) decodeDLMSDatagram(e *ProtocolEvent, w []byte, explicit bool) bool {
	key := binUDPKey{"dlms/" + e.Source, "dlms/" + e.Destination, e.Domain}
	if key.a > key.b {
		key.a, key.b = key.b, key.a
	}
	a.udpMu.Lock()
	store := a.udpSessions
	existing := store != nil && store.entries[key] != nil
	if !explicit && !(len(w) > 0 && w[0] == 0x7e && (existing || dlmsRequestEvidence(w))) {
		a.udpMu.Unlock()
		return false
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "dlms", dlmsProfile(w), "wire-signature", "message"
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
		v := el.Value.(*binUDPEntry)
		if v.flow.dlms != nil && store.clock.Sub(v.touched) >= dlmsIdleTTL {
			v.flow.closeSession()
			delete(store.entries, v.key)
			store.lru.Remove(el)
		}
		el = next
	}
	el := store.entries[key]
	created := el == nil
	f := &binFlow{a: a, dlms: &binDLMS{}, endpoints: [2]string{e.Source, e.Destination}}
	if el != nil {
		v := el.Value.(*binUDPEntry)
		f = v.flow
		e.FlowID = f.id
		if e.Source != f.endpoints[0] {
			e.Direction = 1
		} else {
			e.Direction = 0
		}
		v.touched = store.clock
		store.lru.MoveToBack(el)
	}
	var m *dlmsMessage
	var err error
	oversize := len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	if oversize {
		err = dlmsError(ErrResourceExceeded, "frame exceeds byte budget")
	} else if err = f.reserveSession(f.dlms.storage() + dlmsProjection(w)); err == nil {
		m, err = decodeDLMSBudget(w, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
	}
	if created && m != nil && !m.request && m.kind != "S" {
		e.Direction = 1
	}
	if err == nil {
		if created && (m.kind == "S" || m.kind == "DM") {
			e.Session, e.ResponseTo, err = f.dlms.consume(m, w, e.Direction, e.ID)
		} else if created && !m.request {
			err = dlmsError(ErrContextRequired, "response lacks observed request context")
		} else if created && len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			err = dlmsError(ErrResourceExceeded, "UDP conversation limit")
		} else {
			e.Session, e.ResponseTo, err = f.consumeDecodedDLMS(e.Direction, w, e.ID, m)
			if err == nil && created {
				f.id = a.flows.Add(1)
				store.entries[key] = store.lru.PushBack(&binUDPEntry{key, f, store.clock})
				e.FlowID = f.id
				created = false
			}
		}
	}
	if err != nil {
		var pe *ProtocolError
		// Byte/semantic refusals hide an exchange from observation: retire its old
		// pending wire. Context mismatch preserves the adjacent exact reply behavior.
		if m == nil || errors.As(err, &pe) && pe.Kind == ErrResourceExceeded {
			f.dlms.invalidate()
		}
		e.Session, e.ResponseTo = nil, 0
		if f.dlms.storage() < f.sessionBytes {
			a.buffered.Add(f.dlms.storage() - f.sessionBytes)
			f.sessionBytes = f.dlms.storage()
		}
	}
	if created {
		f.closeSession()
	}
	a.udpMu.Unlock()
	if err == nil {
		e.semanticFields = cloneSession(e.Session)
	}
	var resource *ProtocolError
	if oversize || dlmsListAPDU(w) != nil && errors.As(err, &resource) && resource.Kind == ErrResourceExceeded {
		w = nil
	}
	a.finishProtocolDatagram(e, w, nil, err)
	return true
}
