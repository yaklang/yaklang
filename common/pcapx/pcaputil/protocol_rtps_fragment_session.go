package pcaputil

import (
	"encoding/hex"
	"fmt"
	"sort"
	"time"
	"unsafe"
)

const rtpsFragmentProfile = "rtps-spdp-fragment-session"
const rtpsFragmentTTL = 30 * time.Second // local pending observation policy
const rtpsConversationBase = int64(unsafe.Sizeof(binFlow{})) + 2048

type rtpsPendingSample struct {
	sample *rtpsSample
	cost   int64
	at     time.Time
	ids    []uint64
	refs   []PacketReference
}
type rtpsFragmentConversation struct {
	flow    *binFlow
	clock   time.Time
	seen    map[string]*rtpsPendingSample
	blocked bool
	charge  int64
}
type rtpsFragmentStore struct {
	entries  map[binUDPKey]*rtpsFragmentConversation
	disabled bool
}

// WithSessionRTPSFragments selects the same opt-in UDP profile as DecodeAs.
// It does not set endpoint ports or carrier; those remain explicit caller facts.
func WithSessionRTPSFragments(port uint16) ProtocolSessionOption {
	return func(s *captureSession) error {
		if port == 0 {
			return fmt.Errorf("RTPS fragment port must be nonzero")
		}
		old := s.f.a.datagramDecodeAs[port]
		if old != "" && old != "rtps-spdp-fragments" {
			return fmt.Errorf("conflicting RTPS fragment profile")
		}
		s.f.a.datagramDecodeAs[port] = "rtps-spdp-fragments"
		return nil
	}
}
func (a *binParser) rtpsFragmentsSelected(src, dst uint16) bool {
	p := a.datagramDecodeAs[dst]
	if p == "" {
		p = a.datagramDecodeAs[src]
	}
	return p == "rtps-spdp-fragments"
}
func (p *rtpsPendingSample) retire(c *rtpsFragmentConversation) {
	p.sample.retire()
	p.refs = nil
	p.ids = nil
	c.charge -= p.cost - 2048
	p.cost = 2048
}
func (c *rtpsFragmentConversation) retireAll() {
	for _, s := range c.seen {
		s.retire(c)
	}
}
func (a *binParser) consumeRTPSFragments(e *ProtocolEvent, w []byte, inputErr error) (map[string]any, error) {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	if a.rtpsFragments == nil {
		a.rtpsFragments = &rtpsFragmentStore{entries: map[binUDPKey]*rtpsFragmentConversation{}}
	}
	store := a.rtpsFragments
	if store.disabled {
		return nil, protocolError(ErrResourceExceeded, "RTPS fragment conversation history exhausted")
	}
	key := binUDPKey{e.Source, e.Destination, e.Domain}
	c := store.entries[key]
	if c == nil {
		if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			store.disabled = true
			return nil, protocolError(ErrResourceExceeded, "RTPS directed conversation limit")
		}
		f := &binFlow{a: a, id: a.flows.Add(1), protocol: "rtps", endpoints: [2]string{e.Source, e.Destination}}
		if err := f.reserveSession(rtpsConversationBase); err != nil {
			store.disabled = true
			return nil, err
		}
		c = &rtpsFragmentConversation{flow: f, clock: e.Timestamp, seen: map[string]*rtpsPendingSample{}, charge: rtpsConversationBase}
		store.entries[key] = c
	}
	defer func() {
		if c.flow.sessionBytes > c.charge {
			delta := c.flow.sessionBytes - c.charge
			c.flow.sessionBytes = c.charge
			a.buffered.Add(-delta)
		}
	}()
	e.FlowID = c.flow.id
	if e.Source > e.Destination {
		e.Direction = 1
	}
	if e.Timestamp.Before(c.clock) {
		c.blocked = true
		c.retireAll()
		return nil, protocolError(ErrContextRequired, "RTPS backdated datagram cannot establish progression")
	}
	if e.Timestamp.After(c.clock) {
		c.clock = e.Timestamp
	}
	for _, s := range c.seen {
		if s.sample.data != nil && c.clock.Sub(s.at) >= rtpsFragmentTTL {
			s.retire(c)
		}
	}
	if c.blocked {
		return nil, protocolError(ErrContextRequired, "RTPS conversation has untracked refused history")
	}
	fail := func(err error) (map[string]any, error) { c.blocked = true; c.retireAll(); return nil, err }
	if inputErr != nil {
		return fail(inputErr)
	}
	if a.budget.MaxRecursionDepth < 5 {
		return fail(protocolError(ErrResourceExceeded, "RTPS fragment projection exceeds depth budget"))
	}
	// Charge bounded transient decode/projection space before parsing or copying.
	if err := c.flow.reserveSession(c.charge + 16384 + 64*int64(len(w))); err != nil {
		return fail(err)
	}
	var fragments []rtpsDataFragment
	remaining := a.budget.MaxCollectionElements
	fields, err := decodeRTPSSelectedDatagram(w, a.budget.MaxCollectionElements, &fragments, &remaining)
	if err != nil {
		return fail(err)
	}
	currentQoS := map[string]bool{}
	for _, f := range fragments {
		if len(f.qosWire) > 0 {
			currentQoS[f.key()] = true
		}
	}
	var completed []rtpsCompletedChange
	for _, f := range fragments {
		identity := f.key()
		pending := c.seen[identity]
		if pending == nil {
			if len(c.seen) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
				return fail(protocolError(ErrResourceExceeded, "RTPS sample identity history limit"))
			}
			n := (uint64(f.total) + uint64(f.size) - 1) / uint64(f.size)
			if uint64(f.total) > uint64(min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)) || n > uint64(a.budget.MaxCollectionElements) {
				return fail(protocolError(ErrResourceExceeded, "RTPS assembled sample exceeds byte/fragment budget"))
			}
			// Retain bounded identity history; release sample/projection charges after completion/refusal.
			charge := int64(2048) + int64(f.total)*16 + int64(n)*128 + int64(len(f.qosWire))*8
			if err := c.flow.reserveSession(c.charge + charge + 16384 + 64*int64(len(w))); err != nil {
				return fail(err)
			}
			sample, err := rtpsNewSample(f, min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes), a.budget.MaxCollectionElements)
			if err != nil {
				return fail(err)
			}
			c.charge += charge
			pending = &rtpsPendingSample{sample: sample, at: c.clock, cost: charge}
			c.seen[identity] = pending
		}
		sample := pending.sample
		duplicate, err := sample.add(f)
		if err != nil {
			pending.retire(c)
			return nil, err
		}
		if !duplicate && (len(pending.ids) == 0 || pending.ids[len(pending.ids)-1] != e.ID) {
			if len(pending.ids) >= a.budget.MaxCollectionElements || len(pending.refs)+len(e.SourceBytes.PacketRefs) > a.budget.MaxCollectionElements {
				pending.retire(c)
				return nil, protocolError(ErrResourceExceeded, "RTPS sample contributor limit")
			}
			extra := int64(16 + 64*len(e.SourceBytes.PacketRefs))
			if err := c.flow.reserveSession(c.charge + extra + 16384 + 64*int64(len(w))); err != nil {
				pending.retire(c)
				return nil, err
			}
			pending.cost += extra
			c.charge += extra
			pending.ids = append(pending.ids, e.ID)
			pending.refs = append(pending.refs, e.SourceBytes.PacketRefs...)
		}
		if sample.finished() {
			if !currentQoS[identity] {
				remaining -= sample.first.qosElements
				if remaining < 0 {
					pending.retire(c)
					return nil, protocolError(ErrResourceExceeded, "RTPS retained inline QoS exceeds projection budget")
				}
			}
			var change map[string]any
			if f.fullData {
				change, err = sample.decorate(fields["Changes"].([]map[string]any)[f.position])
			} else {
				change, err = sample.change(&remaining)
			}
			if err != nil {
				pending.retire(c)
				return nil, err
			}
			change["Assembly Scope"] = "selected-directed-UDP-observation"
			change["Fragment Event IDs"] = append([]uint64(nil), pending.ids...)
			change["Serialized Payload"] = hex.EncodeToString(sample.data)
			change["Association Verified"] = false
			change["Sequence Reuse Policy"] = "quarantined for capture lifetime"
			if f.fullData {
				plain := fields["Changes"].([]map[string]any)
				plain[f.position] = change
			} else {
				completed = append(completed, rtpsCompletedChange{f.position, change})
			}
			pending.retire(c)
		}
	}
	changes := rtpsMergeChanges(fields["Changes"].([]map[string]any), completed)
	if len(changes) == 0 {
		return nil, protocolError(ErrNeedMore, "RTPS observed fragments await complete SPDP sample")
	}
	fields["Changes"] = changes
	// Raw/SourceBytes describe this literal datagram. The reconstructed sample
	// bytes and complete association contributors are explicit per-change fields;
	// do not hash only the final wire and label it as the whole reassembled sample.
	return fields, nil
}
func (a *binParser) decodeRTPSFragmentDatagram(e *ProtocolEvent, w []byte, inputErr error) bool {
	e.Protocol, e.Profile, e.Admission, e.Completeness = "rtps", rtpsFragmentProfile, "explicit-decode-as", "message"
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if inputErr == nil && len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		inputErr = protocolError(ErrResourceExceeded, "RTPS wire exceeds frame/message budget")
	}
	fields, err := a.consumeRTPSFragments(e, w, inputErr)
	if err == nil {
		e.semanticFields = cloneSession(fields)
		e.semanticSession = cloneSession(fields)
		e.Session = cloneSession(fields)
	}
	raw := w
	if inputErr != nil {
		raw = nil
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if raw == nil {
		a.messageBytes.Add(uint64(len(w)))
		if e.Status == "limited" {
			a.limited.Add(uint64(len(w)))
		}
	}
	if err != nil {
		e.Completeness = e.Status
	}
	return true
}

