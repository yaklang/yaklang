package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"
)

// This is an observed RADIUS/EAP adjacency chain, not an authenticated TLS
// session. State is opaque and not unique. A chain additionally requires a
// stable NAS + port + station descriptor and a unique observed request root.
type radiusTLSRounds struct {
	owners map[radiusKey]radiusTLSOwner
	rounds []*radiusTLSRound
}
type radiusTLSOwner struct {
	context [32]byte
	refused bool
}
type radiusTLSFragmentView struct {
	key                  radiusKey
	digest               [32]byte
	fragments, collected int
	complete             bool
}
type radiusTLSRound struct {
	history                    []radiusTLSFragmentView
	context                    [32]byte
	root, next, lastReply      radiusKey
	state, data                []byte
	declared                   uint32
	fragments                  int
	identifier                 byte
	replyDigest, ackDigest     [32]byte
	last                       time.Time
	awaitingACK, done, retired bool
}

func (r *radiusTLSRounds) storage() int64 {
	if r == nil {
		return 0
	}
	n := int64(512 + 128*len(r.owners) + 8*cap(r.rounds))
	for _, c := range r.rounds {
		n += 256 + int64(cap(c.state)+cap(c.data)) + 96*int64(cap(c.history))
	}
	return n
}
func (r *radiusTLSRounds) unfinished() (groups, bytes int) {
	if r != nil {
		for _, c := range r.rounds {
			if !c.retired && !c.done {
				groups++
				bytes += len(c.data)
			}
		}
	}
	return
}
func radiusTLSDescriptor(fields map[string]any) ([32]byte, bool) {
	var zero [32]byte
	attrs, ok := fields["Attributes"].([]map[string]any)
	if !ok {
		return zero, false
	}
	var values [256][]byte
	var seen [256]bool
	for _, a := range attrs {
		typ := a["Type"].(byte)
		switch typ {
		case 4, 5, 31, 32, 95:
			if seen[typ] {
				return zero, false
			}
			seen[typ] = true
			values[typ] = a["Raw"].([]byte)
		}
	}
	if len(values[5]) != 4 || len(values[31]) == 0 || len(values[4])+len(values[32])+len(values[95]) == 0 {
		return zero, false
	}
	h := sha256.New()
	for _, typ := range []byte{4, 5, 31, 32, 95} {
		if seen[typ] {
			h.Write([]byte{typ, byte(len(values[typ]))})
			h.Write(values[typ])
		}
	}
	copy(zero[:], h.Sum(nil))
	return zero, true
}
func radiusTLSState(fields map[string]any) ([]byte, bool) {
	var state []byte
	count := 0
	for _, a := range fields["Attributes"].([]map[string]any) {
		if a["Type"].(byte) == 24 {
			state = a["Raw"].([]byte)
			count++
		}
	}
	return state, count == 1 && len(state) > 0
}
func radiusTLSRoundDiagnostic(fields map[string]any, code string) {
	// Replace only the lack-of-round-context diagnosis which this observer can
	// now establish. Never swallow a normative or cipher/codec error.
	old, _ := fields["EAP Diagnostics"].([]string)
	diags := make([]string, 0, len(old)+1)
	for _, v := range old {
		if v != "eap-tls-fragment-context-required" && v != "eap-tls-record-context-required" {
			diags = append(diags, v)
		}
	}
	if code != "" {
		diags = append(diags, code)
	}
	fields["EAP Diagnostics"] = diags
}
func radiusTLSRoundRefusal(fields map[string]any) error {
	radiusTLSRoundDiagnostic(fields, "eap-tls-fragment-context-required")
	return &radiusEAPFailure{protocolError(ErrContextRequired, "EAP-TLS observed round chain is missing or ambiguous")}
}
func radiusTLSRoundCopy(w []byte) []byte {
	if w == nil {
		return nil
	}
	r := make([]byte, len(w))
	copy(r, w)
	return r
}
func (c *radiusTLSRound) discard() {
	clear(c.data)
	c.data = nil
	c.history = nil
	c.retired = true
	c.awaitingACK = false
}
func (c *radiusTLSRound) annotate(fields, t map[string]any, max int) error {
	if c.done {
		// A final fragment may coincidentally contain locally complete records.
		// Replace their dissection, never combine stale local and aggregate fields.
		for _, k := range []string{"Record Count", "Records", "Handshake Incomplete", "Handshake Buffered Bytes", "Handshake Messages", "Alert Messages", "Alert Buffered Bytes"} {
			delete(t, k)
		}
		if err := radiusTLSRecordFields(t, c.data, max, true); err != nil {
			c.discard()
			radiusTLSRoundDiagnostic(fields, "")
			if pe, ok := err.(*ProtocolError); ok {
				switch pe.Kind {
				case ErrContextRequired:
					radiusTLSRoundDiagnostic(fields, "eap-tls-record-context-required")
				case ErrUnsupportedFeature:
					radiusTLSRoundDiagnostic(fields, "eap-tls-record-unimplemented")
				case ErrMalformedMessage:
					old, _ := fields["EAP Diagnostics"].([]string)
					fields["EAP Diagnostics"] = append([]string{"eap-tls-record-malformed"}, old...)
				}
			}
			return &radiusEAPFailure{err}
		}
		t["Fragment Status"] = "reassembled"
		t["Reassembled Data"] = bytes.Clone(c.data)
		radiusTLSRoundDiagnostic(fields, "eap-tls-fragment-reassembled-observed")
	} else {
		t["Fragment Status"] = "pending"
		radiusTLSRoundDiagnostic(fields, "eap-tls-fragment-observed")
	}
	t["Fragment Count"] = c.fragments
	t["Collected Bytes"] = len(c.data)
	t["Reassembled Length"] = c.declared
	return nil
}

