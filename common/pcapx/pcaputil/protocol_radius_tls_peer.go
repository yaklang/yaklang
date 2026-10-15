package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// A Start offer is an observed, independently owned Challenge, never a TLS
// endpoint assertion. Keep consumed offers as bounded tombstones: opaque State
// and EAP Identifier alone cannot identify another authentication generation.
type radiusTLSPeerOffer struct {
	root            radiusKey
	context, digest [32]byte
	state           []byte
	identifier      byte
	last            time.Time
	consumed        bool
}

// This finite profile accepts peer response fragments only after an observed
// Start and an exact server empty-ACK sequence. An arbitrary first fragment,
// source tuple or State string is insufficient authentication-round context.
func (f *binFlow) observeRADIUSTLSPeerRound(dir int, e *ProtocolEvent, w []byte, fields, inner, tls map[string]any, original error) (bool, error) {
	s := f.radius
	r := s.tlsRounds
	if r == nil || tls == nil || inner["Type"] != byte(13) {
		return false, original
	}
	for _, c := range r.rounds {
		if c.peer && !c.retired && e.Timestamp.Sub(c.last) > radiusObserverTTL {
			c.discard()
		}
	}
	key := radiusIdentity(dir, w)
	digest := sha256.Sum256(w[:int(binary.BigEndian.Uint16(w[2:4]))])
	ec, _ := inner["Code"].(byte)
	ei, _ := inner["Identifier"].(byte)
	ctx, haveContext := radiusTLSDescriptor(fields)
	state, haveState := radiusTLSState(fields)
	p, known := s.pending[key]
	owner, hasOwner := r.owners[key]
	if w[0] == 11 && ec == 1 {
		if !known || p.retired || !hasOwner || e.Timestamp.Sub(p.last) > radiusObserverTTL || p.completed && (!p.replied || p.replyDigest != digest) {
			return false, original
		}
		var round *radiusTLSRound
		for _, c := range r.rounds {
			if c.peer && !c.retired && !c.done && c.awaitingACK && c.lastReply == key {
				if round != nil {
					round.discard()
					c.discard()
					return true, radiusTLSRoundRefusal(fields)
				}
				round = c
			}
		}
		if round != nil {
			c := round
			ack, _ := tls["ACK"].(bool)
			if original != nil || !ack || !haveState || ei != c.identifier+1 || owner.context != c.context || e.Timestamp.Before(c.last) {
				c.discard()
				if original != nil {
					return true, original
				}
				return true, radiusTLSRoundRefusal(fields)
			}
			if err := f.reserveSession(s.storage() + int64(max(len(state)-cap(c.state), 0))); err != nil {
				c.discard()
				return true, &radiusEAPFailure{err}
			}
			c.state = radiusTLSRoundCopy(state)
			c.identifier = ei
			c.awaitingACK = false
			c.last = e.Timestamp
			c.next = key
			c.ackDigest = digest
			return true, nil
		}
		start, _ := tls["Start"].(bool)
		if !start || !haveState || original != nil {
			return false, original
		}
		for _, o := range r.peerOffers {
			if o.root == key && o.digest == digest {
				return false, original
			}
		}
		if len(r.peerOffers)+len(r.rounds) >= f.a.budget.MaxCollectionElements {
			return true, &radiusEAPFailure{protocolError(ErrResourceExceeded, "EAP-TLS observed offer count limit")}
		}
		if err := f.reserveSession(s.storage() + int64(200+len(state))); err != nil {
			return true, &radiusEAPFailure{err}
		}
		o := radiusTLSPeerOffer{root: key, context: owner.context, digest: digest, state: radiusTLSRoundCopy(state), identifier: ei, last: e.Timestamp}
		offers := make([]radiusTLSPeerOffer, len(r.peerOffers)+1)
		copy(offers, r.peerOffers)
		offers[len(r.peerOffers)] = o
		r.peerOffers = offers
		return false, original
	}
	if w[0] != 1 || ec != 2 || !haveContext {
		return false, original
	}
	if !known && (s.untracked || len(s.pending) >= f.a.budget.MaxCollectionElements) {
		return true, &radiusEAPFailure{protocolError(ErrResourceExceeded, "RADIUS identity budget")}
	}
	// An accepted AccessRequest may retry only while its own ordinary RADIUS
	// response remains unobserved. Never revive a completed Identifier owner.
	if known && (p.retired || p.completed || p.digest != digest || e.Timestamp.Sub(p.last) > radiusObserverTTL) {
		if original != nil {
			for _, c := range r.rounds {
				if c.peer && c.root.dir == key.dir && c.context == ctx {
					return true, radiusTLSRoundRefusal(fields)
				}
			}
		}
		return false, original
	}
	for _, c := range r.rounds {
		if !c.peer || c.retired {
			continue
		}
		for _, v := range c.history {
			if v.key == key && v.digest == digest {
				if v.complete {
					return true, c.annotate(fields, tls, f.a.budget.MaxCollectionElements)
				}
				tls["Fragment Status"] = "pending"
				tls["Fragment Count"] = v.fragments
				tls["Collected Bytes"] = v.collected
				tls["Reassembled Length"] = c.declared
				radiusTLSRoundDiagnostic(fields, "eap-tls-fragment-observed")
				return true, nil
			}
		}
	}
	var candidates []*radiusTLSRound
	activeContext := false
	havePeerHistory := false
	for _, c := range r.rounds {
		if c.peer && c.root.dir == key.dir && c.context == ctx {
			havePeerHistory = true
		}
		if c.peer && !c.done && !c.retired && c.root.dir == key.dir && c.context == ctx {
			activeContext = true
		}
		if c.peer && !c.done && !c.retired && c.root.dir == key.dir && c.context == ctx && !c.awaitingACK && (!haveState || bytes.Equal(c.state, state)) {
			candidates = append(candidates, c)
		}
	}
	var c *radiusTLSRound
	if len(candidates) > 0 {
		if len(candidates) != 1 || !haveState {
			for _, v := range candidates {
				v.discard()
			}
			return true, radiusTLSRoundRefusal(fields)
		}
		c = candidates[0]
		ackOwner, haveACK := s.pending[c.next]
		if !haveACK || ackOwner.retired || !ackOwner.completed || !ackOwner.replied || ackOwner.replyDigest != c.ackDigest {
			c.discard()
			return true, radiusTLSRoundRefusal(fields)
		}
		if ei != c.identifier || e.Timestamp.Before(c.last) {
			c.discard()
			return true, radiusTLSRoundRefusal(fields)
		}
	} else {
		more, _ := tls["More Fragments"].(bool)
		included, _ := tls["Length Included"].(bool)
		if !more || !included || !haveState {
			if activeContext || original != nil && havePeerHistory {
				return true, radiusTLSRoundRefusal(fields)
			}
			return false, original
		}
		matched := -1
		identityCount := 0
		for i, o := range r.peerOffers {
			if o.root.dir != key.dir || o.context != ctx || !bytes.Equal(o.state, state) || o.identifier != ei {
				continue
			}
			// Idle time or consuming one offer does not establish a fresh wire
			// generation for a reused opaque State/Identifier pair.
			identityCount++
			rootOwner, haveRoot := s.pending[o.root]
			if !o.consumed && haveRoot && !rootOwner.retired && rootOwner.completed && rootOwner.replied && rootOwner.replyDigest == o.digest && !e.Timestamp.Before(o.last) && e.Timestamp.Sub(o.last) <= radiusObserverTTL {
				matched = i
			}
		}
		if identityCount > 1 {
			return true, radiusTLSRoundRefusal(fields)
		}
		if matched < 0 {
			return false, original
		}
		declared, _ := tls["Message Length"].(uint32)
		data, _ := tls["Data"].([]byte)
		if uint64(declared) > uint64(f.a.config.MaxMessageBytes) {
			r.peerOffers[matched].consumed = true
			radiusTLSRoundDiagnostic(fields, "")
			return true, &radiusEAPFailure{protocolError(ErrResourceExceeded, "EAP-TLS peer aggregate byte budget")}
		}
		if len(data) == 0 || uint64(len(data)) >= uint64(declared) {
			return false, original
		}
		if len(r.peerOffers)+len(r.rounds) >= f.a.budget.MaxCollectionElements {
			return true, &radiusEAPFailure{protocolError(ErrResourceExceeded, "EAP-TLS observed round count limit")}
		}
		c = &radiusTLSRound{peer: true, context: ctx, root: key, declared: declared, identifier: ei}
		extra := int64(radiusTLSRoundStorage + 104 + len(data) + len(state))
		if !known {
			extra += 192
		}
		if !hasOwner {
			extra += 128
		}
		if err := f.reserveSession(s.storage() + extra); err != nil {
			r.peerOffers[matched].consumed = true
			return true, &radiusEAPFailure{err}
		}
		c.state = radiusTLSRoundCopy(state)
		// Consume only after the exact offer is established and admission checked.
		r.peerOffers[matched].consumed = true
		rounds := make([]*radiusTLSRound, len(r.rounds)+1)
		copy(rounds, r.rounds)
		rounds[len(r.rounds)] = c
		r.rounds = rounds
	}
	data, _ := tls["Data"].([]byte)
	more, _ := tls["More Fragments"].(bool)
	included, _ := tls["Length Included"].(bool)
	if included && tls["Message Length"].(uint32) != c.declared {
		c.discard()
		return true, radiusTLSRoundRefusal(fields)
	}
	next := len(c.data) + len(data)
	if next > f.a.config.MaxMessageBytes {
		c.discard()
		radiusTLSRoundDiagnostic(fields, "")
		return true, &radiusEAPFailure{protocolError(ErrResourceExceeded, "EAP-TLS peer collected byte budget")}
	}
	if uint64(next) > uint64(c.declared) || !more && uint64(next) != uint64(c.declared) || more && uint64(next) >= uint64(c.declared) {
		c.discard()
		return true, radiusTLSRoundRefusal(fields)
	}
	if c.fragments >= f.a.budget.MaxCollectionElements {
		c.discard()
		radiusTLSRoundDiagnostic(fields, "")
		return true, &radiusEAPFailure{protocolError(ErrResourceExceeded, "EAP-TLS peer fragment count budget")}
	}
	extra := int64(next - cap(c.data) + len(state) - cap(c.state) + 96)
	if !known {
		extra += 192
	}
	if !hasOwner {
		extra += 128
	}
	if err := f.reserveSession(s.storage() + max(extra, 0)); err != nil {
		c.discard()
		return true, &radiusEAPFailure{err}
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
	history[len(c.history)] = radiusTLSFragmentView{key: key, digest: digest, fragments: c.fragments, collected: next, complete: c.done}
	c.history = history
	if err := c.annotate(fields, tls, f.a.budget.MaxCollectionElements); err != nil {
		return true, err
	}
	r.owners[key] = radiusTLSOwner{context: ctx}
	return true, nil
}
