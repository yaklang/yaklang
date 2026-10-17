package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"sort"
	"strings"
	"time"
	"unsafe"
)

const aisMultipartProfile = "nmea-ais-type5-udp-session"
const aisMultipartTTL = 30 * time.Second // local observation policy, not an AIS identity lifetime
const aisConversationBytes = int64(unsafe.Sizeof(binFlow{})) + 1024

type aisFragmentKey struct{ sentence, sequence, channel string }
type aisFragmentPending struct {
	id     uint64
	at     time.Time
	digest [32]byte
	wire   []byte
	refs   []PacketReference
}
type aisConversation struct {
	flow    *binFlow
	seen    map[aisFragmentKey]*aisFragmentPending
	blocked bool
	clock   time.Time
}
type aisMultipartStore struct {
	entries map[binUDPKey]*aisConversation
}

func (a *binParser) aisMultipartSelected(src, dst uint16) bool {
	p := a.datagramDecodeAs[dst]
	if p == "" {
		p = a.datagramDecodeAs[src]
	}
	return p == "nmea-ais-multipart"
}

// A feed contains one whole sentence, not an arbitrary slice of a UDP stream.
// Nonempty sequence and channel labels are required by this explicit profile;
// the stateless decoder continues to observe absent labels within one datagram.
func aisMultipartFragment(w []byte) ([]string, error) {
	if len(w) < 10 || len(w) > 82 || w[0] != '!' || !bytes.HasSuffix(w, []byte("\r\n")) || bytes.Count(w, []byte{'\n'}) != 1 {
		return nil, protocolError(ErrMalformedMessage, "AIS session requires one complete bounded sentence per datagram")
	}
	b := w[1 : len(w)-2]
	star := len(b) - 3
	if star < 1 || b[star] != '*' || bytes.IndexByte(b[:star], '*') >= 0 {
		return nil, protocolError(ErrMalformedMessage, "AIS fragment checksum suffix is invalid")
	}
	for _, c := range b {
		if c < 32 || c > 126 {
			return nil, protocolError(ErrMalformedMessage, "AIS fragment is not printable ASCII")
		}
	}
	hi, lo := nmeaHex(b[star+1]), nmeaHex(b[star+2])
	if hi < 0 || lo < 0 {
		return nil, protocolError(ErrMalformedMessage, "AIS fragment checksum is not hexadecimal")
	}
	var sum byte
	for _, c := range b[:star] {
		sum ^= c
	}
	if sum != byte(hi*16+lo) {
		return nil, protocolError(ErrMalformedMessage, "AIS fragment XOR checksum mismatch")
	}
	p := strings.Split(string(b[:star]), ",")
	if err := nmeaAISEnvelope(p); err != nil {
		return nil, err
	}
	if p[1] != "2" {
		return nil, protocolError(ErrUnsupportedFeature, "AIS session selects two-fragment Type5 reports")
	}
	if p[3] == "" || p[4] == "" {
		return nil, protocolError(ErrContextRequired, "AIS cross-datagram labels must be present")
	}
	if p[2] == "1" && (p[6] != "0" || len(p[5]) > 70) || p[2] == "2" && p[6] != "2" {
		return nil, protocolError(ErrMalformedMessage, "AIS Type5 fragment length/fill is invalid")
	}
	if p[2] == "1" && p[5][0] != '5' {
		return nil, protocolError(ErrUnsupportedFeature, "AIS first fragment is outside Type5")
	}
	return p, nil
}

func (c *aisConversation) retireAll() {
	for _, v := range c.seen {
		v.wire, v.refs = nil, nil
	}
}