// Called under udpMu; callbacks run only after the enclosing Close unlocks.
func (a *binParser) closeRTPSFragmentsLocked() []*ProtocolEvent {
	var events []*ProtocolEvent
	if store := a.rtpsFragments; store != nil {
		for key, c := range store.entries {
			for _, p := range c.seen {
				if p.sample.data != nil {
					err := &ProtocolError{Kind: ErrNeedMore, Message: "RTPS exchange closed with missing sample fragments"}
					e := &ProtocolEvent{FlowID: c.flow.id, Timestamp: c.clock, Domain: key.domain, Source: key.a, Destination: key.b, Transport: "udp", Protocol: "rtps", Profile: rtpsFragmentProfile, Status: "incomplete", Completeness: "incomplete", Error: err.Message, sessionError: err, SourceBytes: ByteSource{Kind: "reassembled", ParentPDUs: append([]uint64(nil), p.ids...), PacketRefs: append([]PacketReference(nil), p.refs...)}}
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
	a.rtpsFragments = nil
	sort.Slice(events, func(i, j int) bool { return events[i].SourceBytes.ParentPDUs[0] < events[j].SourceBytes.ParentPDUs[0] })
	return events
}

func (s *captureSession) probeRTPSFragments(data []byte) (ProbeResult, bool) {
	if !s.f.a.rtpsFragmentsSelected(s.f.ports[0], s.f.ports[1]) {
		return ProbeResult{}, false
	}
	reject := func(reason string) (ProbeResult, bool) {
		return ProbeResult{Verdict: ProbeReject, Reason: reason}, true
	}
	if len(data) > min(s.f.a.budget.MaxFrameBytes, s.f.a.config.MaxMessageBytes) || s.f.a.budget.MaxRecursionDepth < 5 {
		return reject("RTPS fragment byte/depth budget")
	}
	var fragments []rtpsDataFragment
	remaining := s.f.a.budget.MaxCollectionElements
	_, err := decodeRTPSSelectedDatagram(data, remaining, &fragments, &remaining)
	if err != nil {
		return reject(err.Error())
	}
	for _, f := range fragments {
		if uint64(f.total) > uint64(s.f.a.budget.MaxFrameBytes) || (uint64(f.total)+uint64(f.size)-1)/uint64(f.size) > uint64(s.f.a.budget.MaxCollectionElements) {
			return reject("RTPS sample byte/fragment budget")
		}
	}
	return probeAccept("rtps", rtpsFragmentProfile, 98), true
}