// The caller still runs the ordinary per-round RADIUS owner policy. This
// routine only supplies the missing fragmented method context; it neither
// reuses an earlier outer owner nor authenticates the NAS, station or State.
func (f *binFlow) observeRADIUSTLSRound(dir int, e *ProtocolEvent, wire []byte, fields map[string]any, original error) error {
	if fields == nil || fields["EAP"] == nil {
		return original
	}
	inner := fields["EAP"].(map[string]any)
	// Never supersede malformed, budget, unsupported or authentication facts.
	if original != nil {
		var pe *ProtocolError
		if !errors.As(original, &pe) || pe.Kind != ErrContextRequired {
			return original
		}
	}
	s := f.radius
	key := radiusIdentity(dir, wire)
	digest := sha256.Sum256(wire[:int(binary.BigEndian.Uint16(wire[2:4]))])
	typ, _ := inner["Type"].(byte)
	tls, _ := inner["TLS"].(map[string]any)
	ctx, haveContext := radiusTLSDescriptor(fields)
	if wire[0] == 1 && haveContext && original == nil {
		p, known := s.pending[key]
		if !s.untracked && len(s.pending) < f.a.budget.MaxCollectionElements && (!known || !p.retired && !p.completed && p.digest == digest) {
			extra := int64(128)
			if !known {
				extra += 192
			} // reserve the ordinary owner inserted after method parsing
			if s.tlsRounds == nil {
				extra += 512
			}
			if s.tlsRounds == nil || s.tlsRounds.owners[key].context != ctx {
				if err := f.reserveSession(s.storage() + extra); err != nil {
					return &radiusEAPFailure{err}
				}
				if s.tlsRounds == nil {
					s.tlsRounds = &radiusTLSRounds{owners: map[radiusKey]radiusTLSOwner{}}
				}
				s.tlsRounds.owners[key] = radiusTLSOwner{context: ctx}
			}
		}
	}
	r := s.tlsRounds
	if r == nil {
		return original
	}
	for _, c := range r.rounds {
		if !c.retired && e.Timestamp.Sub(c.last) > radiusObserverTTL {
			c.discard()
		}
	}
	if typ != 13 || tls == nil {
		return original
	}
	ec, _ := inner["Code"].(byte)
	ei, _ := inner["Identifier"].(byte)
	state, haveState := radiusTLSState(fields)
	if wire[0] == 1 && ec == 2 && haveContext {
		// A completed server-fragment flight's final ACK remains an independent
		// ordinary outer request. Only active flights constrain ACK adjacency.
		candidates := make([]*radiusTLSRound, 0)
		activeContext := false
		for _, c := range r.rounds {
			if !c.retired && !c.done && c.context == ctx && c.root.dir == key.dir {
				activeContext = true
				if !haveState || bytes.Equal(c.state, state) {
					candidates = append(candidates, c)
				}
			}
		}
		if len(candidates) == 0 {
			if activeContext {
				return radiusTLSRoundRefusal(fields)
			}
			return original
		}
		ack, _ := tls["ACK"].(bool)
		if len(candidates) != 1 || !haveState || !ack || ei != candidates[0].identifier {
			for _, c := range candidates {
				c.discard()
			}
			return radiusTLSRoundRefusal(fields)
		}
		c := candidates[0]
		if !c.awaitingACK {
			if c.next == key && c.ackDigest == digest {
				return original
			}
			c.discard()
			return radiusTLSRoundRefusal(fields)
		}
		if original != nil {
			c.discard()
			return original
		}
		c.next = key
		c.ackDigest = digest
		c.awaitingACK = false
		c.last = e.Timestamp
		return nil
	}
	if wire[0] != 11 || ec != 1 {
		return original
	}
	p, known := s.pending[key]
	owner, hasOwnerContext := r.owners[key]
	ownerContext := owner.context
	if !known || p.retired || e.Timestamp.Sub(p.last) > radiusObserverTTL || !hasOwnerContext {
		if original != nil && (!hasOwnerContext || owner.refused) {
			return radiusTLSRoundRefusal(fields)
		}
		return original
	}
	if p.completed && (!p.replied || p.replyDigest != digest) {
		return original
	}
	// Every accepted fragment has a bounded observation snapshot. Replaying
	// an earlier first/middle fragment cannot create another active flight.
	for _, c := range r.rounds {
		if !c.retired {
			for _, v := range c.history {
				if v.key == key && v.digest == digest {
					if v.complete {
						return c.annotate(fields, tls, f.a.budget.MaxCollectionElements)
					}
					tls["Fragment Status"] = "pending"
					tls["Fragment Count"] = v.fragments
					tls["Collected Bytes"] = v.collected
					tls["Reassembled Length"] = c.declared
					radiusTLSRoundDiagnostic(fields, "eap-tls-fragment-observed")
					return nil
				}
			}
		}
	}
	if p.completed {
		return original
	}
	var linked *radiusTLSRound
	for _, c := range r.rounds {
		if !c.retired && !c.done && !c.awaitingACK && c.next == key {
			if linked != nil {
				linked.discard()
				c.discard()
				return radiusTLSRoundRefusal(fields)
			}
			linked = c
		}
	}
	data, _ := tls["Data"].([]byte)
	more, _ := tls["More Fragments"].(bool)
	included, _ := tls["Length Included"].(bool)
	if linked == nil {
		if !more || !included || !haveState {
			return original
		}
		declared, _ := tls["Message Length"].(uint32)
		if uint64(declared) > uint64(f.a.config.MaxMessageBytes) {
			radiusTLSRoundDiagnostic(fields, "")
			return &radiusEAPFailure{protocolError(ErrResourceExceeded, "EAP-TLS aggregate round byte budget")}
		}
		if len(r.rounds) >= f.a.budget.MaxCollectionElements {
			radiusTLSRoundDiagnostic(fields, "")
			return &radiusEAPFailure{protocolError(ErrResourceExceeded, "EAP-TLS observed round count limit")}
		}
		if len(data) == 0 || uint64(len(data)) >= uint64(declared) {
			return original
		}
		extra := int64(360 + len(state) + len(data))
		if err := f.reserveSession(s.storage() + extra); err != nil {
			radiusTLSRoundDiagnostic(fields, "")
			return &radiusEAPFailure{err}
		}
		c := &radiusTLSRound{context: ownerContext, root: key, lastReply: key, state: radiusTLSRoundCopy(state), data: radiusTLSRoundCopy(data), declared: declared, fragments: 1, identifier: ei, replyDigest: digest, last: e.Timestamp, awaitingACK: true}
		c.history = []radiusTLSFragmentView{{key: key, digest: digest, fragments: 1, collected: len(data)}}
		nextRounds := make([]*radiusTLSRound, len(r.rounds)+1)
		copy(nextRounds, r.rounds)
		nextRounds[len(r.rounds)] = c
		r.rounds = nextRounds
		return c.annotate(fields, tls, f.a.budget.MaxCollectionElements)
	}
	c := linked
	if c.context != ownerContext || (more && !haveState) || ei != c.identifier+1 || e.Timestamp.Before(c.last) {
		c.discard()
		return radiusTLSRoundRefusal(fields)
	}
	if included && tls["Message Length"].(uint32) != c.declared {
		c.discard()
		return radiusTLSRoundRefusal(fields)
	}
	next := len(c.data) + len(data)
	if next > f.a.config.MaxMessageBytes {
		c.discard()
		radiusTLSRoundDiagnostic(fields, "")
		return &radiusEAPFailure{protocolError(ErrResourceExceeded, "EAP-TLS collected byte budget")}
	}
	if uint64(next) > uint64(c.declared) || !more && uint64(next) != uint64(c.declared) || more && uint64(next) >= uint64(c.declared) {
		c.discard()
		return radiusTLSRoundRefusal(fields)
	}
	if c.fragments >= f.a.budget.MaxCollectionElements {
		c.discard()
		radiusTLSRoundDiagnostic(fields, "")
		return &radiusEAPFailure{protocolError(ErrResourceExceeded, "EAP-TLS fragment count budget")}
	}
	extra := int64(next - cap(c.data) + len(state) - cap(c.state) + 96)
	if err := f.reserveSession(s.storage() + max(extra, 0)); err != nil {
		c.discard()
		radiusTLSRoundDiagnostic(fields, "")
		return &radiusEAPFailure{err}
	}
	joined := make([]byte, next)
	copy(joined, c.data)
	copy(joined[len(c.data):], data)
	clear(c.data)
	c.data = joined
	c.state = radiusTLSRoundCopy(state)
	c.fragments++
	c.identifier = ei
	c.last = e.Timestamp
	c.lastReply = key
	c.replyDigest = digest
	c.awaitingACK = more
	c.done = !more
	history := make([]radiusTLSFragmentView, len(c.history)+1)
	copy(history, c.history)
	history[len(c.history)] = radiusTLSFragmentView{key: key, digest: digest, fragments: c.fragments, collected: len(c.data), complete: c.done}
	c.history = history
	return c.annotate(fields, tls, f.a.budget.MaxCollectionElements)
}