// No passive sequence digit proves a sender, generation or authentic message.
// Histories are directed endpoint/domain observations. Completed, expired,
// conflicting and refused identities never reopen during this capture. Exact
// active retries do not extend the deadline. Exhausted history fails closed.
func (a *binParser) assembleAISMultipart(e *ProtocolEvent, w []byte, inputErr error) ([]byte, map[string]any, error) {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	if a.aisMultipartDisabled {
		return nil, nil, protocolError(ErrResourceExceeded, "AIS association history exhausted")
	}
	key := binUDPKey{e.Source, e.Destination, e.Domain} // deliberately directed
	s := a.aisMultipart
	if s == nil {
		s = &aisMultipartStore{entries: map[binUDPKey]*aisConversation{}}
		a.aisMultipart = s
	}
	c := s.entries[key]
	if c == nil {
		if len(s.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			a.aisMultipartDisabled = true
			return nil, nil, protocolError(ErrResourceExceeded, "AIS directed conversation budget")
		}
		f := &binFlow{a: a, id: a.flows.Add(1), protocol: "nmea", endpoints: [2]string{e.Source, e.Destination}}
		if err := f.reserveSession(aisConversationBytes); err != nil {
			a.aisMultipartDisabled = true
			return nil, nil, err
		}
		c = &aisConversation{flow: f, seen: map[aisFragmentKey]*aisFragmentPending{}, clock: e.Timestamp}
		s.entries[key] = c
	}
	e.FlowID = c.flow.id
	if e.Source > e.Destination {
		e.Direction = 1
	}
	if e.Timestamp.After(c.clock) {
		c.clock = e.Timestamp
	}
	for _, v := range c.seen {
		if v.wire != nil && c.clock.Sub(v.at) >= aisMultipartTTL {
			v.wire, v.refs = nil, nil
		}
	}
	if c.blocked {
		return nil, nil, protocolError(ErrContextRequired, "AIS conversation contains untracked or refused fragments")
	}
	// Parsing is bounded to82 bytes. Unknown malformed identities poison the
	// conversation rather than allowing a denied fragment to disappear.
	var p []string
	var err error
	if inputErr != nil {
		err = inputErr
	} else {
		p, err = aisMultipartFragment(w)
	}
	if err != nil {
		c.blocked = true
		c.retireAll()
		return nil, nil, err
	}
	k := aisFragmentKey{p[0], p[3], p[4]}
	v := c.seen[k]
	if e.Timestamp.Before(c.clock) {
		c.blocked = true
		c.retireAll()
		return nil, nil, protocolError(ErrContextRequired, "AIS backdated fragment cannot establish progression")
	}
	if a.budget.MaxCollectionElements < 2 || a.budget.MaxRecursionDepth < 2 {
		c.blocked = true
		c.retireAll()
		return nil, nil, protocolError(ErrResourceExceeded, "AIS two-fragment projection exceeds element/depth budget")
	}
	refs := e.SourceBytes.PacketRefs
	if len(refs) > min(32, sessionCollectionLimit(a.budget.MaxCollectionElements)) {
		c.blocked = true
		c.retireAll()
		return nil, nil, protocolError(ErrResourceExceeded, "AIS contributor budget")
	}
	if v == nil {
		if len(c.seen) >= min(128, sessionCollectionLimit(a.budget.MaxCollectionElements)) {
			c.blocked = true
			c.retireAll()
			return nil, nil, protocolError(ErrResourceExceeded, "AIS sequence history budget")
		}
		// Conservative high-water charge retains identity history until Close;
		// packet reference capacity and bounded wire are charged before copying.
		if err := c.flow.reserveSession(aisConversationBytes + 4096*int64(len(c.seen)+1)); err != nil {
			c.blocked = true
			c.retireAll()
			return nil, nil, err
		}
		v = &aisFragmentPending{id: e.ID, at: c.clock, digest: sha256.Sum256(w)}
		c.seen[k] = v
		if p[2] != "1" {
			return nil, nil, protocolError(ErrContextRequired, "AIS final fragment has no observed first fragment")
		}
		v.wire = bytes.Clone(w)
		v.refs = append([]PacketReference(nil), refs...)
		return nil, nil, protocolError(ErrNeedMore, "AIS first fragment awaits its selected partner")
	}
	if p[2] == "1" {
		if v.wire != nil && v.digest == sha256.Sum256(w) {
			return nil, nil, protocolError(ErrNeedMore, "AIS exact active first-fragment retry")
		}
		v.wire, v.refs = nil, nil
		return nil, nil, protocolError(ErrContextRequired, "AIS sequence label reuse or conflict is ambiguous")
	}
	if v.wire == nil {
		return nil, nil, protocolError(ErrContextRequired, "AIS fragment identity is absent or quarantined")
	}
	first, firstRefs := v.wire, v.refs
	v.wire, v.refs = nil, nil // retire even if projection/length/resource validation fails
	if len(first)+len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		return nil, nil, protocolError(ErrResourceExceeded, "AIS assembled report exceeds message byte budget")
	}
	if len(firstRefs)+len(refs) > min(32, sessionCollectionLimit(a.budget.MaxCollectionElements)) {
		return nil, nil, protocolError(ErrResourceExceeded, "AIS assembled contributor budget")
	}
	projection := &binFlow{a: a}
	defer projection.closeSession()
	if err := projection.reserveSession(16384 + 512*int64(len(first)+len(w))); err != nil {
		return nil, nil, err
	}
	joined := make([]byte, 0, len(first)+len(w))
	joined = append(joined, first...)
	joined = append(joined, w...)
	fields, err := nmeaAISCompleteType5(joined, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
	if err != nil {
		return nil, nil, err
	}
	fields["Assembly Scope"] = "selected-directed-UDP-observation"
	fields["Fragment Event IDs"] = []uint64{v.id, e.ID}
	fields["Sequence Reuse Policy"] = "quarantined for capture lifetime"
	fields["Association Verified"] = false
	e.SourceBytes.PacketRefs = append(append([]PacketReference(nil), firstRefs...), refs...)
	e.SourceBytes.Kind = "reassembled"
	e.SourceBytes.ParentPDUs = []uint64{v.id, e.ID}
	return joined, fields, nil
}

func (a *binParser) decodeAISMultipartDatagram(e *ProtocolEvent, w []byte, inputErr error) bool {
	e.Protocol, e.Profile, e.Admission, e.Completeness = "nmea", aisMultipartProfile, "explicit-decode-as", "message"
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if inputErr == nil && len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		inputErr = protocolError(ErrResourceExceeded, "AIS fragment exceeds message byte budget")
	}
	raw, fields, err := a.assembleAISMultipart(e, w, inputErr)
	if err == nil {
		e.semanticFields = cloneSession(fields)
		e.semanticSession = cloneSession(fields)
		e.Session = cloneSession(fields)
	} else {
		e.Session = nil
		if len(w) <= 82 && inputErr == nil {
			raw = w
		} // refused oversized/truncated carriers never retain raw
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if err != nil {
		e.Completeness = e.Status
	}
	// finish counts assembled bytes; the first fragment was already observed.
	if err == nil {
		a.messageBytes.Add(^uint64(len(raw)-len(w)) + 1)
	} else if raw == nil {
		a.messageBytes.Add(uint64(len(w)))
		if e.Status == "limited" {
			a.limited.Add(uint64(len(w)))
		}
	}
	return true
}

// Called under udpMu. Close events are emitted only after the caller unlocks.
func (a *binParser) closeAISMultipartLocked() []*ProtocolEvent {
	var events []*ProtocolEvent
	if s := a.aisMultipart; s != nil {
		for key, c := range s.entries {
			for _, v := range c.seen {
				if v.wire != nil {
					err := &ProtocolError{Kind: ErrNeedMore, Message: "AIS exchange closed with a missing final fragment"}
					e := &ProtocolEvent{FlowID: c.flow.id, Timestamp: c.clock, Domain: key.domain, Source: key.a, Destination: key.b, Transport: "udp", Protocol: "nmea", Profile: aisMultipartProfile, Status: "incomplete", Completeness: "incomplete", Error: err.Message, sessionError: err, Raw: bytes.Clone(v.wire), Length: len(v.wire), SourceBytes: ByteSource{ParentPDUs: []uint64{v.id}, PacketRefs: append([]PacketReference(nil), v.refs...)}}
					if key.a > key.b {
						e.Direction = 1
					}
					events = append(events, e)
					a.incomplete.Add(1)
				}
			}
			c.retireAll()
			c.flow.closeSession()
		}
	}
	a.aisMultipart = nil
	a.aisMultipartDisabled = false
	sort.Slice(events, func(i, j int) bool { return events[i].SourceBytes.ParentPDUs[0] < events[j].SourceBytes.ParentPDUs[0] })
	return events
}