// Inspect descriptors without allocating protocol fields when the ordinary
// record admission budget has already refused the packet. Unknown malformed
// tails are never scanned or used as a guessed session identity.
func radiusTLSWireContext(w []byte) (ctx [32]byte, state []byte, haveContext, haveState bool) {
	if len(w) < 20 {
		return
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 20 || n > 4096 || n > len(w) {
		return
	}
	w = w[:n]
	if !radiusCompleteByteFrame(w) {
		return
	}
	var values [5][]byte
	var seen [5]bool
	stateCount := 0
	for at := 20; at < len(w); {
		typ, n := w[at], int(w[at+1])
		v := w[at+2 : at+n]
		at += n
		slot := -1
		switch typ {
		case 4:
			slot = 0
		case 5:
			slot = 1
		case 31:
			slot = 2
		case 32:
			slot = 3
		case 95:
			slot = 4
		case 24:
			state = v
			stateCount++
		}
		if slot >= 0 {
			if seen[slot] {
				return
			}
			seen[slot] = true
			values[slot] = v
		}
	}
	haveState = stateCount == 1 && len(state) > 0
	if len(values[1]) != 4 || len(values[2]) == 0 || len(values[0])+len(values[3])+len(values[4]) == 0 {
		return
	}
	h := sha256.New()
	types := []byte{4, 5, 31, 32, 95}
	for i, v := range values {
		if seen[i] {
			h.Write([]byte{types[i], byte(len(v))})
			h.Write(v)
		}
	}
	copy(ctx[:], h.Sum(nil))
	haveContext = true
	return
}
func (f *binFlow) refuseRADIUSTLSRound(dir int, w []byte) {
	if f.radius == nil || f.radius.tlsRounds == nil || len(w) < 2 {
		return
	}
	key := radiusIdentity(dir, w)
	if owner, ok := f.radius.tlsRounds.owners[key]; ok {
		owner.refused = true
		f.radius.tlsRounds.owners[key] = owner
	}
	ctx, state, haveContext, haveState := radiusTLSWireContext(w)
	for _, c := range f.radius.tlsRounds.rounds {
		// Known outer owners identify a particular flight. A refused new ACK has
		// no outer owner yet, so it additionally needs the exact opaque State and
		// stable descriptor. Unrelated concurrent descriptors/States survive.
		linked := c.next == key || c.lastReply == key
		if radiusIsRequest(w[0]) && haveContext && haveState && c.root.dir == key.dir && c.context == ctx && bytes.Equal(c.state, state) {
			linked = true
		}
		if linked {
			c.discard()
		}
	}
	f.releaseRADIUSTLSRoundBudget()
}
func (f *binFlow) releaseRADIUSTLSRoundBudget() {
	if f.radius == nil || f.radius.tlsRounds == nil {
		return
	}
	retained := f.radius.storage()
	if f.sessionBytes > retained {
		f.a.buffered.Add(retained - f.sessionBytes)
		f.sessionBytes = retained
	}
}
